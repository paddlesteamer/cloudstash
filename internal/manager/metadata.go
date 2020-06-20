package manager

import "github.com/paddlesteamer/hdn-drv/internal/db"

const (
	MD_FOLDER = iota
	MD_FILE   = iota
)

type Metadata struct {
	ID     uint64
	Name   string
	Size   uint64
	Mode   uint8
	Type   int
	Parent uint64
}

func newFileMetadata(file *db.File) *Metadata {
	md := &Metadata{
		ID:     file.ID,
		Name:   file.Name,
		Size:   file.Size,
		Mode:   file.Mode,
		Type:   MD_FILE,
		Parent: file.Parent,
	}

	return md
}

func newFolderMetadata(folder *db.Folder) *Metadata {
	md := &Metadata{
		ID:     folder.ID,
		Name:   folder.Name,
		Mode:   folder.Mode,
		Type:   MD_FOLDER,
		Parent: folder.Parent,
	}

	return md
}
