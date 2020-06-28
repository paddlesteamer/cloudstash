package drive

import (
	"errors"
	"io"
)

var ErrNotFound = errors.New("not found")

type Drive interface {
	GetProviderName() string
	GetFile(path string) (*Metadata, io.ReadCloser, error)
	PutFile(path string, content io.Reader) error
	GetFileMetadata(path string) (*Metadata, error)
	ComputeHash(r io.Reader, hchan chan string, echan chan error)
}

type Metadata struct {
	Name string
	Size uint64
	Hash string
}
