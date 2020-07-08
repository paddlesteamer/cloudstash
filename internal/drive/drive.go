package drive

import (
	"errors"
	"fmt"
	"io"
)

var ErrNotFound = errors.New("not found")

type Drive interface {
	GetProviderName() string
	GetFile(path string) (*Metadata, io.ReadCloser, error)
	PutFile(path string, content io.Reader) error
	GetFileMetadata(path string) (*Metadata, error)
	DeleteFile(path string) error
	ComputeHash(r io.Reader, hchan chan string, echan chan error)
}

type Metadata struct {
	Name string
	Size uint64
	Hash string
}

func GetURL(drv Drive, name string) string {
	scheme := drv.GetProviderName()

	if name[0] == '/' {
		name = name[1:]
	}

	return fmt.Sprintf("%s://%s", scheme, name)
}
