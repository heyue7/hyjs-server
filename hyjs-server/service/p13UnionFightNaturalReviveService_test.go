package service

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFNaturalReviveFixture() (*pb.InstUFPlayer, *pb.BattlePlayer) {
	frame := &pb.InstUFPlayer{Guid: proto.Int64(900), Camp: proto.Int32(1), State: proto.Int32(3), DeadNum: proto.Int32(9), KillNum: proto.Int32(4), OwnScore: proto.Float64(125), OwnToken: proto.Float64(80), IsAuto: proto.Int32(1), LeftTime: proto.Float32(35), TotalTime: proto.Float32(40), OwnRes: &pb.InstUFResource{}, Items: []*pb.InstUFItem{{Id: proto.String("ufitem_012"), PlayerGuid: proto.Int64(900), Index: proto.Int32(0), LeftUseNum: proto.Int32(1), TotalUseNum: proto.Int32(2)}}, FightInfo: &pb.InstFightable{Guid: proto.Int64(900), Index: []int32{3, 0, 7, 13}, HpPos: []float64{0, 0, 0, 0}, MaxHpPos: []float64{2054, 1300, 1510, 3482}, Anger: proto.Float64(88)}}
	battle := &pb.BattlePlayer{PlayerId: proto.Int64(900)}
	for i, pose := range frame.FightInfo.Index {
		battle.Formation = append(battle.Formation, p6Actor("hero_047", string(rune('a'+i)), "047_1", pose, frame.FightInfo.MaxHpPos[i]))
	}
	return frame, battle
}
func p13UFNaturalReviveReceiptFixture(t *testing.T) *db.P13UnionFightReviveReceipt {
	t.Helper()
	before, battle := p13UFNaturalReviveFixture()
	after, err := p13UFNaturalReviveRestore(before, battle)
	if err != nil {
		t.Fatal(err)
	}
	row := &db.P13UnionFightReviveReceipt{MatchID: strings.Repeat("a", 64), PlayerID: 900, DeadOrdinal: 9, Status: constUtil.P13UFNaturalReviveApplied, Rule: constUtil.P13UFNaturalReviveRule, DeathKind: constUtil.P13UFNaturalReviveGuard, DeathID: strings.Repeat("b", 64), DeathSourceHash: strings.Repeat("c", 64), DeathAt: 6000, DueAt: 46000, AppliedAt: 50000, MatchHash: strings.Repeat("d", 64), FieldHash: strings.Repeat("e", 64), BattleHash: strings.Repeat("f", 64)}
	row.Before, _ = proto.Marshal(before)
	row.After, _ = proto.Marshal(after)
	row.BeforePosition, _ = proto.Marshal(&pb.PointNew{X: proto.Float32(90), Y: proto.Float32(0), Z: proto.Float32(8)})
	row.AfterPosition, _ = proto.Marshal(&pb.PointNew{X: proto.Float32(3), Y: proto.Float32(0), Z: proto.Float32(-2)})
	p13UFNaturalReviveTestSeal(row)
	return row
}
func p13UFNaturalReviveTestSeal(row *db.P13UnionFightReviveReceipt) {
	row.BeforeHash, row.AfterHash = p6Digest(row.Before), p6Digest(row.After)
	row.BeforePositionHash, row.AfterPositionHash = p6Digest(row.BeforePosition), p6Digest(row.AfterPosition)
	row.SourceHash = p13UFNaturalReviveHash(row)
}

func TestP13UFNaturalReviveAllFrozenPosesAPZeroKeepsCountersAndBag(t *testing.T) {
	before, battle := p13UFNaturalReviveFixture()
	original := proto.Clone(before).(*pb.InstUFPlayer)
	after, err := p13UFNaturalReviveRestore(before, battle)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, original) || after.GetState() != 0 || after.GetLeftTime() != 0 || after.GetTotalTime() != 0 || after.FightInfo.GetAnger() != 0 {
		t.Fatal("source mutation or incomplete revival", after)
	}
	for i, maximum := range before.FightInfo.MaxHpPos {
		if after.FightInfo.HpPos[i] != maximum {
			t.Fatal("partial pose revival", i)
		}
	}
	// Restore the only allowed fields; every remaining original protocol field
	// must be identical, including item012 stock and resources.
	after.State, after.LeftTime, after.TotalTime = before.State, before.LeftTime, before.TotalTime
	after.FightInfo = proto.Clone(before.FightInfo).(*pb.InstFightable)
	if !proto.Equal(after, before) {
		t.Fatal("kills/deaths/scores/tokens/bag/resource changed")
	}
	if _, err = p13UFNaturalReviveRestore(original, nil); err != nil {
		t.Fatal(err)
	}
	battle.Formation[0].Prop.Hp = proto.Float64(2055)
	if _, err = p13UFNaturalReviveRestore(original, battle); err == nil {
		t.Fatal("changed frozen MaxHp accepted")
	}
}

