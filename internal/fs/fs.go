package fs

import (
	"io"
	"os"
	"strings"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/manager"
	"github.com/paddlesteamer/cloudstash/internal/sqlite"
	"github.com/paddlesteamer/go-fuse-c/fuse"

	log "github.com/sirupsen/logrus"
)

type CloudStashFs struct {
	manager *manager.Manager

	fuse.DefaultFileSystem
}

func NewCloudStashFs(m *manager.Manager) *CloudStashFs {
	return &CloudStashFs{manager: m}
}

func (fs *CloudStashFs) GetAttr(ino int64, info *fuse.FileInfo) (*fuse.InoAttr, fuse.Status) {
	log.Debugf("getattr ino: %d", ino)

	md, err := fs.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		log.Errorf("couldn't get metadata of inode %d: %v", ino, err)
		return nil, fuse.EIO
	}

	return newInode(md), fuse.OK
}

func (fs *CloudStashFs) SetAttr(ino int64, attr *fuse.InoAttr, mask fuse.SetAttrMask, fi *fuse.FileInfo) (*fuse.InoAttr, fuse.Status) {
	log.Debugf("setattr ino: %d", ino)

	if mask&fuse.SET_ATTR_MODE == 0 {
		log.Warning("only mode change is supported")

		return nil, fuse.ENOSYS
	}

	md, err := fs.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		log.Errorf("couldn't get metadata of inode %d: %v", ino, err)
		return nil, fuse.EIO
	}

	md.Mode = attr.Mode

	err = fs.manager.UpdateMetadata(md)
	if err != nil {
		log.Errorf("couldn't set attr of inode %d: %v", ino, md.Inode)
		return nil, fuse.EIO
	}

	return newInode(md), fuse.OK
}

func (fs *CloudStashFs) Lookup(parent int64, name string) (*fuse.Entry, fuse.Status) {
	log.Debugf("lookup parent: %d, name: %s", parent, name)

	parentmd, err := fs.manager.GetMetadata(parent)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		log.Errorf("couldn't get parent metadata: %v", err)
		return nil, fuse.EIO
	}

	if parentmd.Type != common.DrvFolder {
		return nil, fuse.ENOTDIR
	}

	md, err := fs.manager.Lookup(parent, name)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		log.Errorf("couldn't lookup for '%s' under %d: %v", name, parent, err)
		return nil, fuse.EIO
	}

	return &fuse.Entry{
		Ino:          md.Inode,
		Attr:         newInode(md),
		AttrTimeout:  1.0,
		EntryTimeout: 1.0,
	}, fuse.OK
}

// @todo: simplify this method
func (fs *CloudStashFs) ReadDir(ino int64, fi *fuse.FileInfo, off int64, size int, w fuse.DirEntryWriter) fuse.Status {
	log.Debugf("readdir ino %d", ino)

	dirmd, err := fs.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't get directory metadata: %v", err)
		return fuse.EIO
	}

	if dirmd.Type != common.DrvFolder {
		return fuse.ENOTDIR
	}

	var next int64 = 1
	if off < 1 {
		w.Add(".", dirmd.Inode, dirmd.Mode, next)
	}
	next++

	if off < 2 {
		// special case: root dir
		if dirmd.Inode == 1 {
			w.Add("..", dirmd.Inode, dirmd.Mode, next)
		} else {
			md, err := fs.manager.GetMetadata(dirmd.Parent)
			if err != nil {
				log.Errorf("couldn't get metadata of parent folder: %v", err)

				return fuse.EIO
			}

			w.Add("..", md.Inode, md.Mode, next)
		}
	}
	next++

	mdList, err := fs.manager.GetDirectoryContent(ino)
	if err != nil { // no need to check for ErrNotFound, already checked
		log.Errorf("couldn't get directory content: %v", err)

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

func (fs *CloudStashFs) Rmdir(parent int64, name string) fuse.Status {
	log.Debugf("rmdir ino: %d name: %s", parent, name)

	parentmd, err := fs.manager.GetMetadata(parent)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't get parent metadata: %v", err)
		return fuse.EIO
	}

	if parentmd.Type != common.DrvFolder {
		return fuse.ENOTDIR
	}

	md, err := fs.manager.Lookup(parent, name)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't lookup for '%s' under %d: %v", name, parent, err)
		return fuse.EIO
	}

	if err := fs.manager.RemoveDirectory(md.Inode); err != nil {
		if err == common.ErrDirNotEmpty {
			return fuse.ENOTEMPTY
		}

		log.Errorf("couldn't remove directory '%s' under %d: %v", name, parent, err)
		return fuse.EIO
	}

	return fuse.OK
}

func (fs *CloudStashFs) Create(parent int64, name string, mode int, fi *fuse.FileInfo) (*fuse.Entry, fuse.Status) {
	log.Debugf("create parent: %d name: %s, mode: %#o", parent, name, mode)

	if !isValidName(name) {
		return nil, fuse.EPERM
	}

	md, err := fs.manager.CreateFile(parent, name, mode)
	if err != nil {
		log.Errorf("couldn't create file: %v", err)
		return nil, fuse.EIO
	}

	return &fuse.Entry{
		Ino:          md.Inode,
		Attr:         newInode(md),
		AttrTimeout:  1.0,
		EntryTimeout: 1.0,
	}, fuse.OK
}

