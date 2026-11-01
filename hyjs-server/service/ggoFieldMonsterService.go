package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type ggoNativeSessionKey struct{}

func ggoNativeProfileSession(tx *gorm.DB, id int64) (*db.Profile, error) {
	var row db.Profile
	if err := tx.First(&row, id).Error; err != nil {
		return nil, err
	}
	session, ok := tx.Statement.Context.Value(ggoNativeSessionKey{}).([]byte)
	if !ok || len(session) != 16 || subtle.ConstantTimeCompare(session, row.Session) != 1 {
		return nil, ggoFieldError(constUtil.GGOFieldInvalidLocation, "游戏会话已变化")
	}
	return &row, nil
}

func ggoMonsterSide(node *ggoMonsterNode, point *model.AdminFieldGGOPoint, now int64) (*pb.SideMonster, error) {
	if node == nil || point == nil {
		return nil, errors.New("沙漠小怪点位缺失")
	}
	shared, err := ggoPatrolPosition(node, point, now)
	if err != nil {
		return nil, err
	}
	position, err := ggoWirePoint(shared)
	if err != nil {
		return nil, err
	}
	state := int32(1)
	if node.ReservedBy != 0 {
		state = 2
	}
	if node.RefreshAtMS > 0 {
		state = 3
	}
	hp := float64(1)
	if state == 3 {
		hp = 0
	}
	return &pb.SideMonster{Uuid: proto.String(node.UUID), MonsterPosId: proto.String(node.PointID), MonId: proto.String(node.MonID), HeroId: proto.String(node.HeroID), CurrentPoint: position, State: proto.Int32(state), CurHp: proto.Float64(hp)}, nil
}

// The caller already holds the profile row. Lock the current presence before
// taking the shared line row, matching gather and PvP lock ordering.
func (s *GameService) ggoMonsterLineLocked(tx *gorm.DB, id int64, g *model.GameState, now int64, targetUUID ...string) (*db.GGOFieldLine, *ggoLineState, error) {
	if s.GGOFieldConfig == nil || g.GGOField == nil {
		return nil, nil, ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠搜打撤暂未开放")
	}
	profile, err := ggoNativeProfileSession(tx, id)
	if err != nil {
		return nil, nil, err
	}
	if err = ggoCheckClientBuildLocked(tx, id, profile.Session, s.GGOFieldConfig); err != nil {
		return nil, nil, err
	}
	actor := &ggoFieldActor{Service: s, Row: profile, Game: g, Now: now}
	if err := ggoCurrentPresence(tx, actor, profile.Session); err != nil {
		return nil, nil, err
	}
	var pointIDs []string
	if len(targetUUID) > 0 {
		var target db.GGOFieldPoint
		if err := tx.Select("point_id").Where("server_id=? AND line=? AND monster_uuid=?", s.OriginalServerID(), actor.Presence.Line, targetUUID[0]).First(&target).Error; err != nil {
			return nil, nil, ggoFieldError(constUtil.GGOFieldBusy, "小怪已刷新，请重试")
		}
		pointIDs = []string{target.PointID}
	}
	if len(pointIDs) == 0 {
		row, state, _, err := ggoReadLineSnapshot(tx, s.OriginalServerID(), int64(actor.Presence.Line), s.GGOFieldConfig, s.GGOFieldRevision, now)
		return row, state, err
	}
	row, state, _, err := ggoLoadLine(tx, s.OriginalServerID(), int64(actor.Presence.Line), s.GGOFieldConfig, s.GGOFieldRevision, now, pointIDs...)
	return row, state, err
}

func (s *GameService) ggoMonsterCheckLocked(tx *gorm.DB, id int64, g *model.GameState, uuid string) (*pb.SCSideMonsterCheck, error) {
	now := time.Now().UnixMilli()
	_, state, err := s.ggoMonsterLineLocked(tx, id, g, now)
	if err != nil {
		return nil, err
	}
	now = time.Now().UnixMilli()
	result := &pb.SCSideMonsterCheck{Code: proto.Int32(2003), Monster: &pb.SideMonster{Uuid: proto.String(uuid), State: proto.Int32(3), CurHp: proto.Float64(0)}}
	for i := range state.Monsters {
		node := &state.Monsters[i]
		if node.UUID != uuid {
			continue
		}
		for j := range s.GGOFieldConfig.Runtime.Points {
			point := &s.GGOFieldConfig.Runtime.Points[j]
			if point.ID != node.PointID {
				continue
			}
			result.Monster, err = ggoMonsterSide(node, point, now)
			if err != nil {
				return nil, err
			}
			if node.RefreshAtMS == 0 && !ggoLegacyBlocked(state, node.PointID) && (node.ReservedBy == 0 || node.ReservedBy == id) {
				result.Code = proto.Int32(1)
			}
			return result, nil
		}
		return nil, errors.New("沙漠小怪点位配置缺失")
	}
	return result, nil
}

