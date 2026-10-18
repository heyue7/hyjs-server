package service

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Each original PVP wrapper keeps its real callback and round limit. This
// representative exercises a grown actor outside the old demo-ID families.
func TestP11GenericMechanismFiveOriginalWrappersAndStrictTerminal(t *testing.T) {
	s := p6OriginalTables(t)
	grown := p6GenericGrownActors(t)
	for _, mode := range []int32{2, 3, 11, 19, 20} {
		input := p6GenericInput(t, s, grown["hero_025"], 40)
		input.BattleType = mode
		if mode == 3 {
			input.MaxRound = 20
		}
		var err error
		input, err = s.FreezeBattleInput(*input)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(input)
		var out *model.CombatFinalHPOutcome
		switch mode {
		case 3:
			out, err = s.ComputeP6PlunderSettlement(t.Context(), input)
		case 19:
			out, err = s.ComputeP6GrabSettlement(t.Context(), input)
		case 20:
			out, err = s.ComputeP6LimitSettlement(t.Context(), input)
		default:
			out, err = s.ComputeP11GenericMechanismSettlement(t.Context(), input)
		}
		if err != nil {
			t.Fatalf("typed%d: %v", mode, err)
		}
		if !p11KnownGenericMechanismProof(input, out.Outcome.CapabilityHash) || validateP11GenericMechanismTerminal(input, out) != nil {
			t.Fatal("new family lost actual frozen source", mode)
		}
		after, _ := json.Marshal(input)
		if !bytes.Equal(before, after) {
			t.Fatal("source changed", mode)
		}
		out.FinalActors[0].CurrentHP++
		if validateP11GenericMechanismTerminal(input, out) == nil {
			t.Fatal("forged HP", mode)
		}
		out.FinalActors[0].CurrentHP--
		out.Outcome.CapabilityHash = constUtil.P11GenericFriendCompatibilityProof
		if mode != 11 && validateP11GenericMechanismTerminal(input, out) == nil {
			t.Fatal("cross-mode proof", mode)
		}
		t.Logf("typed%d retains original rounds=%d and actual source; full terminal checked", mode, input.MaxRound)
	}
}

// Use finite isolated owned cards through the original claim/formation path;
// no live player's inventory or resource acquisition is changed by this test.
func TestP11GenericMechanismFriendActualThreeActorsAndCold(t *testing.T) {
	f, peer, _ := p11ArenaSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.s.P5Ready = true
	for _, id := range []int64{f.id, peer} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
			t.Fatal(err)
		}
		p11ClaimAndFormPartner(t, f, id, "hero_011", 2)
		p11ClaimAndFormPartner(t, f, id, "hero_025", 3)
	}
	low, high := f.id, peer
	if low > high {
		low, high = high, low
	}
	if err := db.GetIns().Create(&db.P10SocialRelation{LowID: low, HighID: high, CreatedAt: time.Now().UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "duel_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSFriendFight{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	replies, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 713001)
	if err != nil || !handled || len(replies) != 1 {
		t.Fatal("public three-actor duel", err)
	}
	response := replies[0].(*pb.SCFriendFight)
	if len(response.PlayerList) != 2 || len(response.PlayerList[0].Formation) != 3 || len(response.PlayerList[1].Formation) != 3 {
		t.Fatal("actual owned formation lost")
	}
	first, _ := proto.MarshalOptions{Deterministic: true}.Marshal(response)
	var receipt db.P11FriendFightSettlement
	if err := db.GetIns().First(&receipt, "player_id=? AND stamp=?", f.id, 713001).Error; err != nil || receipt.ScopeProof != constUtil.P11GenericFriendCompatibilityProof {
		t.Fatal("new source receipt", err)
	}
	f.restart(t)
	replayed, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 713001)
	if err != nil || !handled || len(replayed) != 1 {
		t.Fatal("cold public receipt", err)
	}
	second, _ := proto.MarshalOptions{Deterministic: true}.Marshal(replayed[0])
	if !bytes.Equal(first, second) {
		t.Fatal("cold receipt recomputed")
	}
}
