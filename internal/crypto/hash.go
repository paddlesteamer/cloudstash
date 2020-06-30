package crypto

import (
	"fmt"
	"io"

	"github.com/paddlesteamer/cloudstash/internal/drive"
)

type HashStream struct {
	drv  drive.Drive
	hash string
	err  error
}

func NewHashStream(drv drive.Drive) *HashStream {
	return &HashStream{
		drv:  drv,
		hash: "",
	}
}

func (hs *HashStream) NewHashReader(r io.Reader) io.Reader {
	pr, pw := io.Pipe()

	go hs.computeHash(r, pw)

	return pr
}

func (hs *HashStream) GetComputedHash() (string, error) {
	return hs.hash, hs.err
}

func (hs *HashStream) computeHash(r io.Reader, w io.WriteCloser) {
	defer w.Close()

	pr, pw := io.Pipe()
	defer pw.Close()

	hchan := make(chan string)
	echan := make(chan error)

	go hs.drv.ComputeHash(pr, hchan, echan)

	buffer := make([]byte, 4096)

	for {
		n, err := r.Read(buffer)
		if err != nil && err != io.EOF {
			hs.err = fmt.Errorf("error reading into buffer: %v", err)
			return
		}

		brk := err == io.EOF

		if n == 0 && brk {
			break
		}

		if _, err := w.Write(buffer[:n]); err != nil {
			hs.err = fmt.Errorf("error forwarding stream: %v", err)
			return
		}

		if _, err := pw.Write(buffer[:n]); err != nil {
			hs.err = fmt.Errorf("error forwarding stream to hash function: %v", err)
			return
		}

		if brk {
			break
		}
	}

	pw.Close()

	select {
	case hash := <-hchan:
		hs.hash = hash
	case err := <-echan:
		hs.err = fmt.Errorf("error while hashing: %v", err)
	}
}
