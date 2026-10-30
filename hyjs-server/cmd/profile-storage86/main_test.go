package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func backupFixture86(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".hyjs-owned"), []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "backups"), 0o700); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backups", "fixture.aesgcm")
	data := []byte("private backup fixture")
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return root, backup, hex.EncodeToString(digest[:])
}

func TestProfileStorage86Options(t *testing.T) {
	root, backup, digest := backupFixture86(t)
	base := []string{"--expected-database", "hyjs_test_owned", "--project-root", root, "--backup-file", backup, "--backup-sha256", digest, "--max-rows", "10"}
	options, err := parseOptions86(base, &bytes.Buffer{})
	if err != nil || options.rollback.Apply || options.rollback.After != nil || options.rollback.BatchSize != 50 || options.rollback.MaxRows != 10 {
		t.Fatal("default preview differs", options, err)
	}
	args := append(append([]string{}, base...), "--apply", "--app-stopped", "--after=-2", "--batch-size=1")
	options, err = parseOptions86(args, &bytes.Buffer{})
	if err != nil || !options.rollback.Apply || options.rollback.After == nil || *options.rollback.After != -2 || options.rollback.BatchSize != 1 {
		t.Fatal("explicit apply differs", options, err)
	}
	for _, extra := range [][]string{{"--apply"}, {"--batch-size=0"}, {"--batch-size=51"}, {"--max-rows=0"}, {"--after=invalid"}, {"--backup-sha256=" + strings.ToUpper(digest)}} {
		if _, err = parseOptions86(append(append([]string{}, base...), extra...), &bytes.Buffer{}); err == nil {
			t.Fatal("invalid options accepted", extra)
		}
	}
}

func TestProfileStorage86BackupBinding(t *testing.T) {
	root, backup, digest := backupFixture86(t)
	if err := verifyBackup86(root, backup, digest); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackup86(root, backup, strings.Repeat("0", 64)); err == nil {
		t.Fatal("mismatched backup accepted")
	}
	outside := filepath.Join(t.TempDir(), "other.aesgcm")
	if err := os.WriteFile(outside, []byte("private backup fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackup86(root, outside, digest); err == nil {
		t.Fatal("external backup accepted")
	}
	link := filepath.Join(root, "backups", "link.aesgcm")
	if err := os.Symlink(backup, link); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackup86(root, link, digest); err == nil {
		t.Fatal("symlink backup accepted")
	}
	linkDir := filepath.Join(root, "linked-backups")
	if err := os.Symlink(filepath.Join(root, "backups"), linkDir); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackup86(root, filepath.Join(linkDir, filepath.Base(backup)), digest); err == nil {
		t.Fatal("symlink directory accepted")
	}
	if err := os.Remove(filepath.Join(root, ".hyjs-owned")); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackup86(root, backup, digest); err == nil {
		t.Fatal("unowned project accepted")
	}
}

func TestProfileStorage86WrongDatabaseStopsBeforeConnecting(t *testing.T) {
	root, backup, digest := backupFixture86(t)
	args := []string{"--expected-database", "hyjs_test_owned", "--project-root", root, "--backup-file", backup, "--backup-sha256", digest, "--max-rows", "1"}
	environment := map[string]string{"HYJS_MYSQL_HOST": "host", "HYJS_MYSQL_USER": "user", "HYJS_MYSQL_PASSWORD": "private-secret", "HYJS_MYSQL_DATABASE": "unexpected"}
	var output bytes.Buffer
	err := run86(context.Background(), args, func(key string) string { return environment[key] }, &output, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "private-secret") || output.Len() != 0 {
		t.Fatal("wrong target was accepted or revealed credentials", err)
	}
}
