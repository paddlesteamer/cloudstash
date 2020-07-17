package manager

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/crypto"
	"github.com/paddlesteamer/cloudstash/internal/drive"
	"github.com/paddlesteamer/cloudstash/internal/sqlite"
)

type database struct {
	path     string       // local path of database
	hash     string       // content hash of database computed by extDrive.ComputeHash
	extDrive drive.Drive  // drive client for remote operations
	mux      sync.RWMutex // used in database queries, executions since go-sqlite3 isn't thread safe
}

// newDB creates new database and uploads it
func newDB(extDrive drive.Drive, cipher *crypto.Cipher) (*database, error) {
	file, err := common.NewTempDBFile()
	if err != nil {
		return nil, fmt.Errorf("could not create DB file: %v", err)
	}
	file.Close()

	if err := sqlite.InitDB(file.Name()); err != nil {
		return nil, fmt.Errorf("could not initialize DB: %v", err)
	}

	// reopen file
	file, err = os.Open(file.Name())
	if err != nil {
		os.Remove(file.Name())
		return nil, fmt.Errorf("could not open intitialized DB: %v", err)
	}
	defer file.Close()

	hs := crypto.NewHashStream(extDrive)

	err = extDrive.PutFile(common.DatabaseFileName, hs.NewHashReader(cipher.NewEncryptReader(file)))
	if err != nil {
		os.Remove(file.Name())
		return nil, fmt.Errorf("could not upload initialized DB: %v", err)
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		os.Remove(file.Name())
		return nil, fmt.Errorf("couldn't compute hash of newly installed DB: %v", err)
	}

	return &database{
		path:     file.Name(),
		hash:     hash,
		extDrive: extDrive,
	}, nil
}

// fetchDB fetches database from remote storage
func fetchDB(extDrive drive.Drive, cipher *crypto.Cipher) (*database, error) {
	file, err := common.NewTempDBFile()
	if err != nil {
		return nil, fmt.Errorf("could not create DB file: %v", err)
	}

	reader, err := extDrive.GetFile(common.DatabaseFileName)
	if err != nil {
		return nil, fmt.Errorf("couldn't get database file: %v", err)
	}
	defer reader.Close()

	hs := crypto.NewHashStream(extDrive)

	_, err = io.Copy(file, cipher.NewDecryptReader(hs.NewHashReader(reader)))
	if err != nil {
		os.Remove(file.Name())
		return nil, fmt.Errorf("could not copy contents of DB to local file: %v", err)
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		os.Remove(file.Name())
		return nil, fmt.Errorf("couldn't compute hash of database file: %v", err)
	}

	file.Close()

	db, err := sqlite.NewClient(file.Name())
	if err != nil {
		os.Remove(file.Name())
		return nil, fmt.Errorf("couldn't connect to downloaded DB file: %v", err)
	}
	defer db.Close()

	if !db.IsValidDatabase() {
		os.Remove(file.Name())
		return nil, fmt.Errorf("couldn't verify the downloaded database file")
	}

	return &database{
		path:     file.Name(),
		hash:     hash,
		extDrive: extDrive,
	}, nil
}

// clean deletes database file from local filesystem
// It should be called on exit
func (db *database) clean() {
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
