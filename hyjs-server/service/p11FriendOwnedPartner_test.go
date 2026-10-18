package service

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The fixture uses the real finite P1 operational delivery, original claim and
// formation entry. It never replaces the owned main GUID or edits battle actors.
func p11ClaimAndFormPartner(t *testing.T, f *p1Fixture, id int64, hero string, slot int32) {
	t.Helper()
	var reward *db.RewardDelivery
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		reward, err = f.s.EnqueueReward(tx, id, fmt.Sprintf("p11-owned-%d-%s-%d", id, hero, slot), "真实隔离阵容准备", "explicit finite operational source, not recruit", hero+",1", 0)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	result, err := f.s.RewardCenter(f.ctx, id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}})
	if err != nil {
		t.Fatal(err)
	}
	var card *pb.Warrior
	for _, message := range result {
		if response, ok := message.(*pb.SCAddRewardsToPlayer); ok {
			for _, item := range response.GetResult().GetWars() {
				if item.GetId() == hero {
					card = item
				}
			}
		}
	}
	if card == nil || card.GetGuid() == "" {
		t.Fatal("real claimed partner absent")
	}
	if _, err = f.s.formationOn(f.ctx, id, &pb.CSFormationOn{Id: card.Id, Guid: card.Guid, FormatPos: proto.Int32(slot)}); err != nil {
		t.Fatal(err)
	}
}

func TestP11FriendOwnedPartnerRealClaimFormationPublicConcurrentAndCold(t *testing.T) {
	if constUtil.P6OwnedPartnerNativeProof == "" {
		t.Skip("independent team certificate pending")
	}
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
	}
	p11ClaimAndFormPartner(t, f, f.id, "hero_069", 2)
	p11ClaimAndFormPartner(t, f, peer, "hero_070", 2)
	now := time.Now()
	low, high := f.id, peer
	if low > high {
		low, high = high, low
	}
	if err := db.GetIns().Create(&db.P10SocialRelation{LowID: low, HighID: high, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "duel_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	profile := func(id int64) []byte {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		return []byte(*row.Player + *row.Game)
	}
	ownBefore, peerBefore := profile(f.id), profile(peer)
	request := &pb.CSFriendFight{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	workers := 1
	if f.mysql.Database != "" {
		workers = 8
	}
	var group sync.WaitGroup
	bodies := make([][]byte, workers)
	failures := make([]error, workers)
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			out, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 1301)
			if err == nil {
				if !handled {
					failures[index] = fmt.Errorf("public entry missing")
					return
				}
				bodies[index], err = proto.MarshalOptions{Deterministic: true}.Marshal(out[0])
			}
			failures[index] = err
		}(index)
	}
	group.Wait()
	for index, err := range failures {
		if err != nil || !bytes.Equal(bodies[0], bodies[index]) {
			t.Fatal("actual public team result not once", index, err)
		}
	}
	response := &pb.SCFriendFight{}
	if err := proto.Unmarshal(bodies[0], response); err != nil {
		t.Fatal(err)
	}
	for _, player := range response.PlayerList {
		source, _ := p10LuckyProfile(t, player.GetPlayerId())
		if len(player.Formation) != 2 || player.Formation[0].GetGuid() != source.GetHeroWarId() || player.Formation[0].GetId() != "hero_047" || player.Formation[0].GetPose() != 3 || player.Formation[1].GetPose() != 7 {
			t.Fatal("original owner/layout replaced")
		}
	}
	if response.GetWinPlayerId() != f.id && response.GetWinPlayerId() != peer {
		t.Fatal("actual winner absent")
	}
	if !bytes.Equal(profile(f.id), ownBefore) || !bytes.Equal(profile(peer), peerBefore) {
		t.Fatal("friend team changed profile/resource")
	}
	var receipt db.P11FriendFightSettlement
	if err := db.GetIns().First(&receipt, "player_id=? AND stamp=1301", f.id).Error; err != nil || receipt.ScopeProof != constUtil.P6OwnedPartnerNativeProof {
		t.Fatal("new team proof not persisted", err)
	}
	for _, table := range []any{&db.P11FriendFightSettlement{}, &db.P6BattleRecord{}} {
		var n int64
		if err := db.GetIns().Model(table).Where("player_id=?", f.id).Count(&n).Error; err != nil || n != 1 {
			t.Fatal("multiple team records", n, err)
		}
	}
	// A legal larger team uses the separately named original-mechanism family.
	// It cannot overwrite the earlier accepted two-actor original callback.
	p11ClaimAndFormPartner(t, f, f.id, "hero_071", 3)
	afterExtra := profile(f.id)
	if out, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 1302); err != nil || !handled || len(out) != 1 {
		t.Fatal("legal larger original team rejected", err)
	}
	var larger db.P11FriendFightSettlement
	if err := db.GetIns().First(&larger, "player_id=? AND stamp=1302", f.id).Error; err != nil || larger.ScopeProof != constUtil.P11GenericFriendCompatibilityProof {
		t.Fatal("larger team lost independent family", err)
	}
	if !bytes.Equal(profile(f.id), afterExtra) {
		t.Fatal("unsupported real team changed resources")
	}
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "duel_enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	f.s.Tables["Skill"][0]["Duration"] = "999"
	f.restart(t)
	replay, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 1301)
	if err != nil || !handled {
		t.Fatal("cold team receipt lost", err)
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(replay[0])
	if err != nil || !bytes.Equal(raw, bodies[0]) {
		t.Fatal("accepted team recomputed", err)
	}
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: peer, TargetID: f.id, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if out, _, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 1301); err == nil || out != nil {
		t.Fatal("current blacklist failed old team ACL")
	}
	if !bytes.Equal(profile(f.id), afterExtra) || !bytes.Equal(profile(peer), peerBefore) {
		t.Fatal("receipt/reject changed profiles")
	}
}
