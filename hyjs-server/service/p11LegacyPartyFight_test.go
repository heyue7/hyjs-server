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

// Clone the existing readonly actual source to fresh isolated account IDs.
// All original main/partner GUIDs, gear, talent and original full skills remain.
// The source is explicitly a compatibility clone, never natural creation of001.
func TestP11LegacyPartyActualSourceClonePublicOnceAndProfilesUnchanged(t *testing.T) {
	if constUtil.P6LegacyPartyNativeProof == "" {
		t.Skip("independent actual party certificate pending")
	}
	f, peer, _ := p11ArenaSetup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	f.s.ServerID = 1
	f.s.P5Ready = true
	f.s.P9PetsReady = true
	raw, err := readTestAnalysis75("../../analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.actual-profile-source.json")
	if err != nil || p6Digest(raw) != "89c060a63da435f39a4d15137a0435efb77b68ab786747440f1b081805c0a88b" {
		t.Fatal("actual readonly source SHA", err)
	}
	var sources []struct {
		Player *pb.Player       `json:"player"`
		Game   *model.GameState `json:"game"`
	}
	if err = json.Unmarshal(raw, &sources); err != nil || len(sources) != 2 {
		t.Fatal("actual source pair", err)
	}
	for index, id := range []int64{f.id, peer} {
		sources[index].Player.PlayerId = proto.Int64(id)
		player, err := json.Marshal(sources[index].Player)
		if err != nil {
			t.Fatal(err)
		}
		game, err := json.Marshal(sources[index].Game)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Model(&db.Profile{}).Where("id=?", id).Updates(map[string]any{"player": string(player), "game": string(game)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	low, high := f.id, peer
	if low > high {
		low, high = high, low
	}
	if err = db.GetIns().Create(&db.P10SocialRelation{LowID: low, HighID: high, CreatedAt: time.Now().UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=?", 1).Updates(map[string]any{"enabled": true, "duel_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	profiles := func() []byte {
		var rows []db.Profile
		if err := db.GetIns().Where("id IN ?", []int64{f.id, peer}).Order("id").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	before := profiles()
	request := &pb.CSFriendFight{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	var messages []proto.Message
	var stamp int64
	// Server randomness remains real. Unsupported future executions must roll
	// back every source/receipt/profile and may never be converted into a winner.
	for attempt := int64(0); attempt < 16; attempt++ {
		stamp = 1460 + attempt
		messages, err = f.s.p11FriendScopedFightAt(f.ctx, f.id, request, stamp, time.Now())
		if err == nil {
			break
		}
		if err.Error() != constUtil.CombatUnsupported {
			t.Fatal("public actual source", err)
		}
		if !bytes.Equal(before, profiles()) || p1Count(t, &db.P6BattleRecord{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P11FriendFightSettlement{}, "player_id=?", f.id) != 0 {
			t.Fatal("unsupported execution committed partial state")
		}
	}
	if err != nil {
		t.Fatal("no supported public short source in bounded random attempts", err)
	}
	response := messages[0].(*pb.SCFriendFight)
	if response.GetWinPlayerId() != f.id && response.GetWinPlayerId() != peer {
		t.Fatal("winner not from actual computation")
	}
	if len(response.PlayerList) != 2 || response.PlayerList[0].Formation[0].GetId() != "hero_001" || response.PlayerList[0].Formation[1].GetId() != "hero_003" || response.PlayerList[1].Formation[0].GetId() != "hero_047" || response.PlayerList[1].Formation[1].GetId() != "hero_025" {
		t.Fatal("real party replaced")
	}
	for _, player := range response.PlayerList {
		for _, hero := range player.Formation {
			if len(hero.GetSkillOrder()) != len("001_1,001_2,001_3") {
				t.Fatal("original ultimate stripped")
			}
		}
	}
	if !bytes.Equal(before, profiles()) {
		t.Fatal("public duel changed profile/resources/rank")
	}
	var receipt db.P11FriendFightSettlement
	if err = db.GetIns().First(&receipt, "player_id=?", f.id).Error; err != nil || receipt.ScopeProof != constUtil.P11GenericFriendCompatibilityProof {
		t.Fatal("missing independent party receipt", err)
	}
	if err = db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=?", 1).Update("duel_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	retry, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, stamp, time.Now())
	if err != nil || !proto.Equal(retry[0], response) || !bytes.Equal(before, profiles()) {
		t.Fatal("cold accepted receipt drift or duplicate mutation", err)
	}
	if p1Count(t, &db.P6BattleRecord{}, "player_id=?", f.id) != 1 || p1Count(t, &db.P11FriendFightSettlement{}, "player_id=?", f.id) != 1 {
		t.Fatal("cold retry duplicated records")
	}
}
