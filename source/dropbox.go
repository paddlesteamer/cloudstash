package source

import (
	"fmt"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
	"github.com/vgough/go-fuse-c/fuse"

	"github.com/paddlesteamer/hdn-drv/config"
)

type Dropbox struct {
	client files.Client

	Source
}

func NewDropboxClient(conf config.DropboxCredentials) *Dropbox {
	dbxConfig := dropbox.Config{
		Token: conf.AccessToken,
		LogLevel: dropbox.LogDebug,
	}

	dbx := &Dropbox{
		client: files.New(dbxConfig),
	}

	return dbx
}

func (dbx *Dropbox) ListFolder(path string) ([]FolderEntry, error) {
	if path == "/" {
		path = ""
	}

	args := &files.ListFolderArg{
		Path: path,
		Recursive: false,
		IncludeMediaInfo: false,
		IncludeDeleted: false,
		IncludeHasExplicitSharedMembers: false,
		IncludeMountedFolders: false,
		IncludeNonDownloadableFiles: true,
		Limit: 1000,
	}

	res, err := dbx.client.ListFolder(args)
	if err != nil {
		return nil, fmt.Errorf("dropbox: unable to list directory content: %v", err)
	}

	list := []FolderEntry{}
	for _, entry := range res.Entries {
		typ := fuse.S_IFREG
		switch entry.(type) {
			case *files.FileMetadata:
				typ = fuse.S_IFREG

			case *files.FolderMetadata:
				typ = fuse.S_IFDIR

		}
		name := entry.(*files.Metadata).Name

		listEntry := FolderEntry{
			Name: name,
			Type: typ,
		}

		list = append(list, listEntry)
	}

	return list, nil
}