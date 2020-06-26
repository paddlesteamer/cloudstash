package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/pbkdf2"
)

const chunkSize = 4 * 1024

var salt = []byte{0x32, 0x24, 0x45, 0xa3, 0xb3, 0x89, 0x83, 0x56, 0x24, 0x66, 0x61, 0x18, 0x19, 0xc2, 0xff, 0xd0}

type Crypto struct {
	key []byte
}

func NewCrypto(key string) *Crypto {
	derived := pbkdf2.Key([]byte(key), salt, 4096, aes.BlockSize, sha256.New)

	c := &Crypto{
		key: derived,
	}

	return c
}

func (c *Crypto) NewEncryptReader(r io.Reader) io.Reader {
	pr, pw := io.Pipe()

	go c.encrypt(r, pw)

	return pr
}

func (c *Crypto) NewDecryptReader(r io.Reader) io.Reader {
	pr, pw := io.Pipe()

	go c.decrypt(r, pw)

	return pr
}

func (c *Crypto) encrypt(r io.Reader, w io.WriteCloser) {
	defer w.Close()

	block, err := aes.NewCipher(c.key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't create cipher block: %v\n", err)

		return
	}

	chunk := make([]byte, chunkSize)
	for {
		n, err := r.Read(chunk)
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "couldn't read from file: %v\n", err)

			return
		}

		brk := err == io.EOF

		if brk && n == 0 {
			break
		}

		padded := pad(chunk[:n], block.BlockSize())

		mac := c.computeHMAC(padded)

		_, err = w.Write(mac)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't write MAC to buffer: %v\n", err)

			return
		}

		ciphertext := make([]byte, block.BlockSize()+len(padded))
		iv := ciphertext[:block.BlockSize()]

		_, err = io.ReadFull(rand.Reader, iv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't read random values into IV: %v\n", err)

			return
		}

		enc := cipher.NewCBCEncrypter(block, iv)
		enc.CryptBlocks(ciphertext[block.BlockSize():], padded)

		_, err = w.Write(ciphertext)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't write ciphertext to buffer: %v\n", err)

			return
		}

		if brk {
			break
		}
	}

}

func (c *Crypto) decrypt(r io.Reader, w io.WriteCloser) {
	defer w.Close()

	block, err := aes.NewCipher(c.key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't create cipher block: %v\n", err)

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
				fmt.Fprintf(os.Stderr, "couldn't read HMAC from reader: %v\n", err)

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

		if (len(ciphertext) % block.BlockSize()) != 0 {
			fmt.Fprintf(os.Stderr, "malformed file: blocksize error(ciphertext length: %d)\n", len(ciphertext))

			return
		}

		dec := cipher.NewCBCDecrypter(block, iv)

		dec.CryptBlocks(ciphertext, ciphertext)

		if !hmac.Equal(mac, c.computeHMAC(ciphertext)) {
			fmt.Fprintf(os.Stderr, "file might be altered!\n")

			return
		}

		_, err = w.Write(ciphertext)
		if err != nil {
			fmt.Fprintf(os.Stderr, "couldn't write decrypted data to buffer: %v\n", err)

			return
		}

		if brk {
			break
		}
	}
}

func (c *Crypto) computeHMAC(chunk []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write(chunk)

	return mac.Sum(nil)
}

func pad(chunk []byte, blockSize int) []byte {
	padlength := blockSize - (len(chunk) % blockSize)

	if padlength == blockSize {
		return chunk
	}

	padtext := bytes.Repeat([]byte{0x00}, padlength)

	return append(chunk, padtext...)
}
