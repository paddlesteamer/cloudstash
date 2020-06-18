package drive

import "io"

type Drive interface {
	GetProviderName() string
	GetFile(path string) (*Metadata, io.ReadCloser, error)
	PutFile(path string, content io.Reader) error
}

type Metadata struct {
	Name string
	Size uint64
}
