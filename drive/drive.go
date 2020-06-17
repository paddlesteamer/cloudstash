package drive

import "io"

type Drive interface {
	GetProviderName() string
	GetFile(path string) ([]byte, error)
	PutFile(path string, content io.Reader) error
}
