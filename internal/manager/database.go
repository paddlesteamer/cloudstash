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

	log "github.com/sirupsen/logrus"
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
		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}

		return nil, fmt.Errorf("could not open intitialized DB: %v", err)
	}
	defer file.Close()

	hs := crypto.NewHashStream(extDrive)

	err = extDrive.PutFile(common.DatabaseFileName, hs.NewHashReader(cipher.NewEncryptReader(file)))
	if err != nil {
		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}

		return nil, fmt.Errorf("could not upload initialized DB: %v", err)
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}

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
		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}

		return nil, fmt.Errorf("could not copy contents of DB to local file: %v", err)
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}

		return nil, fmt.Errorf("couldn't compute hash of database file: %v", err)
	}

	file.Close()

	db, err := sqlite.NewClient(file.Name())
	if err != nil {
		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}

		return nil, fmt.Errorf("couldn't connect to downloaded DB file: %v", err)
	}
	defer db.Close()

	if !db.IsValidDatabase() {
		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}

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
	if err := os.Remove(db.path); err != nil {
		log.Warningf("couldn't remove file '%s' from filesystem: %v", db.path, err)
	}
}

// merge tries to merge two databases. returns error if it can not
// and restores local database.
// the rules of merge are:
// - if a file is changed or relocated on remote database, the changes are ignored
// - if a file is removed from remote database, it is added again
// - if a new file is added to remote database, it is added to local database too
// - remote database's inode numbers are used in local db in order to get synchronized with other clients
func (db *database) merge(path string) error {
	// backup local copy just in case
	backup, err := db.backupDatabase()
	if err != nil {
		return fmt.Errorf("couldn't get backup of current DB file: %v", err)
	}
	defer os.Remove(backup)

	localDb, err := sqlite.NewClient(db.path)
	if err != nil {
		return fmt.Errorf("couldn't connect to local DB: %v", err)
	}

	remoteDb, err := sqlite.NewClient(path)
	if err != nil {
		return fmt.Errorf("couldn't connect to local copy of remote DB: %v", err)
	}

	merge(localDb, remoteDb)

	return nil
}

func merge(local *sqlite.Client, remote *sqlite.Client) error {
	defer local.Close()
	defer remote.Close()

	rowCount, err := remote.GetRowCount()
	if err != nil {
		return fmt.Errorf("couldn't get row count: %v", err)
	}

	chunkSize := 1000 //rows
	threadLimit := 32

	offset := 0
	thCount := 0

	mu := sync.RWMutex{}
	wg := sync.WaitGroup{}
	errChan := make(chan error)

	for offset < rowCount {
		if (rowCount - offset) < chunkSize {
			chunkSize = rowCount - offset
		}

		mdList, err := remote.GetRows(chunkSize, offset)
		if err != nil {
			return fmt.Errorf("couldn't get rows: %v", err)
		}

		offset += chunkSize

		go processChunk(mdList, local, &wg, &mu, errChan)

		thCount++

		// don't start any more go routines if thread limit is reached
		if thCount == threadLimit {
			wg.Wait()
			thCount = 0

			select {
			case err := <-errChan:
				return fmt.Errorf("couldn't process chunks: %v", err)
			default:
				// continue
			}
		}
	}

	if thCount > 0 {
		wg.Wait()

		select {
		case err := <-errChan:
			return fmt.Errorf("couldn't process chunks: %v", err)
		default:
			// continue
		}
	}

	return nil
}

func processChunk(mdList []sqlite.Metadata, local *sqlite.Client, wg *sync.WaitGroup, mu *sync.RWMutex, errChan chan error) {
	wg.Add(1)
	defer wg.Done()

	for _, md := range mdList {
		mu.RLock()
		lmd, err := local.Get(md.Inode)
		if err != nil && err != common.ErrNotFound {
			errChan <- fmt.Errorf("couldn't get metadata of inode %d: %v", md.Inode, err)
			mu.RUnlock()

			return
		}

		if err == common.ErrNotFound {
			// force insert with inode
		}

		if lmd.Name != md.Name && lmd.Parent != md.Parent && lmd.Hash != md.Hash {
			// assuming it is a complete different file
			// replace remote row with the local one and
			// re-insert local row with a new inode
			// if we don't do this way, the other client's cache may have
			// wrong files.
			// it is important to delete this inode from local cache.
		}
	}
}

// backupDatabase creates a copy of current database and returns its path
func (db *database) backupDatabase() (string, error) {
	dst, err := common.NewTempDBFile()
	if err != nil {
		return "", fmt.Errorf("couldn't create backup file: %v", err)
	}
	defer dst.Close()

	src, err := os.Open(db.path)
	if err != nil {
		dst.Close()
		if err := os.Remove(dst.Name()); err != nil {
			log.Warningf("couldn't remove new created backup file '%s': %v", dst.Name(), err)
		}

		return "", fmt.Errorf("couldn't open current database: %v", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		if err := os.Remove(dst.Name()); err != nil {
			log.Warningf("couldn't remove new created backup file '%s': %v", dst.Name(), err)
		}

		return "", fmt.Errorf("couldn't copy current database: %v", err)

	}

	return dst.Name(), nil
}

// restoreDatabase restores current database from source
func (db *database) restoreDatabase(source string) error {
	dst, err := os.Create(db.path)
	if err != nil {
		return fmt.Errorf("critical error, couldn't open current database file: %v", err)
	}
	defer dst.Close()

	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("critical error, couldn't open backup file: %v", err)
	}
	defer src.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("critical error: couldn't copy contents from backup file: %v", err)
	}

	return nil
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
