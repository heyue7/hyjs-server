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

// Real query-frozen complete parties must pay once and preserve every actor and
// original skill. Finite isolated account level20 preparation supplies the
// original Arena eligibility through P5 projection; no panel or winner is set.
func TestP11ActualArenaUltimatePublicFrozenCostRankAndColdReceipt(t *testing.T) {
	for _, party := range []int{0, 1} {
		t.Run(string(rune('0'+party)), func(t *testing.T) {
			f, peer := p11ArenaScopedSetup(t)
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
				t.Fatal("source", err)
			}
			var sources []struct {
				Player *pb.Player       `json:"player"`
				Game   *model.GameState `json:"game"`
			}
			if err = json.Unmarshal(raw, &sources); err != nil {
				t.Fatal(err)
			}
			for _, id := range []int64{f.id, peer} {
				player := proto.Clone(sources[party].Player).(*pb.Player)
				player.PlayerId = proto.Int64(id)
				if player.GetLevel() < 20 {
					player.Level = proto.Int32(20)
				}
				pbody, _ := json.Marshal(player)
				gbody, _ := json.Marshal(sources[party].Game)
				if err = db.GetIns().Model(&db.Profile{}).Where("id=?", id).Updates(map[string]any{"player": string(pbody), "game": string(gbody)}).Error; err != nil {
					t.Fatal(err)
				}
			}
			now := time.Now()
			if err = db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.p11ArenaQuery(f.ctx, f.id, &pb.CSPvpFindMatch{}, 15301, now); err != nil {
				t.Fatal(err)
			}
			request := &pb.CSPvpFightResult{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
			beforeOwner, _ := p10LuckyProfile(t, f.id)
			beforePeer, beforeGame := p10LuckyProfile(t, peer)
			if result, err := f.s.p11ArenaScopedFightAt(f.ctx, f.id, request, 15302, now); err == nil || result != nil {
				t.Fatal("defaultoff accepted")
			}
			if p1Count(t, &db.P6BattleRecord{}, "player_id=?", f.id) != 0 {
				t.Fatal("defaultoff partial capture")
			}
			if err = db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			result, err := f.s.p11ArenaScopedFightAt(f.ctx, f.id, request, 15302, now)
			if err != nil || len(result) != 2 {
				t.Fatal("public full party rejected", err)
			}
			response := result[0].(*pb.SCPvpFightResult)
			for _, player := range response.PlayerList {
				if len(player.Formation) != 2 {
					t.Fatal("party stripped")
				}
				for _, actor := range player.Formation {
					prefix := actor.GetId()[5:]
					if actor.GetSkillOrder() != prefix+"_1,"+prefix+"_2,"+prefix+"_3" {
						t.Fatal("skill stripped")
					}
				}
			}
			if len(response.BoxList) != 3 {
				t.Fatal("original flip cards missing")
			}
			afterOwner, _ := p10LuckyProfile(t, f.id)
			afterPeer, afterGame := p10LuckyProfile(t, peer)
			if afterOwner.GetEnergy() != beforeOwner.GetEnergy()-2 || !proto.Equal(beforePeer, afterPeer) {
				t.Fatal("cost or defender mutated")
			}
			a, _ := json.Marshal(beforeGame)
			b, _ := json.Marshal(afterGame)
			if !bytes.Equal(a, b) {
				t.Fatal("defender inventory changed")
			}
			var row db.P11ArenaFightSettlement
			if err = db.GetIns().First(&row, "player_id=? AND stamp=?", f.id, 15302).Error; err != nil {
				t.Fatal(err)
			}
			if row.ScopeProof != constUtil.P6ActualArenaNativeProof || row.ScopeProof == constUtil.P6ActualPartyNativeProof {
				t.Fatal("Arena borrowed Friend proof")
			}
			expectedOwn, expectedDef := row.AttackerOldRank, row.DefenderOldRank
			if row.WinnerID == f.id && expectedOwn > expectedDef {
				expectedOwn, expectedDef = expectedDef, expectedOwn
			}
			if row.AttackerNewRank != expectedOwn || row.DefenderNewRank != expectedDef {
				t.Fatal("rank transition not computed")
			}
			if _, proof, err := P11ArenaVerifiedBestWonRank(db.GetIns(), row.WinnerID); err != nil || proof == "" {
				t.Fatal("computed winner lacks rank qualification", err)
			}
			if err = db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			f.restart(t)
			retry, err := f.s.p11ArenaScopedFightAt(f.ctx, f.id, request, 15302, now.Add(time.Hour))
			if err != nil || !proto.Equal(retry[0], response) {
				t.Fatal("cold frozen receipt changed", err)
			}
			if p1Count(t, &db.P6BattleRecord{}, "player_id=?", f.id) != 1 || p1Count(t, &db.P11ArenaFightSettlement{}, "player_id=?", f.id) != 1 {
				t.Fatal("retry duplicated cost/capture")
			}
			current, _ := p10LuckyProfile(t, f.id)
			if current.GetEnergy() != beforeOwner.GetEnergy()-2 {
				t.Fatal("retry charged twice")
			}
		})
	}
}
