package main

import (
	"fmt"
	"log"
)

func main() {
	conf, err := ParseConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("key: %v, clientID: %v, accessToken: %v\n", conf.EncryptionKey, conf.Dropbox.ClientID, conf.Dropbox.AccessToken)
}