func (fs *CloudStashFs) Open(ino int64, fi *fuse.FileInfo) fuse.Status {
	log.Debugf("open ino: %d", ino)

	md, err := fs.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't get metadata of inode %d: %v", ino, err)
		return fuse.EIO
	}

	if md.Type == common.DrvFolder {
		return fuse.EISDIR
	}

	return fuse.OK
}

func (fs *CloudStashFs) OpenDir(ino int64, fi *fuse.FileInfo) fuse.Status {
	log.Debugf("open dir ino: %d", ino)

	md, err := fs.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't get metadata of inode %d: %v", ino, err)
		return fuse.EIO
	}

	if md.Type != common.DrvFolder {
		return fuse.ENOTDIR
	}

	return fuse.OK
}

func (fs *CloudStashFs) Write(p []byte, ino int64, off int64, fi *fuse.FileInfo) (int, fuse.Status) {
	log.Debugf("write ino: %d len: %d off: %d", ino, len(p), off)

	md, err := fs.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return 0, fuse.ENOENT
		}

		log.Errorf("couldn't get metadata of inode %d: %v", ino, err)
		return 0, fuse.EIO
	}

	writer, err := fs.manager.OpenFile(md, os.O_WRONLY)
	if err != nil {
		log.Errorf("couldn't get writer: %v", err)

		return 0, fuse.EIO
	}
	defer writer.Close()

	_, err = writer.Seek(off, 0)
	if err != nil {
		log.Errorf("couldn't seek to provided offset %d: %v", off, err)
		return 0, fuse.EIO
	}

	n, err := writer.Write(p)
	if err != nil {
		log.Errorf("couldn't write to file: %v", err)
		return n, fuse.EIO
	}

	return n, fuse.OK
}

func (fs *CloudStashFs) Flush(ino int64, fi *fuse.FileInfo) fuse.Status {
	log.Debugf("flush ino: %d", ino)

	if err := fs.manager.UpdateMetadataFromCache(ino); err != nil {
		log.Errorf("flush called on file but couldn't update metadata in db: %v", err)
		return fuse.EIO
	}

	return fuse.OK
}

func (fs *CloudStashFs) Read(ino int64, size int64, off int64, fi *fuse.FileInfo) ([]byte, fuse.Status) {
	log.Debugf("read ino: %d size: %d off: %d", ino, size, off)

	md, err := fs.manager.GetMetadata(ino)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, fuse.ENOENT
		}

		log.Errorf("couldn't get metadata of inode %d: %v", ino, err)
		return nil, fuse.EIO
	}

	reader, err := fs.manager.OpenFile(md, os.O_RDONLY)
	if err != nil {
		log.Errorf("couldn't get reader: %v", err)
		return nil, fuse.EIO
	}
	defer reader.Close()

	if off+size > md.Size {
		size = md.Size - off
	}

	_, err = reader.Seek(off, 0)
	if err != nil {
		log.Errorf("couldn't seek to provided offset %d: %v", off, err)
		return nil, fuse.EIO
	}

	data := make([]byte, size)

	n, err := reader.Read(data)
	if err != nil && err != io.EOF {
		log.Errorf("couldn't read from reader: %v", err)
		return nil, fuse.EIO
	}

	if int64(n) != size {
		log.Errorf("couldn't read full. expected %d, read %d", size, n)
		return nil, fuse.EIO
	}

	return data, fuse.OK
}

func (fs *CloudStashFs) Mkdir(parent int64, name string, mode int) (*fuse.Entry, fuse.Status) {
	log.Debugf("mkdir parent: %d name: %s", parent, name)

	if !isValidName(name) {
		return nil, fuse.EPERM
	}

	md, err := fs.manager.AddDirectory(parent, name, mode)
	if err != nil {
		log.Errorf("couldn't create directory: %v", err)
		return nil, fuse.EIO
	}

	return &fuse.Entry{
		Ino:          md.Inode,
		Attr:         newInode(md),
		AttrTimeout:  1.0,
		EntryTimeout: 1.0,
	}, fuse.OK
}

func (fs *CloudStashFs) Unlink(parent int64, name string) fuse.Status {
	log.Debugf("unlink parent: %d name: %s", parent, name)

	parentmd, err := fs.manager.GetMetadata(parent)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't get parent metadata: %v", err)
		return fuse.EIO
	}

	if parentmd.Type != common.DrvFolder {
		return fuse.ENOTDIR
	}

	md, err := fs.manager.Lookup(parent, name)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't lookup for '%s' under %d: %v", name, parent, err)
		return fuse.EIO
	}

	err = fs.manager.RemoveFile(md)
	if err != nil {
		log.Errorf("couldn't delete file %s: %v", md.Name, err)
		return fuse.EIO
	}

	return fuse.OK
}

