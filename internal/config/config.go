package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paddlesteamer/cloudstash/internal/auth/dropbox"
	"github.com/paddlesteamer/cloudstash/internal/auth/gdrive"
	"github.com/paddlesteamer/cloudstash/internal/common"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

type DropboxCredentials struct {
	AccessToken string
}

type Cfg struct {
	EncryptionKey string
	MountPoint    string
	Dropbox       *DropboxCredentials
	GDrive        *oauth2.Token
}

const (
	cfgFile         = "config.json"
	cfgFolder       = "cloudstash"
	mountFolderName = "cloudstash"
)

func DoesConfigExist(dir string) bool {
	path := getConfigPath(dir)
	_, err := os.Stat(path)
	if err != nil {
		return false
	}
	return true
}

func ReadConfig(dir string) (*Cfg, error) {
	path := getConfigPath(dir)

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open config file: %v", err)
	}
	defer f.Close()

	cfg := Cfg{}

	decoder := json.NewDecoder(f)
	if err = decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("unable to parse config json: %v", err)
	}

	return &cfg, nil
}

func NewConfig(cfgDir, mntDir string, secret []byte) (cfg *Cfg, err error) {
	dbxToken, err := dropbox.GetToken(common.DropboxAppKey)
	if err != nil {
		return nil, fmt.Errorf("couldn't get dropbox access token: %v", err)
	}

	gdrvCfg, _ := google.ConfigFromJSON([]byte(common.GDriveCredentials), drive.DriveFileScope)

	gdrvToken, err := gdrive.GetToken(gdrvCfg)
	if err != nil {
		return nil, fmt.Errorf("couldn't get dropbox access token: %v", err)
	}

	cfg = &Cfg{
		EncryptionKey: string(secret),
		MountPoint:    getMountPoint(mntDir),
		Dropbox:       &DropboxCredentials{dbxToken},
		GDrive:        gdrvToken,
	}

	if err := writeConfig(cfgDir, cfg); err != nil {
		return nil, fmt.Errorf("couldn't create config file: %v", err)
	}

	return cfg, nil
}

func writeConfig(dir string, cfg *Cfg) error {
	path := getConfigPath(dir)

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("couldn't create config directory: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("couldn't create config file: %v", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)

	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("couldn't encode struct: %v", err)
	}

	return nil
}

func getConfigPath(dir string) string {
	if dir != "" {
		dir = strings.TrimRight(dir, "/")
		return fmt.Sprintf("%s/%s", dir, cfgFile)
	}

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = "~/.config"
	}

	return fmt.Sprintf("%s/%s/%s", cfgDir, cfgFolder, cfgFile)
}

func getMountPoint(dir string) string {
	if dir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "~"
		}

		return fmt.Sprintf("%s/%s", homeDir, mountFolderName)
	}

	dir = strings.TrimRight(dir, "/")
	return fmt.Sprintf("%s/%s", dir, mountFolderName)
}
