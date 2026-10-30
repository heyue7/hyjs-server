package service

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Result and original protocol reply share the same immutable business receipt.
// The deterministic request digest prevents a different end payload from being
// accepted under an already settled fight ID.
func ggoMonsterEndHash(request *pb.CSSidePveFightEnd) (string, error) {
	if request == nil || request.GetFightId() == "" {
		return "", errors.New("沙漠小怪结算请求无效")
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		return "", err
	}
	return p6Digest(raw), nil
}

func ggoMonsterRecordedEnd(tx *gorm.DB, id int64, request *pb.CSSidePveFightEnd) (*pb.SCSidePveFightEnd, bool, error) {
	var saved model.GGOFieldMonsterReceipt
	found, err := ggoFindBusinessReceipt(tx, id, constUtil.GGOFieldOpMonsterResult, request.GetFightId(), &saved)
	if err != nil || !found {
		return nil, found, err
	}
	hash, err := ggoMonsterEndHash(request)
	if err != nil {
		return nil, true, err
	}
	if saved.InputHash == "" || saved.EndRequestHash == "" || saved.EndRequestHash != hash || saved.FightID != request.GetFightId() || len(saved.NativeEnd) == 0 {
		return nil, true, ggoFieldError(constUtil.GGOFieldRequestConflict, "战斗结果与原始回执不一致")
	}
	out := &pb.SCSidePveFightEnd{}
	if err = proto.Unmarshal(saved.NativeEnd, out); err != nil || out.GetCode() != 1 {
		return nil, true, errors.New("沙漠战斗原始回执损坏")
	}
	return out, true, nil
}

func ggoMonsterNativeGain(receipt *model.GGOFieldMonsterReceipt) (*pb.ResultObject, error) {
	out := &pb.ResultObject{}
	for _, item := range receipt.Resources {
		if item.Amount < 1 || item.Amount > math.MaxInt32 {
			return nil, errors.New("沙漠小怪回执数量无效")
		}
		out.Simps = append(out.Simps, &pb.SimpleItem{Id: proto.String(item.ID), Num: proto.Int32(int32(item.Amount))})
	}
	out.Wars = append(out.Wars, receipt.Warriors...)
	return out, nil
}

// Validate the original client formation against the authoritative type-6
// replay, not only its submitted total HP and victory flag.
func ggoMonsterReportedHP(playerID int64, reported *pb.BattlePlayer, frozen *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome) error {
	if reported == nil || reported.GetPlayerId() != playerID || frozen == nil || frozen.Attacker == nil || terminal == nil || terminal.Outcome == nil {
		return errors.New("沙漠小怪战斗参与者无效")
	}
	byGUID := map[string]model.CombatFinalActorHP{}
	for _, actor := range terminal.FinalActors {
		if actor.Camp == 0 {
			if _, duplicate := byGUID[actor.GUID]; duplicate {
				return errors.New("沙漠战斗成员重复")
			}
			byGUID[actor.GUID] = actor
		}
	}
	if len(reported.GetFormation()) != len(frozen.Attacker.GetFormation()) || len(byGUID) != len(frozen.Attacker.GetFormation()) {
		return errors.New("沙漠战斗成员数量无效")
	}
	seen := map[string]bool{}
	for _, hero := range reported.GetFormation() {
		actual, ok := byGUID[hero.GetGuid()]
		hp := hero.GetInst().GetCurHp()
		position := actual.Pose
		if actual.RuntimePose != 0 {
			position = actual.RuntimePose
		}
		if !ok || seen[hero.GetGuid()] || hero.GetId() != actual.HeroID || hero.GetPose() != position || math.IsNaN(hp) || math.IsInf(hp, 0) || math.Abs(hp-actual.CurrentHP) > math.Max(0.01, actual.MaxHP*0.00001) {
			return errors.New("沙漠战斗成员结果不一致")
		}
		seen[hero.GetGuid()] = true
	}
	return nil
}

