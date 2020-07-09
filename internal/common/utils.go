package common

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
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

func NewTempCacheFile() (*os.File, error) {
	tmpfile, err := ioutil.TempFile(os.TempDir(), cacheFilePrefix)

	return tmpfile, err
}

func NewTempDBFile() (*os.File, error) {
	tmpfile, err := ioutil.TempFile(os.TempDir(), dbFilePrefix)

	return tmpfile, err
}

func ObfuscateFileName(name string) string {
	h := md5.New()
	io.WriteString(h, name)
	io.WriteString(h, time.Now().String())

	return fmt.Sprintf("%x.dat", h.Sum(nil))
}
