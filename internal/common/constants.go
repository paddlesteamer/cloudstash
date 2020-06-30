package common

import "github.com/vgough/go-fuse-c/fuse"

const (
	DRV_FILE   = fuse.S_IFREG
	DRV_FOLDER = fuse.S_IFDIR
)

const (
	DROPBOX_APP_KEY = "l4v6ipcr1rlwu1x"
	DATABASE_FILE   = "dropbox://cloudstash.sqlite3" // @TODO: to be removed later
)
