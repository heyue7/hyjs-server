package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"os"
	"strings"
	"testing"
	"time"
)

func calendarQuota75TestContext(ctx context.Context, id int64) context.Context {
	ctx = context.WithValue(ctx, constUtil.ServerGameClockBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockOwnerKey, id)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockServerKey, int64(1))
	return context.WithValue(ctx, constUtil.ServerGameClockRevisionKey, int64(0))
}
func calendarQuota75BaseFixture(t *testing.T) (*GameService, context.Context) {
	t.Helper()
	name := os.Getenv("HYJS_CALENDAR75_MYSQL_DATABASE")
	if name == "" {
		return newP2Test(t)
	}
	if !strings.HasPrefix(name, "hyjs_test_calendar75_") {
		t.Fatal("calendar75 requires its managed isolated MySQL schema")
	}
	ctx := context.Background()
	if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_CALENDAR75_MYSQL_HOST"), User: os.Getenv("HYJS_CALENDAR75_MYSQL_USER"), Password: os.Getenv("HYJS_CALENDAR75_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	service := &GameService{}
	if err := service.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	return service, ctx
}

func calendarQuota75Cold(t *testing.T) {
	t.Helper()
	if db.GetIns().Dialector.Name() == "mysql" {
		prefix := "HYJS_CALENDAR75_MYSQL_"
		if os.Getenv(prefix+"DATABASE") == "" {
			prefix = "HYJS_P13_MYSQL_"
		}
		name := os.Getenv(prefix + "DATABASE")
		if !strings.HasPrefix(name, "hyjs_test_calendar75_") && !strings.HasPrefix(name, "hyjs_test_p13_") {
			t.Fatal("cold restart requires the owned calendar/guild schema")
		}
		connection, _ := db.GetIns().DB()
		connection.Close()
		if err := db.InitGameMySQL(context.Background(), db.MySQLConfig{Host: os.Getenv(prefix + "HOST"), User: os.Getenv(prefix + "USER"), Password: os.Getenv(prefix + "PASSWORD"), Database: name}); err != nil {
			t.Fatal(err)
		}
		cold, _ := db.GetIns().DB()
		t.Cleanup(func() { cold.Close() })
		return
	}
	var files []struct {
		Name string
		File string
	}
	if e := db.GetIns().Raw("PRAGMA database_list").Scan(&files).Error; e != nil {
		t.Fatal(e)
	}
	path := ""
	for _, v := range files {
		if v.Name == "main" {
			path = v.File
		}
	}
	if path == "" {
		t.Fatal("missing owned SQLite path")
	}
	connection, _ := db.GetIns().DB()
	connection.Close()
	if e := db.InitGame(context.Background(), path); e != nil {
		t.Fatal(e)
	}
	connection, _ = db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
}

func TestCalendarGuild75ActualLoginStaminaDonationRewind(t *testing.T) {
	f, ids := p13GuildSetup(t)
	id := ids[0]
	now := time.Now()
	p13GuildOpen(t, true)
	p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String("日期回看")}, 1, now)
	if e := db.GetIns().AutoMigrate(&db.LocalPlayActivityOperation{}); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeServerGameClock(f.ctx); e != nil {
		t.Fatal(e)
	}
	ctx := calendarQuota75TestContext(f.ctx, id)
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, f.s)
	readExp := func() int32 {
		var m db.P13GuildMember
		db.GetIns().First(&m, id)
		var g db.P13Guild
		db.GetIns().First(&g, "id=?", m.GuildID)
		return g.Exp
	}
	if e := f.s.p13GuildLoginAt(ctx, id, now); e != nil {
		t.Fatal(e)
	}
	if e := f.s.p13GuildLoginAt(ctx, id, now.AddDate(0, 0, 1)); e != nil {
		t.Fatal(e)
	}
	after := readExp()
	if e := f.s.p13GuildLoginAt(ctx, id, now); e != nil {
		t.Fatal(e)
	}
	if readExp() != after {
		t.Fatal("rewind credited guild login twice")
	}
	calendarQuota75Cold(t)
	if e := f.s.p13GuildLoginAt(ctx, id, now); e != nil {
		t.Fatal(e)
	}
	if readExp() != after {
		t.Fatal("cold login credited twice")
	}
	for i, at := range []time.Time{now, now.AddDate(0, 0, 1)} {
		if _, e := f.s.p13GuildEconomy(ctx, id, &pb.CSGetUnionStamina{}, int64(101+i), at); e != nil {
			t.Fatal(e)
		}
		if _, e := f.s.p13GuildEconomy(ctx, id, &pb.CSUnionDonate{DonateType: proto.String("coin")}, int64(201+i), at); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.s.p13GuildEconomy(ctx, id, &pb.CSGetUnionStamina{}, 103, now); e == nil {
		t.Fatal("rewind gave stamina twice")
	}
	var m db.P13GuildMember
	if e := db.GetIns().First(&m, id).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error { return calendarGuildDay75(tx, &m, now) }); e != nil {
		t.Fatal(e)
	}
	if m.TodayCoin != 100000 || m.StaminaDay != p13GuildDay(now) {
		t.Fatal("daily cache lost", m.TodayCoin, m.StaminaDay)
	}
}

