package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type DropboxCredentials struct {
	ClientID    string
	AccessToken string
}

type Cfg struct {
	EncryptionKey string
	Dropbox       *DropboxCredentials
	DatabaseFile  string
	MountPoint    string
}

func ParseConfig(path string) (cfg Cfg, err error) {
	f, err := os.Open(path)
	if err != nil {
		return cfg, fmt.Errorf("unable to open config file: %v", err)
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	if err = decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("unable to parse config json: %v", err)
	}

	return cfg, nil
}
