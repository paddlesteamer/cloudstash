package manager

import (
	"fmt"

	"github.com/paddlesteamer/hdn-drv/config"
	"github.com/paddlesteamer/hdn-drv/db"
	"github.com/paddlesteamer/hdn-drv/drive"
)

type Manager struct {
	drives []drive.Drive
	key    string
	db     *db.Client
}

func NewManager(conf *config.Configuration) (*Manager, error) {
	key := conf.EncryptionKey

	drives := []drive.Drive{}
	if conf.Dropbox != nil {
		dbx := drive.NewDropboxClient(conf.Dropbox)
		drives = append(drives, dbx)
	}

	db, err := db.NewClient()
	if err != nil {
		return nil, fmt.Errorf("manager: unable to connect db: %v", err)
	}

	m := &Manager{
		drives: drives,
		key: key,
		db: db,
	}

	return m, nil
}