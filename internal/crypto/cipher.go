package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"

	log "github.com/sirupsen/logrus"
)

const (
	iterationCount = 1000000
	keyLength      = 32
)

const chunkSize = 4 * 1024

var salt = []byte{
	0x32, 0x24, 0x45, 0xa3, 0xb3, 0x89, 0x83, 0x56, 0x24, 0x66, 0x61, 0x18, 0x19, 0xc2, 0xff, 0xd0,
}

type Cipher struct {
	key []byte
}

func DeriveKey(key []byte) string {
	derived := pbkdf2.Key(key, salt, iterationCount, keyLength, sha256.New)

	return fmt.Sprintf("%x", derived)
}

func NewCipher(key string) *Cipher {
	decoded, _ := hex.DecodeString(key)

	return &Cipher{decoded}
}

func (c *Cipher) NewEncryptReader(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go c.encrypt(r, pw)
	return pr
}

func (c *Cipher) NewDecryptReader(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go c.decrypt(r, pw)
	return pr
}

func (c *Cipher) encrypt(r io.Reader, w io.WriteCloser) {
	defer w.Close()

	block, err := aes.NewCipher(c.key)
	if err != nil {
		log.Errorf("couldn't create cipher block: %v", err)

		return
	}

	chunk := make([]byte, chunkSize)
	for {
		n, err := r.Read(chunk)
		if err != nil && err != io.EOF {
			log.Errorf("couldn't read from file: %v", err)

			return
		}

		brk := err == io.EOF

		if brk && n == 0 {
			break
		}

		mac := c.computeHMAC(chunk[:n])

		_, err = w.Write(mac)
		if err != nil {
			log.Errorf("couldn't write MAC to buffer: %v", err)

			return
		}

		ciphertext := make([]byte, block.BlockSize()+n)
		iv := ciphertext[:block.BlockSize()]

		_, err = io.ReadFull(rand.Reader, iv)
		if err != nil {
			log.Errorf("couldn't read random values into IV: %v", err)

			return
		}

		enc := cipher.NewCTR(block, iv)
		enc.XORKeyStream(ciphertext[block.BlockSize():], chunk[:n])

		_, err = w.Write(ciphertext)
		if err != nil {
			log.Errorf("couldn't write ciphertext to buffer: %v", err)
			return
		}

		if brk {
			break
		}
	}
}

func (c *Cipher) decrypt(r io.Reader, w io.WriteCloser) {
	defer w.Close()

	block, err := aes.NewCipher(c.key)
	if err != nil {
		log.Errorf("couldn't create cipher block: %v", err)

		return
	}

	chunk := make([]byte, chunkSize+block.BlockSize()+32) // chunk + iv + hmac

	mac := chunk[:32]
	iv := chunk[32 : 32+block.BlockSize()]

	for {
		ntotal := 0
		brk := false

		for {
			n, err := r.Read(chunk[ntotal:])
			if err != nil && err != io.EOF {
				log.Errorf("couldn't read HMAC from reader: %v", err)
				return
			}

			ntotal += n

			if err == io.EOF || ntotal == len(chunk) {
				brk = err == io.EOF
				break
			}
		}

		if ntotal == 0 && brk {
			break
		}

		ciphertext := chunk[32+block.BlockSize() : ntotal]

		dec := cipher.NewCTR(block, iv)
		dec.XORKeyStream(ciphertext, ciphertext)

		if !hmac.Equal(mac, c.computeHMAC(ciphertext)) {
			log.Error("file might be altered!")
			return
		}

		_, err = w.Write(ciphertext)
		if err != nil {
			log.Errorf("couldn't write decrypted data to buffer: %v", err)
			return
		}

		if brk {
			break
		}
	}
}

func (c *Cipher) computeHMAC(chunk []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write(chunk)
	return mac.Sum(nil)
}
