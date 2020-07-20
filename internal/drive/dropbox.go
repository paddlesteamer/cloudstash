package drive

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/users"
	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/config"
)

// Dropbox is holds necessary info about dropbox client
type Dropbox struct {
	client  files.Client
	account users.Client

	mu sync.Mutex
}

// NewDropboxClient creates a new Dropbox client.
func NewDropboxClient(conf *config.DropboxCredentials) *Dropbox {
	dbxConfig := dropbox.Config{
		Token:    conf.AccessToken,
		LogLevel: dropbox.LogDebug,
	}

	return &Dropbox{
		client:  files.New(dbxConfig),
		account: users.New(dbxConfig),
	}
}

// GetProviderName returns 'dropbox'
func (d *Dropbox) GetProviderName() string {
	return "dropbox"
}

// GetFile returns ReadCloser of remote file on dropbox
func (d *Dropbox) GetFile(name string) (io.ReadCloser, error) {
	name = getPath(name)

	args := files.NewDownloadArg(name)
	_, r, err := d.client.Download(args)
	if err != nil {
		// no other way to distinguish not found error
		if strings.Contains(err.Error(), "not_found") {
			return nil, common.ErrNotFound
		}

		return nil, fmt.Errorf("could not get file from dropbox %s: %v", name, err)
	}

	return r, nil
}

// PutFile uploads a new file.
func (d *Dropbox) PutFile(name string, content io.Reader) error {
	name = getPath(name)

	if err := d.DeleteFile(name); err != nil && err != common.ErrNotFound {
		return fmt.Errorf("couldn't delete file from dropbox before upload: %v", err)
	}

	uargs := files.NewCommitInfo(name)
	_, err := d.client.Upload(uargs, content)
	if err != nil {
		return fmt.Errorf("could not upload file to dropbox %s: %v", name, err)
	}

	return nil
}

// GetFileMetadata gets the file metadata given the file path.
func (d *Dropbox) GetFileMetadata(name string) (*Metadata, error) {
	name = getPath(name)

	args := &files.GetMetadataArg{
		Path: name,
	}

	md, err := d.client.GetMetadata(args)
	if err != nil {
		// no other way to distinguish not found error
		if strings.Contains(err.Error(), "not_found") {
			return nil, common.ErrNotFound
		}

		return nil, fmt.Errorf("unable to get metadata of dropbox file '%s': %v", name, err)
	}

	return &Metadata{
		Name: md.(*files.FileMetadata).Name,
		Size: md.(*files.FileMetadata).Size,
		Hash: md.(*files.FileMetadata).ContentHash,
	}, nil
}

// DeleteFile deletes file from dropbox
func (d *Dropbox) DeleteFile(name string) error {
	name = getPath(name)

	dargs := files.NewDeleteArg(name)
	if _, err := d.client.DeleteV2(dargs); err != nil {
		if strings.Contains(err.Error(), "not_found") {
			return common.ErrNotFound
		}

		return fmt.Errorf("couldn't delete file from dropbox: %v", err)
	}

	return nil
}

// MoveFile renames file on dropbox
func (d *Dropbox) MoveFile(name string, newName string) error {
	name = getPath(name)
	newName = getPath(newName)

	args := files.NewRelocationArg(name, newName)
	if _, err := d.client.MoveV2(args); err != nil {
		return fmt.Errorf("couldn't move file from %s to %s on dropbox: %v", name, newName, err)
	}

	return nil
}

// Lock creates a lock file on dropbox
// This ensures lock acquire requests from different
// clients goes into race condition and is completely
// thread/client safe
func (d *Dropbox) Lock() error {
	d.mu.Lock()

	lfile := getPath(lockFile)

	// we need to create random content to lock file otherwise
	// dropbox doesn't return conflict error
	content := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, content); err != nil {
		// fall back to timestamp
		binary.LittleEndian.PutUint64(content, uint64(time.Now().UnixNano()))
	}

	uargs := files.NewCommitInfo(lfile)

	for {
		_, err := d.client.Upload(uargs, bytes.NewReader(content))
		if err != nil {
			if strings.Contains(err.Error(), "conflict") {
				continue
			}

			d.mu.Unlock()
			return fmt.Errorf("could not create lock file on dropbox: %v", err)
		}

		break
	}

	return nil
}

// Unlock deletes lock file from dropbox
// and ignores not found error
func (d *Dropbox) Unlock() error {
	if err := d.DeleteFile(lockFile); err != nil {
		if err == common.ErrNotFound {
			d.mu.Unlock()
			return nil
		}

		return fmt.Errorf("couldn't delete lock file: %v", err)
	}

	d.mu.Unlock()
	return nil
}

// ComputeHash computes content hash value according to
// https://www.dropbox.com/developers/reference/content-hash
func (d *Dropbox) ComputeHash(r io.Reader, hchan chan string, echan chan error) {
	res := []byte{}

	buffer := make([]byte, 4*1024*1024)

	for {

		ntotal := 0
		brk := false

		for {
			n, err := r.Read(buffer[ntotal:])
			if err != nil && err != io.EOF {
				echan <- fmt.Errorf("couldn't read into hash buffer: %v", err)
				return
			}

			ntotal += n

			if err == io.EOF || ntotal == len(buffer) {
				brk = err == io.EOF
				break
			}
		}

		if brk && ntotal == 0 {
			rh := sha256.Sum256(res)

			hchan <- fmt.Sprintf("%x", rh)

			return
		}

		h := sha256.New()
		if _, err := h.Write(buffer[:ntotal]); err != nil {
			echan <- fmt.Errorf("couldn't write to hash from buffer: %v", err)
		}

		res = append(res, h.Sum(nil)...)

		if brk {
			rh := sha256.Sum256(res)

			hchan <- fmt.Sprintf("%x", rh)

			return
		}
	}

}

// GetAvailableSpace returns available space in bytes
func (d *Dropbox) GetAvailableSpace() (int64, error) {
	res, err := d.account.GetSpaceUsage()
	if err != nil {
		return 0, fmt.Errorf("couldn't get available space on dropbox: %v", err)
	}

	return int64(res.Allocation.Individual.Allocated - res.Used), nil
}

func getPath(name string) string {
	if name[0] == '/' {
		return name
	}

	return fmt.Sprintf("/%s", name)
}
