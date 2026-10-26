package service

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFMixedInventoryFixture(t *testing.T, left int32) *db.P13UnionFightFieldPlayer {
	t.Helper()
	frame := &pb.InstUFPlayer{Guid: proto.Int64(1), Camp: proto.Int32(0), Items: []*pb.InstUFItem{{Id: proto.String("ufitem_016"), PlayerGuid: proto.Int64(1), Index: proto.Int32(0), LeftUseNum: proto.Int32(left), TotalUseNum: proto.Int32(2)}}}
	raw, err := proto.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	return &db.P13UnionFightFieldPlayer{MatchID: "fixture", PlayerID: 1, Frame: raw, FrameHash: p6Digest(raw), BattleHash: p6Digest(nil)}
}
func TestP13UFMixedItemInventoryAcquisitionAndRealUses(t *testing.T) {
	actor := p13UFMixedInventoryFixture(t, 1)
	moves := []p13UFMixedInventoryMove{{Item: "ufitem_016", Player: 1, At: 100, Before: 0, After: 2, Key: "buy"}, {Item: "ufitem_016", Player: 1, At: 200, Before: 2, After: 1, Key: "heal"}}
	if err := p13UFMixedInventoryChain(moves, map[int64]*db.P13UnionFightFieldPlayer{1: actor}); err != nil {
		t.Fatal(err)
	}
	if p13UFMixedInventoryChain(moves[:1], map[int64]*db.P13UnionFightFieldPlayer{1: actor}) == nil {
		t.Fatal("missing real consumption accepted")
	}
	if p13UFMixedInventoryChain(moves[1:], map[int64]*db.P13UnionFightFieldPlayer{1: actor}) == nil {
		t.Fatal("inventory without acquisition accepted")
	}
	moves[1].Before = 1
	if p13UFMixedInventoryChain(moves, map[int64]*db.P13UnionFightFieldPlayer{1: actor}) == nil {
		t.Fatal("broken before inventory accepted")
	}
}
func TestP13UFMixedItemInferredCastConsumesOnceAtRealTime(t *testing.T) {
	actor := p13UFMixedInventoryFixture(t, 1)
	moves := []p13UFMixedInventoryMove{{Item: "ufitem_016", Player: 1, At: 100, Before: 0, After: 2, Key: "buy"}, {Item: "ufitem_016", Player: 1, At: 100, Inferred: true, Key: "single-cast"}}
	if err := p13UFMixedInventoryChain(moves, map[int64]*db.P13UnionFightFieldPlayer{1: actor}); err != nil {
		t.Fatal("known acquisition must precede same-tick cast", err)
	}
	moves = append(moves, p13UFMixedInventoryMove{Item: "ufitem_016", Player: 1, At: 100, Inferred: true, Key: "duplicate-target-debit"})
	if p13UFMixedInventoryChain(moves, map[int64]*db.P13UnionFightFieldPlayer{1: actor}) == nil {
		t.Fatal("target fanout debited same cast twice")
	}
}
func TestP13UFMixedTerminalBatchBoundsResultClocks(t *testing.T) {
	f, _ := p13GuildSetup(t)
	ids := []string{}
	for i := 0; i < 129; i++ {
		id := p6Digest([]byte(fmt.Sprintf("%s/%d", t.Name(), i)))
		ids = append(ids, id)
		row := db.P13UnionFightSettlementResult{MatchID: id, EndAt: 10000, TerminalAt: 5000, SettledAt: 5000, AwardAt: 20000, Awards: []byte("[]"), Source: "isolated clock metadata fixture"}
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	queries := 0
	name := "p13-mixed-terminal-batch-query-count"
	if err := db.GetIns().Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "p13_union_fight_guard_battle_attempts" || tx.Statement.Table == "p13_union_fight_settlement_results" {
			queries++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Query().Remove(name) })
	result, err := p13UFMixedTerminalBatch(db.GetIns().WithContext(f.ctx), append(ids, ids[0]))
	if err != nil || len(result) != 129 || queries > 4 {
		t.Fatal("128-match batching/dedup", len(result), queries, err)
	}
	for _, id := range ids {
		if result[id] != 5000 {
			t.Fatal("business cutoff lost", id, result[id])
		}
	}
	if err = db.GetIns().Model(&db.P13UnionFightSettlementResult{}).Where("match_id=?", ids[0]).Update("terminal_at", 11000).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = p13UFMixedTerminalAt(db.GetIns(), ids[0]); err == nil {
		t.Fatal("cutoff beyond original EndAt accepted")
	}
}
func TestP13UFMixedEffectsSpeedTargetsAreOneCast(t *testing.T) {
	_, _ = p13GuildSetup(t)
	match := &db.P13UnionFightMatch{ID: p6Digest([]byte(t.Name())), StartAt: 1000, EndAt: 20000, LeftGuildID: "left", RightGuildID: "right"}
	quote := db.P13UnionFightSpeedItemOffer{MatchID: match.ID, ItemID: "ufitem_015", Multiplier: 1.5, AuthorityMask: 7, Radius: 3, DurationMillis: 5000, CooldownMillis: 5000, Source: "isolated source fixture"}
	quote.SourceHash = p13UFSpeedOfferHash(&quote)
	if err := db.GetIns().Create(&quote).Error; err != nil {
		t.Fatal(err)
	}
	for _, target := range []int64{1, 2} {
		row := db.P13UnionFightSpeedItemEffect{MatchID: match.ID, GUID: p13UFFieldGUID(match.ID, "speed-buff", fmt.Sprintf("1/2000/%d", target)), TargetID: target, CasterID: 1, StartedAt: 2000, ExpiresAt: 7000, EndedAt: 7000, Multiplier: 1.5, OfferHash: quote.SourceHash}
		row.SourceHash = p13UFSpeedEffectHash(&row)
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	evidence, err := p13UFMixedEffects(db.GetIns(), match, map[int64]string{1: "left", 2: "left"})
	if err != nil || len(evidence.Uses) != 1 || len(evidence.Events) != 2 {
		t.Fatal("target fanout must freeze one actual cast", evidence, err)
	}
	if err = db.GetIns().Model(&db.P13UnionFightSpeedItemEffect{}).Where("match_id=? AND target_id=2", match.ID).Update("source_hash", "broken").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = p13UFMixedEffects(db.GetIns(), match, map[int64]string{1: "left", 2: "left"}); err == nil {
		t.Fatal("damaged effect source accepted")
	}
}