func TestCalendarPetWeek75KeepsNativeOffersBoughtAndRefresh(t *testing.T) {
	s, ctx := calendarQuota75BaseFixture(t)
	if e := s.LoadP9Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	if e := s.InitializeP9Pets(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.initializeP9Market(db.GetIns().WithContext(ctx)); e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().AutoMigrate(&db.LocalPlayActivityOperation{}); e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Model(&db.P9PetMarketRule{}).Where("id=?", constUtil.P9MarketRuleID).Update("enabled", true).Error; e != nil {
		t.Fatal(e)
	}
	ctx = calendarQuota75TestContext(ctx, 77)
	now := time.Now()
	load := func(at time.Time) *db.P9PetMarketState {
		t.Helper()
		r, _, e := s.p9MarketState(db.GetIns().WithContext(ctx), 77, at)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	a := load(now)
	a.Bought = []byte(`{"slot1":1}`)
	a.RefreshUsed = 3
	if e := db.GetIns().Save(a).Error; e != nil {
		t.Fatal(e)
	}
	hash := a.SelectedHash
	b := load(now.AddDate(0, 0, 7))
	b.Bought = []byte(`{"slot2":1}`)
	b.RefreshUsed = 5
	if e := db.GetIns().Save(b).Error; e != nil {
		t.Fatal(e)
	}
	calendarQuota75Cold(t)
	a = load(now)
	if a.SelectedHash != hash || string(a.Bought) != `{"slot1":1}` || a.RefreshUsed != 3 {
		t.Fatal("prior week changed", a)
	}
	b = load(now.AddDate(0, 0, 7))
	if string(b.Bought) != `{"slot2":1}` || b.RefreshUsed != 5 {
		t.Fatal("next week reset twice", b)
	}
}
func TestCalendarGGODay75KeepsNativeSlotPurchasesAndRefreshQuota(t *testing.T) {
	s, ctx := calendarQuota75BaseFixture(t)
	if e := db.GetIns().AutoMigrate(&db.LocalPlayActivityOperation{}, &db.P4CommerceState{}); e != nil {
		t.Fatal(e)
	}
	ctx = calendarQuota75TestContext(ctx, 88)
	enabled := true
	rules := p4CommerceRules{Slots: 6, FreeCap: 2, FreeSeconds: 3600, RefreshEnabled: &enabled, Products: []p4Product{p4BaseProduct("calendar75", "item_005,10", "item_9710,50", 1, 1)}}
	cfg := db.P4CommerceRule{ID: "calendar75-ggo", Kind: "ggo"}
	load := func(days int64) (*db.P4CommerceState, *ggoQuoteState) {
		t.Helper()
		c := context.WithValue(ctx, constUtil.ServerGameClockOffsetKey, days)
		r, q, e := s.ggoLoadQuote(db.GetIns().WithContext(c), 88, cfg, rules, true)
		if e != nil {
			t.Fatal(e)
		}
		return r, q
	}
	row, q := load(0)
	q.PaidUsed = 3
	q.SlotBought[q.Slots[0].ID] = 1
	if e := ggoSaveQuote(db.GetIns(), row, q); e != nil {
		t.Fatal(e)
	}
	before, _ := json.Marshal(q.Slots)
	row, q = load(1)
	q.PaidUsed = 5
	q.SlotBought[q.Slots[1].ID] = 1
	if e := ggoSaveQuote(db.GetIns(), row, q); e != nil {
		t.Fatal(e)
	}
	calendarQuota75Cold(t)
	_, q = load(0)
	after, _ := json.Marshal(q.Slots)
	if string(before) != string(after) || q.PaidUsed != 3 || q.SlotBought[q.Slots[0].ID] != 1 || q.SlotBought[q.Slots[1].ID] != 0 {
		t.Fatal("rewind duplicated inventory or quota", q)
	}
	_, q = load(1)
	if q.PaidUsed != 5 || q.SlotBought[q.Slots[1].ID] != 1 {
		t.Fatal("forward lost committed quota", q)
	}
}
