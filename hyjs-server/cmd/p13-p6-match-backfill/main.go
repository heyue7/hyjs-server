// Explicit resumable maintenance; never invoked by startup or a request.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"hyjs-server/db"
	"hyjs-server/service"
)

type checkpoint struct{ DatabaseSHA, Cursor string }

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	apply := flag.Bool("apply", false, "explicitly perform verified scope backfill")
	path := flag.String("checkpoint", "", "private checkpoint path, required")
	batch := flag.Int("batch", 4, "full input rows per bounded batch (1..8)")
	maxBatches := flag.Int("max-batches", 1, "bounded batches this invocation (1..1000)")
	flag.Parse()
	if !*apply || *path == "" || *batch < 1 || *batch > 8 || *maxBatches < 1 || *maxBatches > 1000 {
		return fmt.Errorf("require --apply --checkpoint PATH, --batch 1..8, --max-batches 1..1000")
	}
	cfg := db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: os.Getenv("HYJS_MYSQL_DATABASE")}
	binding := sha256.Sum256([]byte(cfg.Host + "\x00" + cfg.Database))
	state := checkpoint{DatabaseSHA: hex.EncodeToString(binding[:])}
	if raw, e := os.ReadFile(*path); e == nil {
		var old checkpoint
		if json.Unmarshal(raw, &old) != nil || old.DatabaseSHA != state.DatabaseSHA {
			return fmt.Errorf("checkpoint database binding mismatch")
		}
		state = old
	} else if !os.IsNotExist(e) {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := db.InitExistingGameMySQL(ctx, cfg); e != nil {
		return fmt.Errorf("connect existing database failed: %w", e)
	}
	sql, e := db.GetIns().DB()
	if e != nil {
		return e
	}
	defer sql.Close()
	for i := 0; i < *maxBatches; i++ {
		cursor, count, workErr := service.BackfillP13P6MatchScopeBatch(ctx, state.Cursor, *batch)
		state.Cursor = cursor
		raw, _ := json.Marshal(state)
		temporary := *path + ".pending"
		if e := os.WriteFile(temporary, raw, 0600); e != nil {
			return e
		}
		if e := os.Rename(temporary, *path); e != nil {
			return e
		}
		fmt.Printf("verified=%d cursor=%s\n", count, cursor)
		if workErr != nil {
			return workErr
		}
		if count < *batch {
			return nil
		}
	}
	return nil
}
