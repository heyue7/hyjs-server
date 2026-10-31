package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGrowth74PersonalTurntableScheduleGem(t *testing.T) {
	var activities []growth74Activity
	if err := json.Unmarshal(growth74ActivitiesJSON, &activities); err != nil {
		t.Fatal(err)
	}
	var days []int64
	for _, activity := range activities {
		if activity.Form.Mode != "turntable" {
			continue
		}
		days = append(days, activity.StartDay)
		if activity.Timing != "character-day" || activity.DurationDays != 14 {
			t.Fatalf("unexpected personal window: %+v", activity)
		}
		var found bool
		for _, prize := range activity.Form.Prizes {
			if prize.ID == 1 {
				found = true
				if prize.ResourceID != "MineItem_079" || prize.Amount != 1 || prize.Stock != 4 {
					t.Fatalf("day %d: unexpected gem prize %+v", activity.StartDay, prize)
				}
			}
		}
		if !found {
			t.Fatalf("day %d has no gem slot", activity.StartDay)
		}
	}
	if len(days) != 13 {
		t.Fatalf("expected 13 scheduled rounds, got %v", days)
	}
	for index, day := range days {
		if day != int64(1+14*index) {
			t.Fatalf("unexpected round days: %v", days)
		}
	}
}

func TestGrowth74PersonalTurntableProjectionScope(t *testing.T) {
	cfg := db.GameplayPool{ID: "admin_turntable_character_fixture", Source: "成长投放V12 / 2026-10-26"}
	base := []db.GameplayPoolReward{{ID: 1, ResourceID: "MineItem_079", Amount: 4, Stock: 4}, {ID: 2, ResourceID: "item_079", Amount: 4, Stock: 4}}
	for _, tc := range []struct {
		name   string
		cfg    db.GameplayPool
		amount int32
	}{
		{"target", cfg, 1},
		{"foreign source", db.GameplayPool{ID: cfg.ID, Source: "其他活动"}, 4},
		{"calendar pool", db.GameplayPool{ID: "admin_turntable_schedule_fixture", Source: cfg.Source}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rewards := append([]db.GameplayPoolReward(nil), base...)
			if err := growth74PersonalTurntableRewards(&tc.cfg, rewards); err != nil {
				t.Fatal(err)
			}
			if rewards[0].Amount != tc.amount || rewards[0].Stock != 4 || rewards[1] != base[1] {
				t.Fatal("projection changed wrong prize or stock", rewards)
			}
		})
	}
	invalid := append([]db.GameplayPoolReward(nil), base...)
	invalid[0].Stock = 0
	if err := growth74PersonalTurntableRewards(&cfg, invalid); err == nil {
		t.Fatal("unexpected unlimited stock was accepted")
	}
}

