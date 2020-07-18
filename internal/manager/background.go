package manager

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/crypto"
	"github.com/paddlesteamer/cloudstash/internal/sqlite"
	"github.com/paddlesteamer/go-cache"

	log "github.com/sirupsen/logrus"
)

const (
	checkInterval   time.Duration = 10 * time.Second
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
		log.Errorf("couldn't get metadata of remote DB file: %v", err)
		return false
	}

	m.db.wLock()
	defer m.db.wUnlock()

	if mdata.Hash == m.db.hash {
		return false
	}

	reader, err := m.db.extDrive.GetFile(common.DatabaseFileName)
	if err != nil {
		log.Errorf("couldn't get updated db file: %v", err)

		return false
	}
	defer reader.Close()

	file, err := common.NewTempDBFile()
	if err != nil {
		log.Errorf("could not create DB file: %v", err)

		return false
	}

	hs := crypto.NewHashStream(m.db.extDrive)

	_, err = io.Copy(file, m.cipher.NewDecryptReader(reader))
	if err != nil {
		log.Errorf("couldn't copy contents of updated db file to local file: %v", err)

		file.Close()
		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}

		return false
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		log.Errorf("couldn't compute hash of database file: %v", err)

		file.Close()
		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}

		return false
	}

	file.Close()

	db, err := sqlite.NewClient(file.Name())
	if err != nil {
		log.Errorf("couldn't connect to downloaded DB file: %v", err)

		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}
		return false
	}
	defer db.Close()

	if !db.IsValidDatabase() {
		log.Error("couldn't verify the downloaded database file")

		if err := os.Remove(file.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", file.Name(), err)
		}
		return false
	}

	if err := os.Remove(m.db.path); err != nil {
		log.Warningf("couldn't remove file '%s' from filesystem: %v", m.db.path, err)
	}

	m.db.hash = hash
	m.db.path = file.Name()

	return true
}

func updateCache(m *Manager) {
	m.db.rLock()
	defer m.db.rUnlock()

	db, err := m.getSqliteClient()
	if err != nil {
		log.Errorf("couldn't connect to database: %v", err)

		// fallback to flush all
		m.cache.Flush()

		return
	}
	defer db.Close()

	m.cache.FlushWithFilter(func(key string, it *cache.Item) bool {
		entry := it.Object.(cacheEntry)

		md, err := db.Get(common.ToInt64(key))
		if err != nil {
			log.Errorf("couldn't get metadata of %s: %v", key, err)

			// if there is an error, remove it
			if err := os.Remove(entry.path); err != nil {
				log.Warningf("couldn't remove file '%s' from filesytem: %v", entry.path, err)
			}
			return true
		}

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
		log.Errorf("couldn't parse url %s. skipping: %v", url, err)
		return
	}

	drv, err := m.getDriveClient(u.Scheme)
	if err != nil {
		log.Errorf("couldn't find drive client of %s: %v", u.Scheme, err)
		return
	}

	if isDBFile {
		md, err := drv.GetFileMetadata(u.Name)
		if err != nil {
			log.Errorf("couldn't get metadata of DB file: %v", err)

			// re-add to tracker
			m.notifyChangeInDatabase()
			return
		}

		if md.Hash != m.db.hash {
			log.Warning("remote DB file is also changed")

			remoteDb, err := common.NewTempDBFile()
			if err != nil {
				log.Errorf("couldn't create file for remote DB: %v", err)

				// re-add to tracker
				m.notifyChangeInDatabase()
				return
			}

			reader, err := drv.GetFile(common.DatabaseFileName)
			if err != nil {
				log.Errorf("couldn't download remote copy of the DB: %v", err)

				remoteDb.Close()

				if err := os.Remove(remoteDb.Name()); err != nil {
					log.Warningf("couldn't remove DB file '%s': %v", remoteDb.Name(), err)
				}

				// re-add to tracker
				m.notifyChangeInDatabase()
				return
			}
			defer reader.Close()

			if _, err := io.Copy(remoteDb, reader); err != nil {
				log.Errorf("couldn't copy contents of remote DB: %v", err)

				remoteDb.Close()

				if err := os.Remove(remoteDb.Name()); err != nil {
					log.Warningf("couldn't remove DB file '%s': %v", remoteDb.Name(), err)
				}

				// re-add to tracker
				m.notifyChangeInDatabase()
				return
			}

			remoteDb.Close()

			if err := m.db.merge(remoteDb.Name(), m.cache); err != nil {
				log.Errorf("couldn't merge local DB with the remote one: %v", err)

				if err == errDatabaseBricked {
					// if local database is broken, fallback to remote database
					// this is a very dirty thing to do but it's if it comes to this
					// this is our best option

					if err := os.Remove(m.db.path); err != nil {
						log.Warningf("couldn't remove DB file '%s': %v", m.db.path, err)
					}

					m.db.path = remoteDb.Name()
					return
				}

				if err := os.Remove(remoteDb.Name()); err != nil {
					log.Warningf("couldn't remove DB file '%s': %v", remoteDb.Name(), err)
				}

				// there is an error but database file isn't lost. try again next time
				// re-add to tracker
				m.notifyChangeInDatabase()
				return
			}

			// make a conflicted copy just in case
			err = drv.MoveFile(common.DatabaseFileName,
				common.GenerateConflictedFileName(common.DatabaseFileName))
			if err != nil {
				// log and ignore
				log.Warningf("unable to rename remote DB file: %v", err)
			}
		}
	}

	file, err := os.Open(local)
	if err != nil {
		log.Errorf("couldn't open file %s: %v", local, err)

		// re-add to tracker
		m.notifyChangeInFile(local, url)
		return
	}
	defer file.Close()

	hs := crypto.NewHashStream(drv)

	err = drv.PutFile(u.Name, hs.NewHashReader(m.cipher.NewEncryptReader(file)))
	if err != nil {
		log.Errorf("couldn't upload file: %v", err)

		// re-add to tracker
		m.notifyChangeInFile(local, url)
		return
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		// log it and continue
		log.Errorf("couldn't compute hash of file: %v", err)

		// this will force download of database
		hash = ""
	}

	if isDBFile {
		m.db.hash = hash
	}

}
