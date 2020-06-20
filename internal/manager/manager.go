package manager

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"sync"
	"time"

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
		return nil, fmt.Errorf("manager: unable to parse database file url: %v", err)
	}

	var drv drive.Drive = nil
	for _, d := range drives {
		if d.GetProviderName() == u.Scheme {
			drv = d
			break
		}
	}

	if drv == nil {
		return nil, fmt.Errorf("manager: couldn't find a drive matching database file scheme")
	}

	dbf, err := ioutil.TempFile("/tmp", "hdn-drv-db")
	if err != nil {
		return nil, fmt.Errorf("manager: unable to create database file: %v", err)
	}
	// defer dbf.Close() close manually, at least shouldn't be deferred here

	dbExtPath := fmt.Sprintf("/%s", u.Host)
	dbPath := dbf.Name()

	_, dbr, err := drv.GetFile(u.Host)
	if err != nil { // TODO: check specific 'not found' error
		// below is for not found error
		dbf.Close()

		err = db.InitDB(dbPath)
		if err != nil {
			return nil, fmt.Errorf("manager: unable to initialize db: %v", err)
		}

		dbf, err := os.Open(dbPath)
		if err != nil {
			os.Remove(dbPath)
			return nil, fmt.Errorf("manager: unable to open intitialized db: %v", err)
		}
		defer dbf.Close()

		err = drv.PutFile(dbExtPath, dbf)
		if err != nil {
			os.Remove(dbPath)
			return nil, fmt.Errorf("manager: unable to upload initialiezed db: %v", err)
		}

	} else {
		defer dbr.Close()
		defer dbf.Close()

		_, err := io.Copy(dbf, dbr)
		if err != nil {
			os.Remove(dbPath)
			return nil, fmt.Errorf("manager: unable to copy contents of db to local file: %v", err)
		}
	}

	hash, err := drv.ComputeHash(dbPath)
	if err != nil {
		os.Remove(dbPath)
		return nil, fmt.Errorf("manager: unable to compute hash: %v", err)
	}

	// @fixme: literal copies lock value from db
	m := &Manager{
		drives: drives,
		key:    key,
        db :    dbStat{
            extDrive: drv,
            extPath:  dbExtPath,
    
            dbPath: dbPath,
            hash:   hash,
        }
	}

	go m.checkChanges()

	return m, nil
}

func (m *Manager) Close() {
	os.Remove(m.db.dbPath)
}

func (m *Manager) checkChanges() {
	for {
		time.Sleep(checkInterval)

		mdata, err := m.db.extDrive.GetFileMetadata(m.db.extPath)
		if err != nil {
			fmt.Printf("manager: %v\n", err)
			continue
		}

		m.db.mux.Lock()
		if mdata.Hash == m.db.hash {
			m.db.mux.Unlock()
			continue
		}

		_, reader, err := m.db.extDrive.GetFile(m.db.extPath)
		if err != nil {
			fmt.Printf("manager: unable to get updated db file: %v", err)
			m.db.mux.Unlock()
			continue
		}

		file, err := os.Open(m.db.dbPath)
		if err != nil {
			fmt.Printf("manager: unable to open db: %v", err)
			m.db.mux.Unlock()
			continue
		}

		_, err = io.Copy(file, reader)
		if err != nil {
			fmt.Printf("manager: unable to copy contents of updated db file to local file: %v", err)
			m.db.mux.Unlock()
			continue
		}

		reader.Close()
		file.Close()

		m.db.mux.Unlock()
	}
}
