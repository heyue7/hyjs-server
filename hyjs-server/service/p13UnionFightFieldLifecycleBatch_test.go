package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p13LifecycleSQLLog struct {
	logger.Interface
	sync.Mutex
	SQL []string
}

func (l *p13LifecycleSQLLog) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.Lock()
	l.SQL = append(l.SQL, sql)
	l.Unlock()
}

// These are finite database identity/projection fixtures, not enrolled combat
// sources or certified outcomes. Invalid actor PBs deliberately prove that the
// lifecycle only consumes metadata. No fight, inventory or winner is fabricated.
func TestP13UFFieldLifecycleMetadata128FairPoisonAndColdReset(t *testing.T) {
	f, _ := p13GuildSetup(t)
	for _, model := range []any{&db.P13UnionFightFieldPlayer{}, &db.P13UnionFightField{}, &db.P13UnionFightMatch{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Update("management_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Updates(map[string]any{"enrollment_enabled": true, "match_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	scopes := []string{}
	for index := 0; index < 4; index++ {
		scope := p6Digest([]byte(fmt.Sprintf("%s/%d", t.Name(), index)))
		scopes = append(scopes, scope)
		roster := []byte("{}")
		// Non-nil empty previews satisfy the original NOT NULL columns; this
		// metadata-only fixture does not claim a battle preview or authority.
		match := db.P13UnionFightMatch{ID: scope, Day: "2026-10-20", Group: "A", Round: 1, LeftGuildID: fmt.Sprintf("fixture-left-%d", index), RightGuildID: fmt.Sprintf("fixture-right-%d", index), StartAt: 1, EndAt: 10000000, CreatedAt: 1, LeftPreview: []byte{}, RightPreview: []byte{}, Roster: roster, SnapshotHash: p6Digest(roster)}
		if index == 3 {
			match.LeftGuildID = "fixture-left-2"
			match.RightGuildID = "fixture-right-2"
		}
		if err := db.GetIns().Create(&match).Error; err != nil {
			t.Fatal(err)
		}
		snapshot := p13UFFieldSnapshot{KickMillis: 4000, PlayerLimit: 50, PreparationMillis: 300000}
		if index == 2 {
			snapshot.MapProjection = constUtil.P13UnionFightMapProjectionReFlat
			snapshot.LeftSpawn = &pb.PointNew{Y: proto.Float32(0)}
			snapshot.RightSpawn = &pb.PointNew{Y: proto.Float32(0)}
		}
		raw, _ := json.Marshal(snapshot)
		hash := p6Digest(raw)
		if index == 0 {
			hash = "damaged-first-scope"
		}
		if err := db.GetIns().Create(&db.P13UnionFightField{MatchID: scope, Snapshot: raw, SnapshotHash: hash}).Error; err != nil {
			t.Fatal(err)
		}
	}
	game := `{"position":{"scene":"fight_union"},"position_loading":false}`
	profiles := make([]db.Profile, 150)
	for index := range profiles {
		profiles[index] = db.Profile{Uid: fmt.Sprintf("lifecycle-meta-%s-%d", scopes[0][:12], index), Session: []byte("identity-only-fixture"), Game: &game}
	}
	if err := db.GetIns().CreateInBatches(&profiles, 50).Error; err != nil {
		t.Fatal(err)
	}
	actors := make([]db.P13UnionFightFieldPlayer, 150)
	members := make([]db.P13GuildMember, 150)
	for index, profile := range profiles {
		scopeIndex := index / 50
		camp := int64(0)
		guild := fmt.Sprintf("fixture-left-%d", scopeIndex)
		if index%50 >= 25 {
			camp = 1
			guild = fmt.Sprintf("fixture-right-%d", scopeIndex)
		}
		projection := ""
		if scopeIndex == 2 {
			projection = constUtil.P13UnionFightMapProjectionReFlat
		}
		if index == 50 {
			projection = "invalid-row-projection"
		}
		actors[index] = db.P13UnionFightFieldPlayer{MatchID: scopes[scopeIndex], PlayerID: profile.Id, GuildID: guild, Camp: camp, MapProjection: projection, Frame: []byte("not a source frame"), FrameHash: "not a frame proof", Battle: []byte("not a battle source"), BattleHash: "not a battle proof", Source: "metadata projection fixture only"}
		members[index] = db.P13GuildMember{PlayerID: profile.Id, GuildID: guild}
	}
	if err := db.GetIns().CreateInBatches(&actors, 50).Error; err != nil {
		t.Fatal(err)
	}
	// An identity-only tie at the128th boundary exercises the second key.
	// This deliberately does not claim two authoritative/current combat entries.
	tie := actors[127]
	tie.MatchID = scopes[3]
	tie.MapProjection = ""
	tie.Source = "composite metadata key fixture only"
	if err := db.GetIns().Create(&tie).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().CreateInBatches(&members, 50).Error; err != nil {
		t.Fatal(err)
	}
	p13UFFieldLifecycleCursor.Lock()
	p13UFFieldLifecycleCursor.DB = nil
	p13UFFieldLifecycleCursor.Unlock()
	database := db.GetIns()
	originalLogger := database.Config.Logger
	recorder := &p13LifecycleSQLLog{Interface: originalLogger}
	database.Config.Logger = recorder
	defer func() { database.Config.Logger = originalLogger }()
	err := f.s.p13UFFieldLifecycle(f.ctx, time.UnixMilli(1000))
	if err == nil || !strings.Contains(err.Error(), "快照损坏") || !strings.Contains(err.Error(), "投影") {
		t.Fatal("poison scope/row missing from combined errors", err)
	}
	p13UFFieldLifecycleCursor.Lock()
	last := p13UFFieldLifecycleCursor.PlayerID
	boundaryScope := p13UFFieldLifecycleCursor.MatchID
	p13UFFieldLifecycleCursor.Unlock()
	expectedScope := scopes[2]
	if scopes[3] < expectedScope {
		expectedScope = scopes[3]
	}
	if boundaryScope != expectedScope {
		t.Fatal("composite boundary cursor omitted match ID", boundaryScope, expectedScope)
	}
	if last != profiles[127].Id {
		t.Fatal("first tick was not one128 metadata page", last)
	}
	recorder.Lock()
	firstSQL := append([]string(nil), recorder.SQL...)
	recorder.Unlock()
	normalPushes := 0
	for _, statement := range firstSQL {
		sql := strings.ToLower(strings.ReplaceAll(statement, "`", ""))
		if strings.Contains(sql, "from p13_union_fight_field_players") && strings.HasPrefix(sql, "select") {
			selectPart := strings.SplitN(sql, " from ", 2)[0]
			if strings.Contains(selectPart, "*") || strings.Contains(selectPart, "frame") || strings.Contains(selectPart, "battle") {
				t.Fatal("lifecycle loaded actor PB", statement)
			}
		}
		if strings.Contains(sql, "from p13_union_fight_settlement_results") && strings.Contains(sql, "result_kind") && !strings.Contains(sql, " in (") {
			normalPushes++
		}
	}
	if normalPushes == 0 {
		t.Fatal("bad first scope starved valid peers in same page")
	}
	if err = f.s.p13UFFieldLifecycle(f.ctx, time.UnixMilli(1000)); err != nil {
		t.Fatal("later valid flat page blocked", err)
	}
	recorder.Lock()
	secondSQL := append([]string(nil), recorder.SQL[len(firstSQL):]...)
	recorder.Unlock()
	secondPushes := 0
	for _, statement := range secondSQL {
		sql := strings.ToLower(strings.ReplaceAll(statement, "`", ""))
		if strings.Contains(sql, "from p13_union_fight_settlement_results") && strings.Contains(sql, "result_kind") && !strings.Contains(sql, " in (") {
			secondPushes++
		}
	}
	if secondPushes != 23 {
		t.Fatal("same-player match-key peer skipped across page boundary", secondPushes)
	}
	p13UFFieldLifecycleCursor.Lock()
	reset := p13UFFieldLifecycleCursor.PlayerID == 0 && p13UFFieldLifecycleCursor.MatchID == ""
	p13UFFieldLifecycleCursor.Unlock()
	if !reset {
		t.Fatal("short final page did not reset next tick")
	}
	page, err := p13UFFieldLifecycleCandidates(f.ctx)
	if err != nil || len(page) != 128 || page[0].PlayerID != profiles[0].Id {
		t.Fatal("fair next pass", len(page), err)
	}
	// A forced end-of-index makes one empty query; wrapping happens next tick.
	p13UFFieldLifecycleCursor.Lock()
	p13UFFieldLifecycleCursor.PlayerID = profiles[149].Id + 1
	p13UFFieldLifecycleCursor.MatchID = "z"
	p13UFFieldLifecycleCursor.Unlock()
	page, err = p13UFFieldLifecycleCandidates(f.ctx)
	if err != nil || len(page) != 0 {
		t.Fatal("wrapped and scanned twice within one tick", len(page), err)
	}
	page, err = p13UFFieldLifecycleCandidates(f.ctx)
	if err != nil || len(page) != 128 {
		t.Fatal("empty page did not reset next tick", len(page), err)
	}
	// Reopening the DB handle, without running business initialization on the
	// deliberately invalid actor PB fixtures, resets cursor ownership safely.
	sqlDB, _ := database.DB()
	if err = sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	page, err = p13UFFieldLifecycleCandidates(f.ctx)
	if err != nil || len(page) != 128 || page[0].PlayerID != profiles[0].Id {
		t.Fatal("new database pointer kept old cursor", len(page), err)
	}
}

func TestP13UFFieldLifecycleTerminalPoisonIsolationIsBounded(t *testing.T) {
	scopes := make([]string, 128)
	for i := range scopes {
		scopes[i] = fmt.Sprintf("scope-%03d", i)
	}
	calls := 0
	values, broken, err := p13UFFieldLifecycleTerminals(context.Background(), scopes, func(ids []string) (map[string]int64, error) {
		calls++
		for _, id := range ids {
			if id == scopes[0] {
				return nil, p13UFMixedError()
			}
		}
		out := map[string]int64{}
		for _, id := range ids {
			out[id] = 123
		}
		return out, nil
	})
	if err != nil || len(broken) != 1 || len(values) != 127 || calls > 255 {
		t.Fatal(len(values), len(broken), calls, err)
	}
	calls = 0
	_, broken, err = p13UFFieldLifecycleTerminals(context.Background(), scopes, func(ids []string) (map[string]int64, error) { calls++; return nil, p13UFMixedError() })
	if err != nil || len(broken) != 128 || calls != 255 {
		t.Fatal("bounded worst-case tree", len(broken), calls, err)
	}
	calls = 0
	driverErr := errors.New("driver connection unavailable")
	_, _, err = p13UFFieldLifecycleTerminals(context.Background(), scopes, func(ids []string) (map[string]int64, error) { calls++; return nil, driverErr })
	if !errors.Is(err, driverErr) || calls != 1 {
		t.Fatal("DB failure fanned out as business corruption", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	_, _, err = p13UFFieldLifecycleTerminals(ctx, scopes, func(ids []string) (map[string]int64, error) { calls++; return nil, nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled context continued queries", calls, err)
	}
}
