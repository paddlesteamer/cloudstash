package source

type FolderEntry struct {
	Name string
	Type int     // fuse.S_IFDIR, fuse.S_IFREG, etc.
}

type Source interface {
	ListFolder(string)	[]FolderEntry
}
