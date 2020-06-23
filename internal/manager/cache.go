package manager

import (
	"fmt"
	"os"
	"time"

	"github.com/patrickmn/go-cache"
)

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
	t := cache.New(cache.NoExpiration, cache.NoExpiration)

	return t
}
