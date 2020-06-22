package manager

import (
	"fmt"
	"os"
	"time"

	"github.com/patrickmn/go-cache"
)

const cacheExpiration = 30 * time.Minute
const cleanupInterval = 5 * time.Minute

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
