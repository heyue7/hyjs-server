package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p10PowerQueryRecorder struct {
	*adminActivityQueryRecorder
	cache       *p10FamousRankCache
	cacheLocked bool
}

func (r *p10PowerQueryRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if r.cache.mu.TryLock() {
		r.cache.mu.Unlock()
	} else {
		r.cacheLocked = true
	}
	r.adminActivityQueryRecorder.Trace(ctx, begin, fc, err)
}

func TestP10FamousPowerUsesOwnedTitlesAndTreasureMasterWithoutWrites(t *testing.T) {
	f, ids := p10WorshipSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	f.s.InitializeGameTableLookup()
	p10FamousTestServer(t, f, append([]int64{f.id}, ids...)...)
	target := ids[0]
	title := nativePowerTitle(t, "att,5000", 0)
	if err := updateProfile(f.ctx, target, func(player *pb.Player, game *model.GameState) error {
		player.FightPower = proto.Int32(1)
		p5SetKV(game, "p5.title:title_001", title)
		p5SetKV(game, "p5.current-title", "")
		if len(game.Formation) != 1 {
			t.Fatalf("expected one initial formation slot, got %d", len(game.Formation))
		}
		game.Formation[0].Things = append(game.Formation[0].Things,
			&pb.FormationThing{Pos: proto.Int32(5), Id: proto.String("trea_001"), Guid: proto.String("famous-power-ring")},
			&pb.FormationThing{Pos: proto.Int32(6), Id: proto.String("trea_003"), Guid: proto.String("famous-power-necklace")},
		)
		game.Treasures = append(game.Treasures,
			&pb.Treasure{Guid: proto.String("famous-power-ring"), TreasureId: proto.String("trea_001"), StrengthLevel: proto.Int32(30), RefineLevel: proto.Int32(5)},
			&pb.Treasure{Guid: proto.String("famous-power-necklace"), TreasureId: proto.String("trea_003"), StrengthLevel: proto.Int32(20), RefineLevel: proto.Int32(3)},
		)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, ids[1], func(player *pb.Player, _ *model.GameState) error {
		player.FightPower = proto.Int32(2_000_000_000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var before db.Profile
	if err := db.GetIns().Select("id", "player", "game").First(&before, target).Error; err != nil || before.Player == nil || before.Game == nil {
		t.Fatal("target profile", err)
	}
	player, game, err := decodeProfile(&before)
	if err != nil {
		t.Fatal(err)
	}
	copyGame := func() *model.GameState {
		t.Helper()
		raw, err := json.Marshal(game)
		if err != nil {
			t.Fatal(err)
		}
		var copy model.GameState
		if err := json.Unmarshal(raw, &copy); err != nil {
			t.Fatal(err)
		}
		return &copy
	}
	fullPower, err := f.s.formationFightPower(player, copyGame())
	if err != nil {
		t.Fatal(err)
	}
	withoutTitle := copyGame()
	for i, entry := range withoutTitle.KV {
		if entry.GetKey() == "p5.title:title_001" {
			withoutTitle.KV = append(withoutTitle.KV[:i], withoutTitle.KV[i+1:]...)
			break
		}
	}
	noTitlePower, err := f.s.formationFightPower(player, withoutTitle)
	if err != nil {
		t.Fatal(err)
	}
	overlay := make(map[string][]map[string]string, len(f.s.Tables))
	for name, rows := range f.s.Tables {
		overlay[name] = rows
	}
	overlay["StrengthenMaster"] = nil
	for _, row := range f.s.Tables["StrengthenMaster"] {
		if row["type"] != "3" && row["type"] != "4" {
			overlay["StrengthenMaster"] = append(overlay["StrengthenMaster"], row)
		}
	}
	noMasterPower, err := f.s.withRuntimeTables(overlay).formationFightPower(player, copyGame())
	if err != nil {
		t.Fatal(err)
	}
	if fullPower <= noTitlePower || fullPower <= noMasterPower {
		t.Fatalf("actual sources did not affect formation: full=%d without title=%d without treasure masters=%d", fullPower, noTitlePower, noMasterPower)
	}

	tx := db.GetIns()
	oldLogger := tx.Config.Logger
	recorder := &p10PowerQueryRecorder{adminActivityQueryRecorder: &adminActivityQueryRecorder{Interface: oldLogger}, cache: &f.s.p10ClientOwner().famousRankCache}
	tx.Config.Logger = recorder
	defer func() { tx.Config.Logger = oldLogger }()
	rank := p10FamousTestRead(t, f, f.ctx, f.id, 0, time.Now())
	queries := p10FamousTraceStatements(recorder.adminActivityQueryRecorder)
	if len(rank.GetPlayerList()) != 3 || rank.GetPlayerList()[0].GetPlayerId() != target || rank.GetPlayerList()[0].GetFightPower() != fullPower {
		t.Fatalf("rank did not use current source power %d: %v", fullPower, rank)
	}
	if recorder.cacheLocked {
		t.Fatal("public rank cache mutex was held while executing SQL")
	}
	for _, query := range queries {
		lower := strings.ToLower(strings.TrimSpace(query))
		if strings.Contains(lower, "for update") || strings.HasPrefix(lower, "update ") || strings.HasPrefix(lower, "insert ") || strings.HasPrefix(lower, "delete ") || strings.HasPrefix(lower, "replace ") {
			t.Fatalf("rank read locked or wrote a row: %s", query)
		}
	}
	var after db.Profile
	if err := tx.Select("id", "player", "game").First(&after, target).Error; err != nil || after.Player == nil || after.Game == nil {
		t.Fatal("target profile after rank", err)
	}
	if *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("rank read changed player or asset data")
	}
}
