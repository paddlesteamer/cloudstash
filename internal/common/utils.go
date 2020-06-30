package common

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"time"

	"github.com/paddlesteamer/cloudstash/internal/drive"
)

const (
	cacheFilePrefix = "cloudstash-cached-"
	dbFilePrefix    = "cloudstash-db-"
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

func GetURL(drv drive.Drive, name string) string {
	scheme := drv.GetProviderName()

	if name[0] == '/' {
		name = name[1:]
	}

	return fmt.Sprintf("%s://%s", scheme, name)
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
