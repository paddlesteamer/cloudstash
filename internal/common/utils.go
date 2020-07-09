package common

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/url"
	"time"
)

type FileURL struct {
	Scheme string
	Path   string
}

func ParseURL(fileUrl string) (*FileURL, error) {
	u, err := url.Parse(fileUrl)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse url '%s': %v", fileUrl, err)
	}

	return &FileURL{
		Scheme: u.Scheme,
		Path:   fmt.Sprintf("/%s%s", u.Host, u.Path),
	}, nil
}

func ObfuscateFileName(name string) string {
	h := md5.New()
	io.WriteString(h, name)
	io.WriteString(h, time.Now().String())

	return fmt.Sprintf("%x.dat", h.Sum(nil))
}
