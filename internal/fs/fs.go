package fs

import (
	"fmt"
	"os"

	"github.com/paddlesteamer/hdn-drv/internal/common"
	"github.com/paddlesteamer/hdn-drv/internal/manager"
	"github.com/vgough/go-fuse-c/fuse"
)

type HdnDrvFs struct {
	manager *manager.Manager

	fuse.DefaultFileSystem
}

func NewHdnDrvFs(m *manager.Manager) *HdnDrvFs {
	filesystem := &HdnDrvFs{
		manager: m,
	}

	return filesystem
}

func (r *HdnDrvFs) GetAttr(ino int64, info *fuse.FileInfo) (*fuse.InoAttr, fuse.Status) {
	md, err := r.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		fmt.Fprintf(os.Stderr, "couldn't get metadata of inode %d: %v", ino, err)

		return nil, fuse.EIO
	}

	inode := &fuse.InoAttr{
		Ino:     ino,
		NLink:   md.NLink,
		Timeout: 1.0,
	}

	if md.Type == common.DRV_FOLDER {
		inode.Mode = fuse.S_IFDIR | md.Mode
	} else {
		inode.Mode = fuse.S_IFREG | md.Mode
		inode.Size = md.Size
	}

	return inode, fuse.OK
}

func (r *HdnDrvFs) Lookup(parent int64, name string) (*fuse.Entry, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (r *HdnDrvFs) ReadDir(ino int64, fi *fuse.FileInfo, off int64, size int, w fuse.DirEntryWriter) fuse.Status {
	return fuse.ENOSYS
}
