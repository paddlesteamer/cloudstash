package manager

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/crypto"
	"github.com/paddlesteamer/cloudstash/internal/drive"
	"github.com/paddlesteamer/cloudstash/internal/sqlite"
	"zgo.at/zcache"

	log "github.com/sirupsen/logrus"
)

// Manager is where all the business logic happens
// It is responsible for keeping track of drives, cache, database, etc.
// It is used by the `fs` package
type Manager struct {
	drives  []drive.Drive
	db      *database
	cache   *zcache.Cache
	tracker *zcache.Cache
	cipher  *crypto.Cipher

	availableSpace int64
}

// NewManager creates a new Manager struct with provided
// parameters and starts background processes
func NewManager(drives []drive.Drive, dbDrv drive.Drive, cipher *crypto.Cipher) (*Manager, error) {
	m := &Manager{
		drives:  drives,
		cache:   newCache(),
		tracker: newTracker(),
		cipher:  cipher,
	}

	var db *database

	// DB doesn't exist
	if dbDrv == nil {
		drv := m.selectDrive()

		d, err := newDB(drv, cipher)
		if err != nil {
			return nil, fmt.Errorf("couldn't intialize DB: %v", err)
		}

		db = d
	} else {
		d, err := fetchDB(dbDrv, cipher)
		if err != nil {
			return nil, fmt.Errorf("couldn't fetch DB: %v", err)
		}

		db = d
	}

	m.db = db

	go watchRemoteChanges(m)
	go processLocalChanges(m)

	return m, nil
}

// Clean clean ups cached files and process remaining file changes
func (m *Manager) Clean() {
	processChanges(m, forceAll)

	m.cache.DeleteAll()

	m.db.clean()
}

