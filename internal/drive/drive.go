package drive

import (
	"fmt"
	"io"
)

// Metadata contains name, size (in bytes), and content hash of a remote file
type Metadata struct {
	Name string
	Size uint64
	Hash string
}

// Drive is the interface every remote drive client should implement
type Drive interface {

	// GetProviderName returns name of the drive. i.e. dropbox
	GetProviderName() string

	// GetFile returns metadata and reader of the requested remote file
	// If file couldn't be found, it should return common.ErrNotFound
	GetFile(path string) (*Metadata, io.ReadCloser, error)

	// PutFile uploads specified file to the remote drive
	// It overwrites if the file exists
	PutFile(path string, content io.Reader) error

	// GetFileMetadata returns metadata of the remote file
	GetFileMetadata(path string) (*Metadata, error)

	// DeleteFile removes file from the remote drive
	DeleteFile(path string) error

	// ComputeHash computes hash of file with drive's specific method
	ComputeHash(r io.Reader, hchan chan string, echan chan error)
}

// GetURL creates URL of remote file
// i.e. dropbox://filename.ext
func GetURL(drv Drive, name string) string {
	scheme := drv.GetProviderName()

	if name[0] == '/' {
		name = name[1:]
	}

	return fmt.Sprintf("%s://%s", scheme, name)
}
