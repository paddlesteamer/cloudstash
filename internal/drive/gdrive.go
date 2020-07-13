package drive

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

type GDrive struct {
	srv *drive.Service
}

func NewGDriveClient(token *oauth2.Token) (*GDrive, error) {
	config, _ := google.ConfigFromJSON([]byte(common.GDRIVE_CREDENTIALS), drive.DriveFileScope)

	client := config.Client(context.Background(), token)

	srv, err := drive.New(client)
	if err != nil {
		return nil, fmt.Errorf("couldn't create GDrive service: %v", err)
	}

	return &GDrive{
		srv: srv,
	}, nil
}

func (g *GDrive) GetProviderName() string {
	return "gdrive"
}

func (g *GDrive) GetFile(name string) (io.ReadCloser, error) {
	id, err := g.getFileId(name)
	if err != nil {
		return nil, fmt.Errorf("couldn't retrieve file id: %v", err)
	}

	res, err := g.srv.Files.Get(id).Download()
	if err != nil {
		return nil, fmt.Errorf("couldn't download file %s from gdrive: %v", name, err)
	}

	return res.Body, nil
}

func (g *GDrive) PutFile(name string, content io.Reader) error {
	f := &drive.File{
		Name: name,
	}

	// @todo: check specific errors - not all errors are errors
	if _, err := g.srv.Files.Create(f).Media(content).Do(); err != nil {
		return fmt.Errorf("couldn't upload file %s: %v", name, err)
	}

	return nil
}

func (g *GDrive) GetFileMetadata(name string) (*Metadata, error) {
	id, err := g.getFileId(name)
	if err != nil {
		return nil, fmt.Errorf("couldn't retrieve file id: %v", err)
	}

	md, err := g.srv.Files.Get(id).Do()
	// @todo: check specific errors - not all errors are errors
	if err != nil {
		return nil, fmt.Errorf("couldn't get file metadata: %v", err)
	}

	return &Metadata{
		Name: md.Name,
		Size: uint64(md.Size),
		Hash: md.Md5Checksum,
	}, nil
}

func (g *GDrive) DeleteFile(name string) error {
	id, err := g.getFileId(name)
	if err != nil {
		return fmt.Errorf("couldn't retrieve file id: %v", err)
	}

	if err := g.srv.Files.Delete(id).Do(); err != nil {
		return fmt.Errorf("couldn't delete file %s: %v", name, err)
	}

	return nil
}

func (g *GDrive) ComputeHash(r io.Reader, hchan chan string, echan chan error) {
	h := md5.New()

	if _, err := io.Copy(h, r); err != nil {
		echan <- fmt.Errorf("couldn't compute hash: %v", err)
	}

	hchan <- fmt.Sprintf("%x", h.Sum(nil))
}

// @TODO: handle unlimited storage
func (g *GDrive) GetAvailableSpace() (int64, error) {
	res, err := g.srv.About.Get().Fields("storageQuota(limit, usage)").Do()
	if err != nil {
		return 0, fmt.Errorf("couldn't get available space: %v", err)
	}

	return res.StorageQuota.Limit - res.StorageQuota.Usage, nil
}

func (g *GDrive) getFileId(name string) (string, error) {
	res, err := g.srv.Files.List().PageSize(10).
		Q(fmt.Sprintf("name='%s'", name)).Fields("files(id, name)").Do()
	if err != nil {
		return "", fmt.Errorf("couldn't query file %s: %v", name, err)
	}

	if len(res.Files) != 1 {
		return "", fmt.Errorf("unexpected number of files on gdrive %d", len(res.Files))
	}

	return res.Files[0].Id, nil
}
