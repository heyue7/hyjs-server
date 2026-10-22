// Explicit maintenance for the HYJS schema. No schema migrations or background work.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/service"
)

func main() {
	apply := flag.Bool("apply", false, "compact legacy replay inputs and remove expired unaccepted match publications")
	flag.Parse()
	database := os.Getenv("HYJS_MYSQL_DATABASE")
	if database != "hyjs" && !strings.HasPrefix(database, "hyjs_test_") {
		fail("only the HYJS project schema is allowed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cfg := db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: database}
	if err := db.InitExistingGameMySQL(ctx, cfg); err != nil {
		fail("cannot connect to the existing HYJS schema")
	}
	connection, err := db.GetIns().DB()
	if err != nil {
		fail("cannot acquire HYJS connection")
	}
	defer connection.Close()
	connection.SetMaxOpenConns(1)
	connection.SetMaxIdleConns(1)
	s := &service.GameService{}
	cutoff := time.Now()
	cleanup, err := s.CleanupUnusedExpiredMatches(ctx, cutoff, *apply)
	if err != nil {
		fail("expired publication cleanup failed")
	}
	var compaction *model.P6StorageCompactionResult
	if *apply {
		compaction, err = s.CompactLegacyP6BattleInputs(ctx)
		if err != nil {
			fail("legacy input compaction failed")
		}
	}
	report := struct {
		Database   string                           `json:"database"`
		Apply      bool                             `json:"apply"`
		Cutoff     int64                            `json:"cutoff"`
		Expired    []model.ExpiredMatchCleanup      `json:"expired"`
		Compaction *model.P6StorageCompactionResult `json:"compaction,omitempty"`
	}{database, *apply, cutoff.UnixMilli(), cleanup, compaction}
	if err = json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fail("cannot write maintenance result")
	}
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
