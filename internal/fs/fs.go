package fs

import (
	"fmt"
	"io"
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

func (r *HdnDrvFs) StatFs(ino int64) (*fuse.StatVFS, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (r *HdnDrvFs) GetAttr(ino int64, info *fuse.FileInfo) (*fuse.InoAttr, fuse.Status) {
	md, err := r.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		fmt.Fprintf(os.Stderr, "couldn't get metadata of inode %d: %v\n", ino, err)

		return nil, fuse.EIO
	}

	inode := newInode(md)

	return inode, fuse.OK
}

func (r *HdnDrvFs) Lookup(parent int64, name string) (*fuse.Entry, fuse.Status) {
	parentmd, err := r.manager.GetMetadata(parent)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		fmt.Fprintf(os.Stderr, "couldn't get parent metadata: %v\n", err)

		return nil, fuse.EIO
	}

	if parentmd.Type != common.DRV_FOLDER {
		return nil, fuse.ENOTDIR
	}

	md, err := r.manager.Lookup(parent, name)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		fmt.Fprintf(os.Stderr, "couldn't lookup for '%s' under %d: %v\n", name, parent, err)

		return nil, fuse.EIO
	}

	inode := newInode(md)

	entry := &fuse.Entry{
		Ino:          md.Inode,
		Attr:         inode,
		AttrTimeout:  1.0,
		EntryTimeout: 1.0,
	}

	return entry, fuse.OK
}

func (r *HdnDrvFs) ReadDir(ino int64, fi *fuse.FileInfo, off int64, size int, w fuse.DirEntryWriter) fuse.Status {
	dirmd, err := r.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		fmt.Fprintf(os.Stderr, "couldn't get directory metadata: %v\n", err)

		return fuse.EIO
	}

	if dirmd.Type != common.DRV_FOLDER {
		return fuse.ENOTDIR
	}

	var next int64 = 1
	if off < 1 {
		w.Add(".", dirmd.Inode, dirmd.Mode, next)
	}
	next++

	if off < 2 {
		if dirmd.Inode == 1 { // special case: root dir
			w.Add("..", dirmd.Inode, dirmd.Mode, next)
		} else {
			md, err := r.manager.GetMetadata(dirmd.Parent)
			if err != nil {
				fmt.Fprintf(os.Stderr, "couldn't get metadata of parent folder: %v\n", err)

				return fuse.EIO
			}

			w.Add("..", md.Inode, md.Mode, next)
		}
	}
	next++

	mdList, err := r.manager.GetDirectoryContent(ino)
	if err != nil { // no need to check for ErrNotFound, already checked
		fmt.Fprintf(os.Stderr, "couldn't get directory content: %v\n", err)

		return fuse.EIO
	}

	if off > 2 {
		off -= 2
	} else {
		off = 0
	}

	for i, md := range mdList {
		if int64(i) < off {
			continue
		}

		w.Add(md.Name, md.Inode, md.Mode, next+int64(i))
	}

	return fuse.OK
}

func (r *HdnDrvFs) Open(ino int64, fi *fuse.FileInfo) fuse.Status {
	md, err := r.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		fmt.Fprintf(os.Stderr, "couldn't get metadata of inode %d: %v\n", ino, err)

		return fuse.EIO
	}

	if md.Type == common.DRV_FOLDER {
		return fuse.EISDIR
	}

	return fuse.OK
}

func (r *HdnDrvFs) Read(ino int64, size int64, off int64, fi *fuse.FileInfo) ([]byte, fuse.Status) {
	md, err := r.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		fmt.Fprintf(os.Stderr, "couldn't get metadata of inode %d: %v\n", ino, err)

		return nil, fuse.EIO
	}

	reader, err := r.manager.GetFile(md)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't get reader: %v\n", err)

		return nil, fuse.EIO
	}
	defer reader.Close()

	if off+size > md.Size {
		size = md.Size - off
	}

	_, err = reader.Seek(off, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't seek to provided offset %d: %v\n", off, err)
	}

	data := make([]byte, size)

	n, err := reader.Read(data)
	if err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "couldn't read from reader: %v\n", err)

		return nil, fuse.EIO
	}

	if int64(n) != size {
		fmt.Fprintf(os.Stderr, "couldn't read full. expected %d, read %d\n", size, n)

		return nil, fuse.EIO
	}

	return data, fuse.OK
}

func newInode(md *common.Metadata) *fuse.InoAttr {
	inode := &fuse.InoAttr{
		Ino:     md.Inode,
		NLink:   md.NLink,
		Timeout: 1.0,
	}

	if md.Type == common.DRV_FOLDER {
		inode.Mode = fuse.S_IFDIR | md.Mode
	} else {
		inode.Mode = fuse.S_IFREG | md.Mode
		inode.Size = md.Size
	}

	return inode
}