// Lookup searches provided directory for a file provided with 'name' parameter
// If found returns it's metadata, if not found returns ErrNotFound
func (m *Manager) Lookup(parent int64, name string) (*sqlite.Metadata, error) {
	m.db.rLock()
	defer m.db.rUnlock()

	db, err := m.getSqliteClient()
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

// GetMetadata returns metadata of file with provided inode
func (m *Manager) GetMetadata(inode int64) (*sqlite.Metadata, error) {
	m.db.rLock()
	defer m.db.rUnlock()

	db, err := m.getSqliteClient()
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

// UpdateMetadataFromCache checks changes in file from the cached version
// and updates database accordingly. If the content is also changed
// it calls notifyChangeInFile in addition to notifyChangeInDatabase
func (m *Manager) UpdateMetadataFromCache(inode int64) error {
	m.db.wLock()
	defer m.db.wUnlock()

	e, found := m.cache.Touch(common.ToString(inode), cacheExpiration)
	if !found {
		return fmt.Errorf("the file hasn't been cached")
	}

	path := e.(cacheEntry).path

	file, err := os.Open(path)
	if err != nil {
		m.cache.Delete(common.ToString(inode))
		return fmt.Errorf("couldn't open file %s: %v", path, err)
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		m.cache.Delete(common.ToString(inode))
		return fmt.Errorf("couldn't get file stats %s: %v", path, err)
	}

	db, err := m.getSqliteClient()
	if err != nil {
		m.cache.Delete(common.ToString(inode))
		return fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	md, err := db.Get(inode)
	if err != nil {
		m.cache.Delete(common.ToString(inode))
		return fmt.Errorf("couldn't get file: %v", err)
	}

	checksum, err := crypto.MD5Checksum(file)
	if err != nil {
		m.cache.Delete(common.ToString(inode))
		return fmt.Errorf("couldn't compute md5 checksum: %v", err)
	}

	if md.Hash != checksum {
		md.Size = fi.Size()
		md.Hash = checksum
		err = db.Update(md)
		if err != nil {
			m.cache.Delete(common.ToString(inode))
			return fmt.Errorf("couldn't update file metadata: %v", err)
		}

		m.notifyChangeInDatabase()
		m.notifyChangeInFile(path, md.URL)
	}

	return nil
}

// UpdateMetadata updates file metadata
func (m *Manager) UpdateMetadata(md *sqlite.Metadata) error {
	m.db.wLock()
	defer m.db.wUnlock()

	db, err := m.getSqliteClient()
	if err != nil {
		return fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	err = db.Update(md)
	if err != nil {
		return fmt.Errorf("couldn't update file metadata: %v", err)
	}

	m.notifyChangeInDatabase()

	return nil
}

// GetDirectoryContent returns files and folders in the directory identified
// by inode. It doesn't include '.' and '..'.
func (m *Manager) GetDirectoryContent(parent int64) ([]sqlite.Metadata, error) {
	m.db.rLock()
	defer m.db.rUnlock()

	db, err := m.getSqliteClient()
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

	if md.Type != common.DrvFolder {
		return nil, fmt.Errorf("the requested inode is not a directory: %d", md.Type)
	}

	mdList, err := db.GetChildren(parent)
	if err != nil {
		return nil, fmt.Errorf("couldn't get children of %d: %v", parent, err)
	}

	return mdList, nil
}

// RemoveDirectory removes all of the directory contents and then
// removes the directory itself.
func (m *Manager) RemoveDirectory(ino int64) error {
	m.db.wLock()
	defer m.db.wUnlock()

	db, err := m.getSqliteClient()
	if err != nil {
		return fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	mdList, err := db.GetChildren(ino)
	if err != nil {
		return fmt.Errorf("couldn't get children of %d: %v", ino, err)
	}

	if len(mdList) > 0 {
		return common.ErrDirNotEmpty
	}

	err = db.Delete(ino)
	if err != nil {
		return fmt.Errorf("children are removed but couldn't delete the parent itself of inode %d: %v", ino, err)
	}

	m.notifyChangeInDatabase()

	return nil
}

// RemoveFile deletes file
func (m *Manager) RemoveFile(md *sqlite.Metadata) error {
	m.db.wLock()
	defer m.db.wUnlock()

	m.cache.Delete(common.ToString(md.Inode))

	go m.deleteRemoteFile(md)

	db, err := m.getSqliteClient()
	if err != nil {
		return fmt.Errorf("couldn't connect to database: %v", err)
	}
	defer db.Close()

	err = db.Delete(md.Inode)
	if err != nil {
		return fmt.Errorf("couldn't delete file: %v", err)
	}

	m.notifyChangeInDatabase()

	return nil
}

// OpenFile opens file with provided flag. If the file isn't cached already,
// it first fetches file from remote drive
func (m *Manager) OpenFile(md *sqlite.Metadata, flag int) (*os.File, error) {
	var path string

	e, found := m.cache.Touch(common.ToString(md.Inode), cacheExpiration)
	if !found {
		m.cache.Set(common.ToString(md.Inode), newCacheEntry("", fileDownloading, ""), cacheExpiration)

		p, err := m.downloadFile(md)
		if err != nil {
			return nil, fmt.Errorf("couldn't get file from storage %s: %v", md.Name, err)
		}

		path = p

		m.cache.Set(common.ToString(md.Inode), newCacheEntry(path, fileAvailable, md.Hash), cacheExpiration)
	} else {
		for {
			entry := e.(cacheEntry)
			if entry.status == fileAvailable {
				break
			}

			time.Sleep(time.Microsecond * 10)
			e, _ = m.cache.Get(common.ToString(md.Inode))
		}

		path = e.(cacheEntry).path
	}

	file, err := os.OpenFile(path, flag, 0600)
	if err != nil {
		m.cache.Delete(common.ToString(md.Inode))
		return nil, fmt.Errorf("couldn't open file %s: %v", path, err)
	}

	return file, nil
}

// AddDirectory creates a new directory under parent directory identified by inode
func (m *Manager) AddDirectory(parent int64, name string, mode int) (*sqlite.Metadata, error) {
	m.db.wLock()
	defer m.db.wUnlock()

	db, err := m.getSqliteClient()
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to database: %v", err)
	}

	md, err := db.AddDirectory(parent, name, mode)
	if err != nil {
		return nil, fmt.Errorf("couldn't create directory in database: %v", err)
	}

	m.notifyChangeInDatabase()

	return md, nil
}

// CreateFile creates a new empty file with provided permissions
func (m *Manager) CreateFile(parent int64, name string, mode int) (*sqlite.Metadata, error) {
	m.db.wLock()
	defer m.db.wUnlock()

	db, err := m.getSqliteClient()
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to database: %v", err)
	}

	u := drive.GetURL(m.selectDrive(), common.ObfuscateFileName(name))

	tmpfile, err := common.NewTempCacheFile()
	if err != nil {
		return nil, fmt.Errorf("couldn't create cached file: %v", err)
	}
	defer tmpfile.Close()

	checksum, err := crypto.MD5Checksum(tmpfile)
	if err != nil {
		tmpfile.Close()

		if err := os.Remove(tmpfile.Name()); err != nil {
			log.Warningf("couldn't remove file '%s' from filesystem: %v", tmpfile.Name(), err)
		}

		return nil, fmt.Errorf("couldn't compute md5 checksum of newly created file: %v", err)
	}

	md, err := db.CreateFile(parent, name, mode, u, checksum)
	if err != nil {
		return nil, fmt.Errorf("couldn't create file in database: %v", err)
	}

	m.cache.Set(common.ToString(md.Inode), newCacheEntry(tmpfile.Name(), fileAvailable, checksum), cacheExpiration)

	return md, nil
}

// GetTotalAvailableSpace returns total available space in all drives
func (m *Manager) GetTotalAvailableSpace() int64 {
	if m.availableSpace > 0 {
		return m.availableSpace
	}

	var tSpace int64 = 0

	for _, drv := range m.drives {
		space, err := drv.GetAvailableSpace()
		if err != nil {
			log.Warningf("couldn't get available space for %s: %v, ignoring...", drv.GetProviderName(), err)
			continue
		}

		tSpace += space
	}

	m.availableSpace = tSpace

	return tSpace
}

// GetFileCount returns the count of files in the database
func (m *Manager) GetFileCount() (int64, error) {
	db, err := m.getSqliteClient()
	if err != nil {
		return 0, fmt.Errorf("couldn't connect to database: %v", err)
	}

	fc, err := db.GetFileCount()
	if err != nil {
		return 0, fmt.Errorf("couldn't get file count from db: %v", err)
	}

	return fc, nil
}

func (m *Manager) getSqliteClient() (*sqlite.Client, error) {
	return sqlite.NewClient(m.db.path)
}

// getDriveClient returns drive driver of the provided scheme
func (m *Manager) getDriveClient(scheme string) (drive.Drive, error) {
	for _, drv := range m.drives {
		if drv.GetProviderName() == scheme {
			return drv, nil
		}
	}

	return nil, fmt.Errorf("couldn't find driver")
}

// downloadFile downloads remote file to current hosts temp directory
// and returns it's local path
func (m *Manager) downloadFile(md *sqlite.Metadata) (string, error) {
	u, err := common.ParseURL(md.URL)
	if err != nil {
		return "", fmt.Errorf("couldn't parse file url %s: %v", md.URL, err)
	}

	drv, err := m.getDriveClient(u.Scheme)
	if err != nil {
		return "", err
	}

	reader, err := drv.GetFile(u.Name)
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

func (m *Manager) deleteRemoteFile(md *sqlite.Metadata) {
	u, err := common.ParseURL(md.URL)
	if err != nil {
		log.Errorf("couldn't parse URL '%s': %v", md.URL, err)
		return
	}

	drv, err := m.getDriveClient(u.Scheme)
	if err != nil {
		log.Errorf("couldn't find drive '%s': %v", u.Scheme, err)
		return
	}

	if err := drv.DeleteFile(u.Name); err != nil {
		log.Errorf("couldn't delete file from remote drive '%s': %v", md.URL, err)
		return
	}
}

func (m *Manager) selectDrive() drive.Drive {
	var max int64 = 0
	idx := 0

	for i, drv := range m.drives {
		space, err := drv.GetAvailableSpace()
		if err != nil {
			log.Warningf("couldn't get available space for %s: %v, ignoring...", drv.GetProviderName(), err)
			continue
		}

		if space > max {
			max = space
			idx = i
		}
	}

	return m.drives[idx]
}

// notifyChangeInFile is called when file content is changed
// It adds file to the tracker for later processing
func (m *Manager) notifyChangeInFile(cachePath string, remotePath string) {
	m.tracker.Set(cachePath, trackerEntry{
		cachePath:  cachePath,
		remotePath: remotePath,
		accessTime: time.Now(),
	}, cacheForever)
}

// notifyChangeInDatabase is called when database is changed
// It adds database to the tracker for later processing
func (m *Manager) notifyChangeInDatabase() {
	m.tracker.Set(m.db.path, trackerEntry{
		cachePath:  m.db.path,
		remotePath: drive.GetURL(m.db.extDrive, common.DatabaseFileName),
		accessTime: time.Now(),
	}, cacheForever)
}