// ggoMonsterBattleFormation uses original main-story encounter compositions,
// while retaining the desert tier's per-member statistics. The resulting
// formation is frozen at start for the lifetime of the accepted fight.
func (s *GameService) ggoMonsterBattleFormation(node *ggoMonsterNode, tier *model.AdminFieldGGOMobTier) (*pb.BattleMonster, error) {
	if node == nil || tier == nil || node.UUID == "" || node.MonID == "" || node.HeroID != tier.HeroID || node.Level != tier.Level || node.MonID != ggoMonsterGroup(node.HeroID) {
		return nil, errors.New("沙漠小怪档位无效")
	}
	leader := s.fieldLeaderInScene(node.MonID, constUtil.GGOFieldScene)
	if leader == nil {
		return nil, errors.New("沙漠小怪原模型或巡逻组缺失")
	}
	// CopyEnemy tasks 123, 208 and 200 supply these positions and members. Keep
	// the small catalogue here so starting a fight does not scan the full table.
	type member struct {
		hero string
		pose int32
	}
	var members []member
	switch node.HeroID {
	case "monster_026":
		members = []member{{"monster_026", 3}, {"monster_029", 7}, {"monster_029", 9}, {"monster_031", 13}}
	case "monster_032":
		members = []member{{"monster_032", 3}, {"monster_031", 7}, {"monster_031", 9}, {"monster_027", 13}}
	case "monster_031":
		members = []member{{"monster_031", 3}, {"monster_029", 9}, {"monster_029", 7}, {"monster_027", 13}}
	}
	monster := &pb.BattleMonster{MonsterId: proto.Int64(1), ServerId: proto.Int32(1), TaskId: proto.String(node.MonID), GroupIndex: proto.Int32(1), Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
	seen := map[int32]bool{}
	models := map[string]bool{}
	matchedModel := false
	for _, m := range members {
		pose, heroID := m.pose, m.hero
		hero := s.table("Hero", "id", heroID)
		if (pose != 3 && pose != 7 && pose != 9 && pose != 13) || seen[pose] || hero == nil || hero["resName"] == "" {
			return nil, errors.New("沙漠小怪主线阵容无效")
		}
		seen[pose] = true
		models[heroID] = true
		matchedModel = matchedModel || heroID == node.HeroID
		enemyRow := map[string]string{}
		for key, value := range leader {
			enemyRow[key] = value
		}
		enemyRow["HeroId"], enemyRow["Level"], enemyRow["Attack"], enemyRow["Hp"], enemyRow["PhyDefend"], enemyRow["MagicDefend"] = heroID, fmt.Sprint(tier.Level), fmt.Sprint(tier.Attack), fmt.Sprint(tier.HP), fmt.Sprint(tier.PhyDefend), fmt.Sprint(tier.MagicDefend)
		for _, key := range []string{"SkillId1", "SkillId2", "UltSkillId"} {
			enemyRow[key] = hero[key]
		}
		for i := 0; i < 6; i++ {
			delete(enemyRow, fmt.Sprintf("Talent%d", i))
		}
		actor, err := s.actor(enemyRow, fmt.Sprintf("ggo-%s-%d", node.UUID, pose), pose, true)
		if err != nil {
			return nil, err
		}
		monster.Formation = append(monster.Formation, actor)
	}
	if len(monster.Formation) != 4 || len(models) < 2 || !matchedModel {
		return nil, errors.New("沙漠小怪主线阵容缺失")
	}
	return monster, nil
}

// ggoMonsterStartLocked freezes a type-6 input and claims one shared
// generation; the caller's profile transaction owns its receipt.
// A collision happened before the start packet and any database lock wait.
// Check only the monster's authoritative recent path, with the actual player
// position and configured radius; stationary monsters receive no expansion.
func ggoMonsterRecentContact(player *pb.PointNew, current *pb.SideMonster, node *ggoMonsterNode, point *model.AdminFieldGGOPoint, now int64) (bool, error) {
	if p8Near(player, &p8GatherConfig{Point: current.GetCurrentPoint(), Radius: point.Radius}) {
		return true, nil
	}
	// Every published patrol is confined to one metre. Reject distant actors
	// before projecting historical positions inside the shared line lock.
	if !p8Near(player, &p8GatherConfig{Point: current.GetCurrentPoint(), Radius: point.Radius + 1}) {
		return false, nil
	}
	path, err := ggoPatrolPath(point)
	if err != nil || path == nil {
		return false, err
	}
	for age := int64(250); age <= 3000; age += 250 {
		position, err := ggoPatrolPosition(node, point, now-age)
		if err != nil {
			return false, err
		}
		wire, err := ggoWirePoint(position)
		if err != nil {
			return false, err
		}
		if p8Near(player, &p8GatherConfig{Point: wire, Radius: point.Radius}) {
			return true, nil
		}
	}
	return false, nil
}

func (s *GameService) ggoMonsterStartLocked(tx *gorm.DB, id int64, player *pb.Player, g *model.GameState, b *pb.CSSidePveFightStart) (*pb.SCSidePveFightStart, error) {
	if g.Field == nil || g.Field.Side == nil || g.GGOField == nil {
		return nil, ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠搜打撤暂未开放")
	}
	f := g.Field
	if _, err := ggoNativeProfileSession(tx, id); err != nil {
		return nil, err
	}
	if err := s.ggoLockExpiredAndTarget(tx, id, g, b.GetMonsterId(), time.Now().UnixMilli()); err != nil {
		return nil, err
	}
	if _, err := s.ggoExpireMonsterFightLocked(tx, id, g, time.Now().UnixMilli()); err != nil {
		return nil, err
	}
	if f.Fight != nil && !f.Fight.Settled {
		if g.GGOField.MonsterFight != nil && g.GGOField.MonsterFight.MonsterUUID == b.GetMonsterId() && f.Fight.Start != nil && f.Fight.Start.GetFightId() == g.GGOField.MonsterFight.FightID {
			return f.Fight.Start, nil
		}
		return nil, ggoFieldError(constUtil.GGOFieldBusy, "已有战斗进行中")
	}
	if s.GGOFieldConfig == nil || s.GGOFieldConfig.Runtime == nil {
		return nil, ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠搜打撤暂未开放")
	}
	now := time.Now().UnixMilli()
	if g.PositionLoading || f.Side.GetCurHp() <= 0 || g.Run != nil && !g.Run.Complete {
		return nil, ggoFieldError(constUtil.GGOFieldBusy, "当前无法战斗")
	}
	if g.GGOField.ProtectionUntil > now {
		return nil, ggoFieldError(constUtil.GGOFieldProtectionConfirmation, "确认解除入场保护后请再次操作")
	}
	if err := P12FieldPVPRequireIdle(tx, id, time.UnixMilli(now)); err != nil {
		return nil, err
	}
	if err := s.ensureNoActiveDailyCopy(tx, id, time.UnixMilli(now)); err != nil {
		return nil, err
	}
	if s.P8GatherReady {
		if err := s.P8CancelGather(tx, id, g); err != nil {
			return nil, err
		}
	}
	if err := ggoInterruptExtractionLocked(tx, id, g, now); err != nil {
		return nil, err
	}
	line, state, err := s.ggoMonsterLineLocked(tx, id, g, now, b.GetMonsterId())
	if err != nil {
		return nil, err
	}
	// Position authority is evaluated after the potentially contended line lock.
	now = time.Now().UnixMilli()
	var node *ggoMonsterNode
	for i := range state.Monsters {
		if state.Monsters[i].UUID == b.GetMonsterId() {
			node = &state.Monsters[i]
			break
		}
	}
	if node == nil || node.ReservedBy != 0 || node.RefreshAtMS != 0 || ggoLegacyBlocked(state, node.PointID) {
		return nil, ggoFieldError(constUtil.GGOFieldBusy, "小怪已被占用或刷新")
	}
	var point *model.AdminFieldGGOPoint
	for i := range s.GGOFieldConfig.Runtime.Points {
		if s.GGOFieldConfig.Runtime.Points[i].ID == node.PointID {
			point = &s.GGOFieldConfig.Runtime.Points[i]
			break
		}
	}
	if point == nil {
		return nil, errors.New("沙漠小怪点位缺失")
	}
	side, err := ggoMonsterSide(node, point, now)
	if err != nil {
		return nil, err
	}
	near, err := ggoMonsterRecentContact(g.Position.GetCurrent(), side, node, point, now)
	if err != nil {
		return nil, err
	}
	if !near {
		return nil, ggoFieldError(constUtil.GGOFieldInvalidLocation, "请靠近小怪")
	}
	var tier *model.AdminFieldGGOMobTier
	for i := range s.GGOFieldConfig.Runtime.MonsterTiers {
		candidate := &s.GGOFieldConfig.Runtime.MonsterTiers[i]
		if candidate.Level == node.Level && candidate.HeroID == node.HeroID {
			tier = candidate
			break
		}
	}
	if tier == nil || node.MonID != ggoMonsterGroup(node.HeroID) {
		return nil, errors.New("沙漠小怪档位无效")
	}
	values, err := ggoChipValues(s.GGOFieldConfig)
	if err != nil {
		return nil, err
	}
	_, held, err := ggoProtectedChips(g.GGOField.Chips, s.GGOFieldConfig.Runtime.ProtectedChips)
	if err != nil {
		return nil, err
	}
	if held > 0 && (g.GGOField.EconomyVersion != s.GGOFieldConfig.Runtime.EconomyVersion || g.GGOField.ExchangeValues != values) {
		return nil, ggoFieldError(constUtil.GGOFieldEconomyConflict, "请先结算旧版本芯片")
	}
	chips, err := ggoRollChip(s.GGOFieldConfig, tier.ChipTriggerBps)
	if err != nil {
		return nil, err
	}
	baseline := s.fieldLeaderInScene("mon_021", constUtil.GGOFieldScene)
	if baseline == nil {
		return nil, errors.New("原35级小怪奖励组缺失")
	}
	reward, err := ggoRollMonsterReward(baseline["rewardGroupId"])
	if err != nil {
		return nil, err
	}
	monster, err := s.ggoMonsterBattleFormation(node, tier)
	if err != nil {
		return nil, err
	}
	party, err := s.battlePlayer(player, g)
	if err != nil || len(party.GetFormation()) == 0 {
		return nil, errors.New("野外战斗需要可用编队")
	}
	for _, actor := range party.Formation {
		for _, hp := range f.Side.GetBattlePosHps() {
			if actor.GetPose() == hp.GetBattlePos() {
				actor.Inst.CurHp = proto.Float64(actor.GetProp().GetHp() * hp.GetHpPercent())
			}
		}
	}
	party.Inst.Anger = proto.Float64(f.Side.GetAnger())
	seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
	if err != nil {
		return nil, err
	}
	rounds, err := s.fieldConfigInt("FightMaxRoundNum")
	if err != nil {
		return nil, err
	}
	frozen, err := s.freezeAndQualifyP6GenericMechanisms(tx.Statement.Context, model.FrozenBattleInput{Attacker: party, Monster: monster, Seed: seed.Int64(), MaxRound: int32(rounds), BattleType: 6, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
	if err != nil {
		return nil, err
	}
	fightID, err := fieldToken()
	if err != nil {
		return nil, err
	}
	if tier.RespawnSeconds > math.MaxInt64/1000 {
		return nil, errors.New("沙漠小怪计时溢出")
	}
	result := &pb.SCSidePveFightStart{FightId: proto.String(fightID), RandSeed: proto.Int64(frozen.Seed), BattlePlayer: frozen.Attacker, BattleMonsterList: []*pb.BattleMonster{frozen.Monster}, FightScene: proto.String(""), Code: proto.Int32(1), Maxround: proto.Int32(frozen.MaxRound)}
	node.ReservedBy = id
	node.FightID = fightID
	path, err := ggoPatrolPath(point)
	if err != nil {
		return nil, err
	}
	if path != nil {
		node.PatrolPausedAtMS = now
	}
	// Zero means the in-flight reservation has no wall-clock expiry. A late
	// accepted result must not consume a replacement generation.
	node.ReservedUntilMS = 0
	if err = ggoLineSave(tx, line, state, now); err != nil {
		return nil, err
	}
	side.State = proto.Int32(2)
	f.Monsters = []*pb.SideMonster{side}
	f.Fight = &model.FieldFight{MonsterUUID: node.UUID, Start: result, StartedAt: now, BossInput: frozen}
	f.Side.State = proto.Int32(6)
	heal := tableNumber(s.table("GameConfigure", "StaticName", "Resurrection2Safe"), "NameValue")
	if math.IsNaN(heal) || math.IsInf(heal, 0) || heal <= 0 || heal > 1 {
		return nil, errors.New("沙漠死亡返回恢复值无效")
	}
	deathLossPercent := int64(50)
	if s.GGOFieldConfig.Rules.DeathLossPercent != nil {
		deathLossPercent = *s.GGOFieldConfig.Rules.DeathLossPercent
	}
	g.GGOField.MonsterFight = &model.GGOFieldMonsterFight{FightID: fightID, MonsterUUID: node.UUID, DeploymentRevision: s.GGOFieldRevision, PointID: node.PointID, Generation: node.Generation, ReservedUntilMS: node.ReservedUntilMS, Line: line.Line, Reward: reward, RewardNumerator: tier.RewardNumerator, RewardDenominator: tier.RewardDenominator, CooldownMS: tier.RespawnSeconds * 1000, Chips: chips, EconomyVersion: s.GGOFieldConfig.Runtime.EconomyVersion, ExchangeValues: values, ProtectedChips: s.GGOFieldConfig.Runtime.ProtectedChips, DeathLossPercent: ggoInt64(deathLossPercent), SafeReturn: s.GGOFieldConfig.Runtime.SafeReturn, HealRatio: heal}
	return result, nil
}

// GGOFieldMonsterActive identifies a persisted in-flight battle after reconnect.
func (s *GameService) GGOFieldMonsterActive(ctx context.Context, auth model.GGOFieldAuth, session []byte) (*model.GGOFieldMonsterActiveResponse, error) {
	out := &model.GGOFieldMonsterActiveResponse{}
	err := s.ggoWithProfile(ctx, auth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		if a.Game.GGOField == nil || a.Game.GGOField.MonsterFight == nil {
			return nil
		}
		if expired, err := a.Service.ggoExpireMonsterFightLocked(tx, a.Row.Id, a.Game, a.Now); err != nil {
			return err
		} else if expired {
			return saveProfile(tx, a.Row, a.Player, a.Game)
		}
		fight := a.Game.GGOField.MonsterFight
		if a.Game.Field == nil || a.Game.Field.Fight == nil || a.Game.Field.Fight.Settled || a.Game.Field.Fight.Start == nil || a.Game.Field.Fight.Start.GetFightId() != fight.FightID {
			return errors.New("沙漠小怪进行中状态损坏")
		}
		inputHash := a.Game.Field.Fight.BossInputHash
		if inputHash == "" && a.Game.Field.Fight.BossInput != nil {
			raw, e := json.Marshal(a.Game.Field.Fight.BossInput)
			if e != nil {
				return e
			}
			inputHash = p6Digest(raw)
		}
		if inputHash == "" {
			return errors.New("沙漠小怪冻结输入摘要缺失")
		}
		out.InputHash = inputHash
		out.Active = true
		out.FightID = fight.FightID
		out.MonsterUUID = fight.MonsterUUID
		out.PointID = fight.PointID
		out.Generation = fight.Generation
		out.StartedAtMS = a.Game.Field.Fight.StartedAt
		out.ReservedUntilMS = fight.ReservedUntilMS
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Expired original battle leases release their public reservation and change
// generation; an old result can never consume the replacement monster.
func (s *GameService) ggoExpireMonsterFightLocked(tx *gorm.DB, id int64, g *model.GameState, now int64) (bool, error) {
	if g.GGOField == nil || g.GGOField.MonsterFight == nil {
		return false, nil
	}
	pending := g.GGOField.MonsterFight
	if pending.ReservedUntilMS < 1 || now < pending.ReservedUntilMS {
		return false, nil
	}
	if err := ggoLockPeriodsBeforeLine(tx, s); err != nil {
		return false, err
	}
	line, shared, err := ggoLoadReservedPoint(tx, s.OriginalServerID(), pending.Line, pending.PointID)
	if err != nil {
		return false, err
	}
	released := false
	for i := range shared.Monsters {
		node := &shared.Monsters[i]
		if node.PointID == pending.PointID && node.Generation == pending.Generation && node.UUID == pending.MonsterUUID && node.ReservedBy == id && node.FightID == pending.FightID {
			token, err := fieldToken()
			if err != nil {
				return false, err
			}
			node.Generation, err = ggoNextGeneration(shared)
			if err != nil {
				return false, err
			}
			node.UUID = token
			node.ReservedBy = 0
			node.FightID = ""
			node.ReservedUntilMS = 0
			node.PatrolEpochMS, node.PatrolPausedAtMS = now, 0
			if err = ggoLineSave(tx, line, shared, now); err != nil {
				return false, err
			}
			released = true
			break
		}
	}
	if !released && ggoReleaseLegacyBlock(shared, pending.PointID, "monster", id, pending.FightID, pending.Generation) {
		if err := ggoLineSave(tx, line, shared, now); err != nil {
			return false, err
		}
	}
	if g.Field != nil && g.Field.Fight != nil && g.Field.Fight.Start.GetFightId() == pending.FightID {
		g.Field.Fight = nil
		g.Field.Monsters = nil
		if g.Field.Side != nil && g.Field.Side.GetCurHp() > 0 {
			g.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
		}
	}
	g.GGOField.MonsterFight = nil
	return true, nil
}
