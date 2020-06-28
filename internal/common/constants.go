package common

import "github.com/vgough/go-fuse-c/fuse"

const (
	DRV_FILE   = fuse.S_IFREG
	DRV_FOLDER = fuse.S_IFDIR
)

const (
	DROPBOX_APP_KEY = "tzpv39e8hkg06xi"
	DATABASE_FILE   = "dropbox://hdn-drv.sqlite3" // @TODO: to be removed later
)
