package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Read-only combat input from the three original match structures. IDs are
// local stand-ins; original formation, panels, Inst, skill order and seed remain.
func TestP11ArenaFeedbackOriginal062047023Mechanisms(t *testing.T) {
	raw, err := readTestAnalysis75("testdata/p11-arena-feedback-20261018.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Seed     int64            `json:"seed"`
		Attacker *pb.BattlePlayer `json:"attacker"`
		Defender *pb.BattlePlayer `json:"defender"`
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		t.Run(fmt.Sprintf("snapshot%d", i), func(t *testing.T) {
			c.Attacker.PlayerId = proto.Int64(1001)
			c.Attacker.ServerId = proto.Int32(1)
			c.Defender.PlayerId = proto.Int64(1002)
			c.Defender.ServerId = proto.Int32(1)
			input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: c.Attacker, Defender: c.Defender, BattleType: 2, MaxRound: 40, Seed: c.Seed, DefenderFirst: false})
			if err != nil {
				t.Fatal("freeze:", err)
			}
			if _, err = p11GenericMechanismProof(input); err != nil {
				t.Fatal("arena proof:", err)
			}
			if _, err = s.ValidateP6GenericMechanisms(t.Context(), input); err != nil {
				t.Fatal("original mechanism admission:", err)
			}
			out, err := s.ComputeP11GenericMechanismSettlement(t.Context(), input)
			if err != nil {
				t.Fatal("original runtime:", err)
			}
			t.Logf("actual original source terminal winner=%d round=%d actors=%d", out.Outcome.Winner, out.Outcome.FinalRound, len(out.FinalActors))
		})
	}
}

// The table load order matches main: original tables, approved six-green
// compatibility, then feature tables. DB setup remains an isolated finite fixture.
func p11FeedbackStartupTables(t *testing.T, s *GameService) {
	t.Helper()
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP7Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP11Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
}

func TestP11ArenaFeedbackStartupThreeActorsOriginalRequestAndCold(t *testing.T) {
	f, peer, other := p11ArenaSetup(t)
	p11FeedbackStartupTables(t, f.s)
	for _, id := range []int64{peer, other} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(9); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		main := proto.Clone(g.Warriors[0]).(*pb.Warrior)
		main.Level = proto.Int32(10)
		first := proto.Clone(main).(*pb.Warrior)
		first.Id = proto.String("hero_062")
		first.Guid = proto.String("feedback-owned-062")
		first.Level = proto.Int32(7)
		third := proto.Clone(main).(*pb.Warrior)
		third.Id = proto.String("hero_023")
		third.Guid = proto.String("feedback-owned-023")
		third.Level = proto.Int32(7)
		g.Warriors = []*pb.Warrior{first, main, third}
		g.Formation = nil
		for i, w := range g.Warriors {
			g.Formation = append(g.Formation, &pb.FormationItem{FormatPos: proto.Int32(int32(i + 1)), BattlePos: proto.Int32(int32(i + 1)), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: w.Id, Guid: w.Guid}}})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	list, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, &pb.CSPvpFindMatch{}, 994001)
	if err != nil || !handled {
		t.Fatal("original21059", err)
	}
	selected := list[0].(*pb.SCPvpFindMatch).GetRankList()[0].GetInfo()
	request := &pb.CSPvpFightResult{PlayerId: selected.PlayerId, ServerId: selected.ServerId}
	before, _ := p10LuckyProfile(t, f.id)
	out, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 994002)
	if err != nil || !handled {
		t.Fatal("startup-compatible actual23101", err)
	}
	response := out[0].(*pb.SCPvpFightResult)
	if len(response.GetPlayerList()) != 2 || len(response.GetPlayerList()[0].GetFormation()) != 3 || len(response.GetPlayerList()[1].GetFormation()) != 1 {
		t.Fatal("original formations changed", response)
	}
	current, _ := p10LuckyProfile(t, f.id)
	var diamonds int64
	if response.GetWinPlayerId() == f.id {
		diamonds = 32768 - response.GetMyrank().GetPvpRank()
	}
	if current.GetHeroWarId() != before.GetHeroWarId() || current.GetEnergy() != before.GetEnergy()-2 || current.GetDiamond() != before.GetDiamond() {
		t.Fatal("cost/avatar/rank reward", current)
	}
	var row db.P11ArenaFightSettlement
	if err = db.GetIns().First(&row, "player_id=? AND stamp=?", f.id, 994002).Error; err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", row.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	if !record.Authoritative || record.ConfigurationHash != constUtil.P6LinearPhysicalTables || row.ScopeProof != constUtil.P11GenericArenaCompatibilityProof {
		t.Fatal("source normalization/actual proof", record.Authoritative, record.ConfigurationHash, row.ScopeProof)
	}
	raw, _ := proto.Marshal(response)
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	f.s = &GameService{}
	p11FeedbackStartupTables(t, f.s)
	replay, _, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 994002)
	if err != nil {
		t.Fatal("startup-compatible cold receipt", err)
	}
	same, _ := proto.Marshal(replay[0])
	if !bytes.Equal(raw, same) {
		t.Fatal("cold original response changed")
	}
	after, _ := p10LuckyProfile(t, f.id)
	if !proto.Equal(current, after) {
		t.Fatal("cold duplicated cost/reward")
	}
	var count int64
	db.GetIns().Model(&db.P11ArenaFightSettlement{}).Where("player_id=?", f.id).Count(&count)
	if count != 1 {
		t.Fatal("duplicate settlement", count)
	}
	t.Logf("main startup source; actual original21059->23101; actual4 actors winner=%d rank=%d energyDelta=-2 diamonds=%d; cold same receipt", response.GetWinPlayerId(), response.GetMyrank().GetPvpRank(), diamonds)
}
