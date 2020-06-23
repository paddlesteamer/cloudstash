package manager

import (
	"fmt"
	"io"
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
	drives  []drive.Drive
	key     string
	db      dbStat
	c       *cache.Cache
	tracker *cache.Cache
}

const (
	checkInterval   time.Duration = 60 * time.Second
	processInterval time.Duration = 5 * time.Second
)

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

	file, err := common.NewTempDBFile()
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
		c:       newCache(),
		tracker: newTracker(),
	}

	go m.checkRemoteChanges()
	go m.processLocalChanges()

	return m, nil
}

func (m *Manager) Close() {
	os.Remove(m.db.dbPath)
}

func (m *Manager) NotifyChangeInFile(cachePath string, extPath string) {
	m.tracker.Add(cachePath, extPath, cacheForever)
}

func (m *Manager) NotifyChangeInDatabase() {
	m.tracker.Add(m.db.dbPath, common.GetURL(m.db.extDrive, m.db.extPath), cacheForever)
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

func (m *Manager) UpdateMetadata(inode int64) error {
	m.wLock()
	defer m.wUnlock()

	p, found := m.c.Get(strconv.FormatInt(inode, 10))
	if !found {
		return fmt.Errorf("the file hasn't beed cached")
	}

	path := p.(string)

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("couldn't open file %s: %v", path, err)
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return fmt.Errorf("couldn't get file stats %s: %v", path, err)
	}

	db, err := m.getDBClient()
	if err != nil {
		return fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	md, err := db.Get(inode)
	if err != nil {
		return fmt.Errorf("couldn't get file: %v", err)
	}

	md.Size = fi.Size()

	err = db.Update(md)
	if err != nil {
		return fmt.Errorf("couldn't update file metadata: %v", err)
	}

	return nil
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

func (m *Manager) OpenFile(md *common.Metadata, flag int) (*os.File, error) {
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
	m.c.Set(strconv.FormatInt(md.Inode, 10), path, cacheExpiration) // update expiration

	file, err := os.OpenFile(path, flag, os.ModeAppend)
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

	m.NotifyChangeInDatabase()

	return md, nil
}

func (m *Manager) CreateFile(parent int64, name string, mode int) (*common.Metadata, error) {
	m.wLock()
	defer m.wUnlock()

	db, err := m.getDBClient()
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to database: %v", err)
	}

	u := common.GetURL(m.selectDrive(), common.ObfuscateFileName(name))

	md, err := db.CreateFile(parent, name, mode, u)
	if err != nil {
		return nil, fmt.Errorf("couldn't create file in database: %v", err)
	}

	tmpfile, err := common.NewTempCacheFile()
	if err != nil {
		return nil, fmt.Errorf("couldn't create cached file: %v", err)
	}
	tmpfile.Close()

	m.c.Set(strconv.FormatInt(md.Inode, 10), tmpfile.Name(), cacheExpiration)

	m.NotifyChangeInDatabase()
	m.NotifyChangeInFile(tmpfile.Name(), md.URL)

	return md, nil
}

func (m *Manager) checkRemoteChanges() {
	for {
		time.Sleep(checkInterval)

		m.checkChanges()
	}
}

func (m *Manager) checkChanges() {
	mdata, err := m.db.extDrive.GetFileMetadata(m.db.extPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	m.wLock()
	defer m.wUnlock()

	if mdata.Hash == m.db.hash {
		return
	}

	_, reader, err := m.db.extDrive.GetFile(m.db.extPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't get updated db file: %v\n", err)

		return
	}
	defer reader.Close()

	file, err := os.Open(m.db.dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't open db: %v\n", err)

		return
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't copy contents of updated db file to local file: %v\n", err)

		return
	}
}

func (m *Manager) processLocalChanges() {

	for {
		time.Sleep(processInterval)

		m.processChanges()
	}
}

func (m *Manager) processChanges() {
	items := m.tracker.Items()
	m.tracker.Flush()

	m.rLock()
	defer m.rUnlock()

	for local, it := range items {
		url := it.Object.(string)

		u, err := common.ParseURL(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't parse url %s. skipping: %v", url, err)
			continue
		}

		drv, err := m.getDriveClient(u.Scheme)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't find drive client of %s: %v", u.Scheme, err)
			continue
		}

		file, err := os.Open(local)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't open file %s: %v", local, err)
			continue
		}
		defer file.Close()

		err = drv.PutFile(u.Path, file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't upload database file: %v", err)
			return
		}

		if local == m.db.dbPath { // if this file is database file
			m.db.hash, err = m.db.extDrive.ComputeHash(m.db.dbPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "couldn't compute hash of updated database: %v", err)
				return
			}
		}

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

	tmpfile, err := common.NewTempCacheFile()
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

// @TODO: select drive according to available space
func (m *Manager) selectDrive() drive.Drive {
	return m.drives[0]
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
