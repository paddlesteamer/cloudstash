package main

import (
	"fmt"
	"log"
	"time"

	"github.com/paddlesteamer/hdn-drv/internal/common"
	"github.com/paddlesteamer/hdn-drv/internal/config"
	"github.com/paddlesteamer/hdn-drv/internal/manager"
)

func main() {
	conf, err := config.ParseConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf(
		"Key: %s, Client ID: %s, Access Token: %s\n",
		conf.EncryptionKey,
		conf.Dropbox.ClientID,
		conf.Dropbox.AccessToken,
	)

	m, err := manager.NewManager(conf)
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()

	_, err = m.Lookup(1, ".Trash")
	switch {
	case err == nil:
		fmt.Printf("file is found")
	case err == common.ErrNotFound:
		fmt.Printf("file doesn't exist\n")
	default:
		fmt.Printf("error on lookup: %v", err)
	}

	for {
		time.Sleep(time.Second)
	}
}
