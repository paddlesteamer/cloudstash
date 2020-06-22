package manager

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/paddlesteamer/hdn-drv/internal/common"
	"github.com/paddlesteamer/hdn-drv/internal/config"
	"github.com/paddlesteamer/hdn-drv/internal/db"
	"github.com/paddlesteamer/hdn-drv/internal/drive"
	"github.com/patrickmn/go-cache"
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
	c      *cache.Cache
}

const checkInterval time.Duration = 5 * time.Second

func NewManager(conf *config.Configuration) (*Manager, error) {
	key := conf.EncryptionKey

	drives := []drive.Drive{}
	if conf.Dropbox != nil {
		dbx := drive.NewDropboxClient(conf.Dropbox)
		drives = append(drives, dbx)
	}

	fu, err := common.ParseURL(conf.DatabaseFile)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse database file url: %v", err)
	}

	var drv drive.Drive = nil
	for _, d := range drives {
		if d.GetProviderName() == fu.Scheme {
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

	dbExtPath := fu.Path
	dbPath := file.Name()

	_, reader, err := drv.GetFile(dbExtPath)
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
		c: newCache(),
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

func (m *Manager) GetFile(md *common.Metadata) (*os.File, error) {
	var path string

	p, found := m.c.Get(strconv.FormatInt(md.Inode, 10))
	if !found {
		p, err := m.downloadFile(md)
		if err != nil {
			return nil, fmt.Errorf("couldn't get file from storage %s: %v", md.Name, err)
		}

		path = p
	} else {
		path = p.(string)
	}
	m.c.Set(strconv.FormatInt(md.Inode, 10), path, cache.DefaultExpiration) // update expiration

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("couldn't open file %s: %v", path, err)
	}

	return file, nil
}

func (m *Manager) AddDirectory(parent int64, name string, mode int) (*common.Metadata, error) {
	m.wLock()
	defer m.wUnlock()

	db, err := m.getDBClient()
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to database: %v", err)
	}

	md, err := db.AddDirectory(parent, name, mode)
	if err != nil {
		return nil, fmt.Errorf("couldn't create directory in database: %v", err)
	}

	go m.uploadDatabase()

	return md, nil
}

func (m *Manager) uploadDatabase() {
	m.wLock()
	defer m.wUnlock()

	file, err := os.Open(m.db.dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't open database file: %v", err)
		return
	}
	defer file.Close()

	err = m.db.extDrive.PutFile(m.db.extPath, file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't upload database file: %v", err)
		return
	}

	m.db.hash, err = m.db.extDrive.ComputeHash(m.db.dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't compute hash of updated database: %v", err)
		return
	}
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
			fmt.Fprintf(os.Stderr, "couldn't get updated db file: %v\n", err)
			m.wUnlock()
			continue
		}

		file, err := os.Open(m.db.dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't open db: %v\n", err)
			m.wUnlock()
			continue
		}

		_, err = io.Copy(file, reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't copy contents of updated db file to local file: %v\n", err)
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

func (m *Manager) getDriveClient(scheme string) (drive.Drive, error) {
	for _, drv := range m.drives {
		if drv.GetProviderName() == scheme {
			return drv, nil
		}
	}

	return nil, fmt.Errorf("couldn't find driver")
}

func (m *Manager) downloadFile(md *common.Metadata) (string, error) {
	u, err := common.ParseURL(md.URL)
	if err != nil {
		return "", fmt.Errorf("couldn't parse file url %s: %v", md.URL, err)
	}

	drv, err := m.getDriveClient(u.Scheme)
	if err != nil {
		return "", err
	}

	_, reader, err := drv.GetFile(u.Path)
	if err != nil {
		return "", fmt.Errorf("couldn't get file '%s' from storage: %v", md.URL, err)
	}
	defer reader.Close()

	tmpfile, err := ioutil.TempFile("/tmp", "hdn-drv-cached-")
	if err != nil {
		return "", fmt.Errorf("couldn't create cached file: %v", err)
	}
	defer tmpfile.Close()

	_, err = io.Copy(tmpfile, reader)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("couldn't copy contents of downloaded file to cache: %v", err)
	}

	return tmpfile.Name(), nil
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