func TestP13UFNaturalReviveRejectsInvalidPoseHPAndRepeatedRevive(t *testing.T) {
	changes := map[string]func(*pb.InstUFPlayer){
		"alive-item012":     func(f *pb.InstUFPlayer) { f.State = proto.Int32(0); f.FightInfo.HpPos[0] = 1 },
		"partial-survivor":  func(f *pb.InstUFPlayer) { f.FightInfo.HpPos[1] = 1 },
		"duplicate-pose":    func(f *pb.InstUFPlayer) { f.FightInfo.Index[1] = f.FightInfo.Index[0] },
		"bad-array":         func(f *pb.InstUFPlayer) { f.FightInfo.MaxHpPos = f.FightInfo.MaxHpPos[:1] },
		"nonfinite-maximum": func(f *pb.InstUFPlayer) { f.FightInfo.MaxHpPos[0] = math.NaN() },
		"nonfinite-ap":      func(f *pb.InstUFPlayer) { f.FightInfo.Anger = proto.Float64(math.Inf(1)) },
		"wrong-owner":       func(f *pb.InstUFPlayer) { f.FightInfo.Guid = proto.Int64(901) },
		"no-death-ordinal":  func(f *pb.InstUFPlayer) { f.DeadNum = proto.Int32(0) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			frame, battle := p13UFNaturalReviveFixture()
			change(frame)
			if _, err := p13UFNaturalReviveRestore(frame, battle); err == nil {
				t.Fatal("invalid corpse accepted")
			}
		})
	}
	before, battle := p13UFNaturalReviveFixture()
	after, err := p13UFNaturalReviveRestore(before, battle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p13UFNaturalReviveRestore(after, battle); err == nil {
		t.Fatal("second revival accepted")
	}
}

func TestP13UFNaturalReviveLatestFrozenClockColdResumeAndItem012(t *testing.T) {
	frame, _ := p13UFNaturalReviveFixture()
	rules := p13UFGuardBattleRules{Policy: db.P13UnionFightGuardBattlePolicy{ID: 1, Enabled: true, Version: "isolated-natural-revive-test", Source: "fixed fixture, no business authority", PublishedAt: 1, StartsAt: 2, EndsAt: 100000}, LocalRule: constUtil.P13UFGuardLocalRule, DurationMillis: 5000, RecoverySeconds: 120, BuildingKind: "tower", ReviveSeconds: []int64{5, 10, 15, 20, 25, 30, 35, 40}}
	raw, _ := json.Marshal(rules)
	row := &db.P13UnionFightGuardBattleAttempt{ID: strings.Repeat("b", 64), MatchID: strings.Repeat("a", 64), PlayerID: 900, CreatedAt: 1000, DueAt: 6000, SettledAt: 10000, Status: constUtil.P13UFGuardSettled, PlayerDied: true, ReviveSeconds: 40, Rules: raw}
	row.PlayerAfter, _ = proto.Marshal(frame)
	row.SourceHash = p13UFGuardBattleHash(row)
	death, err := p13UFNaturalReviveGuard(row, 900)
	if err != nil || death.due != 46000 {
		t.Fatal("clock must use original DueAt and frozen ordinal seconds", death, err)
	}
	ready, err := p13UFNaturalReviveReady(frame, death, time.UnixMilli(45999))
	if err != nil || ready {
		t.Fatal("early revival", err)
	}
	ready, err = p13UFNaturalReviveReady(frame, death, time.UnixMilli(50000))
	if err != nil || !ready {
		t.Fatal("cold resume must not reset delay from settlement or stale LeftTime", err)
	}
	if err = p13UFItemLife(frame, &db.P13UnionFightItemOffer{ItemID: "ufitem_012", HPPercent: 100}); err != nil {
		t.Fatal(err)
	}
	ready, err = p13UFNaturalReviveReady(frame, death, time.UnixMilli(50000))
	if err != nil || ready {
		t.Fatal("item012 revival must not be repeated", err)
	}
	row.Rules[0] ^= 1
	if _, err = p13UFNaturalReviveGuard(row, 900); err == nil {
		t.Fatal("changed frozen source accepted")
	}
}

