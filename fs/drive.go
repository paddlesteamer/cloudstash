package fs

import "github.com/vgough/go-fuse-c/fuse"

type HdnDrvFs struct {
	fuse.DefaultFileSystem
}

func (r *HdnDrvFs) GetAttr(ino int64, info *fuse.FileInfo) (*fuse.InoAttr, fuse.Status) {
	inode := &fuse.InoAttr{
		Ino: ino,
		Timeout: 1.0,
	}

	if ino == 1 { // root
		inode.Mode = fuse.S_IFDIR | 0755
		inode.NLink = 2
	}
	return nil, fuse.OK
}