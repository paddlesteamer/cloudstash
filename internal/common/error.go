package common

import "errors"

var (
	ErrNotFound    = errors.New("file/folder doesn't exist")
	ErrDirNotEmpty = errors.New("directory isn't empty")
)
