package manager

import (
	"os"
	"sync"

	"github.com/paddlesteamer/cloudstash/internal/drive"
)

type database struct {
	path     string       // local path of database
	extPath  string       // remote path of database (i.e. dropbox://cloudstash.sqlite3)
	hash     string       // content hash of database computed by extDrive.ComputeHash
	extDrive drive.Drive  // driver for remote operations
	mux      sync.RWMutex // used in database queries, executions since go-sqlite3 isn't thread safe
}

// NewDB creates new database with provided parameters
func NewDB(path, extPath, hash string, extDrive drive.Drive) *database {
	return &database{
		path:     path,
		extPath:  extPath,
		hash:     hash,
		extDrive: extDrive,
	}
}

// Close deletes database file from local filesystem
// It should be called on exit
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
