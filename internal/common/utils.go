package common

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"time"
)

// FileURL holds parsed file information
type FileURL struct {
	Scheme string
	Name   string
}

// ParseURL parses provided fileURL and returns FileURL
// a fileURL shouldn't include path info
// i.e. A valid file URL is gdrive://filename
// and i.e. gdrive://filepath/filename is invalid
func ParseURL(fileURL string) (*FileURL, error) {
	u, err := url.Parse(fileURL)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse url '%s': %v", fileURL, err)
	}

	if len(u.Path) > 0 {
		return nil, fmt.Errorf("%s is not a valid file URL", fileURL)
	}

	return &FileURL{
		Scheme: u.Scheme,
		Name:   u.Host,
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
func GenerateConflictedFileName(name string) string {
	return fmt.Sprintf("conflicted_copy_%d_%s", time.Now().UnixNano(), name)
}

func ToString(i int64) string {
	return strconv.FormatInt(i, 10)
}

func ToInt64(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 0)

	return i
}
