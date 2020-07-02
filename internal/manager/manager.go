package manager

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/crypto"
	"github.com/paddlesteamer/cloudstash/internal/drive"
	"github.com/paddlesteamer/cloudstash/internal/sqlite"
	"github.com/paddlesteamer/go-cache"
)

type Manager struct {
	drives  []drive.Drive
	key     string
	db      *database
	cache   *cache.Cache
	tracker *cache.Cache
	cipher  *crypto.Crypto
}

const (
	checkInterval   time.Duration = 60 * time.Second
	processInterval time.Duration = 5 * time.Second
)

func NewManager(drives []drive.Drive, db *database, cipher *crypto.Crypto, key string) *Manager {
	m := &Manager{
		drives:  drives,
		db:      db,
		key:     key,
		cache:   newCache(),
		tracker: newTracker(),
		cipher:  cipher,
	}

	go m.watchRemoteChanges()
	go m.processLocalChanges()
	return m
}

func (m *Manager) NotifyChangeInFile(cachePath string, extPath string) {
	m.tracker.Add(cachePath, extPath, cacheForever)
}

func (m *Manager) NotifyChangeInDatabase() {
	m.tracker.Add(m.db.path, common.GetURL(m.db.extDrive, m.db.extPath), cacheForever)
}

func (m *Manager) Lookup(parent int64, name string) (*common.Metadata, error) {
	m.db.rLock()
	defer m.db.rUnlock()

	db, err := sqlite.NewClient()
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
	m.db.rLock()
	defer m.db.rUnlock()

	db, err := sqlite.NewClient()
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

func (m *Manager) UpdateMetadataFromCache(inode int64) error {
	m.db.wLock()
	defer m.db.wUnlock()

	p, found := m.cache.GetWithExpirationUpdate(strconv.FormatInt(inode, 10), cacheExpiration)
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

	db, err := sqlite.NewClient()
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

	m.NotifyChangeInDatabase()

	return nil
}

func (m *Manager) UpdateMetadata(md *common.Metadata) error {
	m.db.wLock()
	defer m.db.wUnlock()

	db, err := sqlite.NewClient()
	if err != nil {
		return fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	err = db.Update(md)
	if err != nil {
		return fmt.Errorf("couldn't update file metadata: %v", err)
	}

	m.NotifyChangeInDatabase()

	return nil
}

func (m *Manager) GetDirectoryContent(parent int64) ([]common.Metadata, error) {
	m.db.rLock()
	defer m.db.rUnlock()

	db, err := sqlite.NewClient()
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

func (m *Manager) RemoveDirectory(ino int64) error {
	m.db.wLock()
	defer m.db.wUnlock()

	db, err := sqlite.NewClient()
	if err != nil {
		return fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	mdList, err := db.GetChildren(ino)
	if err != nil {
		return fmt.Errorf("couldn't get children of %d: %v", ino, err)
	}

	for _, md := range mdList {
		m.cache.Delete(strconv.FormatInt(md.Inode, 10))

		go m.deleteRemoteFile(&md)
	}

	err = db.DeleteChildren(ino)
	if err != nil {
		return fmt.Errorf("couldn't delete children of inode %d: %v", ino, err)
	}

	err = db.Delete(ino)
	if err != nil {
		return fmt.Errorf("children are removed but couldn't delete the parent itself of inode %d: %v", ino, err)
	}

	m.NotifyChangeInDatabase()

	return nil
}

func (m *Manager) RemoveFile(md *common.Metadata) error {
	m.db.wLock()
	defer m.db.wUnlock()

	m.cache.Delete(strconv.FormatInt(md.Inode, 10))

	go m.deleteRemoteFile(md)

	db, err := sqlite.NewClient()
	if err != nil {
		return fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	err = db.Delete(md.Inode)
	if err != nil {
		return fmt.Errorf("couldn't delete file: %v", err)
	}

	m.NotifyChangeInDatabase()

	return nil
}

func (m *Manager) OpenFile(md *common.Metadata, flag int) (*os.File, error) {
	var path string

	p, found := m.cache.GetWithExpirationUpdate(strconv.FormatInt(md.Inode, 10), cacheExpiration)
	if !found {
		p, err := m.downloadFile(md)
		if err != nil {
			return nil, fmt.Errorf("couldn't get file from storage %s: %v", md.Name, err)
		}

		path = p
	} else {
		path = p.(string)
	}

	file, err := os.OpenFile(path, flag, os.ModeAppend)
	if err != nil {
		return nil, fmt.Errorf("couldn't open file %s: %v", path, err)
	}

	return file, nil
}

func (m *Manager) AddDirectory(parent int64, name string, mode int) (*common.Metadata, error) {
	m.db.wLock()
	defer m.db.wUnlock()

	db, err := sqlite.NewClient()
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
	m.db.wLock()
	defer m.db.wUnlock()

	db, err := sqlite.NewClient()
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

	m.cache.Set(strconv.FormatInt(md.Inode, 10), tmpfile.Name(), cacheExpiration)

	m.NotifyChangeInDatabase()
	m.NotifyChangeInFile(tmpfile.Name(), md.URL)

	return md, nil
}

func (m *Manager) watchRemoteChanges() {
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

	m.db.wLock()
	defer m.db.wUnlock()

	if mdata.Hash == m.db.hash {
		return
	}

	_, reader, err := m.db.extDrive.GetFile(m.db.extPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't get updated db file: %v\n", err)

		return
	}
	defer reader.Close()

	file, err := os.Open(m.db.path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't open db: %v\n", err)

		return
	}
	defer file.Close()

	_, err = io.Copy(file, m.cipher.NewDecryptReader(reader))
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
	m.db.rLock()
	defer m.db.rUnlock()

	for local, it := range items {
		url := it.Object.(string)

		u, err := common.ParseURL(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't parse url %s. skipping: %v\n", url, err)
			continue
		}

		drv, err := m.getDriveClient(u.Scheme)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't find drive client of %s: %v\n", u.Scheme, err)
			continue
		}

		file, err := os.Open(local)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't open file %s: %v\n", local, err)
			continue
		}
		defer file.Close()

		hs := crypto.NewHashStream(drv)

		err = drv.PutFile(u.Path, hs.NewHashReader(m.cipher.NewEncryptReader(file)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't upload file: %v\n", err)
			return
		}

		hash, err := hs.GetComputedHash()
		if err != nil {
			// just log the error and continue
			fmt.Fprintf(os.Stderr, "couldn't compute hash of file: %v\n", err)
		}

		fmt.Println(hash)

		// if this file is database file
		if local == m.db.path {
			m.db.hash = hash
		}
	}
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

	_, err = io.Copy(tmpfile, m.cipher.NewDecryptReader(reader))
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("couldn't copy contents of downloaded file to cache: %v", err)
	}

	return tmpfile.Name(), nil
}

func (m *Manager) deleteRemoteFile(md *common.Metadata) {
	// @TODO: implement
}

// @TODO: select drive according to available space
func (m *Manager) selectDrive() drive.Drive {
	return m.drives[0]
}
