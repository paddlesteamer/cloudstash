package manager

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/paddlesteamer/cloudstash/internal/common"
	"github.com/paddlesteamer/cloudstash/internal/crypto"
)

const (
	checkInterval   time.Duration = 60 * time.Second
	processInterval time.Duration = 10 * time.Second
)

func watchRemoteChanges(m *Manager) {
	for {
		time.Sleep(checkInterval)
		checkChanges(m)
	}
}

func checkChanges(m *Manager) {
	mdata, err := m.db.extDrive.GetFileMetadata(m.db.extPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	m.db.wLock()
	defer m.db.wUnlock()

	if mdata.Hash == m.db.hash {
		return
	}

	_, reader, err := m.db.extDrive.GetFile(m.db.extPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't get updated db file: %v\n", err)

		return
	}
	defer reader.Close()

	file, err := os.Open(m.db.path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't open db: %v\n", err)

		return
	}
	defer file.Close()

	_, err = io.Copy(file, m.cipher.NewDecryptReader(reader))
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't copy contents of updated db file to local file: %v\n", err)

		return
	}
}

func processLocalChanges(m *Manager) {
	for {
		time.Sleep(processInterval)
		processChanges(m)
	}
}

func processChanges(m *Manager) {
	items := m.tracker.Flush()

	if len(items) == 0 {
		return
	}

	m.db.wLock()
	defer m.db.wUnlock()

	wg := sync.WaitGroup{}
	wg.Add(len(items))

	for local, it := range items {
		url := it.Object.(string)

		go processItem(local, url, m, &wg)
	}

	// wait for all uploads to complete otherwise
	// the next processChanges call may conflict with this one
	// also m.db.wUnlock would be called before it should
	wg.Wait()
}

func processItem(local string, url string, m *Manager, wg *sync.WaitGroup) {
	defer wg.Done()

	u, err := common.ParseURL(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't parse url %s. skipping: %v\n", url, err)
		return
	}

	drv, err := m.getDriveClient(u.Scheme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't find drive client of %s: %v\n", u.Scheme, err)
		return
	}

	file, err := os.Open(local)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't open file %s: %v\n", local, err)
		return
	}
	defer file.Close()

	hs := crypto.NewHashStream(drv)

	err = drv.PutFile(u.Path, hs.NewHashReader(m.cipher.NewEncryptReader(file)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't upload file: %v\n", err)
		return
	}

	hash, err := hs.GetComputedHash()
	if err != nil {
		// just log the error and continue
		fmt.Fprintf(os.Stderr, "couldn't compute hash of file: %v\n", err)
	}

	// if this file is database file
	if local == m.db.path {
		m.db.hash = hash
	}

}
