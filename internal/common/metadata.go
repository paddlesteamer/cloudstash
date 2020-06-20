package common

type Metadata struct {
	Inode  uint64
	Name   string
	URL    string
	Size   uint64
	Mode   uint32
	Type   int
	Parent uint64
}
