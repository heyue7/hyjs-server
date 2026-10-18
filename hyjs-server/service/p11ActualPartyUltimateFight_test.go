package service

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// A new public capture cannot promote a diagnostic or borrow the short-family
// proof. Mirror both real complete parties under fresh isolated account IDs.
func TestP11ActualPartyUltimatePublicIndependentGateAtomic(t *testing.T) {
	f, peer, _ := p11ArenaSetup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	f.s.ServerID = 1
	f.s.P5Ready = true
	f.s.P9PetsReady = true
	raw, err := os.ReadFile("../../analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.actual-profile-source.json")
	if err != nil || p6Digest(raw) != "89c060a63da435f39a4d15137a0435efb77b68ab786747440f1b081805c0a88b" {
		t.Fatal("actual source", err)
	}
	var sources []struct {
		Player *pb.Player       `json:"player"`
		Game   *model.GameState `json:"game"`
	}
	if err = json.Unmarshal(raw, &sources); err != nil || len(sources) != 2 {
		t.Fatal(err)
	}
	for _, id := range []int64{f.id, peer} {
		// The complete source party is copied; only local account ownership changes.
		source := sources[0]
		source.Player = proto.Clone(source.Player).(*pb.Player)
		source.Player.PlayerId = proto.Int64(id)
		player, err := json.Marshal(source.Player)
		if err != nil {
			t.Fatal(err)
		}
		game, err := json.Marshal(source.Game)
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
		raw, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := profiles()
	request := &pb.CSFriendFight{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	result, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, 14701, time.Now())
	if constUtil.P6ActualPartyNativeProof == "" {
		if err == nil || result != nil {
			t.Fatal("independent original ultimate proof absent but accepted")
		}
		if p1Count(t, &db.P6BattleRecord{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P11FriendFightSettlement{}, "player_id=?", f.id) != 0 {
			t.Fatal("pending proof committed partial source or receipt")
		}
	} else {
		if err != nil || len(result) == 0 {
			t.Fatal("certified full party rejected", err)
		}
		response := result[0].(*pb.SCFriendFight)
		if response.GetWinPlayerId() != f.id && response.GetWinPlayerId() != peer {
			t.Fatal("winner not computed")
		}
		for _, player := range response.PlayerList {
			if len(player.Formation) != 2 || player.Formation[0].GetSkillOrder() != "001_1,001_2,001_3" || player.Formation[1].GetSkillOrder() != "003_1,003_2,003_3" {
				t.Fatal("actual ultimate/party stripped")
			}
		}
		f.restart(t)
		retry, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, 14701, time.Now())
		if err != nil || !proto.Equal(retry[0], response) {
			t.Fatal("cold accepted source changed", err)
		}
	}
	if !bytes.Equal(before, profiles()) {
		t.Fatal("public ultimate gate changed profile/resources/rank")
	}
}
