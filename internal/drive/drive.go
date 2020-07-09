package drive

import (
	"fmt"
	"io"
)

type Drive interface {
	GetProviderName() string
	GetFile(path string) (*metadata, io.ReadCloser, error)
	PutFile(path string, content io.Reader) error
	GetFileMetadata(path string) (*metadata, error)
	DeleteFile(path string) error
	ComputeHash(r io.Reader, hchan chan string, echan chan error)
}

type metadata struct {
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
