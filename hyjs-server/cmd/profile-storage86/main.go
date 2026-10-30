// profile-storage86 previews or expands stored rosters for a maintenance rollback.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/service"
)

type options86 struct {
	expectedDatabase string
	projectRoot      string
	backupFile       string
	backupSHA256     string
	appStopped       bool
	rollback         service.ProfileStorageRollback86Options
}

func parseOptions86(args []string, help io.Writer) (options86, error) {
	var options options86
	flags := flag.NewFlagSet("profile-storage86", flag.ContinueOnError)
	flags.SetOutput(help)
	flags.StringVar(&options.expectedDatabase, "expected-database", "", "exact configured and connected MySQL database")
	flags.StringVar(&options.projectRoot, "project-root", "", "owned project deployment directory")
	flags.StringVar(&options.backupFile, "backup-file", "", "authenticated backup file within project root")
	flags.StringVar(&options.backupSHA256, "backup-sha256", "", "SHA-256 of the authenticated backup file")
	flags.BoolVar(&options.rollback.Apply, "apply", false, "expand profiles; default is read-only preview")
	flags.BoolVar(&options.appStopped, "app-stopped", false, "operator confirms the application has been stopped")
	flags.IntVar(&options.rollback.BatchSize, "batch-size", constUtil.ProfileStorageRollbackBatchMax86, "profiles per transaction, 1 to 50")
	flags.Int64Var(&options.rollback.MaxRows, "max-rows", 0, "maximum profiles to scan in this run; required")
	flags.Func("after", "resume strictly after this profile ID; omit to begin at the first ID", func(value string) error {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return errors.New("invalid profile cursor")
		}
		options.rollback.After = &id
		return nil
	})
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() != 0 || options.expectedDatabase == "" || strings.TrimSpace(options.expectedDatabase) != options.expectedDatabase ||
		options.projectRoot == "" || options.backupFile == "" || options.rollback.MaxRows < 1 ||
		options.rollback.BatchSize < 1 || options.rollback.BatchSize > constUtil.ProfileStorageRollbackBatchMax86 {
		return options, errors.New("missing or invalid required arguments")
	}
	if options.rollback.Apply && !options.appStopped {
		return options, errors.New("apply requires the operator's stopped-application confirmation")
	}
	if len(options.backupSHA256) != 64 || strings.ToLower(options.backupSHA256) != options.backupSHA256 {
		return options, errors.New("backup SHA-256 must be 64 lowercase hexadecimal characters")
	}
	if _, err := hex.DecodeString(options.backupSHA256); err != nil {
		return options, errors.New("backup SHA-256 must be 64 lowercase hexadecimal characters")
	}
	return options, nil
}

// verifyBackup86 binds the run to the declared file and digest. Authentication
// and restoration verification remain the responsibility of the backup process.
func verifyBackup86(rootPath, backupPath, expected string) error {
	root, err := filepath.Abs(rootPath)
	if err != nil {
		return errors.New("invalid project root")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("project root must be a directory")
	}
	marker, err := os.Lstat(filepath.Join(root, ".hyjs-owned"))
	if err != nil || !marker.Mode().IsRegular() {
		return errors.New("project ownership marker is missing")
	}
	backup, err := filepath.Abs(backupPath)
	if err != nil {
		return errors.New("invalid backup path")
	}
	relative, err := filepath.Rel(root, backup)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return errors.New("backup must be inside the project root")
	}
	// Reject links in every path component below the owned directory.
	current := root
	parts := strings.Split(relative, string(os.PathSeparator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (index < len(parts)-1 && !info.IsDir()) ||
			(index == len(parts)-1 && !info.Mode().IsRegular()) {
			return errors.New("backup must be a regular file without symlink components")
		}
	}
	file, err := os.Open(backup)
	if err != nil {
		return errors.New("cannot open backup")
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() == 0 {
		return errors.New("cannot inspect backup")
	}
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return errors.New("cannot hash backup")
	}
	after, err := file.Stat()
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("backup changed while hashing")
	}
	pathInfo, err := os.Lstat(backup)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(after, pathInfo) {
		return errors.New("backup path changed while hashing")
	}
	if hex.EncodeToString(digest.Sum(nil)) != expected {
		return errors.New("backup SHA-256 mismatch")
	}
	return nil
}

func run86(ctx context.Context, args []string, getenv func(string) string, output, help io.Writer) error {
	options, err := parseOptions86(args, help)
	if err != nil {
		return err
	}
	if err = verifyBackup86(options.projectRoot, options.backupFile, options.backupSHA256); err != nil {
		return err
	}
	config := db.MySQLConfig{
		Host:     getenv("HYJS_MYSQL_HOST"),
		User:     getenv("HYJS_MYSQL_USER"),
		Password: getenv("HYJS_MYSQL_PASSWORD"),
		Database: getenv("HYJS_MYSQL_DATABASE"),
	}
	if config.Host == "" || config.User == "" || config.Password == "" || config.Database != options.expectedDatabase {
		return errors.New("explicit MySQL environment does not match expected database")
	}
	database, err := db.OpenGameMySQL(ctx, config, false)
	if err != nil {
		return errors.New("cannot connect to or validate existing MySQL profiles")
	}
	connection, err := database.DB()
	if err != nil {
		return errors.New("cannot inspect MySQL connection")
	}
	defer connection.Close()
	connection.SetMaxOpenConns(1)
	connection.SetMaxIdleConns(1)
	var selected struct {
		Name sql.NullString `gorm:"column:name"`
	}
	if err = database.WithContext(ctx).Raw("SELECT DATABASE() AS name").Scan(&selected).Error; err != nil || !selected.Name.Valid || selected.Name.String != options.expectedDatabase {
		return errors.New("connected database differs from expected database")
	}
	var policy struct {
		Engine string
	}
	if err = database.WithContext(ctx).Raw("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", "client_update_policy").Scan(&policy).Error; err != nil || policy.Engine != "InnoDB" {
		return errors.New("maintenance policy table must use InnoDB")
	}
	encoder := json.NewEncoder(output)
	if err = encoder.Encode(struct {
		Event            string `json:"event"`
		ExpectedDatabase string `json:"expectedDatabase"`
		BackupSHA256     string `json:"backupSHA256"`
		Apply            bool   `json:"apply"`
		AppStoppedClaim  bool   `json:"appStoppedClaim"`
	}{"start", options.expectedDatabase, options.backupSHA256, options.rollback.Apply, options.appStopped}); err != nil {
		return errors.New("cannot write run header")
	}
	summary, err := service.RunProfileStorageRollback86(ctx, database, options.rollback, func(batch service.ProfileStorageRollback86Batch) error {
		return encoder.Encode(struct {
			Event string `json:"event"`
			service.ProfileStorageRollback86Batch
		}{"batch", batch})
	})
	if err != nil {
		_ = encoder.Encode(struct {
			Event string `json:"event"`
			service.ProfileStorageRollback86Summary
		}{"stopped", summary})
		return err
	}
	if err = encoder.Encode(struct {
		Event string `json:"event"`
		service.ProfileStorageRollback86Summary
	}{"summary", summary}); err != nil {
		return errors.New("cannot write final summary")
	}
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run86(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "profile rollback stopped:", err)
		os.Exit(1)
	}
}
