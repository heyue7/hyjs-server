package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Only initial level and already-used fatigue/monster quota are finite inputs.
// Daily transitions use the original Admin operation and scene consumer.
func TestFieldCalendar75AdminForwardRewindColdAndUnvisitedDates(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, fn := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeDailyCopy, f.s.InitializeLocalPlayActivities, f.s.InitializeServerGameClock} {
		if err := fn(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(80); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Field.Side.Tiredness = proto.Int32(173)
		g.Field.RewardKills = 4
		g.Field.BossRewardDay = "2000-01-01"
		g.Field.BossRewardClaims = 2
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("town_main")}); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	check := func(ctx context.Context, wantTired, wantKills int32) {
		t.Helper()
		if _, err := f.s.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
			t.Fatal(err)
		}
		var row db.Profile
		if err := db.GetIns().First(&row, f.id).Error; err != nil {
			t.Fatal(err)
		}
		var g model.GameState
		if err := json.Unmarshal([]byte(*row.Game), &g); err != nil {
			t.Fatal(err)
		}
		day := p1Day(LocalPlayNow(ctx))
		if g.Field.Side.GetTiredness() != wantTired || g.Field.RewardKills != wantKills || g.Field.RewardDay != day || g.Field.Side.GetTiredNessLastRefreshDate() != day {
			t.Fatal("original field query used wrong game quota", g.Field.Side, wantTired, g.Field.RewardKills, wantKills, day)
		}
		if g.Field.BossRewardClaims != 2 || g.Field.BossRewardDay != "2000-01-01" {
			t.Fatal("life quota overwrote independent boss claims")
		}
		if _, err := f.s.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String("town_main")}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := guildClock75Move(t, f, actor, f.id, 1)
	check(ctx, 200, 0)
	ctx = guildClock75Move(t, f, actor, f.id, -1)
	check(ctx, 173, 4)
	f.restart(t)
	check(ctx, 173, 4)
	// Two Admin shifts without any field consumer in between cannot relabel the
	// old profile's used fatigue as consumption on an unvisited new date.
	ctx = guildClock75Move(t, f, actor, f.id, 1)
	ctx = guildClock75Move(t, f, actor, f.id, 1)
	check(ctx, 200, 0)
	ctx = guildClock75Move(t, f, actor, f.id, -2)
	check(ctx, 173, 4)
}

