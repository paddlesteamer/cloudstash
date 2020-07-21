package drive

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

// GDrive is google drive client
type GDrive struct {
	srv          *drive.Service
	rootFolderID string

	mu     sync.Mutex
	lockID string
}

// NewGDriveClient creates app folder on google drive if it doesn;t exists
// and returns GDrive client
func NewGDriveClient(token *oauth2.Token) (*GDrive, error) {
	config, _ := google.ConfigFromJSON([]byte(common.GDriveCredentials), drive.DriveFileScope)

	client := config.Client(context.Background(), token)

	srv, err := drive.New(client)
	if err != nil {
		return nil, fmt.Errorf("couldn't create GDrive service: %v", err)
	}

	drv := &GDrive{
		srv: srv,
	}

	folder, err := drv.getFileID(common.GDriveAppFolder)
	if err != nil && err != common.ErrNotFound {
		return nil, fmt.Errorf("couldn't query for root folder: %v", err)
	}

	if err == common.ErrNotFound {
		f := &drive.File{
			Name:     common.GDriveAppFolder,
			MimeType: "application/vnd.google-apps.folder",
		}

		finfo, err := srv.Files.Create(f).Do()
		if err != nil {
			return nil, fmt.Errorf("couldn't create app directory on gdrive: %v", err)
		}

		folder = finfo.Id
	}

	drv.rootFolderID = folder

	return drv, nil
}

// GetProviderName returns 'gdrive'
func (g *GDrive) GetProviderName() string {
	return "gdrive"
}

// GetFile returns ReadCloser of remote file on google drive
func (g *GDrive) GetFile(name string) (io.ReadCloser, error) {
	id, err := g.getFileID(name)
	if err != nil && err != common.ErrNotFound {
		return nil, fmt.Errorf("couldn't retrieve file id: %v", err)
	}

	if err == common.ErrNotFound {
		return nil, err
	}

	res, err := g.srv.Files.Get(id).Download()
	if err != nil {
		if strings.Contains(err.Error(), "404") { // ugly hack to distinguish not found error
			return nil, common.ErrNotFound
		}

		return nil, fmt.Errorf("couldn't download file %s from gdrive: %v", name, err)
	}

	return res.Body, nil
}

// PutFile uploads file to google drive
func (g *GDrive) PutFile(name string, content io.Reader) error {
	id, err := g.getFileID(name)
	if err != nil && err != common.ErrNotFound {
		return fmt.Errorf("couldn't retrieve file id: %v", err)
	}

	if err != common.ErrNotFound {
		if err := g.srv.Files.Delete(id).Do(); err != nil {
			return fmt.Errorf("couldn't delete file %s for replacement: %v", name, err)
		}
	}

	f := &drive.File{
		Name:    name,
		Parents: []string{g.rootFolderID},
	}

	if _, err := g.srv.Files.Create(f).Media(content).Do(); err != nil {
		return fmt.Errorf("couldn't upload file %s: %v", name, err)
	}

	return nil
}

// GetFileMetadata retrieve file metadata from google drive
func (g *GDrive) GetFileMetadata(name string) (*Metadata, error) {
	id, err := g.getFileID(name)
	if err != nil && err != common.ErrNotFound {
		return nil, fmt.Errorf("couldn't retrieve file id: %v", err)
	}

	if err == common.ErrNotFound {
		return nil, err
	}

	md, err := g.srv.Files.Get(id).Fields("name,size,md5Checksum").Do()
	if err != nil {
		if strings.Contains(err.Error(), "404") { // ugly hack to distinguish not found error
			return nil, common.ErrNotFound
		}

		return nil, fmt.Errorf("couldn't get file metadata: %v", err)
	}

	return &Metadata{
		Name: md.Name,
		Size: uint64(md.Size),
		Hash: md.Md5Checksum,
	}, nil
}

// DeleteFile move files on google drive to trash
func (g *GDrive) DeleteFile(name string) error {
	id, err := g.getFileID(name)
	if err != nil && err != common.ErrNotFound {
		return fmt.Errorf("couldn't retrieve file id: %v", err)
	}

	if err == common.ErrNotFound {
		return err
	}

	if err := g.srv.Files.Delete(id).Do(); err != nil {
		if strings.Contains(err.Error(), "404") {
			return common.ErrNotFound
		}

		return fmt.Errorf("couldn't delete file %s: %v", name, err)
	}

	return nil
}

