package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func latency62Fixture(t *testing.T) *p1Fixture {
	t.Helper()
	name := os.Getenv("HYJS_LATENCY62_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_latency62_") {
		return p1Setup(t)
	}
	f := &p1Fixture{ctx: context.Background(), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_LATENCY62_HOST"), User: os.Getenv("HYJS_LATENCY62_USER"), Password: os.Getenv("HYJS_LATENCY62_PASSWORD"), Database: name}, s: &GameService{}}
	f.open(t)
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP1(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Fixture session is isolated and explicit; no credential/login behavior is under test.
	created := time.Now().UnixMilli()
	owner := db.Profile{Uid: "latency62-" + t.Name() + fmt.Sprint(time.Now().UnixNano()), Session: []byte("latency62session"), CreatedAt: &created, ServerID: 1}
	if err := db.GetIns().Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(f.ctx, owner.Id, "红点合批回归", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = owner.Id
	t.Cleanup(func() { connection, _ := db.GetIns().DB(); connection.Close() })
	return f
}

func TestRedPointLatency62ExactAndFallback(t *testing.T) {
	f := latency62Fixture(t)
	reward := f.reward(t, "latency62:reward", "coin,1", 0)
	first, err := f.s.InventoryMailRedPoints(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.s.WelfareAchievementRedPoints(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	f.s.InitializeRedPointSchemaReady()
	points, err := f.s.requestRedPoints(f.ctx, f.id, true, true)
	if err != nil || len(points) != 2 || !proto.Equal(first, points[0]) || !proto.Equal(second, points[1]) {
		t.Fatal("merged projection changed", err)
	}
	var owner db.Profile
	if err = db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s, f.id}
	var sent []proto.Message
	p10ClientSinks.Store(key, &p10ClientSink{session: owner.Session, send: func(m proto.Message) error { sent = append(sent, m); return nil }})
	defer p10ClientSinks.Delete(key)
	response := []proto.Message{&pb.BaseResource{Id: proto.String("coin"), Num: proto.Int32(1)}}
	f.s.afterRedPointRequest(f.ctx, f.id, &pb.CSItemUse{}, response)
	if len(sent) != 2 || !proto.Equal(sent[0], first) || !proto.Equal(sent[1], second) {
		t.Fatal("post-commit frame order")
	}
	if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Mailbox(f.ctx, f.id, &pb.CSMailList{}); err != nil {
		t.Fatal(err)
	}
	cleared, err := f.s.requestRedPoints(f.ctx, f.id, true, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range cleared[0].(*pb.SCRedPoint).GetRedPointList() {
		if point.GetValue() != 0 {
			t.Fatal("last claim failed to clear", point)
		}
	}
	first = cleared[0].(*pb.SCRedPoint)
	// Missing welfare state table makes only that group fail. Inventory must still refresh.
	if err = db.GetIns().Migrator().DropTable(&db.PeriodProgress{}); err != nil {
		t.Fatal(err)
	}
	// An active period guarantees the missing table is consumed.
	p1Instance(t, f, fmt.Sprintf("latency62_active_%d", f.id), "copyWin", "cumulative", 1, time.Now().Add(time.Hour), 0)
	sent = nil
	f.s.afterRedPointRequest(f.ctx, f.id, &pb.CSItemUse{}, response)
	if len(sent) != 1 || !proto.Equal(sent[0], first) {
		t.Fatal("one failed group hid independent inventory", len(sent))
	}
	p10ClientSinks.Delete(key)
	// No sink remains harmless and still follows the synchronous computation contract.
	f.s.afterRedPointRequest(f.ctx, f.id, &pb.CSItemUse{}, response)
}

func TestRedPointLatency62SelectionAndBatchAuthorization(t *testing.T) {
	if inventoryMailRequestRefresh(nil, nil) || welfareAchievementRequestRefresh(nil, nil) || inventoryMailRequestRefresh(&pb.CSPingTcp{}, nil) || welfareAchievementRequestRefresh(&pb.CSPingTcp{}, nil) {
		t.Fatal("empty transport selected")
	}
	if !inventoryMailRequestRefresh(&pb.CSMailList{}, nil) || welfareAchievementRequestRefresh(&pb.CSMailList{}, nil) || !welfareAchievementRequestRefresh(&pb.CSAchievementList{}, nil) {
		t.Fatal("explicit selection changed")
	}
	f := latency62Fixture(t)
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s, f.id}
	defer p10ClientSinks.Delete(key)
	points := []proto.Message{&pb.SCRedPoint{}, &pb.SCRedPoint{}}
	calls := 0
	sink := &p10ClientSink{session: owner.Session, send: func(proto.Message) error { calls++; return nil }}
	p10ClientSinks.Store(key, sink)
	f.s.publishP10PrivateBatch(f.ctx, f.id, points)
	if calls != 2 {
		t.Fatal("batch not synchronous")
	}
	if err := db.GetIns().Model(&owner).Update("session", []byte("rotated-session!")).Error; err != nil {
		t.Fatal(err)
	}
	f.s.publishP10PrivateBatch(f.ctx, f.id, points)
	if calls != 2 {
		t.Fatal("rotated session received batch")
	}
	// A current profile session still loses push rights when its account is disabled or revised.
	if err := db.GetIns().AutoMigrate(&db.GameAccount{}); err != nil {
		t.Fatal(err)
	}
	account := db.GameAccount{Username: fmt.Sprintf("latency62-authorization-%d", f.id), PasswordSalt: []byte{}, PasswordHash: []byte{}, Enabled: true, AuthRevision: 1}
	if err := db.GetIns().Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&owner).Updates(map[string]any{"session": owner.Session, "account_id": account.ID, "account_auth_revision": int64(1)}).Error; err != nil {
		t.Fatal(err)
	}
	sink.session = owner.Session
	for _, change := range []map[string]any{{"enabled": false}, {"enabled": true, "auth_revision": int64(2)}} {
		if err := db.GetIns().Model(&account).Updates(change).Error; err != nil {
			t.Fatal(err)
		}
		p10ClientSinks.Store(key, sink)
		f.s.publishP10PrivateBatch(f.ctx, f.id, points)
		if calls != 2 {
			t.Fatal("disabled/revised account received batch")
		}
	}
	if err := db.GetIns().Model(&account).Updates(map[string]any{"enabled": true, "auth_revision": int64(1)}).Error; err != nil {
		t.Fatal(err)
	}
	// Send failure preserves the previous stop/cleanup behavior.
	sink.session = owner.Session
	calls = 0
	sink.send = func(proto.Message) error { calls++; return errors.New("closed fixture sink") }
	p10ClientSinks.Store(key, sink)
	f.s.publishP10PrivateBatch(f.ctx, f.id, points)
	if calls != 1 {
		t.Fatal("send failure continued batch")
	}
}

type latency62Pool struct {
	*sql.DB
	begins int
}

func (p *latency62Pool) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	p.begins++
	return p.DB.BeginTx(ctx, opts)
}

func TestRedPointLatency62DispatchMySQL(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("HYJS_LATENCY62_DATABASE"), "hyjs_test_latency62_") {
		t.Skip("owned MySQL allocation required")
	}
	f := latency62Fixture(t)
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	// Repeated claiming of an already committed reward is a stable, valid original request.
	reward := f.reward(t, "latency62:performance", "coin,1", 0)
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}}); err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s, f.id}
	defer p10ClientSinks.Delete(key)
	var sent []proto.Message
	p10ClientSinks.Store(key, &p10ClientSink{session: owner.Session, send: func(m proto.Message) error { sent = append(sent, m); return nil }})
	request := &model.GamePacket{Opcode: 20104, PlayerId: f.id, ZoneId: 1, Session: owner.Session, Timestamp: 123456789, Body: &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}}}
	if os.Getenv("HYJS_LATENCY62_SCHEMA_READY") != "0" {
		f.s.InitializeRedPointSchemaReady()
	}
	// Instrument actual driver BEGIN separately from Gorm's query trace.
	d := db.GetIns()
	connection, _ := d.DB()
	pool := &latency62Pool{DB: connection}
	previousPool, previousStatementPool := d.ConnPool, d.Statement.ConnPool
	d.ConnPool = pool
	d.Statement.ConnPool = pool
	defer func() { d.ConnPool = previousPool; d.Statement.ConnPool = previousStatementPool }()
	previousLogger := d.Config.Logger
	recorder := &adminActivityQueryRecorder{Interface: previousLogger}
	d.Config.Logger = recorder
	defer func() { d.Config.Logger = previousLogger }()
	var timings []time.Duration
	for n := 0; n < 51; n++ {
		recorder.reset()
		pool.begins = 0
		sent = nil
		start := time.Now()
		out := f.s.Dispatch(f.ctx, request, "127.0.0.1")
		elapsed := time.Since(start)
		for _, m := range out {
			if e, ok := m.(*pb.ErrorMessage); ok {
				t.Fatal("Dispatch fixture", e.GetMsg())
			}
		}
		if n == 0 {
			t.Logf("Dispatch queries=%d transactions=%d profileReads=%d metadata=%d frames=%d", len(recorder.sqls), pool.begins, recorder.countContaining("SELECT * FROM `profiles`"), recorder.countContaining("information_schema"), len(sent))
		} else {
			timings = append(timings, elapsed)
		}
	}
	sort.Slice(timings, func(i, j int) bool { return timings[i] < timings[j] })
	t.Logf("actual MySQL same-input Dispatch n=50 p50=%s p95=%s", timings[24], timings[47])
}
