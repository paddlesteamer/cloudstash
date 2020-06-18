package manager

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"sync"

	"github.com/paddlesteamer/hdn-drv/config"
	"github.com/paddlesteamer/hdn-drv/db"
	"github.com/paddlesteamer/hdn-drv/drive"
)

type dbconn struct {
	extFilePath  string
	extDrive     drive.Drive

	dbPath       string

	mux          sync.RWMutex
}

type Manager struct {
	drives []drive.Drive
	key    string
	db     dbconn
}

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
			return nil, fmt.Errorf("manager: unable to copy contents of db to local file: %v", err)
		}
	}

	db := dbconn{
		extDrive:    drv,
		extFilePath: dbExtPath,

		dbPath:      dbPath,
	}

	m := &Manager{
		drives: drives,
		key: key,
		db: db,
	}

	return m, nil
}

func (m *Manager) Close() {
	os.Remove(m.db.dbPath)
}
