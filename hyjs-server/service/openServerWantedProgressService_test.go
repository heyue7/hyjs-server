package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestOpenServerWantedCommittedProgressAndReplay(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.UnixMilli(time.Now().UnixMilli())
	zone := time.FixedZone("UTC+8", 8*3600)
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day()-3, 0, 0, 0, 0, zone)
	end := start.AddDate(0, 0, 14)
	if err := db.GetIns().Model(&db.OpenServerActivityRule{}).Where("id=?", constUtil.OpenServerActivityID).Updates(map[string]any{
		"start_at": start.UnixMilli(), "end_at": end.UnixMilli(), "reward_end_at": end.AddDate(0, 0, 1).UnixMilli(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if gameSchemaHasTable(db.GetIns(), &db.PlayerOpeningCalendar{}) {
		if err := db.GetIns().Model(&db.PlayerOpeningCalendar{}).Where("player_id=?", f.id).Update("start_at", start.UnixMilli()).Error; err != nil {
			t.Fatal(err)
		}
	}

	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	instance := p12WantedOwnershipInstance(t, f, fmt.Sprintf("opening-wanted-%d", f.id), now)
	request := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	started, err := f.s.p12WantedStart(startCtx, f.id, request, 999201, now)
	if err != nil {
		t.Fatal(err)
	}
	startReply := started[0].(*pb.SCRebelFightStart)
	var fight db.P12WantedFight
	if err := db.GetIns().First(&fight, "id=?", startReply.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("real Wanted terminal is missing")
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	finish := &pb.CSRebelFightEnd{FightId: startReply.FightId, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, finish)
	finished, err := f.s.p12WantedEnd(endCtx, f.id, finish, now)
	if err != nil {
		t.Fatal(err)
	}
	receipt := finished[0].(*pb.SCRebelFightEnd)
	if err := db.GetIns().First(&fight, "id=?", startReply.GetFightId()).Error; err != nil || !fight.Committed || fight.AppliedDamage < 1 || fight.Performance < 1 {
		t.Fatal("fixture needs a real positive committed Wanted settlement", fight.AppliedDamage, fight.Performance, err)
	}
	if fight.AppliedDamage != receipt.GetDamage() || fight.Performance != receipt.GetRecordAdd() {
		t.Fatal("Wanted receipt and committed facts differ")
	}
	query := func() map[int32]*pb.OpenServer7DayItem {
		t.Helper()
		messages, err := f.s.openServerActivityAt(f.ctx, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(4), Tap: proto.Int32(2)}, 0, now)
		if err != nil {
			t.Fatal(err)
		}
		items := make(map[int32]*pb.OpenServer7DayItem)
		for _, item := range messages[0].(*pb.SCNewOpenSevenDayByTitle).GetItemList() {
			items[item.GetItemId()] = item
		}
		return items
	}
	items := query()
	if items[58] == nil || items[65] == nil || items[58].GetCurrentNum() != int32(min(fight.AppliedDamage, float64(math.MaxInt32))) || items[65].GetCurrentNum() != int32(min(fight.Performance, int64(math.MaxInt32))) {
		t.Fatal("opening progress did not read the real settlement", items[58], items[65])
	}
	replayed, err := f.s.p12WantedEnd(endCtx, f.id, finish, now)
	if err != nil || !proto.Equal(receipt, replayed[0]) || p1Count(t, &db.P12WantedFight{}, "player_id=?", f.id) != 1 {
		t.Fatal("Wanted replay changed committed fights", err)
	}

	// These isolated rows exercise window and date boundaries with values that
	// clearly distinguish one day's sum from the sum across separate days.
	for _, test := range []struct {
		name        string
		playerID    int64
		at          time.Time
		committed   bool
		damage      float64
		performance int64
	}{
		{"start", f.id, start, true, 1000000.75, 4000},
		{"first-day-end", f.id, start.AddDate(0, 0, 1).Add(-time.Millisecond), true, 300001, 5000},
		{"second-day-start", f.id, start.AddDate(0, 0, 1), true, 700000, 7000},
		{"before-start", f.id, start.Add(-time.Millisecond), true, 4000000, 40000},
		{"after-now", f.id, now.Add(time.Millisecond), true, 4000000, 40000},
		{"at-end", f.id, end, true, 4000000, 40000},
		{"other-player", f.id + 1000000, start.Add(time.Hour), true, 4000000, 40000},
		{"pending", f.id, start.Add(time.Hour), false, 4000000, 40000},
		{"end-probe-inside", f.id + 2000000, end.Add(-time.Millisecond), true, 33.9, 42},
		{"end-probe-at-end", f.id + 2000000, end, true, 4000000, 40000},
	} {
		row := fight
		row.ID = fmt.Sprintf("opening-%d-%s", f.id, test.name)
		row.PlayerID, row.CommittedAt, row.Committed = test.playerID, test.at.UnixMilli(), test.committed
		row.AppliedDamage, row.Performance = test.damage, test.performance
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(test.name, err)
		}
	}
	probe, err := readOpenServerWantedProgress(db.GetIns(), f.id+2000000, &db.OpenServerActivityRule{StartAt: start.UnixMilli(), EndAt: end.UnixMilli()}, end.Add(time.Millisecond))
	if err != nil || probe.damage != 33 || probe.performance != 42 {
		t.Fatal("activity end must be exclusive", probe, err)
	}
	items = query()
	wantDamage := max(int32(1000000), int32(min(fight.AppliedDamage, float64(math.MaxInt32))))
	wantPerformance := max(int32(9000), int32(min(fight.Performance, int64(math.MaxInt32))))
	if items[64] == nil || items[71] == nil || items[64].GetCurrentNum() != wantDamage || items[71].GetCurrentNum() != wantPerformance || items[71].GetState() != 0 {
		t.Fatal("opening personal window or UTC+8 daily totals", items[64], items[71])
	}
	if items[58].GetState() != 1 || items[65].GetState() != 1 {
		t.Fatal("qualified Wanted rewards unavailable", items[58], items[65])
	}
	frozenDamage, frozenPerformance := items[58].GetCurrentNum(), items[65].GetCurrentNum()
	additional := fight
	additional.ID = fmt.Sprintf("opening-%d-later-achievement", f.id)
	additional.CommittedAt = start.AddDate(0, 0, 2).UnixMilli()
	additional.AppliedDamage, additional.Performance = 1200000, 10000
	if err := db.GetIns().Create(&additional).Error; err != nil {
		t.Fatal(err)
	}
	items = query()
	if items[58].GetCurrentNum() != frozenDamage || items[65].GetCurrentNum() != frozenPerformance || items[64].GetCurrentNum() != max(wantDamage, int32(1200000)) || items[71].GetCurrentNum() != max(wantPerformance, int32(10000)) {
		t.Fatal("achieved tiers changed or pending tiers missed newer facts", items[58], items[65], items[64], items[71])
	}
	claim := &pb.CSGetOpenServerReward{ItemId: proto.Int32(58)}
	claimed, err := f.s.openServerActivityAt(f.ctx, f.id, claim, 999202, now)
	if err != nil {
		t.Fatal(err)
	}
	before, beforeGame := f.profile(t)
	f.restart(t)
	again, err := f.s.openServerActivityAt(f.ctx, f.id, claim, 999202, now)
	if err != nil || !proto.Equal(claimed[0], again[0]) {
		t.Fatal("cold opening claim replay", err)
	}
	after, afterGame := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, beforeGame, afterGame) || query()[58].GetState() != 2 {
		t.Fatal("opening claim replay changed the saved reward or state")
	}
}