func TestP13UFNaturalReviveLatestThreeFamiliesRejectsAmbiguityAndStaleCorpse(t *testing.T) {
	frame, _ := p13UFNaturalReviveFixture()
	value := func(kind string, at int64) *p13UFNaturalReviveDeath {
		return &p13UFNaturalReviveDeath{kind: kind, id: strings.Repeat(kind[:1], 64), hash: strings.Repeat("a", 64), at: at, due: at + 5000, settledAt: at, bound: proto.Clone(frame).(*pb.InstUFPlayer)}
	}
	old, guard, tower := value("ordinary", 1000), value("guard", 2000), value("tower", 3000)
	chosen, err := p13UFNaturalReviveChoose([]*p13UFNaturalReviveDeath{tower, old, guard}, 900)
	if err != nil || chosen != tower {
		t.Fatal("latest death must win across families", err)
	}
	guard.at = tower.at
	guard.settledAt = guard.at
	if _, err = p13UFNaturalReviveChoose([]*p13UFNaturalReviveDeath{tower, guard}, 900); err == nil {
		t.Fatal("same-time distinct death ambiguity accepted")
	}
	guard.at = 2000
	guard.settledAt = guard.at
	tower.bound.FightInfo.HpPos[0] = 1
	if _, err = p13UFNaturalReviveChoose([]*p13UFNaturalReviveDeath{old, guard, tower}, 900); err == nil {
		t.Fatal("invalid newest death fell back to older source")
	}
	tower.bound = proto.Clone(frame).(*pb.InstUFPlayer)
	frame.DeadNum = proto.Int32(10)
	if _, err = p13UFNaturalReviveReady(frame, tower, time.UnixMilli(9000)); err == nil {
		t.Fatal("stale death ordinal accepted")
	}
	frame.DeadNum = proto.Int32(9)
	frame.FightInfo.Anger = proto.Float64(0)
	if _, err = p13UFNaturalReviveReady(frame, tower, time.UnixMilli(9000)); err == nil {
		t.Fatal("changed dead FightInfo accepted")
	}
}

func TestP13UFNaturalReviveReceiptResealCannotChangeEffectsOrSpawn(t *testing.T) {
	row := p13UFNaturalReviveReceiptFixture(t)
	if err := P13UnionFightNaturalReviveVerifyLoaded(row); err != nil {
		t.Fatal(err)
	}
	spawn := &pb.PointNew{X: proto.Float32(3), Y: proto.Float32(0), Z: proto.Float32(-2)}
	if err := P13UnionFightNaturalReviveVerifyPosition(row, spawn); err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*db.P13UnionFightReviveReceipt){
		"early":               func(r *db.P13UnionFightReviveReceipt) { r.AppliedAt = r.DueAt - 1 },
		"ordinal":             func(r *db.P13UnionFightReviveReceipt) { r.DeadOrdinal++ },
		"rule":                func(r *db.P13UnionFightReviveReceipt) { r.Rule = "unpublished-recovery" },
		"partial-millisecond": func(r *db.P13UnionFightReviveReceipt) { r.DueAt++ },
		"reward": func(r *db.P13UnionFightReviveReceipt) {
			a := &pb.InstUFPlayer{}
			_ = proto.Unmarshal(r.After, a)
			a.OwnToken = proto.Float64(a.GetOwnToken() + 1)
			r.After, _ = proto.Marshal(a)
		},
		"stock": func(r *db.P13UnionFightReviveReceipt) {
			a := &pb.InstUFPlayer{}
			_ = proto.Unmarshal(r.After, a)
			a.Items[0].LeftUseNum = proto.Int32(0)
			r.After, _ = proto.Marshal(a)
		},
		"rage": func(r *db.P13UnionFightReviveReceipt) {
			a := &pb.InstUFPlayer{}
			_ = proto.Unmarshal(r.After, a)
			a.FightInfo.Anger = proto.Float64(88)
			r.After, _ = proto.Marshal(a)
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			r := p13UFNaturalReviveReceiptFixture(t)
			change(r)
			p13UFNaturalReviveTestSeal(r)
			if err := P13UnionFightNaturalReviveVerifyLoaded(r); err == nil {
				t.Fatal("resealed invalid effect accepted")
			}
		})
	}
	row.AfterPosition, _ = proto.Marshal(&pb.PointNew{X: proto.Float32(4), Y: proto.Float32(0), Z: proto.Float32(-2)})
	p13UFNaturalReviveTestSeal(row)
	if err := P13UnionFightNaturalReviveVerifyPosition(row, spawn); err == nil {
		t.Fatal("non-frozen spawn accepted")
	}
	row = p13UFNaturalReviveReceiptFixture(t)
	row.Before[0] ^= 1
	if err := P13UnionFightNaturalReviveVerifyLoaded(row); err == nil {
		t.Fatal("PB hash corruption accepted")
	}
}

