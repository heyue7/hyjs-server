package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Preserve the previous per-kind projection as an independent comparison.
func (s *GameService) dailyCopyStateLegacyRound2(tx *gorm.DB, player *pb.Player, now time.Time) (*pb.SCDailyCopyState, error) {
	// 星期开放与剩余次数跟随个人游玩日，金融及战斗时钟保持真实时间。
	now = LocalPlayAt(tx.Statement.Context, now)
	policy, err := loadDailyCopyPolicy(tx)
	if err != nil {
		return nil, err
	}
	var active db.DailyCopyFight
	activeErr := tx.Where("player_id = ? AND day = ? AND settled = ?", player.GetPlayerId(), dailyCopyDay(now), false).First(&active).Error
	if activeErr != nil && !errors.Is(activeErr, gorm.ErrRecordNotFound) {
		return nil, activeErr
	}
	result := &pb.SCDailyCopyState{}
	for _, rule := range s.Tables["DailyCopy"] {
		open, _, limit, err := dailyCopyTypeRule(rule, now)
		if err != nil {
			return nil, err
		}
		state, err := loadDailyCopyState(tx, player.GetPlayerId(), rule["Type"], dailyCopyDay(now), limit)
		if err != nil {
			return nil, err
		}
		item := &pb.DailyCopyStateType{Type: proto.String(rule["Type"]), IsOpen: proto.Int32(0), LastTime: proto.Int32(state.Limit - state.Used)}
		if policy.Enabled && open {
			item.IsOpen = proto.Int32(1)
		}
		// 原按钮要求lastTime>0；已扣费未结算的挑战投影为一次续接资格，数据库次数保持已使用。
		if activeErr == nil {
			item.IsOpen = proto.Int32(0)
			if active.Type == rule["Type"] {
				item.IsOpen = proto.Int32(1)
				item.LastTime = proto.Int32(max(item.GetLastTime(), 1))
			}
		}
		for _, chapter := range s.Tables["DailyCopyChapter"] {
			if chapter["Type"] != rule["Type"] {
				continue
			}
			level, err := dailyCopyPositive(chapter["LevelLimit"])
			if err != nil {
				return nil, err
			}
			recommend, err := dailyCopyPositive(chapter["Recommend"])
			if err != nil {
				return nil, err
			}
			entry := &pb.DailyCopyState{ChapterId: proto.String(chapter["ChapterId"]), LevelLimit: proto.Int32(level), Recommend: proto.Int32(recommend), Drop: proto.String(chapter["Drop"]), IsOpen: proto.Int32(0)}
			if item.GetIsOpen() == 1 && player.GetLevel() >= level {
				entry.IsOpen = proto.Int32(1)
			}
			if activeErr == nil {
				entry.IsOpen = proto.Int32(0)
				if active.ChapterID == chapter["ChapterId"] {
					entry.IsOpen = proto.Int32(1)
				}
			}
			item.DailyCopyState = append(item.DailyCopyState, entry)
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

type dailyCopyStateRound2Trace struct {
	logger.Interface
	statements []string
}

func (trace *dailyCopyStateRound2Trace) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	statement, _ := query()
	trace.statements = append(trace.statements, strings.ToLower(statement))
}

func dailyCopyStateRound2Projection(t *testing.T, statement string, expected ...string) {
	t.Helper()
	clean := strings.NewReplacer("`", "", "\"", "", "[", "", "]", "").Replace(statement)
	selectPart, _, found := strings.Cut(clean, " from ")
	if !found || !strings.HasPrefix(strings.TrimSpace(selectPart), "select ") {
		t.Fatalf("unexpected SELECT: %s", statement)
	}
	fields := strings.Split(strings.TrimPrefix(strings.TrimSpace(selectPart), "select "), ",")
	want := make(map[string]bool, len(expected))
	for _, field := range expected {
		want[field] = true
	}
	if len(fields) != len(want) {
		t.Fatalf("unexpected projection: %s", selectPart)
	}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if dot := strings.LastIndex(field, "."); dot >= 0 {
			field = field[dot+1:]
		}
		if !want[field] {
			t.Fatalf("unexpected projection field %q: %s", field, selectPart)
		}
		delete(want, field)
	}
	if len(want) != 0 {
		t.Fatalf("missing projection fields: %v", want)
	}
}

func TestDailyCopyStateRound2SQLite(t *testing.T) {
	s, ctx, _ := newDailyCopyTest(t)
	d := db.GetIns()
	if d.Dialector.Name() != "sqlite" {
		t.Fatal("fixture must use isolated SQLite")
	}
	id := dailyCopyTestPlayer(t, ctx, "daily-state-round2", 75)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	day := dailyCopyDay(now)
	// A frozen limit may differ from today's configuration; an unrelated kind
	// remains ignored even if its stored quota is invalid.
	for _, state := range []db.DailyCopyState{
		{PlayerID: id, Day: day, Type: "HeroExp", Used: 1, Limit: 2},
		{PlayerID: id, Day: day, Type: "unconfigured", Used: 0, Limit: 0},
		{PlayerID: id, Day: dailyCopyDay(now.AddDate(0, 0, -1)), Type: "HeroExp", Used: 0, Limit: 99},
	} {
		if err := d.Create(&state).Error; err != nil {
			t.Fatal(err)
		}
	}
	var profile db.Profile
	if err := d.First(&profile, id).Error; err != nil || profile.Player == nil {
		t.Fatalf("missing fixture player: %v", err)
	}
	originalPlayer := *profile.Player
	var player pb.Player
	if err := json.Unmarshal([]byte(originalPlayer), &player); err != nil {
		t.Fatal(err)
	}
	compare := func(at context.Context, atTime time.Time) *pb.SCDailyCopyState {
		t.Helper()
		want, err := s.dailyCopyStateLegacyRound2(d.WithContext(at), &player, atTime)
		if err != nil {
			t.Fatal(err)
		}
		messages, err := s.dailyCopyAt(at, id, &pb.CSDailyCopyState{}, 1, atTime)
		if err != nil || len(messages) != 1 {
			t.Fatalf("state response: count=%d err=%v", len(messages), err)
		}
		got, ok := messages[0].(*pb.SCDailyCopyState)
		if !ok || !proto.Equal(got, want) {
			t.Fatal("state projection differs from previous implementation")
		}
		return got
	}
	// Compare original weekday and level projections before an active fight.
	compare(ctx, now.AddDate(0, 0, -1))
	compare(ctx, now)
	if err := d.Model(&db.DailyCopyPolicy{}).Where("id=?", constUtil.DailyCopyPolicyID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range compare(ctx, now).GetItems() {
		if item.GetIsOpen() != 0 {
			t.Fatal("disabled policy opened a new copy")
		}
	}
	if err := d.Model(&db.DailyCopyPolicy{}).Where("id=?", constUtil.DailyCopyPolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}

	started, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}, 101, now)
	if err != nil || len(started) == 0 {
		t.Fatalf("start actual unfinished copy: %v", err)
	}
	if _, ok := started[0].(*pb.SCDailyCopyFightStart); !ok {
		t.Fatal("actual unfinished copy did not return native start")
	}
	if err := s.InitializeServerGameClock(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.First(&profile, id).Error; err != nil || profile.Player == nil {
		t.Fatalf("missing post-start player: %v", err)
	}
	originalPlayer = *profile.Player
	player = pb.Player{}
	if err := json.Unmarshal([]byte(originalPlayer), &player); err != nil {
		t.Fatal(err)
	}
	bound, err := s.BindServerGameClockContext(ctx, &profile)
	if err != nil || !ServerGameClockContextBound(bound) {
		t.Fatalf("bind clock: %v", err)
	}
	want, err := s.dailyCopyStateLegacyRound2(d.WithContext(bound), &player, now)
	if err != nil {
		t.Fatal(err)
	}
	queryCallback := d.Callback().Query()
	const callbackName = "daily_copy_state_round2_lock"
	var updateLocks int
	var clockShared bool
	collecting := true
	if err := queryCallback.Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if !collecting {
			return
		}
		cl, ok := tx.Statement.Clauses["FOR"]
		if !ok {
			return
		}
		var strength string
		switch lock := cl.Expression.(type) {
		case clause.Locking:
			strength = lock.Strength
		case *clause.Locking:
			strength = lock.Strength
		}
		if strings.EqualFold(strength, "UPDATE") {
			updateLocks++
		}
		if tx.Statement.Table == "server_game_clocks" && strings.EqualFold(strength, "SHARE") {
			clockShared = true
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = queryCallback.Remove(callbackName) })
	previous := d.Config.Logger
	trace := &dailyCopyStateRound2Trace{Interface: previous}
	d.Config.Logger = trace
	defer func() { d.Config.Logger = previous }()
	messages, err := s.dailyCopyAt(bound, id, &pb.CSDailyCopyState{}, 102, now)
	d.Config.Logger = previous
	collecting = false
	if err != nil || len(messages) != 1 {
		t.Fatalf("state response: count=%d err=%v", len(messages), err)
	}
	got, ok := messages[0].(*pb.SCDailyCopyState)
	if !ok || !proto.Equal(got, want) {
		t.Fatal("active state projection differs from previous implementation")
	}
	if updateLocks != 0 || !clockShared {
		t.Fatalf("clock/profile locking differs: update=%d clockShare=%t", updateLocks, clockShared)
	}
	var profileQueries, stateQueries, fightQueries, clockQueries int
	for _, statement := range trace.statements {
		query := strings.NewReplacer("`", "", "\"", "", "[", "", "]", "").Replace(strings.TrimSpace(statement))
		if strings.HasPrefix(query, "update ") || strings.HasPrefix(query, "insert ") || strings.HasPrefix(query, "delete ") || strings.HasPrefix(query, "replace ") || strings.Contains(query, " for update") {
			t.Fatalf("state request executed a write or update lock: %s", query)
		}
		switch {
		case strings.Contains(query, " from profiles"):
			profileQueries++
			dailyCopyStateRound2Projection(t, query, "id", "player")
		case strings.Contains(query, " from daily_copy_states"):
			stateQueries++
		case strings.Contains(query, " from daily_copy_fights"):
			fightQueries++
			dailyCopyStateRound2Projection(t, query, "type", "chapter_id")
		case strings.Contains(query, " from server_game_clocks"):
			clockQueries++
		}
	}
	if profileQueries != 1 || stateQueries != 1 || fightQueries != 1 || clockQueries != 1 {
		t.Fatalf("unexpected state query counts: profile=%d states=%d fights=%d clock=%d", profileQueries, stateQueries, fightQueries, clockQueries)
	}
	var activeType bool
	for _, item := range got.GetItems() {
		if item.GetType() != "TreasureExp" {
			if item.GetIsOpen() != 0 {
				t.Fatal("unrelated type opened during unfinished battle")
			}
			continue
		}
		activeType = true
		if item.GetIsOpen() != 1 || item.GetLastTime() != 1 {
			t.Fatal("exhausted active fight was not projected as resumable")
		}
		for _, chapter := range item.GetDailyCopyState() {
			if (chapter.GetIsOpen() == 1) != (chapter.GetChapterId() == "5") {
				t.Fatal("active fight resumed a different chapter")
			}
		}
	}
	if !activeType {
		t.Fatal("missing active copy type")
	}

	if err := d.Model(&db.DailyCopyPolicy{}).Where("id=?", constUtil.DailyCopyPolicyID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	compare(bound, now)
	for _, invalid := range []struct{ limit, used int32 }{{0, 0}, {1, -1}, {1, 2}} {
		if err := d.Model(&db.DailyCopyState{}).Where("player_id=? AND day=? AND type=?", id, day, "HeroExp").Updates(map[string]any{"limit": invalid.limit, "used": invalid.used}).Error; err != nil {
			t.Fatal(err)
		}
		_, oldErr := s.dailyCopyStateLegacyRound2(d.WithContext(bound), &player, now)
		_, newErr := s.dailyCopyAt(bound, id, &pb.CSDailyCopyState{}, 103, now)
		if oldErr == nil || newErr == nil || oldErr.Error() != newErr.Error() {
			t.Fatalf("invalid stored quota changed behavior: old=%v new=%v", oldErr, newErr)
		}
	}
	if err := d.Model(&db.DailyCopyState{}).Where("player_id=? AND day=? AND type=?", id, day, "HeroExp").Updates(map[string]any{"limit": 2, "used": 1}).Error; err != nil {
		t.Fatal(err)
	}
	stale := context.WithValue(bound, constUtil.ServerGameClockRevisionKey, int64(-1))
	if _, err := s.dailyCopyAt(stale, id, &pb.CSDailyCopyState{}, 103, now); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("stale server clock accepted: %v", err)
	}
	wrongPlayer, err := json.Marshal(&pb.Player{PlayerId: proto.Int64(id + 1), Level: proto.Int32(75)})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"missing player", nil},
		{"invalid JSON", "{"},
		{"different player ID", string(wrongPlayer)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := d.Model(&db.Profile{}).Where("id=?", id).UpdateColumn("player", tc.value).Error; err != nil {
				t.Fatal(err)
			}
			if out, err := s.dailyCopyAt(bound, id, &pb.CSDailyCopyState{}, 104, now); err == nil || len(out) != 0 {
				t.Fatal("invalid player save was accepted")
			}
		})
	}
	if err := d.Model(&db.Profile{}).Where("id=?", id).UpdateColumn("player", originalPlayer).Error; err != nil {
		t.Fatal(err)
	}
}
