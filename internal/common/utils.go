package common

import (
	"fmt"
	"net/url"
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

	fu := &FileURL{
		Scheme: u.Scheme,
		Path:   fmt.Sprintf("/%s", u.Host),
	}

	return fu, nil
}
