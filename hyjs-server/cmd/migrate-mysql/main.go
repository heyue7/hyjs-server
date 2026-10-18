package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"hyjs-server/db"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

// main 默认仅检查；--apply 执行保守导入，密码只接受环境变量。
func main() {
	source := flag.String("source", "", "read-only source SQLite file")
	apply := flag.Bool("apply", false, "insert identical-or-missing records and preserve sequence")
	host := flag.String("host", env("HYJS_MYSQL_HOST", "127.0.0.1:33068"), "MySQL host:port")
	user := flag.String("user", env("HYJS_MYSQL_USER", "hyjs"), "MySQL user")
	database := flag.String("database", env("HYJS_MYSQL_DATABASE", "hyjs"), "MySQL database")
	flag.Parse()
	if *source == "" {
		fmt.Fprintln(os.Stderr, "--source is required")
		os.Exit(2)
	}
	path, err := filepath.Abs(*source)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	report, err := db.MigrateSQLiteToMySQL(ctx, path, db.MySQLConfig{Host: *host, User: *user, Database: *database, Password: os.Getenv("HYJS_MYSQL_PASSWORD")}, *apply)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(report)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
