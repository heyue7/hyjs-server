package service

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Two frozen encounters overlap. Each applies its own proven damage under the
// event row lock; dead attendants remain at zero and do not affect boss rank.
func TestGGOBossCarryHealthSameEventAndCold(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	if event.Boss.HeroID != "monster_047" {
		t.Fatal("four-member event required")
	}
	var other db.SharedFieldBoss55
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, _, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, time.Now().UnixMilli(), true, 2)
		if err == nil {
			other = *row
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	leader := ggoBossGUID(event.ID)
	monster, err := f.s.ggoBossMonster(event, float64(event.Boss.HP))
	if err != nil || len(monster.Formation) != 4 {
		t.Fatal("initial original formation", err)
	}
	input := &model.FrozenBattleInput{Monster: monster}
	fight := &model.FieldFight{BossLeaderGUID: leader, GGOBoss: &model.GGOFieldBossFight{EventID: event.ID, Line: event.Line, Boss: &pb.SideBoss{Uuid: proto.String(event.ID)}}}
	terminal := func(deltas map[string]float64) *model.CombatFinalHPOutcome {
		out := &model.CombatFinalHPOutcome{}
		for _, actor := range monster.Formation {
			out.FinalActors = append(out.FinalActors, model.CombatFinalActorHP{Camp: 1, GUID: actor.GetGuid(), MaxHP: actor.GetProp().GetHp(), CurrentHP: actor.GetInst().GetCurHp() - deltas[actor.GetGuid()]})
		}
		return out
	}
	first := terminal(map[string]float64{leader + "-3": 573045, leader + "-7": 100})
	query := func() (db.SharedFieldBoss55, map[string]float64) {
		t.Helper()
		var row db.SharedFieldBoss55
		if err := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, event.Line).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		_, _, hp, err := ggoBossDecode(&row)
		if err != nil {
			t.Fatal(err)
		}
		return row, hp
	}
	before, _ := query()
	forced := errors.New("rollback attendant damage")
	apply := func(result *model.CombatFinalHPOutcome, rollback bool) (float64, error) {
		var damage float64
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, damage, err = f.s.ggoBossApplyDamageLocked(tx, f.id, fight, input, result, time.Now().UnixMilli())
			if err != nil {
				return err
			}
			if rollback {
				return forced
			}
			return nil
		})
		return damage, err
	}
	if _, err := apply(first, true); !errors.Is(err, forced) {
		t.Fatal("attendant-only rollback", err)
	}
	afterRollback, _ := query()
	if !bytes.Equal(before.HP, afterRollback.HP) || !bytes.Equal(before.GGOEvent, afterRollback.GGOEvent) {
		t.Fatal("rolled-back attendant damage changed event")
	}
	if damage, err := apply(first, false); err != nil || damage != 0 {
		t.Fatal("attendant-only commit", damage, err)
	}
	committed, hp := query()
	if hp[leader] != float64(event.Boss.HP) || hp[leader+"-3"] != 0 || hp[leader+"-7"] != 573045-100 || hp[leader+"-9"] != 573045 {
		t.Fatal("attendant damage not persisted independently", hp)
	}
	var legacyHP map[string]float64
	var legacyEvent struct {
		Source string                  `json:"source"`
		ID     string                  `json:"id"`
		Boss   model.AdminFieldGGOBoss `json:"boss"`
	}
	if json.Unmarshal(committed.HP, &legacyHP) != nil || len(legacyHP) != 1 || legacyHP[leader] != float64(event.Boss.HP) || json.Unmarshal(committed.GGOEvent, &legacyEvent) != nil || legacyEvent.ID != event.ID || legacyEvent.Boss.HP != event.Boss.HP {
		t.Fatal("old server cannot read extended event")
	}
	if count := p1Count(t, &db.FieldBossContribution73{}, "server_id=? AND spawn_uuid=? AND player_id=?", f.s.OriginalServerID(), event.ID, f.id); count != 0 {
		t.Fatal("attendant damage created boss contribution", count)
	}
	member := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: event.ID, PlayerID: f.id, Rules: []byte("{}"), DailyLimit: 3}
	if err := db.GetIns().Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	// This input was frozen before the first result, so its damage is merged
	// against current HP; an already-dead attendant cannot become alive again.
	second := terminal(map[string]float64{leader: 1000, leader + "-3": 573045, leader + "-7": 200, leader + "-9": 300})
	if damage, err := apply(second, false); err != nil || damage != 1000 {
		t.Fatal("overlapping frozen result", damage, err)
	}
	f.restart(t)
	_, hp = query()
	if hp[leader] != float64(event.Boss.HP)-1000 || hp[leader+"-3"] != 0 || hp[leader+"-7"] != 573045-300 || hp[leader+"-9"] != 573045-300 {
		t.Fatal("cold or overlapping event health", hp)
	}
	if err = db.GetIns().First(&member, member.ID).Error; err != nil || member.Damage != 1000 {
		t.Fatal("only leader damage contributes", member.Damage, err)
	}
	var otherAfter db.SharedFieldBoss55
	if err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 2).Take(&otherAfter).Error; err != nil || !bytes.Equal(other.HP, otherAfter.HP) {
		t.Fatal("other line changed", err)
	}
	start, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal("native next challenge", err)
	}
	list := start[0].(*pb.SCSidePveBossFightStart).GetBattleMonsterList()
	if len(list) != 1 || len(list[0].Formation) != 4 {
		t.Fatal("native formation changed", list)
	}
	for _, actor := range list[0].Formation {
		if actor.GetInst().GetCurHp() != hp[actor.GetGuid()] {
			t.Fatal("native start ignored event health", actor.GetGuid())
		}
	}
	_, game := f.profile(t)
	frozen, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	end, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, frozen, nil)
	if err != nil {
		t.Fatal("zero-HP attendant original engine", err)
	}
	for _, actor := range end.FinalActors {
		if actor.Camp == 1 && actor.GUID == leader+"-3" && actor.CurrentHP != 0 {
			t.Fatal("zero-HP attendant revived", actor)
		}
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start[0].(*pb.SCSidePveBossFightStart).FightId, Uuid: proto.String(event.ID), Result: proto.Int32(0), OptRecord: end.OriginalOperates}
	if end.Outcome.Winner == 0 {
		request.Result = proto.Int32(1)
	}
	if _, err = f.s.fieldBossFightEnd(ctx, f.id, request); err != nil {
		t.Fatal("native zero-HP formation settlement", err)
	}
	_, finalHP := query()
	if finalHP[leader+"-3"] != 0 {
		t.Fatal("native settlement restored defeated attendant", finalHP)
	}
}

