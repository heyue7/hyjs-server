// backupseal encrypts an owned gzip snapshot before it enters the backup directory.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

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
	// Explicit operational bound: large snapshots require a reviewed streaming tool.
	const maximum = 256 * 1024 * 1024
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return err
	}
	if len(raw) > maximum {
		return errors.New("backup exceeds 256 MiB limit; original snapshot retained")
	}
	out, err := transform(raw, key, decrypt)
	if err != nil {
		return errors.New("backup encryption/authentication failed")
	}
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
	if _, err = destination.Write(out); err != nil {
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

func main() {
	input := flag.String("input", "", "owned input file")
	output := flag.String("output", "", "new output file")
	key := flag.String("key-file", "", "private 32-byte key file")
	decrypt := flag.Bool("decrypt", false, "authenticate and decrypt without restoring a database")
	flag.Parse()
	if err := run(*input, *output, *key, *decrypt); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
