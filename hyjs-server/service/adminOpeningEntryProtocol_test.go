package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"path/filepath"
	"testing"
	"time"
)

// The production-derived finite fixture separates the original two page groups
// from the realm unlock date and the already accepted character lifetime.
func TestAdminOpeningOriginalEntryDayTabsAndReceipt(t *testing.T) {
	f, actor := reuseFixture(t, true)
	// The legacy fixture owner is protected; exercise an ordinary serving role.
	account, err := LoginAccount(f.ctx, "opening-entry-ordinary", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "开服原入口", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	p4Fund(t, f)
	zone := time.FixedZone("UTC+8", 8*3600)
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	roleStart := today.AddDate(0, 0, -1)
	if err := db.GetIns().Model(&db.PlayerOpeningCalendar{}).Where("player_id=?", f.id).Update("start_at", roleStart.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminGameServer{}).Where("id=?", 1).Update("open_at", today.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var definition model.AdminActivityDefinitionDTO
	for _, d := range definitions {
		if d.ID == "original_opening_14" {
			definition = d
		}
	}
	slot := reusePublish(t, f, actor, definition, 1, 14, "character-day")
	bound := serverClockBindTest(t, f, f.id)
	header := func(dayType int32) *pb.SCOpenServerActivity {
		t.Helper()
		requestWire, e := proto.Marshal(&pb.CSOpenServerActivity{DayType: proto.Int32(dayType)})
		request := &pb.CSOpenServerActivity{}
		if e != nil || proto.Unmarshal(requestWire, request) != nil {
			t.Fatal("original request wire", e)
		}
		messages, e := f.s.openServerActivityAt(bound, f.id, request, 0, now)
		if e != nil || len(messages) != 1 {
			t.Fatal("opening header failed before client initialization", e)
		}
		wire, e := proto.Marshal(messages[0])
		result := &pb.SCOpenServerActivity{}
		if e != nil || proto.Unmarshal(wire, result) != nil {
			t.Fatal("original response wire", e)
		}
		return result
	}
	first := header(7)
	if first.GetCurrentDay() != 1 || first.GetActivityEndTime() != roleStart.AddDate(0, 0, 14).UnixMilli() {
		t.Fatal("day-one page received incorrect currentDay or role TTL", first)
	}
	states, err := f.s.openServerOpenStatesAt(bound, f.id, now)
	if err != nil || states[0].GetIsOpen() != 1 || states[1].GetIsOpen() != 0 {
		t.Fatal("premature second-week entrance", states, err)
	}
	for tap := int32(1); tap <= 4; tap++ {
		messages, e := f.s.openServerActivityAt(bound, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(1), Tap: proto.Int32(tap)}, 0, now)
		if e != nil || len(messages) != 1 {
			t.Fatal("day-one original tab blocked", tap, e)
		}
	}
	if _, err = f.s.openServerActivityAt(bound, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(2), Tap: proto.Int32(1)}, 0, now); err == nil {
		t.Fatal("future day bypassed realm unlock")
	}
	if header(14).GetCurrentDay() != 1 {
		t.Fatal("second-week request fabricated unlock")
	}
	claim := &pb.CSGetOpenServerReward{ItemId: proto.Int32(7)}
	paid, err := f.s.openServerActivityAt(bound, f.id, claim, 94001, now)
	if err != nil {
		t.Fatal("day-one achieved original task could not claim", err)
	}
	var accepted db.AdminCharacterActivityInstance
	if err = db.GetIns().First(&accepted, "player_id=? AND occurrence_id=?", f.id, slot.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Changing only the serving realm date must not rewrite the role's issued TTL,
	// ledger or receipt; it changes which original task-day tabs are unlocked.
	if err = db.GetIns().Model(&db.AdminGameServer{}).Where("id=?", 1).Update("open_at", today.AddDate(0, 0, -8).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if header(7).GetCurrentDay() != 7 || header(14).GetCurrentDay() != 9 {
		t.Fatal("original seven/fourteen page currentDay mapping")
	}
	for day := int32(1); day <= 9; day++ {
		if _, e := f.s.openServerActivityAt(bound, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(day), Tap: proto.Int32(1)}, 0, now); e != nil {
			t.Fatal("unlocked original day blocked", day, e)
		}
	}
	var current db.AdminCharacterActivityInstance
	if err = db.GetIns().First(&current, "player_id=? AND occurrence_id=?", f.id, slot.ID).Error; err != nil || string(adminJSON(current)) != string(adminJSON(accepted)) {
		t.Fatal("realm date changed accepted lifetime", err)
	}
	again, err := f.s.openServerActivityAt(bound, f.id, claim, 94001, now)
	if err != nil || !proto.Equal(paid[0], again[0]) {
		t.Fatal("same original receipt changed with realm unlock", err)
	}
	t.Log("production-shaped metadata: day-one wire=1, all four day-one tabs/claim work; second-week icon closed; historical realm day9 wire 7/9 and days1..9 work; accepted TTL/receipt preserved")
}

// First management registration must not make a running realm young again or
// attach a different serving realm's characters to the current process.
func TestAdminGameServerInitialOpeningDateAndRealmIsolation(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "opening-registration.sqlite")); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.GetIns().DB()
	t.Cleanup(func() { sqlDB.Close() })
	zone := time.FixedZone("UTC+8", 8*3600)
	ownDate := time.Date(2026, 10, 11, 10, 0, 0, 0, zone).UnixMilli()
	foreignDate := time.Date(2026, 10, 1, 10, 0, 0, 0, zone).UnixMilli()
	empty := "{}"
	for _, row := range []db.Profile{
		{Uid: "own-opening-role", Session: []byte{1}, Player: &empty, Game: &empty, CreatedAt: &ownDate, ServerID: 1},
		{Uid: "foreign-opening-role", Session: []byte{2}, Player: &empty, Game: &empty, CreatedAt: &foreignDate, ServerID: 2},
		{Uid: "account-without-character", Session: []byte{3}, CreatedAt: &foreignDate, ServerID: 1},
	} {
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := &GameService{ServerID: 1}
	if err := service.InitializeAdminGameServers(ctx); err != nil {
		t.Fatal(err)
	}
	var original db.AdminGameServer
	if err := db.GetIns().First(&original, "id=?", 1).Error; err != nil || original.OpenAt != ownDate {
		t.Fatal("existing serving realm lost its persisted history", original, err)
	}
	var foreign db.Profile
	if err := db.GetIns().First(&foreign, "uid=?", "foreign-opening-role").Error; err != nil || foreign.ServerID != 2 {
		t.Fatal("foreign realm was reassigned at registry creation", err)
	}
	manual := ownDate - 3*86400000
	if err := db.GetIns().Model(&original).Update("open_at", manual).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.InitializeAdminGameServers(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&original, "id=?", 1).Error; err != nil || original.OpenAt != manual {
		t.Fatal("existing configured opening date overwritten", err)
	}
	before := time.Now().UnixMilli()
	emptyService := &GameService{ServerID: 3}
	if err := emptyService.InitializeAdminGameServers(ctx); err != nil {
		t.Fatal(err)
	}
	var fresh db.AdminGameServer
	if err := db.GetIns().First(&fresh, "id=?", 3).Error; err != nil || fresh.OpenAt < before || fresh.OpenAt > time.Now().UnixMilli() {
		t.Fatal("empty realm inherited unrelated realm date", fresh, err)
	}
	if err := db.GetIns().First(&foreign, "uid=?", "foreign-opening-role").Error; err != nil || foreign.ServerID != 2 {
		t.Fatal("empty realm registration moved foreign ownership", err)
	}
}