func TestGGOBossDefeatReceiptUsesActualPositionWithoutChips(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, session, event := ggoBossIntegrationFixture(t)
	messages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal(err)
	}
	start := messages[0].(*pb.SCSidePveBossFightStart)
	if err = updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.GGOField.Chips = [6]int64{}
		game.GGOField.PositionRevision = 7
		fight := game.Field.Fight
		input, err := fieldBossFrozenInput(fight)
		if err != nil {
			return err
		}
		for _, hero := range input.Attacker.Formation {
			hero.Inst.CurHp = proto.Float64(1)
		}
		for _, enemy := range input.Monster.Formation {
			enemy.Prop.Att = proto.Float64(1e9)
		}
		input.MaxRound = 20
		fight.BossStart.Maxround, fight.Start.Maxround = proto.Int32(20), proto.Int32(20)
		fight.BossStart.BattlePlayer, fight.Start.BattlePlayer = input.Attacker, input.Attacker
		fight.BossStart.BattleMonsterList, fight.Start.BattleMonsterList = []*pb.BattleMonster{input.Monster}, []*pb.BattleMonster{input.Monster}
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		fight.BossInput, fight.BossInputStored, fight.BossInputHash = input, nil, ""
		fight.GGOBoss.InputHash = p6Digest(raw)
		return nil
	}); err != nil {
		t.Fatal("prepare finite death fixture", err)
	}
	_, before := f.profile(t)
	input, err := fieldBossFrozenInput(before.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	var remaining float64
	for _, actor := range terminal.FinalActors {
		if actor.Camp == 0 {
			remaining += actor.CurrentHP
		}
	}
	if remaining != 0 {
		t.Fatal("fixture did not produce an actual defeat", remaining)
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(0), OptRecord: terminal.OriginalOperates}
	result, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil {
		t.Fatal("native defeat", err)
	}
	_, after := f.profile(t)
	if after.GGOField.Chips != ([6]int64{}) || after.GGOField.PositionRevision != 8 || !after.PositionLoading {
		t.Fatal("zero-chip defeat did not save safe position")
	}
	read := func() *model.GGOFieldBossBattleReceipt {
		t.Helper()
		value, err := f.s.GGOFieldBossReceipt(ctx, model.GGOFieldBossReceiptRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}, FightID: start.GetFightId()}, session)
		if err != nil {
			t.Fatal("read receipt", err)
		}
		return value
	}
	receipt := read()
	p := after.Position.GetCurrent()
	if !receipt.Defeated || receipt.Lost != ([6]int64{}) || receipt.SafeReturn == nil || receipt.PositionRevision != 8 || *receipt.SafeReturn.X != float64(p.GetX()) || *receipt.SafeReturn.Y != float64(p.GetY()) || *receipt.SafeReturn.Z != float64(p.GetZ()) {
		t.Fatal("defeat receipt does not match committed safe position", receipt)
	}
	raw, _ := json.Marshal(receipt)
	if !bytes.Contains(raw, []byte(`"defeated":true`)) || !bytes.Contains(raw, []byte(`"safeReturn"`)) || !bytes.Contains(raw, []byte(`"positionRevision":8`)) {
		t.Fatal("death JSON contract", string(raw))
	}
	f.restart(t)
	replay, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || !proto.Equal(result[0], replay[0]) {
		t.Fatal("cold native receipt", err)
	}
	cold := read()
	if !reflect.DeepEqual(receipt, cold) || math.Abs(cold.Damage-receipt.Damage) > 0.001 {
		t.Fatal("cold death receipt changed", receipt, cold)
	}
}

