package service

import (
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// An isolated accepted native fight uses its genuine calculated terminal.
// Only the prior contribution and shared remaining HP are controlled inputs.
func TestGGOBossContributionFinalHitNativeReceipt(t *testing.T) {
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	const maximum = float64(43800000)
	f.s.GGOFieldConfig.Runtime.Boss.HP = int64(maximum)
	f.s.GGOFieldConfig.Runtime.Boss.PartyRounds = 5
	leader := ggoBossGUID(event.ID)
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, current, boss, hp, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, time.Now().UnixMilli(), true, 1)
		if err != nil {
			return err
		}
		current.Boss.HP, current.Boss.PartyRounds = int64(maximum), 5
		hp[leader] = 1
		if current.Boss.HeroID == "monster_047" {
			for _, pose := range []string{"3", "7", "9"} {
				hp[leader+"-"+pose] = 0
			}
		}
		boss.CurHp = proto.Float64(1 / maximum)
		return saveGGOBossState(tx, row, current, boss, hp)
	}); err != nil {
		t.Fatal(err)
	}
	starts, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal("native accepted boss start", err)
	}
	start := starts[0].(*pb.SCSidePveBossFightStart)
	var profile db.Profile
	if err = db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	input, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil || terminal == nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("real frozen terminal", err)
	}
	damage, err := ggoBossActorDamage(game.Field.Fight, input, terminal)
	if err != nil || damage[leader] != 1 {
		t.Fatal("actual native battle did not reach remaining leader HP", damage[leader], err)
	}
	var member db.FieldBossContribution73
	if err = db.GetIns().Where("server_id=? AND spawn_uuid=? AND player_id=?", f.s.OriginalServerID(), event.ID, f.id).Take(&member).Error; err != nil {
		t.Fatal(err)
	}
	prior := math.Nextafter(maximum-1, math.Inf(1))
	if err = db.GetIns().Model(&member).Update("damage", prior).Error; err != nil {
		t.Fatal(err)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	ended, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil {
		t.Fatal("actual last-hit end", err)
	}
	receipt := ended[0].(*pb.SCSidePveBossFightEnd)
	if receipt.GetCode() != 1 {
		t.Fatal("native success receipt missing", receipt.GetCode())
	}
	var stored db.GGOFieldReceipt
	if err = db.GetIns().Where("player_id=? AND operation=? AND business_key=?", f.id, constUtil.GGOFieldBossFightOperation, start.GetFightId()).First(&stored).Error; err != nil {
		t.Fatal("durable exact fight receipt", err)
	}
	if err = db.GetIns().First(&member, member.ID).Error; err != nil || !member.Frozen || member.Damage != maximum {
		t.Fatal("frozen capped contribution", err)
	}
	paid, paidGame := f.profile(t)
	replayed, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || !proto.Equal(receipt, replayed[0]) {
		t.Fatal("native end replay", err)
	}
	changed := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
	changed.Result = proto.Int32(1 - result)
	if _, err = f.s.fieldBossFightEnd(ctx, f.id, changed); err == nil {
		t.Fatal("different end reused immutable receipt")
	}
	f.restart(t)
	replayed, err = f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || !proto.Equal(receipt, replayed[0]) {
		t.Fatal("cold native receipt replay", err)
	}
	last, lastGame := f.profile(t)
	if !proto.Equal(paid, last) || !p6CopySeriesJSONEqual(t, paidGame, lastGame) {
		t.Fatal("replaying last hit changed profile or rewards")
	}
}
