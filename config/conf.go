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

type Configuration struct {
	EncryptionKey string
	Dropbox       DropboxCredentials
}

func ParseConfig(path string) (*Configuration, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open configuration file: %v", err)
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	conf := Configuration{}

	err = decoder.Decode(&conf);
	if err != nil {
		return nil, fmt.Errorf("unable to parse config json: %v", err)
	}

	return &conf, nil
}
