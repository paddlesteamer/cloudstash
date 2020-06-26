package main

import (
	"fmt"
	"log"
	"os"

	"github.com/paddlesteamer/hdn-drv/internal/common"
	"github.com/paddlesteamer/hdn-drv/internal/config"
	"github.com/paddlesteamer/hdn-drv/internal/drive"
	"github.com/paddlesteamer/hdn-drv/internal/fs"
	"github.com/paddlesteamer/hdn-drv/internal/manager"
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

	m, err := manager.NewManager(cfg, url, drives, drives[idx])
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()

	fs := fs.NewHdnDrvFs(m)

	fuse.MountAndRun([]string{os.Args[0], cfg.MountPoint}, fs)
}

// collectDrives returns a slice of clients for each enabled drive.
func collectDrives(cfg config.Cfg) []drive.Drive {
	drives := []drive.Drive{}
	if cfg.Dropbox != nil {
		dbx := drive.NewDropboxClient(cfg.Dropbox)
		drives = append(drives, dbx)
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
