package manager

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/crypto"
	"github.com/paddlesteamer/go-cache"
)

const (
	checkInterval   time.Duration = 60 * time.Second
	processInterval time.Duration = 2 * time.Second
)

func watchRemoteChanges(m *Manager) {
	for {
		time.Sleep(checkInterval)
		if checkChanges(m) {
			updateCache(m)
		}
	}
}

// checkChanges checks whether the remote database file is changed
// and updates local database file if necessary
func checkChanges(m *Manager) bool {
	mdata, err := m.db.extDrive.GetFileMetadata(common.DatabaseFileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return false
	}

	m.db.wLock()
	defer m.db.wUnlock()

	if mdata.Hash == m.db.hash {
		return false
	}

	reader, err := m.db.extDrive.GetFile(common.DatabaseFileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't get updated db file: %v\n", err)

		return false
	}
	defer reader.Close()

	file, err := os.Create(m.db.path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't open db: %v\n", err)

		return false
	}
	defer file.Close()

	hs := crypto.NewHashStream(m.db.extDrive)

	_, err = io.Copy(file, m.cipher.NewDecryptReader(reader))
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't copy contents of updated db file to local file: %v\n", err)

		return false
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't compute hash of database file: %v", err)

		return false
	}

	m.db.hash = hash

	return true
}

func updateCache(m *Manager) {
	m.db.rLock()
	defer m.db.rUnlock()

	db, err := m.getSqliteClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't connect to database: %v\n", err)

		return
	}
	defer db.Close()

	m.cache.FlushWithFilter(func(key string, it *cache.Item) bool {
		md, err := db.Get(common.ToInt64(key))
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't get metadata of %s: %v\n", key, err)

			return false
		}

		entry := it.Object.(cacheEntry)

		return entry.hash != md.Hash
	})
}

const (
	forceAll        = iota
	checkAccessTime = iota
)

func processLocalChanges(m *Manager) {
	for {
		time.Sleep(processInterval)
		processChanges(m, checkAccessTime)
	}
}

// processChanges uploads changed local files to remote drive
// if forceAll is provided, it ignores access time
// and uploads all files in the tracker
func processChanges(m *Manager, flag int) {
	var items map[string]*cache.Item

	if flag == forceAll {
		items = m.tracker.Flush()
	} else {
		items = m.tracker.FlushWithFilter(accessFilter)
	}

	if len(items) == 0 {
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(len(items))

	for _, it := range items {
		entry := it.Object.(trackerEntry)

		go processItem(entry.cachePath, entry.remotePath, m, &wg)
	}

	// wait for all uploads to complete otherwise
	// the next processChanges call may conflict with this one
	wg.Wait()
}

func processItem(local string, url string, m *Manager, wg *sync.WaitGroup) {
	defer wg.Done()

	isDBFile := local == m.db.path

	if isDBFile {
		m.db.wLock()
		defer m.db.wUnlock()
	}

	u, err := common.ParseURL(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't parse url %s. skipping: %v\n", url, err)
		return
	}

	drv, err := m.getDriveClient(u.Scheme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't find drive client of %s: %v\n", u.Scheme, err)
		return
	}

	if isDBFile {
		md, err := drv.GetFileMetadata(u.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't fet metadata of DB file: %v\n", err)
			return
		}

		if md.Hash != m.db.hash {
			// @TODO: try to merge databases first
			fmt.Fprintf(os.Stderr, "remote DB file is also changed, moving remote file...\n")

			err := drv.MoveFile(common.DatabaseFileName,
				common.GenerateConflictedFileName(common.DatabaseFileName))
			if err != nil {
				// log and ignore
				fmt.Fprintf(os.Stderr, "unable to rename remote DB file: %v\n", err)
			}
		}
	}

	file, err := os.Open(local)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't open file %s: %v\n", local, err)
		return
	}
	defer file.Close()

	hs := crypto.NewHashStream(drv)

	err = drv.PutFile(u.Name, hs.NewHashReader(m.cipher.NewEncryptReader(file)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't upload file: %v\n", err)
		return
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		// just log the error and continue
		fmt.Fprintf(os.Stderr, "couldn't compute hash of file: %v\n", err)
	}

	if isDBFile {
		m.db.hash = hash
	}

}
