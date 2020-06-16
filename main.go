package main

import (
	"fmt"
	"log"

	"github.com/paddlesteamer/hdn-drv/config"
	"github.com/paddlesteamer/hdn-drv/source"
)

func main() {
	conf, err := config.ParseConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("key: %v, clientID: %v, accessToken: %v\n", conf.EncryptionKey, conf.Dropbox.ClientID, conf.Dropbox.AccessToken)

	source := source.NewDropboxClient(conf.Dropbox)

	entries, err := source.ListFolder("/")
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		fmt.Println(entry.Name)
	}
}
