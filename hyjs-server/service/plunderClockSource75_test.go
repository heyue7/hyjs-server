package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Finite inventory and an explicitly approved permanent source are the only
// fixture prerequisites. Candidates come from the real holder projection; no
// battle result, settlement, timestamp receipt or certificate is fabricated.
func TestPlunderClockSource75PermanentFrozenAndNewOfferRewind(t *testing.T) {
	f, peer, _, real := p11PlunderFightSetup(t)
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	for _, id := range []int64{f.id, peer} {
		if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}); err != nil {
			t.Fatal(err)
		}
	}
	p11PlunderFiniteItems(t, f, peer, "frag_trea_001_1,1")
	var policy db.P11PlunderFightPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	policy.Enabled, policy.Permanent = true, true
	policy.Version = "plunder-clock75-permanent"
	policy.PublishedAt, policy.StartAt, policy.EndAt = real.UnixMilli(), real.UnixMilli(), 0
	if err := db.GetIns().Save(&policy).Error; err != nil {
		t.Fatal(err)
	}
	originalPolicy, _ := json.Marshal(policy)
	ctx := guildClock75Move(t, f, actor, f.id, 6)
	forward := LocalPlayNow(ctx)
	query := func(ctx context.Context) {
		t.Helper()
		out, err := f.s.p11PlunderListAt(ctx, f.id, "frag_trea_001_1", LocalPlayNow(ctx))
		if err != nil {
			t.Fatal("published permanent list unavailable on visited game date", err)
		}
		info := out[0].(*pb.SCTreasureBattleList).InfoList
		if len(info) != 1 || info[0].GetSimplePlayerInfo().GetPlayerId() != peer {
			t.Fatal("real holder missing", out)
		}
	}
	query(ctx)
	var frozen db.P11PlunderFightOffer
	var future db.P11PlunderFightMatch
	if err := db.GetIns().First(&frozen, "version=?", policy.Version).Error; err != nil {
		t.Fatal(err)
	}
	frozenRaw, _ := json.Marshal(frozen)
	if err := db.GetIns().Where("player_id=? AND opponent_id=?", f.id, peer).First(&future).Error; err != nil {
		t.Fatal(err)
	}
	futureRaw, _ := json.Marshal(future)
	ctx = guildClock75Move(t, f, actor, f.id, -15)
	back := LocalPlayNow(ctx)
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	beforeGame, _ := json.Marshal(beforeG)
	request := &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(peer), TargetFlagId: proto.String("frag_trea_001_1")}
	if _, err := f.s.p11PlunderFightAt(ctx, f.id, request, 751901, back); err == nil {
		t.Fatal("future-only candidate consumed after rewind")
	}
	query(ctx)
	var unchanged db.P11PlunderFightOffer
	if err := db.GetIns().First(&unchanged, "version=?", frozen.Version).Error; err != nil {
		t.Fatal(err)
	}
	unchangedRaw, _ := json.Marshal(unchanged)
	if !bytes.Equal(frozenRaw, unchangedRaw) {
		t.Fatal("rewind rewrote frozen offer bytes")
	}
	var savedPolicy db.P11PlunderFightPolicy
	if err := db.GetIns().First(&savedPolicy, 1).Error; err != nil {
		t.Fatal(err)
	}
	savedPolicyRaw, _ := json.Marshal(savedPolicy)
	if !bytes.Equal(originalPolicy, savedPolicyRaw) {
		t.Fatal("rewind backdated published policy")
	}
	var current db.P11PlunderFightMatch
	if err := db.GetIns().Where("player_id=? AND created_at<=?", f.id, back.Add(time.Second).UnixMilli()).Order("created_at DESC,id DESC").First(&current).Error; err != nil || current.ID == future.ID {
		t.Fatal("rewind did not create its own actual candidate", err)
	}
	if err := db.GetIns().First(&future, "id=?", future.ID).Error; err != nil {
		t.Fatal(err)
	}
	savedFutureRaw, _ := json.Marshal(future)
	if !bytes.Equal(futureRaw, savedFutureRaw) {
		t.Fatal("future match rewritten")
	}
	// A newly selected, already published permanent version has no old offer.
	// Its strict snapshot capture must remain inside its original source window.
	policy.Version = "plunder-clock75-new-permanent"
	policy.PublishedAt, policy.StartAt = forward.Add(-time.Second).UnixMilli(), forward.Add(-time.Second).UnixMilli()
	if err := db.GetIns().Save(&policy).Error; err != nil {
		t.Fatal(err)
	}
	query(ctx)
	var fresh db.P11PlunderFightOffer
	if err := db.GetIns().First(&fresh, "version=?", policy.Version).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p11PlunderFightOfferRules(&fresh); err != nil || fresh.CapturedAt < policy.StartAt {
		t.Fatal("new offer bypassed original strict capture validator", err)
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	afterGame, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || !bytes.Equal(beforeGame, afterGame) {
		t.Fatal("queries or future-match rejection changed assets")
	}
	var n int64
	if err := db.GetIns().Model(&db.P11PlunderFightSettlement{}).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("source query created settlement", n, err)
	}
	// Closure, a source beyond the visited horizon and finite future windows
	// remain closed. Each case has its own version, so no frozen rule is replaced.
	for _, tc := range []string{"disabled", "unpublished-future", "permanent-future-start", "finite-not-started"} {
		blocked := policy
		blocked.Version = "plunder-clock75-" + tc
		switch tc {
		case "disabled":
			blocked.Enabled = false
		case "unpublished-future":
			blocked.PublishedAt = forward.AddDate(0, 0, 2).UnixMilli()
			blocked.StartAt = blocked.PublishedAt
		case "permanent-future-start":
			blocked.StartAt = forward.AddDate(0, 0, 2).UnixMilli()
		case "finite-not-started":
			blocked.Permanent = false
			blocked.EndAt = blocked.StartAt + int64(time.Hour/time.Millisecond)
		}
		if err := db.GetIns().Save(&blocked).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.p11PlunderListAt(ctx, f.id, "frag_trea_001_1", LocalPlayNow(ctx)); err == nil || !strings.Contains(err.Error(), constUtil.P11PlunderFightUnavailable) {
			t.Fatal(tc, "opened outside its source", err)
		}
		if err := db.GetIns().Model(&db.P11PlunderFightOffer{}).Where("version=?", blocked.Version).Count(&n).Error; err != nil || n != 0 {
			t.Fatal(tc, "persisted rejected source", n, err)
		}
	}
}