func TestP13UFNaturalReviveMetadataAndLegacyZeroStateJSON(t *testing.T) {
	row := p13UFNaturalReviveReceiptFixture(t)
	meta := P13UnionFightNaturalReviveMetadata(row)
	light := *row
	light.Before, light.After, light.BeforePosition, light.AfterPosition = nil, nil, nil, nil
	if err := P13UnionFightNaturalReviveVerifyMetadata(&light); err != nil {
		t.Fatal("blob-free scalar seal rejected", err)
	}
	light.AfterHash = strings.Repeat("0", 64)
	if err := P13UnionFightNaturalReviveVerifyMetadata(&light); err == nil {
		t.Fatal("changed light hash accepted")
	}
	if meta.MatchID != row.MatchID || meta.DeathSourceHash != row.DeathSourceHash || meta.BeforeHash != row.BeforeHash || meta.AfterPositionHash != row.AfterPositionHash || meta.SourceHash != row.SourceHash {
		t.Fatal("missing light proof binding")
	}
	actor := db.P13UnionFightFieldPlayer{MatchID: row.MatchID, PlayerID: row.PlayerID}
	raw, err := json.Marshal(actor)
	if err != nil || bytes.Contains(raw, []byte("frameState")) {
		t.Fatal("legacy zero JSON changed", err)
	}
	actor.FrameState = 3
	raw, err = json.Marshal(actor)
	if err != nil || !bytes.Contains(raw, []byte(`"frameState":3`)) {
		t.Fatal("death query scalar missing", err)
	}
}

func TestP13UFNaturalReviveProfileOnlyCoordinatesPreservesRawResourcesAndGuards(t *testing.T) {
	raw := `{"position":{"playerId":900,"scene":"fight_union","current":{"x":90,"y":0,"z":8},"future_field":"kept"},"position_loading":false,"bag":{"unknownItem":{"count":17}},"resource_baseline":12345,"unknown_extra":{"v":2}}`
	spawn := &pb.PointNew{X: proto.Float32(3), Y: proto.Float32(0), Z: proto.Float32(-2)}
	profile := &db.Profile{Id: 900, Game: &raw}
	text, eligible, err := p13UFNaturalReviveProfilePosition(profile, 900, spawn)
	if err != nil || !eligible {
		t.Fatal("same scene position rejected", err)
	}
	var before, after map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &before)
	_ = json.Unmarshal([]byte(text), &after)
	for _, key := range []string{"bag", "resource_baseline", "unknown_extra", "position_loading"} {
		if !bytes.Equal(before[key], after[key]) {
			t.Fatal("unrelated game state changed", key)
		}
	}
	var position map[string]json.RawMessage
	_ = json.Unmarshal(after["position"], &position)
	if string(position["future_field"]) != `"kept"` || *profile.Game != raw {
		t.Fatal("source or unknown position fields changed")
	}
	for _, change := range []string{strings.Replace(raw, `"position_loading":false`, `"position_loading":true`, 1), strings.Replace(raw, `"fight_union"`, `"town_main"`, 1), strings.Replace(raw, `"playerId":900`, `"playerId":901`, 1)} {
		profile.Game = &change
		if _, eligible, err = p13UFNaturalReviveProfilePosition(profile, 900, spawn); err != nil || eligible {
			t.Fatal("loading/scene/identity guard failed", err)
		}
	}
}

func TestP13UFNaturalReviveTowerUsesActualAppliedReviveAtFractionalDelay(t *testing.T) {
	row := p13UFNaturalReviveReceiptFixture(t)
	row.DeathKind, row.DeathID = constUtil.P13UFNaturalReviveTower, "11/1"
	row.DueAt = 46123
	p13UFNaturalReviveTestSeal(row)
	if err := P13UnionFightNaturalReviveVerifyLoaded(row); err != nil {
		t.Fatal("tower actual applied clock must retain non-second delay; source verifier binds Hit.ReviveAt", err)
	}
	row.DeathKind, row.DeathID = constUtil.P13UFNaturalReviveGuard, strings.Repeat("b", 64)
	p13UFNaturalReviveTestSeal(row)
	if err := P13UnionFightNaturalReviveVerifyLoaded(row); err == nil {
		t.Fatal("guard frozen seconds became arbitrary")
	}
}
