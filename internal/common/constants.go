package common

import "github.com/vgough/go-fuse-c/fuse"

const (
	DRV_FILE   = fuse.S_IFREG
	DRV_FOLDER = fuse.S_IFDIR
)
