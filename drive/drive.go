package drive

type Drive interface {
	GetFile(path string) ([]byte, error)
}
