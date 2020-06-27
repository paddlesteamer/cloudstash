package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/paddlesteamer/hdn-drv/internal/common"
	"github.com/paddlesteamer/hdn-drv/internal/config"
	"github.com/paddlesteamer/hdn-drv/internal/crypto"
	"github.com/paddlesteamer/hdn-drv/internal/drive"
	"github.com/paddlesteamer/hdn-drv/internal/fs"
	"github.com/paddlesteamer/hdn-drv/internal/manager"
	"github.com/paddlesteamer/hdn-drv/internal/sqlite"
	"github.com/vgough/go-fuse-c/fuse"
)

func main() {
	cfg, err := config.ParseConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	// @TODO: check if mount point exists, create directory if necessary
	fmt.Printf("mount point: %s\n", cfg.MountPoint)

	drives := collectDrives(cfg)
	url, err := common.ParseURL(cfg.DatabaseFile)
	if err != nil {
		log.Fatalf("could not parse DB file URL: %v", err)
	}

	idx, err := findMatchingDriveIdx(url, drives)
	if err != nil {
		log.Fatalf("could not match DB file to any of the available drives: %v", err)
	}

	cipher := crypto.NewCrypto(cfg.EncryptionKey)

	dbPath, err := initOrImportDB(drives[idx], url.Path, cipher)
	if err != nil {
		log.Fatalf("could not initialize or import an existing DB file: %v", err)
	}

	hash, err := drives[idx].ComputeHash(dbPath)
	if err != nil {
		os.Remove(dbPath)
		log.Fatalf("could not compute hash: %v", err)
	}

	db := manager.NewDB(dbPath, url.Path, hash, drives[idx])
	defer db.Close()

	m := manager.NewManager(drives, db, cipher, cfg.EncryptionKey)

	fs := fs.NewHdnDrvFs(m)
	fuse.MountAndRun([]string{os.Args[0], cfg.MountPoint}, fs)
}

// collectDrives returns a slice of clients for each enabled drive.
func collectDrives(cfg config.Cfg) []drive.Drive {
	drives := []drive.Drive{}
	if cfg.Dropbox != nil {
		dbox := drive.NewDropboxClient(cfg.Dropbox)
		drives = append(drives, dbox)
	}

	// @TODO: add GDrive

	return drives
}

// findMatchingDrive returns the drive from the given list that matches the DB file scheme.
func findMatchingDriveIdx(url *common.FileURL, drives []drive.Drive) (idx int, err error) {
	for i, d := range drives {
		if d.GetProviderName() == url.Scheme {
			return i, nil
		}
	}

	return -1, fmt.Errorf("could not find a drive matching database file scheme")
}

func initAndUploadDB(drv *drive.Drive, dbPath, dbExtPath string, cipher *crypto.Crypto) error {
	if err := sqlite.InitDB(dbPath); err != nil {
		return fmt.Errorf("could not initialize DB: %v", err)
	}

	dbFile, err := os.Open(dbPath)
	if err != nil {
		os.Remove(dbPath)
		return fmt.Errorf("could not open intitialized DB: %v", err)
	}
	defer dbFile.Close()

	err = (*drv).PutFile(dbExtPath, cipher.NewEncryptReader(dbFile))
	if err != nil {
		os.Remove(dbPath)
		return fmt.Errorf("could not upload initialized DB: %v", err)
	}

	return nil
}

func initOrImportDB(drv drive.Drive, extPath string, cipher *crypto.Crypto) (string, error) {
	file, err := common.NewTempDBFile()
	if err != nil {
		return "", fmt.Errorf("could not create DB file: %v", err)
	}
	defer file.Close()

	_, reader, err := drv.GetFile(extPath)

	if err == drive.ErrNotFound {
		file.Close() // should be closed before initialization

		if err := initAndUploadDB(&drv, file.Name(), extPath, cipher); err != nil {
			return "", fmt.Errorf("could not initialize DB: %v", err)
		}

		return file.Name(), nil
	} else if err != nil {
		return "", fmt.Errorf("could not get file: %v", err)
	}
	defer reader.Close()

	_, err = io.Copy(file, cipher.NewDecryptReader(reader))
	if err != nil {
		os.Remove(file.Name())
		return "", fmt.Errorf("could not copy contents of DB to local file: %v", err)
	}

	sqlite.SetPath(file.Name())

	return file.Name(), nil
}
