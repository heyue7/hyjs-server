package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// An explicitly unpublished skill source remains rejected even when the public
// policy is enabled. No failed public request can leave a P6 audit,
// receipt, roster edit, resource edit or diagnostic result behind.
func TestP11FriendScopedPublicDefaultsPermissionsAndUnsupportedRollback(t *testing.T) {
	f, _, third := p11ArenaSetup(t)
	login, err := LoginAccount(f.ctx, fmt.Sprintf("p11-unsupported-main-%d", time.Now().UnixNano()), "local-isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, login.Id, "原合法未知主角", "hero_044"); err != nil {
		t.Fatal(err)
	}
	peer := login.Id
	// hero044 is now supported by the published generic family. Corrupt only
	// this finite fixture's source to exercise the actual unsupported boundary.
	hero := f.s.table("Hero", "id", "hero_044")
	originalSkill := hero["SkillId1"]
	hero["SkillId1"] = "fixture75-unpublished-skill"
	t.Cleanup(func() { hero["SkillId1"] = originalSkill })
	now := time.Now()
	request := &pb.CSFriendFight{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	profiles := func() []byte {
		var result []any
		for _, id := range []int64{f.id, peer, third} {
			p, g := p10LuckyProfile(t, id)
			result = append(result, p, g)
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := string(profiles())
	reject := func(stamp int64) {
		t.Helper()
		reply, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, stamp, now)
		if err == nil || reply != nil {
			t.Fatal("unsupported public result", reply, err)
		}
		for _, m := range []any{&db.P11FriendFightSettlement{}, &db.P6BattleRecord{}} {
			var n int64
			if err = db.GetIns().Model(m).Where("player_id=?", f.id).Count(&n).Error; err != nil || n != 0 {
				t.Fatal("partial public battle persisted", n, err)
			}
		}
		if string(profiles()) != before {
			t.Fatal("rejected friend fight mutated profiles")
		}
	}
	reject(0)
	reject(501)
	low, high := f.id, peer
	if low > high {
		low, high = high, low
	}
	if err := db.GetIns().Create(&db.P10SocialRelation{LowID: low, HighID: high, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	var cfg db.P11ArenaPolicy
	if err := db.GetIns().First(&cfg, 1).Error; err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.DuelEnabled {
		t.Fatal("public battle policy must default off")
	}
	reject(502)
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"enabled": true, "duel_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	reject(503)
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: peer, TargetID: f.id, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	reject(504)
}

// A public accepted fight remains its same frozen original callback after current
// roster/table/config changes and DB reopen. Real InnoDB runs race eight callers;
// SQLite uses one caller because row-lock concurrency is validated in InnoDB.
func TestP11FriendScopedPublicComputedReceiptConcurrentAndCold(t *testing.T) {
	if constUtil.P6StarterNativeProof == "" {
		t.Skip("original starter capability pending")
	}
	f, peer, _ := p11ArenaSetup(t)
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
	profiles := func() []byte {
		t.Helper()
		p, g := p10LuckyProfile(t, f.id)
		a, b := p10LuckyProfile(t, peer)
		raw, err := json.Marshal([]any{p, g, a, b})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := profiles()
	request := &pb.CSFriendFight{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	workers := 1
	if f.mysql.Database != "" {
		workers = 8
	}
	var group sync.WaitGroup
	replies := make([][]byte, workers)
	errors := make([]error, workers)
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			out, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, 601, now)
			if err == nil {
				replies[index], err = proto.MarshalOptions{Deterministic: true}.Marshal(out[0])
			}
			errors[index] = err
		}(index)
	}
	group.Wait()
	for index, err := range errors {
		if err != nil || !bytes.Equal(replies[index], replies[0]) {
			t.Fatal("same-stamp concurrent source/response drift", index, err)
		}
	}
	if !bytes.Equal(before, profiles()) {
		t.Fatal("public duel changed original inventories/resources/profiles")
	}
	var receipt db.P11FriendFightSettlement
	if err := db.GetIns().First(&receipt, "player_id=? AND stamp=601", f.id).Error; err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err := db.GetIns().First(&record, "id=?", receipt.P6RecordID).Error; err != nil || record.Source != constUtil.P11FriendScopedSource || record.Status != constUtil.P6BattleComputed || !record.Authoritative || receipt.ScopeProof != constUtil.P6StarterNativeProof {
		t.Fatal("accepted source proof", err)
	}
	for _, table := range []any{&db.P11FriendFightSettlement{}, &db.P6BattleRecord{}} {
		var n int64
		if err := db.GetIns().Model(table).Where("player_id=?", f.id).Count(&n).Error; err != nil || n != 1 {
			t.Fatal("duplicated public transaction", n, err)
		}
	}
	if err := updateProfile(f.ctx, peer, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(21); return nil }); err != nil {
		t.Fatal(err)
	}
	expected := profiles()
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "duel_enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	f.s.Tables["Skill"][0]["Duration"] = "999"
	f.restart(t)
	retry, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, 601, now.Add(time.Hour))
	if err != nil {
		t.Fatal("accepted immutable receipt lost after restart/current source changes", err)
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(retry[0])
	if err != nil || !bytes.Equal(raw, replies[0]) {
		t.Fatal("old fight recomputed under new profile/table/config", err)
	}
	if out, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, 602, now); err == nil || out != nil {
		t.Fatal("closed new request succeeded")
	}
	if err := db.GetIns().Model(&receipt).Update("scope_proof", "unknown-proof").Error; err != nil {
		t.Fatal(err)
	}
	if out, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, 601, now); err == nil || out != nil {
		t.Fatal("unrecognized stored certificate returned")
	}
	if err := db.GetIns().Model(&receipt).Update("scope_proof", constUtil.P6StarterNativeProof).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: peer, TargetID: f.id, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if out, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, 601, now); err == nil || out != nil {
		t.Fatal("current blacklist failed to revoke old receipt")
	}
	if !bytes.Equal(expected, profiles()) {
		t.Fatal("receipt retry mutated current profiles")
	}
}
