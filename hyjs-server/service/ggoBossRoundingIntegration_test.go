package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Exact incident values are used only in a disposable local database. The
// selected terminal is controlled; native battle and MySQL races have separate
// integration coverage.
func TestGGOBossContributionFinalHitTransaction(t *testing.T) {
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	const maximum = float64(43800000)
	const prior = float64(43339921.424241684)
	const remaining = float64(460078.57575832424)
	event.Boss.HP = int64(maximum)
	f.s.GGOFieldConfig.Runtime.Boss.HP = int64(maximum)
	monster, err := f.s.ggoBossMonster(event, remaining)
	if err != nil {
		t.Fatal(err)
	}
	leader := ggoBossGUID(event.ID)
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, current, boss, hp, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, time.Now().UnixMilli(), true, 1)
		if err != nil {
			return err
		}
		current.Boss.HP = int64(maximum)
		hp[leader] = remaining
		boss.CurHp = proto.Float64(remaining / maximum)
		return saveGGOBossState(tx, row, current, boss, hp)
	}); err != nil {
		t.Fatal(err)
	}
	rules, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	member := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: event.ID, PlayerID: f.id, Damage: prior, Rules: rules, DailyLimit: 3}
	if err = db.GetIns().Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	fight := &model.FieldFight{BossLeaderGUID: leader, GGOBoss: &model.GGOFieldBossFight{EventID: event.ID, Line: 1, Boss: &pb.SideBoss{Uuid: proto.String(event.ID)}}}
	input := &model.FrozenBattleInput{Monster: monster}
	terminal := &model.CombatFinalHPOutcome{}
	for _, actor := range monster.Formation {
		hp := actor.GetInst().GetCurHp()
		if actor.GetGuid() == leader {
			hp = 0
		}
		terminal.FinalActors = append(terminal.FinalActors, model.CombatFinalActorHP{Camp: 1, GUID: actor.GetGuid(), CurrentHP: hp, MaxHP: actor.GetProp().GetHp()})
	}
	query := func() db.SharedFieldBoss55 {
		t.Helper()
		var row db.SharedFieldBoss55
		if err := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	apply := func(abort bool) (float64, error) {
		var damage float64
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			_, damage, err = f.s.ggoBossApplyDamageLocked(tx, f.id, fight, input, terminal, time.Now().UnixMilli())
			if err != nil {
				return err
			}
			if abort {
				return errors.New("controlled rollback after reward freeze")
			}
			return nil
		})
		return damage, err
	}
	before := query()
	bound := maximum - remaining
	ulp := math.Nextafter(maximum, math.Inf(1)) - maximum
	invalidPrior := bound + 4*ulp
	if err = db.GetIns().Model(&member).Update("damage", invalidPrior).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = apply(false); err == nil || err.Error() != "首领贡献状态无效" {
		t.Fatal("true contribution overflow accepted", err)
	}
	after := query()
	if after.KillerID != 0 || !bytes.Equal(before.HP, after.HP) || !bytes.Equal(before.GGOEvent, after.GGOEvent) {
		t.Fatal("rejected over-limit settlement changed shared boss")
	}
	if err = db.GetIns().Model(&member).Updates(map[string]any{"damage": prior, "frozen": true}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = apply(false); err == nil || err.Error() != "首领贡献状态无效" {
		t.Fatal("already frozen contribution was accepted", err)
	}
	if err = db.GetIns().Model(&member).Update("frozen", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = apply(true); err == nil || err.Error() != "controlled rollback after reward freeze" {
		t.Fatal("forced transaction rollback", err)
	}
	after = query()
	if err = db.GetIns().First(&member, member.ID).Error; err != nil || after.KillerID != 0 || !bytes.Equal(before.HP, after.HP) || member.Frozen || member.Damage != prior {
		t.Fatal("rollback changed HP, contribution or frozen reward", err)
	}
	damage, err := apply(false)
	if err != nil || damage != remaining {
		t.Fatal("incident last hit did not settle", damage, err)
	}
	after = query()
	if err = db.GetIns().First(&member, member.ID).Error; err != nil || after.KillerID != f.id || !member.Frozen || member.Damage != maximum || member.Rank != 1 || len(member.GGOChips) == 0 {
		t.Fatal("kill, contribution or durable award not committed", err)
	}
	award := append([]byte(nil), member.GGOChips...)
	if damage, err = apply(false); err != nil || damage != 0 {
		t.Fatal("already killed event accepted damage again", damage, err)
	}
	if err = db.GetIns().First(&member, member.ID).Error; err != nil || !bytes.Equal(member.GGOChips, award) || member.Damage != maximum {
		t.Fatal("duplicate result altered frozen award", err)
	}
	gain, err := f.s.CollectFieldBossPendingRewards(ctx, f.id)
	if err != nil || gain == nil {
		t.Fatal("actual reward collection", err)
	}
	paid, game := f.profile(t)
	if again, err := f.s.CollectFieldBossPendingRewards(ctx, f.id); err != nil || again != nil {
		t.Fatal("reward replay", err)
	}
	replayed, replayGame := f.profile(t)
	if !proto.Equal(paid, replayed) || !p6CopySeriesJSONEqual(t, game, replayGame) {
		t.Fatal("reward replay changed profile")
	}
}
