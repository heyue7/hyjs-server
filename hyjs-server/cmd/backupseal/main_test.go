package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type repeatingBackupByte struct{}

func (repeatingBackupByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0x5a
	}
	return len(p), nil
}

type checkedBackupBytes struct{ count int64 }

func (w *checkedBackupBytes) Write(p []byte) (int, error) {
	for _, value := range p {
		if value != 0x5a {
			return 0, io.ErrUnexpectedEOF
		}
	}
	w.count += int64(len(p))
	return len(p), nil
}

func TestBackupSealStreamingBeyondLegacyLimit(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	const size = 260*1024*1024 + 7
	go func() {
		err := streamTransform(io.LimitReader(repeatingBackupByte{}, size), writer, key, false)
		writer.CloseWithError(err)
		done <- err
	}()
	defer reader.Close()
	checked := &checkedBackupBytes{}
	if err := streamTransform(reader, checked, key, true); err != nil {
		reader.CloseWithError(err)
		<-done
		t.Fatal(err)
	}
	if err := <-done; err != nil || checked.count != size {
		t.Fatal("large streaming roundtrip", checked.count, err)
	}
}

func TestBackupSealStreamingRejectsRecordAndEndTampering(t *testing.T) {
	key := bytes.Repeat([]byte{5}, 32)
	var empty bytes.Buffer
	if err := streamTransform(bytes.NewReader(nil), &empty, key, false); err != nil {
		t.Fatal(err)
	}
	if err := streamTransform(bytes.NewReader(empty.Bytes()), io.Discard, key, true); err != nil {
		t.Fatal("empty authenticated snapshot", err)
	}
	raw := bytes.Repeat([]byte("bounded snapshot"), streamChunk/10)
	var output bytes.Buffer
	if err := streamTransform(bytes.NewReader(raw), &output, key, false); err != nil {
		t.Fatal(err)
	}
	sealed := output.Bytes()
	firstEnd := 28 + 4 + streamChunk + 16
	mutations := [][]byte{
		append([]byte(nil), sealed[:len(sealed)-20]...), // Missing authenticated EOF.
		append(append([]byte(nil), sealed...), 1),
		append(append([]byte(nil), sealed[:28]...), sealed[firstEnd:]...), // Omitted first record.
	}
	for _, at := range []int{8, 24, 32, firstEnd + 4} {
		changed := append([]byte(nil), sealed...)
		changed[at] ^= 1
		mutations = append(mutations, changed)
	}
	oversized := append([]byte(nil), sealed...)
	binary.BigEndian.PutUint32(oversized[28:32], streamChunk+1)
	mutations = append(mutations, oversized)
	for i, changed := range mutations {
		if err := streamTransform(bytes.NewReader(changed), io.Discard, key, true); err == nil {
			t.Fatal("altered record/end accepted", i)
		}
	}
	if err := streamTransform(bytes.NewReader(sealed), io.Discard, bytes.Repeat([]byte{6}, 32), true); err == nil {
		t.Fatal("wrong streaming key accepted")
	}
}

func TestBackupSealReadsLegacyAndRemovesFailedOutput(t *testing.T) {
	dir := t.TempDir()
	key := bytes.Repeat([]byte{7}, 32)
	keyPath, input, output := filepath.Join(dir, "key"), filepath.Join(dir, "input"), filepath.Join(dir, "output")
	os.WriteFile(keyPath, key, 0600)
	sealed, err := transform([]byte("existing legacy snapshot"), key, false)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(input, sealed, 0600)
	if err = run(input, output, keyPath, true); err != nil {
		t.Fatal(err)
	}
	opened, _ := os.ReadFile(output)
	if string(opened) != "existing legacy snapshot" {
		t.Fatal("legacy bytes changed")
	}
	os.Remove(output)
	sealed[len(sealed)-1] ^= 1
	os.WriteFile(input, sealed, 0600)
	if err = run(input, output, keyPath, true); err == nil {
		t.Fatal("corrupt legacy accepted")
	}
	if _, err = os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("partial unauthenticated file retained", err)
	}
	var streamed bytes.Buffer
	if err = streamTransform(io.LimitReader(repeatingBackupByte{}, streamChunk+7), &streamed, key, false); err != nil {
		t.Fatal(err)
	}
	corrupt := streamed.Bytes()
	corrupt[len(corrupt)-1] ^= 1 // Fail after writing an earlier authenticated chunk.
	os.WriteFile(input, corrupt, 0600)
	if err = run(input, output, keyPath, true); err == nil {
		t.Fatal("corrupt streaming end accepted")
	}
	if _, err = os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("partial streaming plaintext retained", err)
	}
}

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

func TestVerifyBackupWithoutPlaintextCopy(t *testing.T) {
	dir := t.TempDir()
	key := bytes.Repeat([]byte{3}, 32)
	keyPath, input := filepath.Join(dir, "key"), filepath.Join(dir, "sealed")
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatal(err)
	}
	raw := bytes.Repeat([]byte("private compressed snapshot"), streamChunk/20)
	wanted := fmt.Sprintf("%x", sha256.Sum256(raw))
	for _, legacy := range []bool{false, true} {
		var sealed []byte
		if legacy {
			var err error
			sealed, err = transform(raw, key, false)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			var out bytes.Buffer
			if err := streamTransform(bytes.NewReader(raw), &out, key, false); err != nil {
				t.Fatal(err)
			}
			sealed = out.Bytes()
		}
		if err := os.WriteFile(input, sealed, 0600); err != nil {
			t.Fatal(err)
		}
		if err := verify(input, keyPath, wanted); err != nil {
			t.Fatal("valid snapshot", legacy, err)
		}
		if err := verify(input, keyPath, fmt.Sprintf("%x", sha256.Sum256([]byte("wrong")))); err == nil {
			t.Fatal("wrong digest accepted")
		}
		for _, changed := range [][]byte{sealed[:len(sealed)-1], append(append([]byte(nil), sealed...), 1)} {
			os.WriteFile(input, changed, 0600)
			if err := verify(input, keyPath, wanted); err == nil {
				t.Fatal("invalid EOF accepted")
			}
		}
		sealed[32] ^= 1
		os.WriteFile(input, sealed, 0600)
		if err := verify(input, keyPath, wanted); err == nil {
			t.Fatal("tampered record accepted")
		}
	}
	if err := verify(input, keyPath, "bad"); err == nil {
		t.Fatal("malformed digest accepted")
	}
	os.Chmod(keyPath, 0644)
	if err := verify(input, keyPath, wanted); err == nil {
		t.Fatal("unsafe key accepted")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 2 {
		t.Fatal("verification created a plaintext file", err)
	}
}