// A seeded positive contribution and an explicit freeze model one event death;
// the normal recipient transaction then grants actual resources and chips once.
func TestGGOBossRewardStatusFollowsDurableDelivery(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, session, event := ggoBossIntegrationFixture(t)
	fightID := "reward-status-fixture"
	receipt := model.GGOFieldBossBattleReceipt{State: "settled", FightID: fightID, EventID: event.ID, InputHash: "fixture-input", Damage: 100}
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return ggoSaveReceipt(tx, f.id, ggoSystemReceiptID("boss-fight", fightID), constUtil.GGOFieldBossFightOperation, "fixture", event.EconomyVersion, &fightID, [6]int64{}, 0, time.Now().UnixMilli(), receipt)
	}); err != nil {
		t.Fatal(err)
	}
	readID := func(id string) *model.GGOFieldBossBattleReceipt {
		t.Helper()
		out, err := f.s.GGOFieldBossReceipt(ctx, model.GGOFieldBossReceiptRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}, FightID: id}, session)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	read := func() *model.GGOFieldBossBattleReceipt { return readID(fightID) }
	if got := read(); got.RewardStatus != "no_contribution" {
		t.Fatal("empty contribution", got)
	}
	rules, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	member := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: event.ID, PlayerID: f.id, Damage: float64(event.Boss.HP) * .04, Rules: rules, DailyLimit: 3}
	if err = db.GetIns().Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	if got := read(); got.RewardStatus != "pending_boss_defeat" || got.Damage != 100 || len(got.RewardResources) != 0 || got.RewardChips != nil {
		t.Fatal("surviving boss must show pending without prizes", got)
	}
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error { return f.s.ggoBossFreezeAwardsLocked(tx, event, f.id+1000) }); err != nil {
		t.Fatal("freeze death award", err)
	}
	if got := read(); got.RewardStatus != "pending_delivery" || len(got.RewardResources) != 0 || got.RewardChips != nil {
		t.Fatal("frozen award incorrectly shown as delivered", got)
	}
	beforePlayer, beforeGame := f.profile(t)
	if gain, err := f.s.CollectFieldBossPendingRewards(ctx, f.id); err != nil || gain == nil {
		t.Fatal("normal award delivery", gain, err)
	}
	afterPlayer, afterGame := f.profile(t)
	got := read()
	amount := map[string]int64{}
	for _, item := range got.RewardResources {
		amount[item.ID] = item.Amount
	}
	if got.RewardStatus != "delivered" || got.RewardChips == nil || *got.RewardChips != ([6]int64{0, 0, 2, 1, 0, 0}) || amount["diamond"] != 2000 || amount["coin"] != 8000000 || amount[constUtil.GGOOverclockID] < 1 {
		t.Fatal("actual awarded contents", got)
	}
	if afterPlayer.GetDiamond()-beforePlayer.GetDiamond() != 2000 || afterPlayer.GetCoin()-beforePlayer.GetCoin() != 8000000 || afterGame.GGOField.Chips[2]-beforeGame.GGOField.Chips[2] != 2 || afterGame.GGOField.Chips[3]-beforeGame.GGOField.Chips[3] != 1 {
		t.Fatal("claimed resources differ from receipt")
	}
	if gain, err := f.s.CollectFieldBossPendingRewards(ctx, f.id); err != nil || gain != nil {
		t.Fatal("repeat collection", gain, err)
	}
	// The read projection does not modify the frozen battle/death record.
	var stored db.GGOFieldReceipt
	if err = db.GetIns().Where("player_id=? AND request_id=?", f.id, ggoSystemReceiptID("boss-fight", fightID)).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	var saved model.GGOFieldBossBattleReceipt
	if err = json.Unmarshal(stored.Response, &saved); err != nil || saved.RewardStatus != "" || saved.Damage != 100 {
		t.Fatal("read changed frozen fight", saved, err)
	}
	limitedEvent := *event
	limitedEvent.ID += "-limited"
	limitedFight := "reward-limited-fixture"
	limitedRules, _ := json.Marshal(limitedEvent)
	limited := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: limitedEvent.ID, PlayerID: f.id, Damage: 100, Rules: limitedRules, DailyLimit: 0}
	if err = db.GetIns().Create(&limited).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := f.s.ggoBossFreezeAwardsLocked(tx, &limitedEvent, f.id+1000); err != nil {
			return err
		}
		return ggoSaveReceipt(tx, f.id, ggoSystemReceiptID("boss-fight", limitedFight), constUtil.GGOFieldBossFightOperation, "fixture", event.EconomyVersion, &limitedFight, [6]int64{}, 0, time.Now().UnixMilli(), model.GGOFieldBossBattleReceipt{State: "settled", FightID: limitedFight, EventID: limitedEvent.ID, InputHash: "fixture-input"})
	}); err != nil {
		t.Fatal(err)
	}
	if gain, err := f.s.CollectFieldBossPendingRewards(ctx, f.id); err != nil || gain != nil {
		t.Fatal("daily limit collection", gain, err)
	}
	if got = readID(limitedFight); got.RewardStatus != "daily_limit" || len(got.RewardResources) != 0 || got.RewardChips != nil {
		t.Fatal("daily quota status", got)
	}
}