// Field PVE sends each actor's final MainPosition. Keep the native HP-per-slot
// projection aligned with that final position after the common HP/anger checks.
func ggoMonsterApplyTerminal(party *pb.BattlePlayer, side *pb.PlayerSide, terminal *model.CombatFinalHPOutcome) error {
	if err := p12FieldPVPApplyTerminal(party, side, terminal, 0); err != nil {
		return err
	}
	actors := map[string]model.CombatFinalActorHP{}
	for _, actor := range terminal.FinalActors {
		if actor.Camp == 0 {
			actors[actor.GUID] = actor
		}
	}
	positions := map[int32]bool{}
	side.BattlePosHps = nil
	side.BattlePos3, side.BattlePos7, side.BattlePos9, side.BattlePos13 = nil, nil, nil, nil
	for _, hero := range party.Formation {
		actor, ok := actors[hero.GetGuid()]
		pos := actor.Pose
		if actor.RuntimePose != 0 {
			pos = actor.RuntimePose
		}
		if !ok || pos < 1 || pos > 15 || positions[pos] || actor.MaxHP <= 0 {
			return errors.New("沙漠战斗最终阵位无效")
		}
		positions[pos] = true
		ratio := actor.CurrentHP / actor.MaxHP
		side.BattlePosHps = append(side.BattlePosHps, &pb.BattlePosHp{BattlePos: proto.Int32(pos), HpPercent: proto.Float64(ratio)})
		switch pos {
		case 3:
			side.BattlePos3 = proto.Float64(ratio)
		case 7:
			side.BattlePos7 = proto.Float64(ratio)
		case 9:
			side.BattlePos9 = proto.Float64(ratio)
		case 13:
			side.BattlePos13 = proto.Float64(ratio)
		}
	}
	return nil
}

func (s *GameService) ggoPVEDeathLocked(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, frozen *model.GGOFieldMonsterFight, now int64) ([6]int64, error) {
	var zero [6]int64
	if frozen == nil || g.GGOField == nil || g.Field == nil || g.Field.Side == nil {
		return zero, errors.New("沙漠死亡状态缺失")
	}
	lost, err := ggoDeathLoss(g.GGOField.Chips, frozen.ProtectedChips, frozen.DeathLossPercent, nil)
	if err != nil {
		return zero, err
	}
	var count int64
	for _, n := range lost {
		count += n
	}
	if count > 0 {
		if g.GGOField.Revision == math.MaxInt64 {
			return zero, errors.New("沙漠终端版本溢出")
		}
		if err = ggoApplyChipTransfer(&g.GGOField.Chips, nil, lost); err != nil {
			return zero, err
		}
		g.GGOField.Revision++
	}
	point, err := ggoWirePoint(frozen.SafeReturn)
	if err != nil {
		return zero, err
	}
	if math.IsNaN(frozen.HealRatio) || math.IsInf(frozen.HealRatio, 0) || frozen.HealRatio <= 0 || frozen.HealRatio > 1 {
		return zero, errors.New("沙漠返回恢复值无效")
	}
	if err = s.p8Heal(p, g, frozen.HealRatio, true); err != nil {
		return zero, err
	}
	if err = ggoPositionBump(g); err != nil {
		return zero, err
	}
	g.Position.Current = point
	g.PositionLoading = true
	g.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
	g.Field.Side.Anger = proto.Float64(0)
	state, err := p8LifeState(tx, id)
	if err != nil {
		return zero, err
	}
	state.DeathAt = 0
	state.SafeRecoveryAt = now
	state.InvincibleUntil = 0
	state.InSafe = true
	state.LastFightID = frozen.FightID
	if err = tx.Save(state).Error; err != nil {
		return zero, err
	}
	var delta [6]int64
	for i, n := range lost {
		delta[i] = -n
	}
	key := frozen.FightID
	if err = ggoSaveReceipt(tx, id, ggoSystemReceiptID("pve-loss", key), constUtil.GGOFieldOpPVELoss, p6Digest([]byte(key)), g.GGOField.EconomyVersion, &key, delta, 0, now, map[string]any{"fightId": key, "lost": lost}); err != nil {
		return zero, err
	}
	return lost, nil
}

