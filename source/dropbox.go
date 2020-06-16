package source

import "github.com/paddlesteamer/hdn-drv/config"

type Dropbox struct {
	AccessToken string

	Source
}

func NewDropboxClient(conf config.DropboxCredentials) *Dropbox {
	dbx := &Dropbox{
		AccessToken: conf.AccessToken,
	}

	return dbx
}

func (dbx *Dropbox) ListFolder(path string) []FolderEntry {
	return []FolderEntry{}
}