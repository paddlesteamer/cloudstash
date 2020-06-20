package main

import (
	"fmt"
	"log"
	"time"

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

	for {
		time.Sleep(time.Second)
	}
}
