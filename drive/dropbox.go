package drive

import (
	"fmt"
	"io"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"

	"github.com/paddlesteamer/hdn-drv/config"
)

type Dropbox struct {
	client files.Client
}

func NewDropboxClient(conf *config.DropboxCredentials) *Dropbox {
	dbxConfig := dropbox.Config{
		Token: conf.AccessToken,
		LogLevel: dropbox.LogDebug,
	}

	dbx := &Dropbox{
		client: files.New(dbxConfig),
	}

	return dbx
}

func (dbx *Dropbox) GetProviderName() string {
	return "dropbox"
}

func (dbx *Dropbox) GetFile(path string) ([]byte, error) {
	args := files.NewDownloadArg(path)

	metadata, r, err := dbx.client.Download(args)
	if err != nil {
		return nil, fmt.Errorf("dropbox: unable to get file %v: %v", path, err)
	}

	content := make([]byte, metadata.Size)

	_, err = r.Read(content)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("dropbox: error while reading from file %v: %v", path, err)
	}

	return content, nil
}

func (dbx *Dropbox) PutFile(path string, content io.Reader) error {
	args := files.NewCommitInfo(path)

	_, err := dbx.client.Upload(args, content)
	if err != nil {
		return fmt.Errorf("dropbox: unable to upload file %v: %v", path, err)
	}

	return nil
}