// MoveFile renames file on google drive
func (g *GDrive) MoveFile(name string, newName string) error {
	id, err := g.getFileID(name)
	if err != nil {
		return fmt.Errorf("couldn't retrieve file %s's id: %v", name, err)
	}

	f := &drive.File{
		Name: newName,
	}

	if _, err := g.srv.Files.Update(id, f).Do(); err != nil {
		return fmt.Errorf("couldn't move file from %s to %s on gdrive: %v", name, newName, err)
	}

	return nil
}

// Lock creates a lock file on gdrive
// If a lock file doesn't exists before, it creates a lock file then checks
// the count of lock files on the gdrive. If it is more than one(somebody else
// also created a lock file), it removes its own lock file, waits random amount of
// time and start checking lock files again
func (g *GDrive) Lock() error {
	g.mu.Lock()

	sTime := time.Now()
	lockID := ""

	query := g.srv.Files.List().PageSize(10).
		Q(fmt.Sprintf("name='%s' and trashed=false", lockFile)).Fields("files(id, name)")

	f := &drive.File{
		Name:    lockFile,
		Parents: []string{g.rootFolderID},
	}

	for {
		res, err := query.Do()
		if err != nil {
			g.mu.Unlock()
			return fmt.Errorf("couldn't query lock file: %v", err)
		}

		if len(res.Files) > 0 {
			if lockID != res.Files[0].Id {
				lockID = res.Files[0].Id
				sTime = time.Now()
			}

			if time.Now().Sub(sTime) > lockTimeout {
				g.srv.Files.Delete(res.Files[0].Id).Do()

				lockID = ""
			}

			continue
		}

		file, err := g.srv.Files.Create(f).Do()
		if err != nil {
			g.mu.Unlock()
			return fmt.Errorf("couldn't create lock file on gdrive: %v", err)
		}

		g.lockID = file.Id

		// lock file is created now
		// errors after here are critical
		// if not handled, may block all other clients
		res, err = query.Do()
		if err != nil {
			if derr := g.srv.Files.Delete(file.Id).Do(); derr != nil {
				g.mu.Unlock()
				return fmt.Errorf("critical error: couldn't delete lock file: %v", derr)
			}

			g.mu.Unlock()
			return fmt.Errorf("couldn't query lock file: %v", err)
		}

		if len(res.Files) == 1 {
			break
		}

		if err := g.srv.Files.Delete(file.Id).Do(); err != nil {
			g.mu.Unlock()
			return fmt.Errorf("critical error: couldn't delete lock file: %v", err)
		}

		time.Sleep(time.Duration(rand.Int63n(400)+100) * time.Second)
	}

	return nil
}

// Unlock removes lock file from google drive
func (g *GDrive) Unlock() error {
	if err := g.srv.Files.Delete(g.lockID).Do(); err != nil {
		if strings.Contains(err.Error(), "404") { // ugly hack to distinguish not found
			g.mu.Unlock()
			return nil
		}

		return fmt.Errorf("couldn't delete lock file: %v", err)
	}

	g.mu.Unlock()
	return nil
}

// ComputeHash computes md5 checksum of provided file
func (g *GDrive) ComputeHash(r io.Reader, hchan chan string, echan chan error) {
	h := md5.New()

	if _, err := io.Copy(h, r); err != nil {
		echan <- fmt.Errorf("couldn't compute hash: %v", err)
	}

	hchan <- fmt.Sprintf("%x", h.Sum(nil))
}

// GetAvailableSpace returns available space in bytes
// @TODO: handle unlimited storage
func (g *GDrive) GetAvailableSpace() (int64, error) {
	res, err := g.srv.About.Get().Fields("storageQuota(limit, usage)").Do()
	if err != nil {
		return 0, fmt.Errorf("couldn't get available space: %v", err)
	}

	return res.StorageQuota.Limit - res.StorageQuota.Usage, nil
}

func (g *GDrive) getFileID(name string) (string, error) {
	res, err := g.srv.Files.List().PageSize(10).
		Q(fmt.Sprintf("name='%s' and trashed=false", name)).Fields("files(id, name)").Do()
	if err != nil {
		return "", fmt.Errorf("couldn't query file %s: %v", name, err)
	}

	if len(res.Files) == 0 {
		return "", common.ErrNotFound
	}

	if len(res.Files) > 1 {
		return "", fmt.Errorf("unexpected number of files on gdrive %d", len(res.Files))
	}

	return res.Files[0].Id, nil
}