// Exercise a frozen old pool, its genuine old receipt, and independent stock
// through the actual private wheel transaction in an isolated SQLite fixture.
func TestGrowth74PersonalTurntableFrozenInventoryAndReceipt(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	for _, initialize := range []func(context.Context) error{f.s.InitializeTurntable, f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules, f.s.InitializeServerGameClock} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	other, err := LoginAccount(f.ctx, fmt.Sprintf("wheel-other-%d", time.Now().UnixNano()), "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(f.ctx, other.Id, "转盘乙", "hero_043"); err != nil {
		t.Fatal(err)
	}
	owners := []int64{f.id, other.Id}
	contexts := map[int64]context.Context{}
	for _, id := range owners {
		contexts[id] = serverClockBindTest(t, f, id)
		if err := updateProfile(contexts[id], id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(5000); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	form := adminModeForm("turntable")
	form.Source = "成长投放V12 / 2026-10-26"
	form.Options.ScorePerDraw = 1
	form.Options.PrizePityRules = []model.TurntablePrizePityRule{{PrizeID: 1, Enabled: true, HardPity: 3, SoftStart: 1, BasePPM: 1, SoftMaxPPM: 2}}
	for id := int64(0); id < 9; id++ {
		prize := model.AdminActivityPrize{ID: id, ResourceID: "coin", Amount: 1, Weight: 1}
		if id == 1 {
			prize.ResourceID, prize.Amount, prize.Stock = "MineItem_079", 4, 4
		}
		form.Prizes = append(form.Prizes, prize)
	}
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, DefaultDays: 14, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	occurrence := model.AdminActivityOccurrenceDTO{TemplateID: definition.ID, TemplateVersion: definition.Version, Mode: form.Mode, Timing: "character-day", StartDay: 1, DurationDays: 14, StartAt: now.UnixMilli(), EndAt: now.AddDate(0, 0, 14).UnixMilli(), ClaimEndAt: now.AddDate(0, 0, 14).UnixMilli(), Enabled: true}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{occurrence}}); err != nil {
		t.Fatal(err)
	}
	var definitionBefore db.AdminActivityDefinition
	if err := db.GetIns().First(&definitionBefore, "id=?", definition.ID).Error; err != nil {
		t.Fatal(err)
	}
	var occurrenceBefore db.AdminActivityOccurrence
	if err := db.GetIns().First(&occurrenceBefore, "mode=?", "turntable").Error; err != nil {
		t.Fatal(err)
	}
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	seenDefinition := false
	for _, item := range definitions {
		if item.ID == definition.ID {
			seenDefinition = true
			if item.Form.Prizes[1].Amount != 1 || item.Version != adminVersion(definitionBefore) {
				t.Fatal("template presentation or version mismatch", item)
			}
		}
	}
	if !seenDefinition {
		t.Fatal("projected template missing")
	}
	calendar, err = f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil || len(calendar.Items) != 1 || calendar.Items[0].Form.Prizes[1].Amount != 1 || calendar.Items[0].Version != adminVersion(occurrenceBefore) {
		t.Fatal("schedule presentation or version mismatch", calendar, err)
	}
	var definitionAfter db.AdminActivityDefinition
	var occurrenceAfter db.AdminActivityOccurrence
	if err := db.GetIns().First(&definitionAfter, "id=?", definition.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&occurrenceAfter, "id=?", occurrenceBefore.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(definitionBefore.FormJSON, definitionAfter.FormJSON) || adminVersion(definitionBefore) != adminVersion(definitionAfter) || !bytes.Equal(occurrenceBefore.FormJSON, occurrenceAfter.FormJSON) || adminVersion(occurrenceBefore) != adminVersion(occurrenceAfter) {
		t.Fatal("read-only presentation mutated stored form or version")
	}
	keys := map[int64]string{}
	for _, id := range owners {
		if err = f.s.PrepareAdminCharacterActivitySchedules(contexts[id], id); err != nil {
			t.Fatal(err)
		}
		var issued db.AdminCharacterActivityInstance
		if err = db.GetIns().Where("player_id=? AND mode=?", id, "turntable").First(&issued).Error; err != nil {
			t.Fatal(err)
		}
		keys[id] = issued.ConsumerKey
	}
	forceGem := func(id int64) {
		t.Helper()
		state := db.TurntableState{PoolID: keys[id], PlayerID: id}
		if err := db.GetIns().Where("pool_id=? AND player_id=?", keys[id], id).FirstOrCreate(&state).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Model(&state).Update("prize_pity_state", `{"1":{"key":"1:MineItem_079","since":2}}`).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Reproduce a committed win before the fix using the old four-gem payload.
	if err = db.GetIns().Model(&db.GameplayPool{}).Where("id=?", keys[f.id]).Update("source", "old release fixture").Error; err != nil {
		t.Fatal(err)
	}
	forceGem(f.id)
	req := &pb.CSPlayBigTurn{Count: proto.Int32(1)}
	old, err := f.s.playBigTurn(contexts[f.id], f.id, req, 91001)
	if err != nil || old[0].(*pb.SCPlayBigTurn).GetIdNumStr()[0] != "MineItem_079,4" {
		t.Fatal("legacy draw", old, err)
	}
	if err = db.GetIns().Model(&db.GameplayPool{}).Where("id=?", keys[f.id]).Update("source", form.Source).Error; err != nil {
		t.Fatal(err)
	}
	info, err := f.s.bigTurnInfo(contexts[f.id], f.id)
	if err != nil {
		t.Fatal(err)
	}
	gem := info[0].(*pb.SCBigTurnTableInfo).GetRewardItems()[1]
	if gem.GetItem().GetId() != "MineItem_079" || gem.GetItem().GetNum() != 1 || gem.GetSurplus() != 3 || gem.GetAllCount() != 4 {
		t.Fatal("incorrect post-upgrade presentation", gem)
	}
	for n := int64(0); n < 3; n++ {
		forceGem(f.id)
		result, e := f.s.playBigTurn(contexts[f.id], f.id, req, 91002+n)
		if e != nil || result[0].(*pb.SCPlayBigTurn).GetIdNumStr()[0] != "MineItem_079,1" {
			t.Fatal("remaining gem draw", n, result, e)
		}
	}
	forceGem(f.id)
	result, err := f.s.playBigTurn(contexts[f.id], f.id, req, 91005)
	if err != nil || result[0].(*pb.SCPlayBigTurn).GetRewardItemId() == 1 {
		t.Fatal("exhausted prize was selected", result, err)
	}
	forceGem(other.Id)
	result, err = f.s.playBigTurn(contexts[other.Id], other.Id, req, 92001)
	if err != nil || result[0].(*pb.SCPlayBigTurn).GetIdNumStr()[0] != "MineItem_079,1" {
		t.Fatal("other owner's stock", result, err)
	}
	_, inventory := f.profile(t)
	if got := gemCount(inventory, "MineItem_079"); got != 7 {
		t.Fatalf("legacy four plus three subsequent gems: got %d", got)
	}
	f.restart(t)
	replay, err := f.s.playBigTurn(serverClockBindTest(t, f, f.id), f.id, req, 91001)
	if err != nil || !proto.Equal(replay[0], old[0]) {
		t.Fatal("legacy cold receipt changed", replay, err)
	}
	var stored db.GameplayPoolReward
	if err := db.GetIns().First(&stored, "pool_id=? AND id=?", keys[f.id], 1).Error; err != nil || stored.Amount != 4 || stored.Stock != 4 {
		t.Fatal("frozen prize was rewritten", stored, err)
	}
	var personalBefore db.TurntablePersonal
	if err := db.GetIns().First(&personalBefore, "pool_id=? AND player_id=?", keys[f.id], f.id).Error; err != nil {
		t.Fatal(err)
	}
	receiptsBefore := p1Count(t, &db.TurntableDrawReceipt{}, "player_id=?", f.id)
	calendar, err = f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil || len(calendar.Items) != 1 || !calendar.Items[0].Frozen || calendar.Items[0].Form.Prizes[1].Amount != 1 {
		t.Fatal("frozen schedule presentation", calendar, err)
	}
	if _, err := f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: calendar.Items}); err != nil {
		t.Fatal("saving unchanged projected schedule", err)
	}
	var personalAfter db.TurntablePersonal
	if err := db.GetIns().First(&personalAfter, "pool_id=? AND player_id=?", keys[f.id], f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&occurrenceAfter, "id=?", occurrenceBefore.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(personalBefore.UsedStock, personalAfter.UsedStock) || receiptsBefore != p1Count(t, &db.TurntableDrawReceipt{}, "player_id=?", f.id) || !bytes.Equal(occurrenceBefore.FormJSON, occurrenceAfter.FormJSON) || occurrenceAfter.StartDay != occurrenceBefore.StartDay || occurrenceAfter.DurationDays != occurrenceBefore.DurationDays {
		t.Fatal("saving schedule altered frozen form, timing, inventory or receipts")
	}
}
