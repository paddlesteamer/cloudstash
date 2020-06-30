package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"syscall"

	"github.com/paddlesteamer/cloudstash/internal/auth"
	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/config"
	"github.com/paddlesteamer/cloudstash/internal/crypto"
	"github.com/paddlesteamer/cloudstash/internal/drive"
	"github.com/paddlesteamer/cloudstash/internal/fs"
	"github.com/paddlesteamer/cloudstash/internal/manager"
	"github.com/paddlesteamer/cloudstash/internal/sqlite"
	"github.com/vgough/go-fuse-c/fuse"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	var cfgDir string
	var mntDir string

	flag.StringVar(&cfgDir, "c", "", "Application config directory. Optional.")
	flag.StringVar(&mntDir, "m", "", "Application mount directory. Optional.")
	flag.Parse()

	var cfg *config.Cfg
	if !config.DoesConfigExist(cfgDir) {
		fmt.Print("Encryption key: ")
		pwd, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatalf("couldn't read password from terminal\n")
		}

		dbxToken, err := auth.GetDropboxToken(common.DROPBOX_APP_KEY)
		if err != nil {
			log.Fatalf("couldn't get dropbox access token: %v\n", err)
		}

		mnt := config.GetMountPoint(mntDir)

		cfg = &config.Cfg{
			EncryptionKey: string(pwd),
			MountPoint:    mnt,
			Dropbox: &config.DropboxCredentials{
				AccessToken: dbxToken,
			},
		}

		if err := config.WriteConfig(cfgDir, cfg); err != nil {
			log.Fatalf("couldn't create config file: %v\n", err)
		}

	} else {
		c, err := config.ParseConfig(cfgDir)
		if err != nil {
			log.Fatal(err)
		}

		cfg = c
	}

	if err := config.CreateMountPoint(cfg.MountPoint); err != nil {
		log.Fatalf("couldn't create mount directory: %v\n", err)
	}

	fmt.Printf("mount point: %s\n", cfg.MountPoint)

	drives := collectDrives(cfg)
	url, err := common.ParseURL(common.DATABASE_FILE)
	if err != nil {
		log.Fatalf("could not parse DB file URL: %v", err)
	}

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

	fs := fs.NewCloudStashFs(m)
	fuse.MountAndRun([]string{os.Args[0], cfg.MountPoint}, fs)
}

// collectDrives returns a slice of clients for each enabled drive.
func collectDrives(cfg *config.Cfg) []drive.Drive {
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
