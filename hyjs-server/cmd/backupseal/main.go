// backupseal encrypts an owned gzip snapshot before it enters the backup directory.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const (
	streamMagic = "HYJSBK02"
	streamChunk = 4 * 1024 * 1024
	streamLimit = int64(8 * 1024 * 1024 * 1024)
)

// Each file has a random 128-bit nonce prefix. Record counters make every
// nonce unique within that file; the authenticated empty record seals EOF.
func streamRecord(prefix, header []byte, counter uint64, length uint32) ([]byte, []byte) {
	nonce := append([]byte(nil), prefix...)
	nonce = binary.BigEndian.AppendUint64(nonce, counter)
	aad := append([]byte(nil), header...)
	aad = binary.BigEndian.AppendUint64(aad, counter)
	aad = binary.BigEndian.AppendUint32(aad, length)
	return nonce, aad
}

func streamTransform(input io.Reader, output io.Writer, key []byte, decrypt bool) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, 24)
	if err != nil {
		return err
	}
	header := make([]byte, 28)
	copy(header, streamMagic)
	if decrypt {
		if _, err = io.ReadFull(input, header); err != nil || string(header[:8]) != streamMagic || binary.BigEndian.Uint32(header[24:]) != streamChunk {
			return errors.New("invalid streaming backup header")
		}
	} else {
		if _, err = rand.Read(header[8:24]); err != nil {
			return err
		}
		binary.BigEndian.PutUint32(header[24:], streamChunk)
		if _, err = output.Write(header); err != nil {
			return err
		}
	}
	plain := make([]byte, streamChunk)
	sealed := make([]byte, streamChunk+gcm.Overhead())
	var length [4]byte
	var total int64
	for counter := uint64(0); ; counter++ {
		var n int
		if decrypt {
			if _, err = io.ReadFull(input, length[:]); err != nil {
				return errors.New("streaming backup has no authenticated end")
			}
			n = int(binary.BigEndian.Uint32(length[:]))
			if n > streamChunk {
				return errors.New("invalid streaming backup record size")
			}
		} else {
			n, err = io.ReadFull(input, plain)
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				return err
			}
			binary.BigEndian.PutUint32(length[:], uint32(n))
		}
		total += int64(n)
		if total > streamLimit {
			return errors.New("backup exceeds 8 GiB streaming limit")
		}
		nonce, aad := streamRecord(header[8:24], header, counter, uint32(n))
		if decrypt {
			if _, err = io.ReadFull(input, sealed[:n+gcm.Overhead()]); err != nil {
				return errors.New("truncated streaming backup record")
			}
			opened, e := gcm.Open(plain[:0], nonce, sealed[:n+gcm.Overhead()], aad)
			if e != nil {
				return errors.New("backup authentication failed")
			}
			if n == 0 {
				var trailing [1]byte
				if count, e := input.Read(trailing[:]); count != 0 || e != io.EOF {
					return errors.New("data follows authenticated backup end")
				}
				return nil
			}
			if _, err = output.Write(opened); err != nil {
				return err
			}
		} else {
			ciphertext := gcm.Seal(sealed[:0], nonce, plain[:n], aad)
			if _, err = output.Write(length[:]); err != nil {
				return err
			}
			if _, err = output.Write(ciphertext); err != nil {
				return err
			}
			if n == 0 {
				return nil
			}
		}
	}
}

func transform(input, key []byte, decrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	header := []byte("HYJSBK01")
	if decrypt {
		if len(input) < len(header)+gcm.NonceSize()+gcm.Overhead() || !bytes.Equal(input[:len(header)], header) {
			return nil, errors.New("invalid encrypted backup")
		}
		return gcm.Open(nil, input[len(header):len(header)+gcm.NonceSize()], input[len(header)+gcm.NonceSize():], header)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(append(header, nonce...), nonce, input, header), nil
}

func run(inputPath, outputPath, keyPath string, decrypt bool) error {
	if inputPath == "" || outputPath == "" || keyPath == "" {
		return errors.New("input, output and key-file are required")
	}
	info, err := os.Lstat(keyPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 32 {
		return errors.New("key must be a private 32-byte regular file")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	destination, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		destination.Close()
		if !ok {
			os.Remove(outputPath)
		}
	}()
	if decrypt {
		err = decryptBackup(file, destination, key)
	} else {
		err = streamTransform(file, destination, key, false)
	}
	if err != nil {
		return err
	}
	if err = destination.Sync(); err != nil {
		return err
	}
	if err = destination.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

// Verification authenticates every record and EOF before comparing the original
// compressed snapshot digest, without writing another plaintext copy.
func decryptBackup(file *os.File, output io.Writer, key []byte) error {
	var magic [8]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return errors.New("invalid encrypted backup")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if string(magic[:]) != "HYJSBK01" {
		return streamTransform(file, output, key, true)
	}
	const maximum = 256*1024*1024 + 64
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || len(raw) > maximum {
		return errors.New("invalid legacy backup size")
	}
	opened, err := transform(raw, key, true)
	if err != nil {
		return errors.New("backup authentication failed")
	}
	_, err = output.Write(opened)
	return err
}

func verify(inputPath, keyPath, expected string) error {
	wanted, err := hex.DecodeString(expected)
	if err != nil || len(wanted) != sha256.Size {
		return errors.New("verify-sha256 must be a SHA256 digest")
	}
	info, err := os.Lstat(keyPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 32 {
		return errors.New("key must be a private 32-byte regular file")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	if err := decryptBackup(file, digest, key); err != nil {
		return err
	}
	if !bytes.Equal(digest.Sum(nil), wanted) {
		return errors.New("verified backup digest differs from original snapshot")
	}
	return nil
}

func main() {
	input := flag.String("input", "", "owned input file")
	output := flag.String("output", "", "new output file")
	key := flag.String("key-file", "", "private 32-byte key file")
	decrypt := flag.Bool("decrypt", false, "authenticate and decrypt without restoring a database")
	verifySHA := flag.String("verify-sha256", "", "authenticate without writing plaintext and compare its SHA256")
	flag.Parse()
	var err error
	if *verifySHA != "" {
		if *output != "" || *decrypt {
			err = errors.New("verify-sha256 cannot be combined with output or decrypt")
		} else {
			err = verify(*input, *key, *verifySHA)
		}
	} else {
		err = run(*input, *output, *key, *decrypt)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
