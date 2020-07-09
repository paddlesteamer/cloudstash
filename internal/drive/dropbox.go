package drive

import (
	"crypto/sha256"
	"fmt"
	"io"
	"strings"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/config"
)

type Dropbox struct {
	client files.Client
}

// NewDropboxClient creates a new Dropbox client.
func NewDropboxClient(conf *config.DropboxCredentials) *Dropbox {
	dbxConfig := dropbox.Config{
		Token:    conf.AccessToken,
		LogLevel: dropbox.LogDebug,
	}

	return &Dropbox{files.New(dbxConfig)}
}

func (d *Dropbox) GetProviderName() string {
	return "dropbox"
}

// @todo: add descriptive comment
func (d *Dropbox) GetFile(path string) (*metadata, io.ReadCloser, error) {
	args := files.NewDownloadArg(path)
	md, r, err := d.client.Download(args)
	if err != nil {
		// no other way to distinguish not found error
		if strings.Contains(err.Error(), "not_found") {
			return nil, nil, common.ErrNotFound
		}

		return nil, nil, fmt.Errorf("could not get file from dropbox %s: %v", path, err)
	}

	m := &metadata{
		Name: md.Name,
		Size: md.Size,
		Hash: md.ContentHash,
	}
	return m, r, nil
}

// PutFile uploads a new file.
func (d *Dropbox) PutFile(path string, content io.Reader) error {
	if err := d.DeleteFile(path); err != nil && err != common.ErrNotFound {
		return fmt.Errorf("couldn't delete file from dropbox before upload: %v", err)
	}

	uargs := files.NewCommitInfo(path)
	_, err := d.client.Upload(uargs, content)
	if err != nil {
		return fmt.Errorf("could not upload file to dropbox %s: %v", path, err)
	}

	return nil
}

// GetFileMetadata gets the file metadata given the file path.
func (d *Dropbox) GetFileMetadata(path string) (*metadata, error) {
	args := &files.GetMetadataArg{
		Path: path,
	}

	md, err := d.client.GetMetadata(args)
	if err != nil {
		return nil, fmt.Errorf("unable to get metadata of dropbox file '%s': %v", path, err)
	}

	return &metadata{
		Name: md.(*files.FileMetadata).Name,
		Size: md.(*files.FileMetadata).Size,
		Hash: md.(*files.FileMetadata).ContentHash,
	}, nil
}

// DeleteFile deletes file from dropbox
func (d *Dropbox) DeleteFile(path string) error {
	dargs := files.NewDeleteArg(path)
	_, err := d.client.DeleteV2(dargs) //@TODO: ignore notfound error but check other errors
	if err != nil {
		if strings.Contains(err.Error(), "not_found") {
			return common.ErrNotFound
		}

		return fmt.Errorf("couldn't delete file from dropbox: %v", err)
	}

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
