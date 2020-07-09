package common

import "github.com/paddlesteamer/go-fuse-c/fuse"

const (
	DRV_FILE   = fuse.S_IFREG
	DRV_FOLDER = fuse.S_IFDIR

	CACHE_FILE_PREFIX = "cloudstash-cached-"
	DB_FILE_PREFIX    = "cloudstash-db-"
)
