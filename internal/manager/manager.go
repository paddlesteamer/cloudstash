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

// Manager is where all the business logic happens
// It is responsible for keeping track of drives, cache, database, etc.
// It is used by the `fs` package
type Manager struct {
	drives  []drive.Drive
	key     string
	db      *database
	cache   *cache.Cache
	tracker *cache.Cache
	cipher  *crypto.Crypto
}

// NewManager creates a new Manager struct with provided
// parameters and starts background processes
func NewManager(drives []drive.Drive, db *database, cipher *crypto.Crypto, key string) *Manager {
	m := &Manager{
		drives:  drives,
		db:      db,
		key:     key,
		cache:   newCache(),
		tracker: newTracker(),
		cipher:  cipher,
	}

	go watchRemoteChanges(m)
	go processLocalChanges(m)

	return m
}

// Close cleanups cached files and process remaining file changes
func (m *Manager) Close() {
	processChanges(m, forceAll)

	items := m.cache.Flush()

	for _, item := range items {
		path := item.Object.(cacheEntry).path

		if path != "" {
			os.Remove(path)
		}
	}
}

// NotifyChangeInFile is called when file content is changed
// It adds file to the tracker for later processing
func (m *Manager) NotifyChangeInFile(cachePath string, remotePath string) {
	m.tracker.Set(cachePath, trackerEntry{
		cachePath:  cachePath,
		remotePath: remotePath,
		accessTime: time.Now(),
	}, cacheForever)
}

// NotifyChangeInDatabase is called when database is changed
// It adds database to the tracker for later processing
func (m *Manager) NotifyChangeInDatabase() {
	m.tracker.Set(m.db.path, trackerEntry{
		cachePath:  m.db.path,
		remotePath: drive.GetURL(m.db.extDrive, m.db.extPath),
		accessTime: time.Now(),
	}, cacheForever)
}

// Lookup searches provided directory for a file provided with 'name' parameter
// If found returns it's metadata, if not found returns ErrNotFound
func (m *Manager) Lookup(parent int64, name string) (*sqlite.Metadata, error) {
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

// GetMetadata returns metadata of file with provided inode
func (m *Manager) GetMetadata(inode int64) (*sqlite.Metadata, error) {
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

// UpdateMetadataFromCache checks changes in file from the cached version
// and updates database accordingly. If the content is also changed
// it calls NotifyChangeInFile in addition to NotifyChangeInDatabase
func (m *Manager) UpdateMetadataFromCache(inode int64) error {
	m.db.wLock()
	defer m.db.wUnlock()

	e, found := m.cache.GetWithExpirationUpdate(strconv.FormatInt(inode, 10), cacheExpiration)
	if !found {
		return fmt.Errorf("the file hasn't beed cached")
	}

	path := e.(cacheEntry).path

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

	checksum, err := crypto.MD5Checksum(file)
	if err != nil {
		return fmt.Errorf("couldn't compute md5 checksum: %v", err)
	}

	if md.Hash != checksum {
		md.Size = fi.Size()
		md.Hash = checksum
		err = db.Update(md)
		if err != nil {
			return fmt.Errorf("couldn't update file metadata: %v", err)
		}

		m.NotifyChangeInDatabase()
		m.NotifyChangeInFile(path, md.URL)
	}

	return nil
}

// UpdateMetadata updates file metadata
func (m *Manager) UpdateMetadata(md *sqlite.Metadata) error {
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

// GetDirectoryContent returns files and folders in the directory identified
// by inode. It doesn't include '.' and '..'.
func (m *Manager) GetDirectoryContent(parent int64) ([]sqlite.Metadata, error) {
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

// RemoveDirectory removes all of the directory contents and then
// removes the directory itself.
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

	if len(mdList) > 0 {
		return common.ErrDirNotEmpty
	}

	err = db.Delete(ino)
	if err != nil {
		return fmt.Errorf("children are removed but couldn't delete the parent itself of inode %d: %v", ino, err)
	}

	m.NotifyChangeInDatabase()

	return nil
}

// RemoveFile deletes file
func (m *Manager) RemoveFile(md *sqlite.Metadata) error {
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

// OpenFile opens file with provided flag. If the file isn't cached already,
// it first fetches file from remote drive
func (m *Manager) OpenFile(md *sqlite.Metadata, flag int) (*os.File, error) {
	var path string

	e, found := m.cache.GetWithExpirationUpdate(strconv.FormatInt(md.Inode, 10), cacheExpiration)
	if !found {
		m.cache.Set(strconv.FormatInt(md.Inode, 10), newCacheEntry("", fileDownloading), cacheExpiration)

		p, err := m.downloadFile(md)
		if err != nil {
			return nil, fmt.Errorf("couldn't get file from storage %s: %v", md.Name, err)
		}

		path = p

		m.cache.Set(strconv.FormatInt(md.Inode, 10), newCacheEntry(path, fileAvailable), cacheExpiration)
	} else {
		for {
			entry := e.(cacheEntry)
			if entry.status == fileAvailable {
				break
			}

			time.Sleep(time.Microsecond * 10)
			e, _ = m.cache.Get(strconv.FormatInt(md.Inode, 10))
		}

		path = e.(cacheEntry).path
	}

	file, err := os.OpenFile(path, flag, os.ModeAppend)
	if err != nil {
		return nil, fmt.Errorf("couldn't open file %s: %v", path, err)
	}

	return file, nil
}

// AddDirectory creates a new directory under parent directory identified by inode
func (m *Manager) AddDirectory(parent int64, name string, mode int) (*sqlite.Metadata, error) {
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

// CreateFile creates a new empty file with provided permissions
func (m *Manager) CreateFile(parent int64, name string, mode int) (*sqlite.Metadata, error) {
	m.db.wLock()
	defer m.db.wUnlock()

	db, err := sqlite.NewClient()
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
		os.Remove(tmpfile.Name())
		return nil, fmt.Errorf("couldn't compute md5 checksum of newly created file: %v", err)
	}

	md, err := db.CreateFile(parent, name, mode, u, checksum)
	if err != nil {
		return nil, fmt.Errorf("couldn't create file in database: %v", err)
	}

	m.cache.Set(strconv.FormatInt(md.Inode, 10), newCacheEntry(tmpfile.Name(), fileAvailable), cacheExpiration)

	return md, nil
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

func (m *Manager) deleteRemoteFile(md *sqlite.Metadata) {
	u, err := common.ParseURL(md.URL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't parse URL '%s': %v\n", md.URL, err)
		return
	}

	drv, err := m.getDriveClient(u.Scheme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't find drive '%s': %v\n", u.Scheme, err)
		return
	}

	if err := drv.DeleteFile(u.Path); err != nil {
		fmt.Fprintf(os.Stderr, "couldn't delete file from remote drive '%s': %v\n", md.URL, err)
		return
	}
}

// @TODO: select drive according to available space
func (m *Manager) selectDrive() drive.Drive {
	return m.drives[0]
}
