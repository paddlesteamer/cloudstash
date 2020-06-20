package drive

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"

	"github.com/paddlesteamer/hdn-drv/internal/config"
)

type Dropbox struct {
	client files.Client
}

func NewDropboxClient(conf *config.DropboxCredentials) *Dropbox {
	dbxConfig := dropbox.Config{
		Token:    conf.AccessToken,
		LogLevel: dropbox.LogDebug,
	}

	dbx := &Dropbox{
		client: files.New(dbxConfig),
	}

	return dbx
}

func (dbx *Dropbox) GetProviderName() string {
	return "dropbox"
}

func (dbx *Dropbox) GetFile(path string) (*Metadata, io.ReadCloser, error) {
	args := files.NewDownloadArg(path)

	metadata, r, err := dbx.client.Download(args)
	if err != nil {
		return nil, nil, fmt.Errorf("dropbox: unable to get file %v: %v", path, err)
	}

	m := &Metadata{
		Name: metadata.Name,
		Size: metadata.Size,
		Hash: metadata.ContentHash,
	}

	return m, r, nil
}

func (dbx *Dropbox) PutFile(path string, content io.Reader) error {
	args := files.NewCommitInfo(path)

	_, err := dbx.client.Upload(args, content)
	if err != nil {
		return fmt.Errorf("dropbox: unable to upload file %v: %v", path, err)
	}

	return nil
}

func (dbx *Dropbox) GetFileMetadata(path string) (*Metadata, error) {
	args := &files.GetMetadataArg{
		Path: path,
	}

	metadata, err := dbx.client.GetMetadata(args)
	if err != nil {
		return nil, fmt.Errorf("dropbox: unable to get metadata of %v: %v", path, err)
	}

	m := &Metadata{
		Name: metadata.(*files.FileMetadata).Name,
		Size: metadata.(*files.FileMetadata).Size,
		Hash: metadata.(*files.FileMetadata).ContentHash,
	}

	return m, nil
}

// ComputeHash ...
// Computes content hash value according to
// https://www.dropbox.com/developers/reference/content-hash
func (dbx *Dropbox) ComputeHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("dropbox: unable to open file %v: %v", path, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("dropbox: unable to get file size of %v: %v", path, err)
	}

	bytesLeft := fi.Size()
	res := []byte{}
	var cpSize int64 = 4 * 1024 * 1024

	for bytesLeft > 0 {
		h := sha256.New()

		if bytesLeft < 4096 {
			cpSize = bytesLeft
		}

		_, err := io.CopyN(h, f, cpSize)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("dropbox: error while reading file %v: %v", path, err)
		}

		res = append(res, h.Sum(nil)...)

		bytesLeft -= cpSize

		f.Seek(cpSize, 1)
	}

	rh := sha256.Sum256(res)

	return fmt.Sprintf("%x", rh), nil
}
