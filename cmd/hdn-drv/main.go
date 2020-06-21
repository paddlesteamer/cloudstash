package main

import (
	"fmt"
	"log"
	"os"

	"github.com/paddlesteamer/hdn-drv/internal/config"
	"github.com/paddlesteamer/hdn-drv/internal/fs"
	"github.com/paddlesteamer/hdn-drv/internal/manager"
	"github.com/vgough/go-fuse-c/fuse"
)

func main() {
	conf, err := config.ParseConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	//@TODO: check if mount point exists, create directory if necessary
	fmt.Printf(
		"mount point: %s\n",
		conf.MountPoint,
	)

	m, err := manager.NewManager(conf)
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()

	fs := fs.NewHdnDrvFs(m)

	fmt.Println([]string{os.Args[0], conf.MountPoint})
	fuse.MountAndRun([]string{os.Args[0], conf.MountPoint}, fs)
}
