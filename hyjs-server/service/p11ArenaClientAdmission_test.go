package service

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Original PVPRankPanel.OnEnemyFightButtonClick copies only the selected
// PvpRankPlayerInfo.info playerId/serverId into original CSPvpFightResult23101.
// This fixture keeps the original avatar owned but fields another owned actor,
// which is legal after the at-least-one-person formation restoration.
func TestP11ArenaOriginalClientSelectedNPCWithMainOffFormation(t *testing.T) {
	f, _, _ := p11ArenaSetup(t)
	var substituteGUID string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		substitute := proto.Clone(g.Warriors[0]).(*pb.Warrior)
		substituteGUID = "isolated-arena-substitute"
		substitute.Guid = proto.String(substituteGUID)
		g.Warriors = append(g.Warriors, substitute)
		g.Formation[0].Things[0].Guid = proto.String(substituteGUID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	listing, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, &pb.CSPvpFindMatch{}, 993001)
	if err != nil || !handled {
		t.Fatal(listing, handled, err)
	}
	query := listing[0].(*pb.SCPvpFindMatch)
	selected := query.GetRankList()[0].GetInfo()
	request := &pb.CSPvpFightResult{PlayerId: selected.PlayerId, ServerId: selected.ServerId}
	wire, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	original := &pb.CSPvpFightResult{}
	if err = proto.Unmarshal(wire, original); err != nil {
		t.Fatal(err)
	}
	before, _ := p10LuckyProfile(t, f.id)
	out, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, original, 993002)
	if err != nil || !handled {
		t.Fatal("original selected NPC challenge rejected", handled, err)
	}
	response := out[0].(*pb.SCPvpFightResult)
	current, _ := p10LuckyProfile(t, f.id)
	if len(response.GetPlayerList()) != 2 || response.GetPlayerList()[0].GetFormation()[0].GetGuid() != substituteGUID || response.GetPlayerList()[1].GetPlayerId() != selected.GetPlayerId() {
		t.Fatal("wrong frozen formation", response)
	}
	// Normal arena preserves the real seed/winner; a valid admitted defeat
	// spends energy and grants no newly won rank diamonds.
	var expectedDiamond int64
	if response.GetWinPlayerId() == f.id {
		expectedDiamond = 32768 - response.GetMyrank().GetPvpRank()
	}
	if current.GetHeroWarId() != before.GetHeroWarId() || current.GetEnergy() != before.GetEnergy()-2 || current.GetDiamond() != before.GetDiamond()+expectedDiamond {
		t.Fatal("avatar/cost/rank reward changed", before, current)
	}
	raw, _ := proto.Marshal(response)
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	replay, _, err := f.s.DispatchP11Arena(f.ctx, f.id, original, 993002)
	if err != nil {
		t.Fatal("cold original request", err)
	}
	same, _ := proto.Marshal(replay[0])
	if !bytes.Equal(raw, same) {
		t.Fatal("cold original response changed")
	}
	var count int64
	db.GetIns().Model(&db.P11ArenaFightSettlement{}).Where("player_id=?", f.id).Count(&count)
	if count != 1 {
		t.Fatal("duplicate real settlement", count)
	}
	t.Log("original21059->selectedInfo{playerId,serverId}->23101 accepted with main avatar off formation; originalEnergy2/actualWinnerRankReward/cold receipt")
}
