package service

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP10LuckyDropRound2MySQLCapacityAndIsolation(t *testing.T) {
	name := os.Getenv("HYJS_P10_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p10_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p10_") || len(name) == len("hyjs_test_p10_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
	f, b, c := p10LuckySetup(t)
	d := db.GetIns()
	if d.Dialector.Name() != "mysql" {
		t.Fatal("requires MySQL for the concurrent capacity check")
	}
	pool, err := d.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	p10LuckyEnable(t)
	ids := []int64{f.id, b, c}
	for _, id := range ids {
		if err := updateProfile(f.ctx, id, func(player *pb.Player, _ *model.GameState) error {
			player.Level = proto.Int32(70)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		p10LuckyItems(t, f, id, "item_349,1")
	}
	closeA := p10SceneConnect(t, f, f.id, &p10SceneRecorder{})
	p10SceneConnect(t, f, b, &p10SceneRecorder{})
	p10SceneConnect(t, f, c, &p10SceneRecorder{})
	if _, handled, err := f.s.DispatchP10Scene(f.ctx, c, &pb.CSTransferLine{Line: proto.Int32(2)}, 5001); !handled || err != nil {
		t.Fatalf("move actual third player to line two: %v", err)
	}
	for _, check := range []struct {
		id   int64
		line int32
	}{{f.id, 1}, {b, 1}, {c, 2}} {
		var row db.P10ScenePresence
		if err := d.First(&row, "player_id=?", check.id).Error; err != nil || row.Scene != "town_main" || row.Line != check.line {
			t.Fatalf("unexpected fixture scene/line: player=%d line=%d err=%v", check.id, row.Line, err)
		}
	}
	before := make(map[int64]int64)
	for _, id := range ids {
		player, game := p10LuckyProfile(t, id)
		before[id] = simpleOwned(player, game, constUtil.P10LuckyBagItem)
		if before[id] != 1 {
			t.Fatalf("fixture player %d did not receive one real bag", id)
		}
	}
	now := time.Now().Add(-time.Minute)
	rows := make([]db.P10LuckyObject, 0, constUtil.P10LuckyActiveScopeCap-1)
	for i := int64(0); i < constUtil.P10LuckyActiveScopeCap-1; i++ {
		guid := uuid.NewString()
		frame, err := proto.Marshal(&pb.LuckyBag{Guid: proto.String(guid), ItemId: proto.String(constUtil.P10LuckyBagItem), PlayerId: proto.Int64(f.id), CreateTime: proto.Int64(now.UnixMilli()), Pos: &pb.PointNew{X: proto.Float32(1)}})
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, db.P10LuckyObject{GUID: guid, OwnerID: f.id, OwnerName: "fixture", Scene: "town_main", Line: 1,
			CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Hour).UnixMilli(), Frame: frame, Reward: "coin,1", RewardHash: p6Digest([]byte("coin,1"))})
	}
	if err := d.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	type markerKey struct{}
	marker := new(int)
	aCtx := context.WithValue(ctx, markerKey{}, marker)
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	type outcome struct {
		messages []proto.Message
		err      error
	}
	var pendingA, pendingB, pendingC <-chan outcome
	callback := d.Callback().Create()
	const callbackName = "p10_lucky_round2_pause_first_insert"
	registered := false
	defer func() {
		releaseOnce.Do(func() { close(release) })
		cancel()
		for _, pending := range []<-chan outcome{pendingA, pendingB, pendingC} {
			if pending == nil {
				continue
			}
			select {
			case <-pending:
			case <-time.After(3 * time.Second):
				t.Error("pending drop did not exit after cleanup")
			}
		}
		if registered {
			if err := callback.Remove(callbackName); err != nil {
				t.Errorf("remove create callback: %v", err)
			}
		}
	}()
	if err := callback.Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "p10_lucky_objects" || tx.Statement.Context.Value(markerKey{}) != marker {
			return
		}
		enteredOnce.Do(func() {
			close(entered)
			select {
			case <-release:
			case <-tx.Statement.Context.Done():
			}
		})
	}); err != nil {
		t.Fatal(err)
	}
	registered = true
	start := func(callCtx context.Context, id, stamp int64) <-chan outcome {
		result := make(chan outcome, 1)
		go func() {
			messages, err := f.s.p10LuckyAt(callCtx, id, p10LuckyDropRequest(1), stamp, now)
			result <- outcome{messages: messages, err: err}
		}()
		return result
	}
	pendingA = start(aCtx, f.id, 930001)
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first actual throw did not reach its insert")
	}
	pendingB = start(ctx, b, 930002)
	key := p10LuckyDropKey{database: pool, scene: "town_main", line: 1}
	waitForSecond := time.NewTimer(3 * time.Second)
	tick := time.NewTicker(5 * time.Millisecond)
	waiting := false
	for !waiting {
		p10LuckyDropRegistry.Lock()
		entry := p10LuckyDropRegistry.entries[key]
		waiting = entry != nil && entry.refs >= 2
		p10LuckyDropRegistry.Unlock()
		if waiting {
			break
		}
		select {
		case <-tick.C:
		case <-waitForSecond.C:
			tick.Stop()
			t.Fatal("second same-line throw did not reach the held scope")
		case <-ctx.Done():
			tick.Stop()
			t.Fatal("waiting for same-line throw exceeded deadline")
		}
	}
	tick.Stop()
	waitForSecond.Stop()
	select {
	case result := <-pendingB:
		pendingB = nil
		t.Fatalf("same-line throw finished before the first commit: %v", result.err)
	case <-time.After(100 * time.Millisecond):
	}

	pendingC = start(ctx, c, 930003)
	select {
	case result := <-pendingC:
		pendingC = nil
		if result.err != nil || len(result.messages) == 0 {
			t.Fatalf("different-line actual throw blocked or failed: %v", result.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("different-line throw waited for another line")
	case <-ctx.Done():
		t.Fatal("different-line throw exceeded deadline")
	}
	select {
	case result := <-pendingA:
		pendingA = nil
		t.Fatalf("first throw completed before release: %v", result.err)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	var first outcome
	select {
	case first = <-pendingA:
		pendingA = nil
		if first.err != nil || len(first.messages) == 0 {
			t.Fatalf("first actual throw failed: %v", first.err)
		}
	case <-ctx.Done():
		t.Fatal("first throw did not commit")
	}
	select {
	case second := <-pendingB:
		pendingB = nil
		if second.err == nil || !strings.Contains(second.err.Error(), "1000上限") {
			t.Fatalf("last slot did not reject second throw: %v", second.err)
		}
	case <-ctx.Done():
		t.Fatal("second throw did not finish after first commit")
	}
	for _, check := range []struct {
		line int32
		want int64
	}{{1, constUtil.P10LuckyActiveScopeCap}, {2, 1}} {
		var count int64
		if err := d.Model(&db.P10LuckyObject{}).Where("scene=? AND line=? AND claimed_at=0 AND expires_at>?", "town_main", check.line, now.UnixMilli()).Count(&count).Error; err != nil || count != check.want {
			t.Fatalf("line %d active objects=%d want=%d err=%v", check.line, count, check.want, err)
		}
	}
	for _, id := range ids {
		player, game := p10LuckyProfile(t, id)
		want := before[id]
		if id != b {
			want--
		}
		if got := simpleOwned(player, game, constUtil.P10LuckyBagItem); got != want {
			t.Fatalf("player %d inventory=%d want=%d", id, got, want)
		}
	}
	closeA()
	replay, err := f.s.p10LuckyAt(ctx, f.id, p10LuckyDropRequest(1), 930001, now)
	if err != nil || len(replay) != 1 || !proto.Equal(first.messages[0], replay[0]) {
		t.Fatalf("old receipt did not replay after scene exit: %v", err)
	}
	p10LuckyDropRegistry.Lock()
	var remaining int
	for key := range p10LuckyDropRegistry.entries {
		if key.database == pool {
			remaining++
		}
	}
	p10LuckyDropRegistry.Unlock()
	if remaining != 0 {
		t.Fatalf("scope lock registry retained %d completed entries", remaining)
	}
}
