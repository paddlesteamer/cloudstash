package drive

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"

	"github.com/paddlesteamer/hdn-drv/internal/config"
)

type Dropbox struct {
	client files.Client
}

func (d *Dropbox) GetProviderName() string {
	return "dropbox"
}

// NewDropboxClient creates a new Dropbox client.
func NewDropboxClient(conf *config.DropboxCredentials) *Dropbox {
	dbxConfig := dropbox.Config{
		Token:    conf.AccessToken,
		LogLevel: dropbox.LogDebug,
	}

	return &Dropbox{files.New(dbxConfig)}
}

// @todo: add descriptive comment
func (d *Dropbox) GetFile(path string) (*Metadata, io.ReadCloser, error) {
	args := files.NewDownloadArg(path)
	metadata, r, err := d.client.Download(args)
	if err != nil {
		if strings.Contains(err.Error(), "not_found") { // no other way to distinguish not found error
			return nil, nil, ErrNotFound
		}

		fmt.Println(err.Error())

		return nil, nil, fmt.Errorf("could not get file from dropbox %s: %v", path, err)
	}

	m := &Metadata{
		Name: metadata.Name,
		Size: metadata.Size,
		Hash: metadata.ContentHash,
	}
	return m, r, nil
}

// PutFile uploads a new file.
func (d *Dropbox) PutFile(path string, content io.Reader) error {
	dargs := files.NewDeleteArg(path)
	d.client.DeleteV2(dargs) //@TODO: ignore notfound error but check other errors

	uargs := files.NewCommitInfo(path)
	_, err := d.client.Upload(uargs, content)
	if err != nil {
		return fmt.Errorf("could not upload file to dropbox %s: %v", path, err)
	}

	return nil
}

// GetFileMetadata gets the file metadata given the file path.
func (d *Dropbox) GetFileMetadata(path string) (*Metadata, error) {
	args := &files.GetMetadataArg{
		Path: path,
	}

	metadata, err := d.client.GetMetadata(args)
	if err != nil {
		return nil, fmt.Errorf("dropbox: unable to get metadata of %v: %v", path, err)
	}

	return &Metadata{
		Name: metadata.(*files.FileMetadata).Name,
		Size: metadata.(*files.FileMetadata).Size,
		Hash: metadata.(*files.FileMetadata).ContentHash,
	}, nil
}

// ComputeHash computes content hash value according to
// https://www.dropbox.com/developers/reference/content-hash
func (d *Dropbox) ComputeHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("could not open dropbox file %s: %v", path, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("could not get dropbox file size of %s: %v", path, err)
	}

	bytesLeft := fi.Size()
	res := []byte{}
	var cpSize int64 = 4 * 1024 * 1024

	for bytesLeft > 0 {
		if bytesLeft < 4096 {
			cpSize = bytesLeft
		}

		h := sha256.New()
		_, err := io.CopyN(h, f, cpSize)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("could not copy btyes from  %s: %v", path, err)
		}

		res = append(res, h.Sum(nil)...)
		bytesLeft -= cpSize

		_, err = f.Seek(cpSize, 1)
		if err != nil {
			return "", fmt.Errorf("could not seek file: %v", err)
		}
	}

	rh := sha256.Sum256(res)
	return fmt.Sprintf("%x", rh), nil
}
