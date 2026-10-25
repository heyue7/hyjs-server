package service

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm/logger"
	"hyjs-server/db"
)

// Record statement counts without retaining bound credentials or complete saves.
type p5AwakeSQLTrace struct {
	logger.Interface
	count, joins, metadata int
}

var p5AwakeJoinWord = regexp.MustCompile(`(?i)\bJOIN\b`)

func (r *p5AwakeSQLTrace) Trace(_ context.Context, _ time.Time, statement func() (string, int64), _ error) {
	sql, _ := statement()
	r.count++
	if count := len(p5AwakeJoinWord.FindAllStringIndex(sql, -1)); count > r.joins {
		r.joins = count
	}
	if regexp.MustCompile(`(?i)information_schema`).MatchString(sql) {
		r.metadata++
	}
}

var p5AwakePerformanceWrite sync.Mutex

func p5MeasureAwake(t *testing.T, name string, action func() ([]proto.Message, error)) ([]proto.Message, error) {
	t.Helper()
	output := os.Getenv("HYJS_AWAKE_SQL_OUTPUT")
	if output == "" {
		return action()
	}
	previous := db.GetIns().Config.Logger
	trace := &p5AwakeSQLTrace{Interface: previous}
	db.GetIns().Config.Logger = trace
	started := time.Now()
	out, err := action()
	elapsed := time.Since(started)
	db.GetIns().Config.Logger = previous
	p5AwakePerformanceWrite.Lock()
	defer p5AwakePerformanceWrite.Unlock()
	f, openErr := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer f.Close()
	if encodeErr := json.NewEncoder(f).Encode(map[string]any{"case": t.Name(), "operation": name,
		"milliseconds": float64(elapsed.Microseconds()) / 1000, "sql": trace.count,
		"maxJoins": trace.joins, "metadataQueries": trace.metadata, "success": err == nil}); encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if trace.joins > 3 {
		t.Errorf("single statement exceeded 3 JOINs: %d", trace.joins)
	}
	return out, err
}
