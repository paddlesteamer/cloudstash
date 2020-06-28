package config

import (
	"fmt"
	"os"
	"strings"
)

const mountFolderName string = "hdn-drv"

func GetMountPoint(dir string) string {
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

func CreateMountPoint(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("couldn't create mount directory: %v", err)
	}

	return nil
}
