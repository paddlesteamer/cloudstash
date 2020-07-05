package manager

import (
	"fmt"
	"os"
	"time"

	"github.com/paddlesteamer/go-cache"
)

const (
	fileDownloading = iota
	fileAvailable   = iota
)

type cacheEntry struct {
	path   string
	status int
}

func newCacheEntry(path string, status int) cacheEntry {
	return cacheEntry{
		path:   path,
		status: status,
	}
}

const (
	cacheExpiration = 30 * time.Minute
	cleanupInterval = 5 * time.Minute
	cacheForever    = 0
)

func newCache() *cache.Cache {
	c := cache.New(cacheExpiration, cleanupInterval)
	c.OnEvicted(expirationHandler)
	return c
}

func expirationHandler(ino string, path interface{}) {
	err := os.Remove(path.(string))
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't delete cached file %s: %v", path, err)
	}
}

func newTracker() *cache.Cache {
	return cache.New(cache.NoExpiration, cache.NoExpiration)
}
