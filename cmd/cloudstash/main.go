package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/config"
	"github.com/paddlesteamer/cloudstash/internal/crypto"
	"github.com/paddlesteamer/cloudstash/internal/drive"
	"github.com/paddlesteamer/cloudstash/internal/fs"
	"github.com/paddlesteamer/cloudstash/internal/manager"
	"github.com/paddlesteamer/cloudstash/internal/sqlite"
	"github.com/paddlesteamer/go-fuse-c/fuse"
)

func main() {
	cfgDir, mntDir := parseFlags()

	// read existing or create new configuration
	cfg, err := config.Configure(cfgDir, mntDir)
	if err != nil {
		log.Fatalf("could not configure: %v", err)
	}

	// create mount directory
	if err := os.MkdirAll(cfg.MountPoint, 0755); err != nil {
		log.Fatalf("could not create mount directory: %v", err)
	}
	log.Printf("mount point: %s\n", cfg.MountPoint)

	url, err := common.ParseURL(common.DATABASE_FILE)
	if err != nil {
		log.Fatalf("could not parse DB file URL: %v", err)
	}

	drives := collectDrives(cfg)
	idx, err := findMatchingDriveIdx(url, drives)
	if err != nil {
		log.Fatalf("could not match DB file to any of the available drives: %v", err)
	}

	cipher := crypto.NewCrypto(cfg.EncryptionKey)
	dbPath, hash, err := initOrImportDB(drives[idx], url.Path, cipher)
	if err != nil {
		log.Fatalf("could not initialize or import an existing DB file: %v", err)
	}

	db := manager.NewDB(dbPath, url.Path, hash, drives[idx])
	defer db.Close()

	m := manager.NewManager(drives, db, cipher, cfg.EncryptionKey)
	defer m.Close()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go handleSignal(signalCh, cfg.MountPoint)

	fs := fs.NewCloudStashFs(m)
	fuse.MountAndRun([]string{os.Args[0], cfg.MountPoint}, fs)
}

// parseFlags parses the command-line flags.
func parseFlags() (cfgDir, mntDir string) {
	flag.StringVar(&cfgDir, "c", "", "Application config directory, optional.")
	flag.StringVar(&mntDir, "m", "", "Application mount directory, optional.")
	flag.Parse()
	return cfgDir, mntDir
}

// collectDrives returns a slice of clients for each enabled drive.
func collectDrives(cfg *config.Cfg) (drives []drive.Drive) {
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

func initAndUploadDB(drv drive.Drive, dbPath, dbExtPath string, cipher *crypto.Crypto) (string, error) {
	if err := sqlite.InitDB(dbPath); err != nil {
		return "", fmt.Errorf("could not initialize DB: %v", err)
	}

	dbFile, err := os.Open(dbPath)
	if err != nil {
		os.Remove(dbPath)
		return "", fmt.Errorf("could not open intitialized DB: %v", err)
	}
	defer dbFile.Close()

	hs := crypto.NewHashStream(drv)

	err = drv.PutFile(dbExtPath, hs.NewHashReader(cipher.NewEncryptReader(dbFile)))
	if err != nil {
		os.Remove(dbPath)
		return "", fmt.Errorf("could not upload initialized DB: %v", err)
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		os.Remove(dbPath)
		return "", fmt.Errorf("couldn't compute hash of newly installed DB: %v", err)
	}

	return hash, nil
}

func initOrImportDB(drv drive.Drive, extPath string, cipher *crypto.Crypto) (string, string, error) {
	file, err := common.NewTempDBFile()
	if err != nil {
		return "", "", fmt.Errorf("could not create DB file: %v", err)
	}
	defer file.Close()

	_, reader, err := drv.GetFile(extPath)

	if err == drive.ErrNotFound {
		file.Close() // should be closed before initialization

		hash, err := initAndUploadDB(drv, file.Name(), extPath, cipher)
		if err != nil {
			return "", "", fmt.Errorf("could not initialize DB: %v", err)
		}

		return file.Name(), hash, nil
	} else if err != nil {
		return "", "", fmt.Errorf("could not get file: %v", err)
	}
	defer reader.Close()

	hs := crypto.NewHashStream(drv)

	_, err = io.Copy(file, cipher.NewDecryptReader(hs.NewHashReader(reader)))
	if err != nil {
		os.Remove(file.Name())
		return "", "", fmt.Errorf("could not copy contents of DB to local file: %v", err)
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		os.Remove(file.Name())
		return "", "", fmt.Errorf("couldn't compute hash of database file: %v", err)
	}

	sqlite.SetPath(file.Name())

	return file.Name(), hash, nil
}

func handleSignal(ch chan os.Signal, mountpoint string) {
	_ = <-ch

	fuse.UMount(mountpoint)
}