func TestFieldCalendar75LegacyNoGiftSameDayNoSQLAndRollback(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	makeGame := func() *model.GameState {
		return &model.GameState{Field: &model.FieldState{RewardDay: "2026-10-20", RewardKills: 4, Side: &pb.PlayerSide{Tiredness: proto.Int32(173), MaxTiredness: proto.Int32(200), TiredNessLastRefreshDate: proto.String("2026-10-20")}}}
	}
	g := makeGame()
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		changed, err := prepareFieldCalendar75(tx, f.id, g, "2026-11-02")
		if err != nil {
			return err
		}
		if !changed || g.Field.Side.GetTiredness() != 173 || g.Field.RewardKills != 4 {
			t.Fatal("legacy first read gifted resources")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	count := 0
	session := db.GetIns().Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	key := "field-calendar75-no-sql"
	if err := session.Callback().Query().Before("gorm:query").Register(key, func(tx *gorm.DB) { count++ }); err != nil {
		t.Fatal(err)
	}
	defer session.Callback().Query().Remove(key)
	changed, err := prepareFieldCalendar75(session, f.id, g, "2026-11-02")
	if changed || err != nil || count != 0 {
		t.Fatal("same-day hot read queried DB", changed, err, count)
	}
	session.Callback().Query().Remove(key)
	sentinel := errors.New("rollback finite quota")
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, e := prepareFieldCalendar75(tx, f.id, g, "2026-11-03")
		if e != nil {
			return e
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	var q fieldCalendarQuota75
	found, err := calendarQuota75Load(db.GetIns(), f.id, constUtil.CalendarQuota75FieldLife, "2026-11-03", &q)
	if err != nil || found {
		t.Fatal("failed profile transaction retained a new quota", found, err)
	}
	for _, bad := range []fieldCalendarQuota75{{Day: "bad", Max: 200}, {Day: "2026-11-02", Tired: -1, Max: 200}, {Day: "2026-11-02", Tired: 201, Max: 200}, {Day: "2026-11-02", Max: -1}, {Day: "2026-11-02", Kills: -1}} {
		if bad.valid() {
			t.Fatal("invalid quota accepted", bad)
		}
	}
	// Admin capture prepares detached JSON only; it never mutates the supplied
	// persisted profile bytes while archiving the calendar being left.
	raw, _ := json.Marshal(makeGame())
	text := string(raw)
	row := db.Profile{Id: f.id, Game: &text}
	before := *row.Game
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		return captureFieldCalendar75BeforeShift(tx, []db.Profile{row}, time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	}); err != nil {
		t.Fatal(err)
	}
	if *row.Game != before {
		t.Fatal("Admin capture rewrote profile assets")
	}
}

// An unread Admin date leaves an old profile in place. Returning to that
// profile date and consuming more must still archive the latest old-day use.
func TestFieldCalendar75MarkerAheadPreservesRevisitedConsumption(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	g := &model.GameState{Field: &model.FieldState{RewardDay: "2026-11-02", RewardKills: 4, LifeCalendar75: true, Side: &pb.PlayerSide{Tiredness: proto.Int32(160), MaxTiredness: proto.Int32(200)}}}
	tx := db.GetIns()
	if err := calendarQuota75Save(tx, f.id, constUtil.CalendarQuota75FieldLife, "current", struct {
		Day string `json:"day"`
	}{"2026-11-03"}); err != nil {
		t.Fatal(err)
	}
	if err := calendarQuota75Save(tx, f.id, constUtil.CalendarQuota75FieldLife, "2026-11-03", fieldCalendarQuota75{Day: "2026-11-03", Tired: 200, Max: 200}); err != nil {
		t.Fatal(err)
	}
	changed, err := prepareFieldCalendar75(tx, f.id, g, "2026-11-02")
	if err != nil || changed || g.Field.Side.GetTiredness() != 160 {
		t.Fatal("same-day profile state lost", err)
	}
	if _, err = prepareFieldCalendar75(tx, f.id, g, "2026-11-03"); err != nil {
		t.Fatal(err)
	}
	if _, err = prepareFieldCalendar75(tx, f.id, g, "2026-11-02"); err != nil {
		t.Fatal(err)
	}
	if g.Field.Side.GetTiredness() != 160 || g.Field.RewardKills != 4 {
		t.Fatal("marker ahead refunded old-day fatigue", g.Field.Side, g.Field.RewardKills)
	}
}

// The first same-day read records migration in the profile with zero DB reads;
// the very first natural next day must reset normally, not preserve yesterday.
func TestFieldCalendar75FirstSameDayThenNaturalNewDay(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	g := &model.GameState{Field: &model.FieldState{RewardDay: "2026-11-02", RewardKills: 4, Side: &pb.PlayerSide{Tiredness: proto.Int32(173), MaxTiredness: proto.Int32(200)}}}
	count := 0
	key := "field-calendar75-first-day-no-sql"
	tx := db.GetIns()
	if err := tx.Callback().Query().Before("gorm:query").Register(key, func(*gorm.DB) { count++ }); err != nil {
		t.Fatal(err)
	}
	changed, err := prepareFieldCalendar75(tx, f.id, g, "2026-11-02")
	tx.Callback().Query().Remove(key)
	if err != nil || !changed || !g.Field.LifeCalendar75 || count != 0 || g.Field.Side.GetTiredness() != 173 || g.Field.RewardKills != 4 {
		t.Fatal("first same day failed neutral marker", changed, err, count)
	}
	if _, err = prepareFieldCalendar75(tx, f.id, g, "2026-11-03"); err != nil {
		t.Fatal(err)
	}
	if g.Field.Side.GetTiredness() != 200 || g.Field.RewardKills != 0 {
		t.Fatal("first natural next day failed reset")
	}
	if _, err = prepareFieldCalendar75(tx, f.id, g, "2026-11-02"); err != nil {
		t.Fatal(err)
	}
	if g.Field.Side.GetTiredness() != 173 || g.Field.RewardKills != 4 {
		t.Fatal("first natural day lost old consumption")
	}
}

func TestFieldCalendar75FixedRewardCloneKeepsRequestDay(t *testing.T) {
	f := p1Setup(t)
	g := &model.GameState{FieldCalendarDay: "2026-11-02", Field: &model.FieldState{LifeCalendar75: true, RewardDay: "2026-11-02", RewardKills: 4, Side: &pb.PlayerSide{Tiredness: proto.Int32(173), MaxTiredness: proto.Int32(200)}}}
	p := &pb.Player{PlayerId: proto.Int64(f.id)}
	if _, err := f.s.GrantFixedReward(p, g, "coin,1"); err != nil {
		t.Fatal(err)
	}
	if g.FieldCalendarDay != "2026-11-02" || g.Field.Side.GetTiredness() != 173 || g.Field.RewardKills != 4 {
		t.Fatal("fixed reward clone lost request date or daily use")
	}
	if changed, err := prepareFieldCalendar75(db.GetIns(), f.id, g, g.FieldCalendarDay); err != nil || changed {
		t.Fatal("same request reward reset the field quota", changed, err)
	}
}
