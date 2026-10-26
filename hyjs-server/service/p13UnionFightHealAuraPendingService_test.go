package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13AuraPendingQuote(scope string) *db.P13UnionFightHealAuraOffer {
	row := &db.P13UnionFightHealAuraOffer{MatchID: scope, ItemID: "ufitem_013", EffectRule: constUtil.P13UFHealAuraPendingRule, HPPercent: 2, Radius: 3, TickMillis: 1000, AuthorityMask: 6, DurationMillis: 5000, CooldownMillis: 5000, Source: "isolated explicitly chosen pending-heal rule fixture"}
	row.SourceHash = p13UFHealAuraOfferHash(row)
	return row
}

// The quote and one war-bag item are explicit local preparation fixtures.
// Native player capture/calculation/application and aura admission/ticks/drain
// use their production entry points; no native result/certificate is invented.
func TestP13UFHealAuraPendingRealPlayerCaptureNoLostTicksPureTerminal(t *testing.T) {
	f, id, enemy, clock, match := p13UFPlayerBattle172ActualSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	// The setup admits actors before a TCP sink exists; attach the real sessions
	// before broadcasts so lifecycle synchronization keeps both participants live.
	for _, pid := range []int64{id, enemy} {
		var profile db.Profile
		if err := db.GetIns().First(&profile, pid).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(f.s.RegisterP10Client(f.ctx, pid, profile.Session, func(proto.Message) error { return nil }))
		if _, err := f.s.SyncP13UnionFightField(f.ctx, pid, false); err != nil {
			t.Fatal(err)
		}
	}
	quote := p13AuraPendingQuote(match.ID)
	if err := db.GetIns().Create(quote).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	var actor db.P13UnionFightFieldPlayer
	read := func() *pb.InstUFPlayer {
		t.Helper()
		if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	frame := read()
	frame.Items = []*pb.InstUFItem{{Id: proto.String("ufitem_013"), Index: proto.Int32(0), PlayerGuid: proto.Int64(id), LeftUseNum: proto.Int32(1), TotalUseNum: proto.Int32(1)}}
	if err := p13UFFieldSaveActor(db.GetIns(), &actor, frame); err != nil {
		t.Fatal(err)
	}
	castAt := clock.Load()
	if _, err := f.s.p13UFHealAuraUse(f.ctx, id, &pb.CSUFPlayerUseItem{ItemId: proto.String("ufitem_013")}, 601, now()); err != nil {
		t.Fatal("actual new-rule cast", err)
	}
	if _, _, err := f.s.DispatchP13UnionFight(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 602); err != nil {
		t.Fatal("actual player capture", err)
	}
	var attempt db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&attempt, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	clock.Store(castAt + 1000)
	if err := f.s.TickP13UnionFightHealAura(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var effect db.P13UnionFightHealAuraEffect
	if err := db.GetIns().First(&effect, "match_id=?", match.ID).Error; err != nil || effect.EndedAt != 0 || effect.NextTickAt != castAt+2000 {
		t.Fatal("combat canceled emitter or consumed tick without a queued fact", effect, err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P13UnionFightHealAuraPending{}).Where("match_id=? AND player_id=? AND status=?", match.ID, id, constUtil.P13UFHealAuraPending).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("first occupied tick not durable", count, err)
	}
	clock.Store(attempt.DueAt)
	if err := f.s.TickP13UnionFightHealAura(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightHealAuraPending{}).Where("match_id=? AND player_id=?", match.ID, id).Count(&count).Error; err != nil || count != 5 {
		t.Fatal("five accepted ticks not retained", count, err)
	}
	if err := f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&attempt, "id=?", attempt.ID).Error; err != nil {
		t.Fatal(err)
	}
	var pure []*pb.InstUFPlayer
	if json.Unmarshal(attempt.After, &pure) != nil {
		t.Fatal("pure P6 row decode")
	}
	var own *pb.InstUFPlayer
	for _, candidate := range pure {
		if candidate.GetGuid() == id {
			own = candidate
		}
	}
	if own == nil {
		t.Fatal("native actor missing")
	}
	expected := proto.Clone(own).(*pb.InstUFPlayer)
	if own.GetState() != 3 {
		for i := 0; i < 5; i++ {
			if err := p13UFItemLife(expected, &db.P13UnionFightItemOffer{ItemID: "ufitem_013", HPPercent: 2}); err != nil {
				t.Fatal(err)
			}
		}
	}
	current := read()
	if !proto.Equal(expected.FightInfo, current.FightInfo) || current.GetDeadNum() != own.GetDeadNum() || current.GetKillNum() != own.GetKillNum() || current.GetOwnScore() != own.GetOwnScore() || current.GetOwnToken() != own.GetOwnToken() {
		t.Fatal("queued environment changed native result or lost healing")
	}
	if _, _, _, _, err := f.s.p13UFPlayerBattleVerify(db.GetIns().WithContext(f.ctx), &attempt); err != nil {
		t.Fatal("pure P6 After no longer independently verifies", err)
	}
	var pending []db.P13UnionFightHealAuraPending
	if err := db.GetIns().Where("match_id=? AND player_id=?", match.ID, id).Find(&pending).Error; err != nil {
		t.Fatal(err)
	}
	for i := range pending {
		row := &pending[i]
		if row.Status == constUtil.P13UFHealAuraPending || p13UFAuraPendingVerifyLoaded(row) != nil {
			t.Fatal("drain left bad pending", row.ID)
		}
		if own.GetState() == 3 && row.Status != constUtil.P13UFHealAuraCanceled {
			t.Fatal("healing resurrected native death")
		}
	}
	frozenHash := actor.FrameHash
	if err := f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	_ = read()
	if actor.FrameHash != frozenHash {
		t.Fatal("repeated cold candidate applied healing twice")
	}
	ordinary, _, err := f.s.p13UFMixedOrdinary(db.GetIns().WithContext(f.ctx), match)
	if err != nil {
		t.Fatal(err)
	}
	effects, err := p13UFMixedEffects(db.GetIns(), match, map[int64]string{id: actor.GuildID, enemy: func() string {
		if actor.GuildID == match.LeftGuildID {
			return match.RightGuildID
		}
		return match.LeftGuildID
	}()})
	if err != nil {
		t.Fatal(err)
	}
	var actors []db.P13UnionFightFieldPlayer
	if err = db.GetIns().Where("match_id=?", match.ID).Find(&actors).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = p13UFMixedAuraPendings(db.GetIns(), match, append(ordinary, effects.Events...), actors); err != nil {
		t.Fatal("first full new-domain proof binding", err)
	}
}

func TestP13UFHealAuraPendingMiningPreservesClockAndAP(t *testing.T) {
	f, _ := p13GuildSetup(t)
	scope := p6Digest([]byte(t.Name()))
	quote := p13AuraPendingQuote(scope)
	if err := db.GetIns().Create(quote).Error; err != nil {
		t.Fatal(err)
	}
	frame := p13UFMixedFixturePlayer(99, 0, 50)
	frame.State = proto.Int32(2)
	frame.LeftTime = proto.Float32(2.5)
	frame.TotalTime = proto.Float32(3)
	frame.FightInfo.Anger = proto.Float64(88)
	frame.DeadNum = proto.Int32(3)
	actor := &db.P13UnionFightFieldPlayer{MatchID: scope, PlayerID: 99, GuildID: "guild", Camp: 0, BattleHash: strings.Repeat("b", 64)}
	effect := &db.P13UnionFightHealAuraEffect{MatchID: scope, GUID: 123, CasterID: 99, StartedAt: 1000, ExpiresAt: 6000, NextTickAt: 2000, OfferHash: quote.SourceHash}
	effect.SourceHash = p13UFHealAuraEffectHash(effect)
	var row *db.P13UnionFightHealAuraPending
	var after *pb.InstUFPlayer
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		row, err = p13UFAuraQueue(tx, effect, quote, actor, actor, frame, p13UFAuraOccupancy{}, 50, time.UnixMilli(2000))
		if err != nil {
			return err
		}
		after, err = p13UFAuraApply(tx, row, actor, frame, p13UFAuraOccupancy{}, time.UnixMilli(2000))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if after.GetState() != 2 || after.GetLeftTime() != 2.5 || after.GetTotalTime() != 3 || after.FightInfo.GetAnger() != 88 || after.GetDeadNum() != 3 || after.FightInfo.HpPos[0] != 52 {
		t.Fatal("mining aura changed another lifecycle", after)
	}
	var recovery db.P13UnionFightPlayerRecovery
	if err := db.GetIns().First(&recovery, "id=?", row.RecoveryID).Error; err != nil || recovery.PendingID != row.ID {
		t.Fatal("healing lost pending source", err)
	}
	if _, _, err := p13UFPlayerRecoveryVerify(&recovery); err != nil {
		t.Fatal(err)
	}
}

func TestP13UFHealAuraPendingBackpressureCountsFutureBeforeCost(t *testing.T) {
	_, _ = p13GuildSetup(t)
	scope := p6Digest([]byte(t.Name()))
	quote := p13AuraPendingQuote(scope)
	if err := db.GetIns().Create(quote).Error; err != nil {
		t.Fatal(err)
	}
	effects := []db.P13UnionFightHealAuraEffect{}
	// With N=2, two accepted five-tick casts exhaust the conservative reserve.
	for i := int64(1); i <= 2; i++ {
		row := db.P13UnionFightHealAuraEffect{MatchID: scope, GUID: i, CasterID: i, StartedAt: 1000, ExpiresAt: 6000, NextTickAt: 2000, OfferHash: quote.SourceHash}
		row.SourceHash = p13UFHealAuraEffectHash(&row)
		effects = append(effects, row)
	}
	if err := db.GetIns().Create(&effects).Error; err != nil {
		t.Fatal(err)
	}
	if err := p13UFAuraReserve(db.GetIns(), scope, quote, 2); err == nil || err.Error() != constUtil.P13UFHealAuraBackpressure {
		t.Fatal("future accepted ticks were not reserved", err)
	}
	for i := range effects {
		effects[i].EndedAt = 2000
		effects[i].SourceHash = p13UFHealAuraEffectHash(&effects[i])
		if err := db.GetIns().Save(&effects[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := p13UFAuraReserve(db.GetIns(), scope, quote, 2); err != nil {
		t.Fatal("ended emitters did not release future reserve", err)
	}
	legacy := *quote
	legacy.EffectRule = ""
	legacy.SourceHash = p13UFHealAuraOfferHash(&legacy)
	if err := p13UFAuraReserve(db.GetIns(), scope, &legacy, 0); err != nil {
		t.Fatal("old rule acquired new admission semantics", err)
	}
}

func TestP13UFHealAuraPendingChangedLifeCancelsWithoutResurrection(t *testing.T) {
	f, _ := p13GuildSetup(t)
	scope := p6Digest([]byte(t.Name()))
	quote := p13AuraPendingQuote(scope)
	frame := p13UFMixedFixturePlayer(99, 0, 50)
	frame.State = proto.Int32(1)
	frame.DeadNum = proto.Int32(3)
	frame.FightInfo.Anger = proto.Float64(88)
	actor := &db.P13UnionFightFieldPlayer{MatchID: scope, PlayerID: 99, GuildID: "guild", Camp: 0, BattleHash: strings.Repeat("b", 64)}
	effect := &db.P13UnionFightHealAuraEffect{MatchID: scope, GUID: 123, CasterID: 99, StartedAt: 1000, ExpiresAt: 6000, NextTickAt: 2000, OfferHash: quote.SourceHash}
	effect.SourceHash = p13UFHealAuraEffectHash(effect)
	occupancy := p13UFAuraOccupancy{Kind: "guard", ID: strings.Repeat("c", 64), InputHash: strings.Repeat("d", 64), DueAt: 5000}
	var row *db.P13UnionFightHealAuraPending
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		row, err = p13UFAuraQueue(tx, effect, quote, actor, actor, frame, occupancy, 50, time.UnixMilli(2000))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	dead := proto.Clone(frame).(*pb.InstUFPlayer)
	dead.State = proto.Int32(3)
	dead.DeadNum = proto.Int32(4)
	dead.FightInfo.HpPos[0] = 0
	var result *pb.InstUFPlayer
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = p13UFAuraApply(tx, row, actor, dead, occupancy, time.UnixMilli(5000))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if row.Status != constUtil.P13UFHealAuraCanceled || row.CancelReason != "life-changed" || !proto.Equal(result, dead) || row.RecoveryID != "" {
		t.Fatal("old-life tick revived the corpse", row)
	}
	meta := p13UFAuraPendingMetadata(row)
	if err := p13UFAuraPendingVerifyMeta(&meta); err != nil {
		t.Fatal("payload-free metadata", err)
	}
	meta.OfferHash = "bad"
	if p13UFAuraPendingVerifyMeta(&meta) == nil {
		t.Fatal("changed pending source admitted")
	}
}

// These are small conversion/order fixtures, with no claim of P6 authority.
func TestP13UFHealAuraPendingTowerSameTickPrecedesHealing(t *testing.T) {
	nativeID := strings.Repeat("c", 64)
	base := p13UFMixedFixturePlayer(1, 0, 50)
	towerBefore := proto.Clone(base).(*pb.InstUFPlayer)
	towerAfter := proto.Clone(base).(*pb.InstUFPlayer)
	towerAfter.FightInfo.HpPos[0] = 40
	healed := proto.Clone(towerAfter).(*pb.InstUFPlayer)
	healed.FightInfo.HpPos[0] = 42
	native := p13UFMixedEvent{Kind: "ordinary", Key: nativeID, After: []*pb.InstUFPlayer{base}}
	tower := p13UFMixedEvent{Kind: "tower", Key: "7/1", Before: []*pb.InstUFPlayer{towerBefore}, After: []*pb.InstUFPlayer{towerAfter}, Meta: p13UFMixedMeta{Data: p13UFMixedJSON(P13UnionFightTowerHitMeta{TargetKind: "player", TargetGUID: 1, TowerGUID: 7, Sequence: 1, HitAt: 2000, OccupancyKind: "ordinary", OccupancyID: nativeID})}}
	recovery := p13UFMixedEvent{Kind: "recovery", Key: "heal", Before: []*pb.InstUFPlayer{towerAfter}, After: []*pb.InstUFPlayer{healed}}
	row := db.P13UnionFightHealAuraPending{ID: "tick", PlayerID: 1, TickAt: 2000, OccupancyKind: "ordinary", OccupancyID: nativeID, RecoveryID: "heal"}
	row.Final, _ = proto.Marshal(healed)
	if err := p13UFMixedEnvironmentOrder([]p13UFMixedEvent{recovery, tower, native}, []db.P13UnionFightHealAuraPending{row}); err != nil {
		t.Fatal("correct same-tick order", err)
	}
	reverseHeal := proto.Clone(base).(*pb.InstUFPlayer)
	reverseHeal.FightInfo.HpPos[0] = 52
	recovery.Before = []*pb.InstUFPlayer{base}
	recovery.After = []*pb.InstUFPlayer{reverseHeal}
	tower.Before = []*pb.InstUFPlayer{reverseHeal}
	row.Final, _ = proto.Marshal(reverseHeal)
	if p13UFMixedEnvironmentOrder([]p13UFMixedEvent{recovery, tower, native}, []db.P13UnionFightHealAuraPending{row}) == nil {
		t.Fatal("HP chain silently accepted healing before the same-tick tower")
	}
	if !p13UFEnvironmentEventLess(2000, 0, "tower", 2000, 1, "heal") || p13UFEnvironmentEventLess(2000, 1, "heal", 2000, 0, "tower") {
		t.Fatal("drain order and proof order differ")
	}
}
