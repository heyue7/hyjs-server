package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupSealAuthentication(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	raw := bytes.Repeat([]byte("private backup bytes"), 1000)
	sealed, err := transform(raw, key, false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, raw[:100]) {
		t.Fatal("plaintext retained")
	}
	opened, err := transform(sealed, key, true)
	if err != nil || !bytes.Equal(opened, raw) {
		t.Fatal("roundtrip", err)
	}
	for _, invalid := range [][]byte{sealed[:len(sealed)-1], append([]byte(nil), sealed...)} {
		invalid[len(invalid)-1] ^= 1
		if _, err := transform(invalid, key, true); err == nil {
			t.Fatal("tamper accepted")
		}
	}
	if _, err := transform(sealed, bytes.Repeat([]byte{8}, 32), true); err == nil {
		t.Fatal("wrong key accepted")
	}
}

func TestBackupSealPrivateFiles(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	plain := filepath.Join(dir, "input")
	sealed := filepath.Join(dir, "sealed")
	restored := filepath.Join(dir, "restored")
	if err := os.WriteFile(key, bytes.Repeat([]byte{6}, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plain, []byte("owned gzip snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(plain, sealed, key, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(sealed)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("backup permissions", err)
	}
	if err := run(sealed, restored, key, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(restored)
	if err != nil || string(data) != "owned gzip snapshot" {
		t.Fatal("restore mismatch", err)
	}
	if err := run(plain, sealed, key, false); err == nil {
		t.Fatal("existing backup overwritten")
	}
	if err := os.Chmod(key, 0644); err != nil {
		t.Fatal(err)
	}
	if err := run(plain, filepath.Join(dir, "unsafe"), key, false); err == nil {
		t.Fatal("public key permissions accepted")
	}
}
