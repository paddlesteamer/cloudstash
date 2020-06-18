package main

import (
	"fmt"
	"log"

	"github.com/paddlesteamer/hdn-drv/config"
	"github.com/paddlesteamer/hdn-drv/manager"
)

func main() {
	conf, err := config.ParseConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("key: %v, clientID: %v, accessToken: %v\n", conf.EncryptionKey, conf.Dropbox.ClientID, conf.Dropbox.AccessToken)

	m, err := manager.NewManager(conf)
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()
}