func (fs *CloudStashFs) Rename(oparent int64, oname string, tparent int64, tname string) fuse.Status {
	log.Debugf("rename p: %d name: %s", oparent, oname)

	if !isValidName(tname) {
		return fuse.EPERM
	}

	oparentmd, err := fs.manager.GetMetadata(oparent)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't get parent metadata: %v", err)
		return fuse.EIO
	}

	tparentmd, err := fs.manager.GetMetadata(tparent)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't get parent metadata: %v", err)
		return fuse.EIO
	}

	if oparentmd.Type != common.DrvFolder || tparentmd.Type != common.DrvFolder {
		return fuse.ENOTDIR
	}

	md, err := fs.manager.Lookup(oparent, oname)
	if err != nil {
		if err == common.ErrNotFound {
			return fuse.ENOENT
		}

		log.Errorf("couldn't lookup for '%s' under %d: %v", oname, oparent, err)
		return fuse.EIO
	}

	md.Parent = tparent
	md.Name = tname

	if err := fs.manager.UpdateMetadata(md); err != nil {
		log.Errorf("couldn't rename file %s under inode %d: %v", oname, oparent, err)
		return fuse.EIO
	}

	return fuse.OK
}

func (fs *CloudStashFs) StatFS(ino int64) (*fuse.StatVFS, fuse.Status) {
	log.Debug("statfs")

	return &fuse.StatVFS{
		NameMax: 255,
	}, fuse.OK
}

func (fs *CloudStashFs) Release(ino int64, fi *fuse.FileInfo) fuse.Status {
	log.Debugf("release ino: %d", ino)

	return fuse.OK
}

func (fs *CloudStashFs) ReleaseDir(ino int64, fi *fuse.FileInfo) fuse.Status {
	log.Debugf("releasedir ino: %d", ino)

	return fuse.OK
}

func (fs *CloudStashFs) Link(ino int64, newparent int64, name string) (*fuse.Entry, fuse.Status) {
	log.Debugf("link ino: %d", ino)

	return nil, fuse.EPERM
}

func (fs *CloudStashFs) Mknod(p int64, name string, mode int, rdev int) (*fuse.Entry, fuse.Status) {
	log.Debugf("mknod name: %s", name)

	return nil, fuse.EPERM
}

func (fs *CloudStashFs) ReadLink(ino int64) (string, fuse.Status) {
	log.Debugf("readlink ino: %d", ino)

	return "", fuse.EPERM
}

func (fs *CloudStashFs) Symlink(link string, p int64, name string) (*fuse.Entry, fuse.Status) {
	log.Debugf("symlink name: %s", name)

	return nil, fuse.EPERM
}

func (fs *CloudStashFs) Access(ino int64, mode int) fuse.Status {
	log.Debugf("access ino: %d", ino)

	return fuse.ENOSYS
}

func (fs *CloudStashFs) Destroy() {
	log.Debug("destroy")
}

func (fs *CloudStashFs) FSync(ino int64, dataOnly bool, fi *fuse.FileInfo) fuse.Status {
	log.Debugf("fsync ino: %d", ino)

	return fuse.ENOSYS
}

func (fs *CloudStashFs) FSyncDir(ino int64, dataOnly bool, fi *fuse.FileInfo) fuse.Status {
	log.Debugf("fsyncdir ino: %d", ino)

	return fuse.ENOSYS
}

func (fs *CloudStashFs) Forget(ino int64, n int) {
	log.Debugf("foreget ino: %d", ino)
}

func (fs *CloudStashFs) GetXAttr(ino int64, name string, out []byte) (int, fuse.Status) {
	log.Debugf("getxattr ino: %d, name: %s", ino, name)

	return 0, fuse.ENOSYS
}

func (fs *CloudStashFs) GetXAttrSize(ino int64, name string) (int, fuse.Status) {
	log.Debugf("getxattrsize ino: %d, name: %s", ino, name)

	return 0, fuse.ENOSYS
}

func (fs *CloudStashFs) ListXAttrs(ino int64) ([]string, fuse.Status) {
	log.Debugf("listxattrs ino: %d", ino)

	return nil, fuse.ENOSYS
}

func (fs *CloudStashFs) RemoveXAttr(ino int64, name string) fuse.Status {
	log.Debugf("removexattr ino: %d, name: %s", ino, name)

	return fuse.ENOSYS
}

func (fs *CloudStashFs) SetXAttr(ino int64, name string, value []byte, flags int) fuse.Status {
	log.Debugf("setxattr ino: %d, name: %s", ino, name)

	return fuse.ENOSYS
}

func newInode(md *sqlite.Metadata) *fuse.InoAttr {
	inode := &fuse.InoAttr{
		Ino:     md.Inode,
		NLink:   md.NLink,
		Timeout: 1.0,
	}

	if md.Type == common.DrvFolder {
		inode.Mode = fuse.S_IFDIR | md.Mode
	} else {
		inode.Mode = fuse.S_IFREG | md.Mode
		inode.Size = md.Size
	}

	return inode
}

// isValidName returns if provided name is allowed in filesystem.
// '/' character in name is not allowed.
// '.' and '..' as name also is not allowed.
func isValidName(name string) bool {
	if name == "." {
		return false
	}

	if name == ".." {
		return false
	}

	return !strings.Contains(name, "/")
}
