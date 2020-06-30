package manager

import (
	"os"
	"sync"

	"github.com/paddlesteamer/cloudstash/internal/drive"
)

type database struct {
	path     string
	extPath  string
	hash     string
	extDrive drive.Drive
	mux      sync.RWMutex
}

func NewDB(path, extPath, hash string, extDrive drive.Drive) *database {
	return &database{
		path:     path,
		extPath:  extPath,
		hash:     hash,
		extDrive: extDrive,
	}
}

func (db *database) Close() {
	os.Remove(db.path)
}

func (db *database) wLock() {
	db.mux.Lock()
}

func (db *database) wUnlock() {
	db.mux.Unlock()
}

func (db *database) rLock() {
	db.mux.RLock()
}

func (db *database) rUnlock() {
	db.mux.RUnlock()
}
