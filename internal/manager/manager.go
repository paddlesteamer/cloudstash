package manager

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/paddlesteamer/hdn-drv/internal/common"
	"github.com/paddlesteamer/hdn-drv/internal/config"
	"github.com/paddlesteamer/hdn-drv/internal/db"
	"github.com/paddlesteamer/hdn-drv/internal/drive"
)

type dbStat struct {
	extPath  string
	extDrive drive.Drive
	dbPath   string
	hash     string
	mux      sync.RWMutex
}

type Manager struct {
	drives []drive.Drive
	key    string
	db     dbStat
}

const checkInterval time.Duration = 5 * time.Second

func NewManager(conf *config.Configuration) (*Manager, error) {
	key := conf.EncryptionKey

	drives := []drive.Drive{}
	if conf.Dropbox != nil {
		dbx := drive.NewDropboxClient(conf.Dropbox)
		drives = append(drives, dbx)
	}

	u, err := url.Parse(conf.DatabaseFile)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse database file url: %v", err)
	}

	var drv drive.Drive = nil
	for _, d := range drives {
		if d.GetProviderName() == u.Scheme {
			drv = d
			break
		}
	}

	if drv == nil {
		return nil, fmt.Errorf("couldn't find a drive matching database file scheme")
	}

	file, err := ioutil.TempFile("/tmp", "hdn-drv-db")
	if err != nil {
		return nil, fmt.Errorf("manager: unable to create database file: %v", err)
	}
	// defer dbf.Close() close manually, at least shouldn't be deferred here

	dbExtPath := fmt.Sprintf("/%s", u.Host)
	dbPath := file.Name()

	_, reader, err := drv.GetFile(u.Host)
	if err != nil { // TODO: check specific 'not found' error
		// below is for not found error
		file.Close()

		err = db.InitDB(dbPath)
		if err != nil {
			return nil, fmt.Errorf("couldn't initialize db: %v", err)
		}

		dbf, err := os.Open(dbPath)
		if err != nil {
			os.Remove(dbPath)
			return nil, fmt.Errorf("couldn't open intitialized db: %v", err)
		}
		defer dbf.Close()

		err = drv.PutFile(dbExtPath, dbf)
		if err != nil {
			os.Remove(dbPath)
			return nil, fmt.Errorf("couldn't upload initialiezed db: %v", err)
		}

	} else {
		defer reader.Close()
		defer file.Close()

		_, err := io.Copy(file, reader)
		if err != nil {
			os.Remove(dbPath)
			return nil, fmt.Errorf("couldn't copy contents of db to local file: %v", err)
		}
	}

	hash, err := drv.ComputeHash(dbPath)
	if err != nil {
		os.Remove(dbPath)
		return nil, fmt.Errorf("couldn't compute hash: %v", err)
	}

	m := &Manager{
		drives: drives,
		key:    key,
		db: dbStat{
			extDrive: drv,
			extPath:  dbExtPath,

			dbPath: dbPath,
			hash:   hash,
		},
	}

	go m.checkChanges()

	return m, nil
}

func (m *Manager) Close() {
	os.Remove(m.db.dbPath)
}

func (m *Manager) Lookup(parent int64, name string) (*common.Metadata, error) {
	m.rLock()
	defer m.rUnlock()

	db, err := m.getDBClient()
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	md, err := db.Search(parent, name)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, err
		}

		return nil, fmt.Errorf("something went wrong with query: %v", err)
	}

	return md, nil
}

func (m *Manager) GetMetadata(inode int64) (*common.Metadata, error) {
	m.rLock()
	defer m.rUnlock()

	db, err := m.getDBClient()
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	md, err := db.Get(inode)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, err
		}

		return nil, fmt.Errorf("something went wrong with query: %v", err)
	}

	return md, nil
}

func (m *Manager) GetDirectoryContent(parent int64) ([]common.Metadata, error) {
	m.rLock()
	defer m.rUnlock()

	db, err := m.getDBClient()
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	md, err := db.Get(parent)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, err
		}

		return nil, fmt.Errorf("something went wrong with query: %v", err)
	}

	if md.Type != common.DRV_FOLDER {
		return nil, fmt.Errorf("the requested inode is not a directory: %d", md.Type)
	}

	mdList, err := db.GetChildren(parent)
	if err != nil {
		return nil, fmt.Errorf("couldn't get children of %d: %v", parent, err)
	}

	return mdList, nil
}

func (m *Manager) checkChanges() {
	for {
		time.Sleep(checkInterval)

		mdata, err := m.db.extDrive.GetFileMetadata(m.db.extPath)
		if err != nil {
			fmt.Printf("%v\n", err)
			continue
		}

		m.wLock()
		if mdata.Hash == m.db.hash {
			m.wUnlock()
			continue
		}

		_, reader, err := m.db.extDrive.GetFile(m.db.extPath)
		if err != nil {
			fmt.Printf("couldn't get updated db file: %v", err)
			m.wUnlock()
			continue
		}

		file, err := os.Open(m.db.dbPath)
		if err != nil {
			fmt.Printf("couldn't open db: %v", err)
			m.wUnlock()
			continue
		}

		_, err = io.Copy(file, reader)
		if err != nil {
			fmt.Printf("couldn't copy contents of updated db file to local file: %v", err)
			m.wUnlock()
			continue
		}

		reader.Close()
		file.Close()

		m.wUnlock()
	}
}

func (m *Manager) getDBClient() (*db.Client, error) {
	cli, err := db.NewClient(m.db.dbPath)

	return cli, err
}

func (m *Manager) wLock() {
	m.db.mux.Lock()
}

func (m *Manager) wUnlock() {
	m.db.mux.Unlock()
}

func (m *Manager) rLock() {
	m.db.mux.RLock()
}

func (m *Manager) rUnlock() {
	m.db.mux.RUnlock()
}