// ggoMonsterEndLocked derives a server result, then commits the public node,
// profile and frozen reward receipt under one database transaction.
func (s *GameService) ggoMonsterEndLocked(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, b *pb.CSSidePveFightEnd) (*pb.SCSidePveFightEnd, *pb.PlayerSide, bool, error) {
	if g.Field == nil || g.Field.Fight == nil || g.Field.Fight.Start == nil || g.Field.Fight.Start.GetFightId() != b.GetFightId() {
		return nil, nil, false, errors.New("未知沙漠小怪战斗")
	}
	if _, err := ggoNativeProfileSession(tx, id); err != nil {
		return nil, nil, false, err
	}
	f := g.Field
	fight := f.Fight
	if fight.Settled {
		if fight.Result == nil {
			return nil, nil, false, errors.New("沙漠小怪结算回执缺失")
		}
		return fight.Result, f.Side, false, nil
	}
	if g.GGOField == nil || g.GGOField.MonsterFight == nil || g.GGOField.MonsterFight.FightID != b.GetFightId() || b.GetResult() != 0 && b.GetResult() != 1 {
		return nil, nil, false, errors.New("沙漠小怪结算参数无效")
	}
	frozenFight := *g.GGOField.MonsterFight
	if g.Position == nil || g.Position.GetScene() != constUtil.GGOFieldScene || f.Scene != constUtil.GGOFieldScene {
		return nil, nil, false, errors.New("沙漠小怪战斗场景不一致")
	}
	input, err := fieldBossFrozenInput(fight)
	if err != nil {
		return nil, nil, false, err
	}
	inputHash := fight.BossInputHash
	if inputHash == "" {
		raw, e := json.Marshal(input)
		if e != nil {
			return nil, nil, false, e
		}
		inputHash = p6Digest(raw)
	}
	if input.BattleType != 6 || input.Monster == nil || len(input.Monster.Formation) != 1 {
		return nil, nil, false, errors.New("沙漠小怪冻结战斗无效")
	}
	if _, err = s.ValidateP6GenericMechanisms(tx.Statement.Context, input); err != nil {
		return nil, nil, false, err
	}
	terminal, expected, err := s.ggoBattleTerminal(tx.Statement.Context, input, b.GetOptRecord(), b.GetResult(), b.GetBattlePlayer(), id, true)
	if err != nil {
		return nil, nil, false, err
	}
	if err = ggoMonsterApplyTerminal(input.Attacker, f.Side, terminal); err != nil {
		return nil, nil, false, err
	}
	now := time.Now().UnixMilli()
	var line db.GGOFieldLine
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&line, "server_id=? AND line=?", s.OriginalServerID(), frozenFight.Line).Error; err != nil {
		return nil, nil, false, err
	}
	var shared ggoLineState
	if json.Unmarshal(line.Payload, &shared) != nil {
		return nil, nil, false, errors.New("沙漠场景状态损坏")
	}
	node, detached, err := ggoResolveMonsterReservation(&shared, &frozenFight, id, now)
	if err != nil {
		return nil, nil, false, ggoFieldError(constUtil.GGOFieldStaleGeneration, "沙漠小怪战斗占用已失效")
	}
	monster := fieldFind(g, frozenFight.MonsterUUID)
	if monster == nil {
		return nil, nil, false, errors.New("沙漠小怪战斗投影缺失")
	}
	node.ReservedBy = 0
	node.FightID = ""
	node.ReservedUntilMS = 0
	receipt := &model.GGOFieldMonsterReceipt{FightID: frozenFight.FightID, Status: "settled", DeliveryStatus: "lost", Resources: []model.GGOFieldResourceAmount{}, Warriors: []*pb.Warrior{}, CreditBalance: ggoCreditBalance(g)}
	var gain *pb.ResultObject
	needRefresh := false
	if expected == 1 {
		if frozenFight.CooldownMS < 1 || now > math.MaxInt64-frozenFight.CooldownMS {
			return nil, nil, false, errors.New("沙漠小怪重生时间无效")
		}
		node.RefreshAtMS = now + frozenFight.CooldownMS
		if detached && node.PointID != "" && node.PatrolEpochMS > 0 {
			node.PatrolPausedAtMS = now
		}
		monster.State = proto.Int32(3)
		monster.CurHp = proto.Float64(0)
		// Daily ordinary rewards stay shared across all original field maps.
		if err = s.refreshField(id, g); err != nil {
			return nil, nil, false, err
		}
		f = g.Field
		limit, e := s.fieldConfigInt("MaxSideMonsterKillTimesReward")
		if e != nil {
			return nil, nil, false, e
		}
		scaled := ""
		if int64(f.RewardKills) < limit {
			var next map[string]int64
			scaled, next, e = ggoScaleMonsterReward(frozenFight.Reward, frozenFight.RewardNumerator, frozenFight.RewardDenominator, g.GGOField.MonsterRemainder)
			if e != nil {
				return nil, nil, false, e
			}
			g.GGOField.MonsterRemainder = next
			f.RewardKills++
		}
		pending := ggoPendingMonster{FightID: frozenFight.FightID, Reward: scaled, Chips: frozenFight.Chips, EconomyVersion: frozenFight.EconomyVersion, ExchangeValues: frozenFight.ExchangeValues}
		delivered, full, e := s.ggoMonsterAward(p, g, &pending)
		if e != nil {
			return nil, nil, false, e
		}
		// GrantFixedReward may replace the decoded game with its checked clone.
		f = g.Field
		fight = f.Fight
		monster = fieldFind(g, frozenFight.MonsterUUID)
		if full {
			receipt.DeliveryStatus = "pending_delivery"
			receipt.TerminalRevision = g.GGOField.Revision
			receipt.CreditBalance = ggoCreditBalance(g)
			key := frozenFight.FightID
			if e = ggoSaveReceipt(tx, id, ggoSystemReceiptID("monster-ready", key), constUtil.GGOFieldOpMonsterReady, p6Digest([]byte(key)), frozenFight.EconomyVersion, &key, [6]int64{}, 0, now, pending); e != nil {
				return nil, nil, false, e
			}
		} else {
			receipt = delivered
		}
		gain, e = ggoMonsterNativeGain(receipt)
		if e != nil {
			return nil, nil, false, e
		}
	} else {
		if err = ggoPatrolResume(node, now); err != nil {
			return nil, nil, false, err
		}
		monster.State = proto.Int32(1)
		monster.CurHp = proto.Float64(1)
		if f.Side.GetCurHp() <= 0 {
			var lost [6]int64
			lost, err = s.ggoPVEDeathLocked(tx, id, p, g, &frozenFight, now)
			if err != nil {
				return nil, nil, false, err
			}
			receipt.Lost = lost
			receipt.TerminalRevision = g.GGOField.Revision
			needRefresh = true
		}
		gain = &pb.ResultObject{}
	}
	if receipt.CreditBalance < 0 {
		return nil, nil, false, errors.New("信用余额无效")
	}
	if receipt.DeliveryStatus == "lost" {
		receipt.TerminalRevision = g.GGOField.Revision
	}
	if err = ggoLineSave(tx, &line, &shared, now); err != nil {
		return nil, nil, false, err
	}
	key := frozenFight.FightID
	delta := [6]int64{}
	if receipt.DeliveryStatus == "delivered" {
		delta = receipt.Chips
	}
	side := g.Field.Side
	result := &pb.SCSidePveFightEnd{Getn: gain, FightResult: proto.Int32(expected), Code: proto.Int32(1), Anger: side.Anger, State: side.State, Monster: monster}
	for _, hp := range side.GetBattlePosHps() {
		result.BattlePos = append(result.BattlePos, hp.GetBattlePos())
		result.Hps = append(result.Hps, hp.GetHpPercent())
		switch hp.GetBattlePos() {
		case 3:
			result.Hp3 = hp.HpPercent
		case 7:
			result.Hp7 = hp.HpPercent
		case 9:
			result.Hp9 = hp.HpPercent
		case 13:
			result.Hp13 = hp.HpPercent
		}
	}
	if needRefresh {
		result.State = proto.Int32(constUtil.P8FieldDead)
	}
	endHash, err := ggoMonsterEndHash(b)
	if err != nil {
		return nil, nil, false, err
	}
	nativeEnd, err := proto.MarshalOptions{Deterministic: true}.Marshal(result)
	if err != nil {
		return nil, nil, false, err
	}
	stored := *receipt
	stored.InputHash, stored.EndRequestHash, stored.NativeEnd = inputHash, endHash, nativeEnd
	if err = ggoSaveReceipt(tx, id, ggoSystemReceiptID("monster-result", key), constUtil.GGOFieldOpMonsterResult, endHash, frozenFight.EconomyVersion, &key, delta, 0, now, stored); err != nil {
		return nil, nil, false, err
	}
	fight.Settled = true
	fight.Result = result
	fight.BossInput = nil
	fight.BossInputStored = nil
	fight.BossInputHash = ""
	g.GGOField.MonsterFight = nil
	return result, side, needRefresh, nil
}
