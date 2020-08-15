package manager

import (
	"os"
	"time"

	"github.com/paddlesteamer/zcache"

	log "github.com/sirupsen/logrus"
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

func newCache() *zcache.Cache {
	c := zcache.New(cacheExpiration, cleanupInterval)
	c.OnEvicted(expirationHandler)
	return c
}

func expirationHandler(ino string, ent interface{}) {
	entry := ent.(cacheEntry)

	if err := os.Remove(entry.path); err != nil {
		log.Warningf("couldn't delete cached file %s: %v", entry.path, err)
	}
}

type trackerEntry struct {
	cachePath  string
	remotePath string
	accessTime time.Time
}

const idleTimeThreshold time.Duration = 10 * time.Second

func accessFilter(key string, it zcache.Item) (bool, bool) {
	entry := it.Object.(trackerEntry)

	return time.Now().Sub(entry.accessTime) > idleTimeThreshold, false
}

func newTracker() *zcache.Cache {
	return zcache.New(zcache.NoExpiration, zcache.NoExpiration)
}
