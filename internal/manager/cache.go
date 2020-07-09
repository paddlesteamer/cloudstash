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
	hash   string
}

func newCacheEntry(path string, status int, hash string) cacheEntry {
	return cacheEntry{
		path:   path,
		status: status,
		hash:   hash,
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

func expirationHandler(ino string, ent interface{}) {
	entry := ent.(cacheEntry)

	err := os.Remove(entry.path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't delete cached file %s: %v", entry.path, err)
	}
}

type trackerEntry struct {
	cachePath  string
	remotePath string
	accessTime time.Time
}

const idleTimeThreshold time.Duration = 10 * time.Second

func accessFilter(key string, it *cache.Item) bool {
	entry := it.Object.(trackerEntry)

	return time.Now().Sub(entry.accessTime) > idleTimeThreshold
}

func newTracker() *cache.Cache {
	return cache.New(cache.NoExpiration, cache.NoExpiration)
}
