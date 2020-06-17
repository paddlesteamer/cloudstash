package manager

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/paddlesteamer/hdn-drv/config"
	"github.com/paddlesteamer/hdn-drv/db"
	"github.com/paddlesteamer/hdn-drv/drive"
)

type dbcon struct {
	client       *db.Client

	extFilePath  string
	extDrive     drive.Drive

	lastAccess   time.Time
	lastFileName string
	mux          sync.Mutex
}

type Manager struct {
	drives []drive.Drive
	key    string
	db     dbcon
}

const dbconTimeout time.Duration = 1000 * time.Millisecond

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
	for _, d = range drives {
		if d.GetProviderName() == u.Scheme {
			drv = d
			break
		}
	}

	if drv == nil {
		return nil, fmt.Errorf("manager: couldn't find a drive matching database file schema")
	}

	db := dbcon{
		client: nil,
		extDrive: drv,
		extFilePath: u.Host,
		lastAccess: time.Time{},
	}

	m := &Manager{
		drives: drives,
		key: key,
		db: db,
	}

	return m, nil
}

func (m *Manager) getDBConnection() (*db.Client, error) {
	m.db.mux.Lock()
	defer m.db.mux.Unlock()

	if time.Now().Sub(m.db.lastAccess) < dbconTimeout {
		m.db.lastAccess = time.Now()
		return m.db.client, nil
	}

	if m.db.client != nil {
		m.db.client.Close()
	}

	f, err := ioutil.TempFile("", "hdn-drv")
	if err != nil {
		return nil, fmt.Errorf("maanger: unable to create temporary file: %v", err)
	}

	firstTime := false

	content, err := m.db.extDrive.GetFile(m.db.extFilePath)
	if err != nil { // file doesn't exist (TODO: check error type, maybe file exists but net is down)
		f.Write(content)
		f.Close()

		firstTime = true
	}

	m.db.lastFileName = f.Name()
	m.db.client, _ = db.NewClient(m.db.lastFileName)
	m.db.lastAccess = time.Now()

	if firstTime {
		m.db.extDrive.PutFile(m.db.extFilePath, f)
		f.Close()
	}

	return m.db.client, nil
}

func (m *Manager) updateDBFile() error {
	m.db.mux.Lock()
	defer m.db.mux.Unlock()

	ior, err := os.Open(m.db.lastFileName)
	if err != nil {
		return fmt.Errorf("manager: unable to open db file: %v", err)
	}
	defer ior.Close()

	m.db.extDrive.PutFile(m.db.extFilePath, ior)

	return nil
}
