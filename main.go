package main

import (
	"fmt"
	"log"

	"github.com/paddlesteamer/hdn-drv/config"
	"github.com/paddlesteamer/hdn-drv/drive"
)

func main() {
	conf, err := config.ParseConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("key: %v, clientID: %v, accessToken: %v\n", conf.EncryptionKey, conf.Dropbox.ClientID, conf.Dropbox.AccessToken)

	drive := drive.NewDropboxClient(conf.Dropbox)

	content, err := drive.GetFile("/a.txt")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(content))
}
