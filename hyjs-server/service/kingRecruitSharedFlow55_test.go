package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

func TestKingSharedFlow55OriginalWorldReopenCold(t *testing.T) {
	f, actor, base := adminKingFixture(t)
	bound := serverClockBindTest(t, f, f.id)
	if err := updateProfile(bound, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.KingRecruitSharedLuck = nil
		g.KingRecruitLuck = map[int32]int32{1: 490, 2: 730, 3: 0}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The native info query must persist migration inside its profile transaction.
	if got := adminKingInfo(t, f, bound).GetLuck(); got != 1220 {
		t.Fatal("original migrated query", got)
	}
	_, g := f.profile(t)
	if g.KingRecruitSharedLuck == nil || *g.KingRecruitSharedLuck != 1220 || g.KingRecruitLuck != nil {
		t.Fatal("query migration not persisted")
	}
	draw, err := f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 55001)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 55001)
	if err != nil || !proto.Equal(draw[0], retry[0]) {
		t.Fatal("original draw replay", err)
	}
	if got := adminKingInfo(t, f, bound).GetLuck(); got != 1230 {
		t.Fatal("draw duplicated", got)
	}
	if err = f.s.InitializeKingRecruitWorlds(f.ctx); err != nil {
		t.Fatal(err)
	}
	session := []byte("shared-flow55-01")
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("session", session).Error; err != nil {
		t.Fatal(err)
	}
	originalPrizes := append([]model.AdminActivityPrize(nil), base.Prizes...)
	base.Options.KingWorld = 1
	base.Prizes = base.Prizes[:1]
	definition := adminKingDefinition(t, f, actor, base)
	slot := reusePublish(t, f, actor, definition, 1, 1, "calendar")
	_, key, _ := strings.Cut(slot.ConsumerID, ":")
	catalogue, loadErr := f.s.KingRecruitWorlds(bound, f.id, session, "", "")
	if loadErr != nil {
		t.Fatal("read current catalogue", loadErr)
	}
	if _, err = f.s.KingRecruitWorlds(bound, f.id, session, key, catalogue.Version); err != nil {
		t.Fatal(err)
	}
	info := adminKingInfo(t, f, bound)
	if info.GetSpace() != 1 || info.GetLuck() != 1230 {
		t.Fatal("original to world", info)
	}
	req := &pb.CSWarriorRecruitLuckyReward{Reward: proto.String(info.GetRewardList()[0])}
	claim, err := f.s.kingRecruitLucky(bound, f.id, req, 55002)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.kingRecruitLucky(bound, f.id, req, 55002)
	if err != nil || !proto.Equal(claim[0], replay[0]) {
		t.Fatal("exchange replay", err)
	}
	if got := adminKingInfo(t, f, bound).GetLuck(); got != 730 {
		t.Fatal("exchange duplicated or remainder lost", got)
	}
	reuseToggle(t, f, actor, slot.ID, false)
	if adminKingInfo(t, f, bound).GetOn() != 0 {
		t.Fatal("activity not stopped")
	}
	reuseToggle(t, f, actor, slot.ID, true)
	if got := adminKingInfo(t, f, bound).GetLuck(); got != 730 {
		t.Fatal("reopen lost luck", got)
	}
	f.restart(t)
	bound = serverClockBindTest(t, f, f.id)
	if got := adminKingInfo(t, f, bound).GetLuck(); got != 730 {
		t.Fatal("cold lost luck", got)
	}
	replay, err = f.s.kingRecruitLucky(bound, f.id, req, 55002)
	if err != nil || !proto.Equal(claim[0], replay[0]) {
		t.Fatal("cold exchange replay", err)
	}
	replay, err = f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 55001)
	if err != nil || !proto.Equal(draw[0], replay[0]) {
		t.Fatal("cold original draw replay", err)
	}
	if got := adminKingInfo(t, f, bound).GetLuck(); got != 730 {
		t.Fatal("cold replay mutated luck", got)
	}
	// Return to the original rotating-world activity after disabling the split pool.
	reuseToggle(t, f, actor, slot.ID, false)
	base.Options.KingWorld = 0
	base.Prizes = originalPrizes
	original := adminKingDefinition(t, f, actor, base)
	originalSlot := reusePublish(t, f, actor, original, 1, 1, "calendar")
	_, originalKey, _ := strings.Cut(originalSlot.ConsumerID, ":")
	catalogue, loadErr = f.s.KingRecruitWorlds(bound, f.id, session, "", "")
	if loadErr != nil {
		t.Fatal("read current catalogue", loadErr)
	}
	if _, err = f.s.KingRecruitWorlds(bound, f.id, session, originalKey, catalogue.Version); err != nil {
		t.Fatal("return to original", err)
	}
	if got := adminKingInfo(t, f, bound).GetLuck(); got != 730 {
		t.Fatal("world to original lost luck", got)
	}

	// Advance the shared server clock past the one-day occurrence, then open a new occurrence.
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	if adminKingInfo(t, f, bound).GetOn() != 0 {
		t.Fatal("expired activity remained open")
	}
	next := reusePublish(t, f, actor, original, 2, 1, "calendar")
	_, nextKey, _ := strings.Cut(next.ConsumerID, ":")
	catalogue, loadErr = f.s.KingRecruitWorlds(bound, f.id, session, "", "")
	if loadErr != nil {
		t.Fatal("read current catalogue", loadErr)
	}
	if _, err = f.s.KingRecruitWorlds(bound, f.id, session, nextKey, catalogue.Version); err != nil {
		t.Fatal("new occurrence", err)
	}
	if got := adminKingInfo(t, f, bound).GetLuck(); got != 730 {
		t.Fatal("expired/recreated occurrence lost luck", got)
	}

}
