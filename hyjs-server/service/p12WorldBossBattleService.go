package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeP12WorldBossBattle 安装本地策略及持久回执；重启保留显式开关和既有战斗。
func (s *GameService) InitializeP12WorldBossBattle(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P12WorldBossBattlePolicy{}, &db.P12WorldBossBattleInstance{}, &db.P12WorldBossBattleParticipation{}, &db.P12WorldBossBattleReceipt{}, &db.P12WorldBossBoost{}, &db.P12WorldBossAward{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P12WorldBossBattlePolicy{ID: 1, PolicyName: constUtil.P12WorldBossBattlePolicy, Source: constUtil.P12WorldBossBattleSource, EconomySource: constUtil.P12WorldBossEconomySource}).Error
}

func p12WorldBossBattlePolicy(tx *gorm.DB) error {
	var policy db.P12WorldBossBattlePolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Enabled || policy.PolicyName != constUtil.P12WorldBossBattlePolicy || policy.Source != constUtil.P12WorldBossBattleSource {
		return errors.New(constUtil.P12WorldBossBattleClosed)
	}
	return nil
}

// p12WorldBossBattleMonster 以原完整四成员和当前冻结等级构造战斗快照。
func (s *GameService) p12WorldBossBattleMonster(def *p12WorldBossDefinition, level int64) (*pb.BattleMonster, int64, error) {
	monster := &pb.BattleMonster{Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
	seen := map[int32]bool{}
	total := float64(0)
	for _, source := range def.Tables["WorldBoss"] {
		pose, err := strconv.ParseInt(source["Location"], 10, 32)
		if err != nil || seen[int32(pose)] {
			return nil, 0, errors.New("世界首领四部位阵位无效")
		}
		seen[int32(pose)] = true
		var factor map[string]string
		for _, row := range def.Tables["WorldBossFactor"] {
			if row["BossId"] == source["BossId"] && row["Level"] == strconv.FormatInt(level, 10) && row["Location"] == source["Location"] {
				factor = row
			}
		}
		if factor == nil {
			return nil, 0, errors.New("世界首领等级倍率缺失")
		}
		row := map[string]string{}
		for key, value := range source {
			row[key] = value
		}
		for _, key := range []string{"Attack", "PhyDefend", "MagicDefend", "Hp", "hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce"} {
			base, e1 := strconv.ParseFloat(source[key], 64)
			multiple, e2 := strconv.ParseFloat(factor[key], 64)
			if e1 != nil || e2 != nil || !combatFinite(base*multiple) || base*multiple < 0 {
				return nil, 0, errors.New("世界首领原属性/等级倍率无效")
			}
			row[key] = strconv.FormatFloat(base*multiple, 'f', -1, 64)
		}
		row["Level"] = strconv.FormatInt(level, 10)
		actor, err := s.actor(row, "worldboss-"+source["Id"], int32(pose), true)
		if err != nil {
			return nil, 0, err
		}
		monster.Formation = append(monster.Formation, actor)
		total += actor.GetInst().GetCurHp()
	}
	if len(monster.Formation) != 4 || total <= 0 || total >= float64(math.MaxInt64) {
		return nil, 0, errors.New("世界首领四成员总HP无效")
	}
	return monster, int64(math.Floor(total)), nil
}

// p12WorldBossBattleInstanceLocked 在原周期互斥范围内创建共享实例，再锁住HP行。
func (s *GameService) p12WorldBossBattleInstanceLocked(tx *gorm.DB, cycle *db.P12WorldBossCycle, def *p12WorldBossDefinition) (*db.P12WorldBossBattleInstance, error) {
	var instance db.P12WorldBossBattleInstance
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, "id=?", cycle.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_, total, buildErr := s.p12WorldBossBattleMonster(def, cycle.Level)
		if buildErr != nil {
			return nil, buildErr
		}
		instance = db.P12WorldBossBattleInstance{ID: cycle.ID, TotalHP: total, CurrentHP: total, PolicyName: constUtil.P12WorldBossBattlePolicy}
		err = tx.Create(&instance).Error
	}
	if err != nil {
		return nil, err
	}
	if instance.PolicyName != constUtil.P12WorldBossBattlePolicy || instance.TotalHP <= 0 || instance.CurrentHP < 0 || instance.CurrentHP > instance.TotalHP {
		return nil, errors.New(constUtil.P12WorldBossBattleInvalid)
	}
	return &instance, nil
}

func p12WorldBossBattleDamage(input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome) (int64, error) {
	initial := map[string]float64{}
	for _, actor := range input.Monster.GetFormation() {
		initial[actor.GetGuid()] = actor.GetInst().GetCurHp()
	}
	total := float64(0)
	count := 0
	for _, actor := range terminal.FinalActors {
		if actor.Camp != 1 {
			continue
		}
		hp, ok := initial[actor.GUID]
		if !ok || !combatFinite(actor.CurrentHP) {
			return 0, errors.New(constUtil.P12WorldBossBattleInvalid)
		}
		total += math.Max(0, hp-math.Max(0, actor.CurrentHP))
		delete(initial, actor.GUID)
		count++
	}
	if count != 4 || len(initial) != 0 || !combatFinite(total) || total >= float64(math.MaxInt64) {
		return 0, errors.New(constUtil.P12WorldBossBattleInvalid)
	}
	return int64(math.Floor(total)), nil
}

func p12WorldBossBattleStartProto(row *db.P12WorldBossBattleReceipt) (*pb.SCWorldBossFight, error) {
	if row.PolicyName != constUtil.P12WorldBossBattlePolicy || row.StartHash != p6Digest(row.StartProto) || row.InputHash != p6Digest([]byte(row.InputJSON)) || row.TerminalHash != p6Digest([]byte(row.TerminalJSON)) {
		return nil, errors.New(constUtil.P12WorldBossBattleInvalid)
	}
	var (
		input    model.FrozenBattleInput
		terminal model.CombatFinalHPOutcome
	)
	if json.Unmarshal([]byte(row.InputJSON), &input) != nil || json.Unmarshal([]byte(row.TerminalJSON), &terminal) != nil {
		return nil, errors.New(constUtil.P12WorldBossBattleInvalid)
	}
	if err := validateP12WorldBossCertifiedReceipt(&input, &terminal); err != nil {
		return nil, err
	}
	out := &pb.SCWorldBossFight{}
	if err := proto.Unmarshal(row.StartProto, out); err != nil {
		return nil, err
	}
	if out.GetFightId() != row.FightID || out.GetBattlePlayer().GetPlayerId() != row.PlayerID || input.Attacker.GetPlayerId() != row.PlayerID || out.GetRandSeed() != input.Seed || out.GetMaxround() != input.MaxRound || len(out.BattleMonsterList) != 1 || !proto.Equal(out.BattlePlayer, input.Attacker) || !proto.Equal(out.BattleMonsterList[0], input.Monster) || out.GetCode() != terminal.Outcome.Winner+1 {
		return nil, errors.New(constUtil.P12WorldBossBattleInvalid)
	}
	damage, err := p12WorldBossBattleDamage(&input, &terminal)
	if err != nil || damage != row.PredictedDamage || out.GetDamange() != int32(min(damage, int64(math.MaxInt32))) {
		return nil, errors.New(constUtil.P12WorldBossBattleInvalid)
	}
	return out, nil
}

// StartP12WorldBossBattle 沿原入口预计算并持久化，调用本函数不会扣共享HP或写伤害排行。
func (s *GameService) StartP12WorldBossBattle(ctx context.Context, id int64, bossUUID string, stamp int64) (*pb.SCWorldBossFight, error) {
	return s.startP12WorldBossBattleAt(ctx, id, bossUUID, stamp, time.Now())
}

func (s *GameService) startP12WorldBossBattleAt(ctx context.Context, id int64, bossUUID string, stamp int64, now time.Time) (*pb.SCWorldBossFight, error) {
	var out *pb.SCWorldBossFight
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// 所有写路径先角色后实例，角色锁同时串行该人的重入与扣次。
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		if player.GetPlayerId() != id || stamp < 0 {
			return errors.New(constUtil.P12WorldBossBattleInvalid)
		}
		if err = p12WorldBossBattlePolicy(tx); err != nil {
			return err
		}
		if _, err = s.ExpireP12WorldBossPendingLocked(tx, player, game, now); err != nil {
			return err
		}
		var existing db.P12WorldBossBattleReceipt
		if stamp > 0 {
			err = tx.Where("player_id=? AND stamp=?", id, stamp).First(&existing).Error
			if err == nil {
				if existing.AbandonedAt != 0 {
					return errors.New("该首领战斗已放弃，请重新挑战")
				}
				if existing.InstanceID != bossUUID {
					return errors.New(constUtil.P12WorldBossBattleInvalid)
				}
				out, err = p12WorldBossBattleStartProto(&existing)
				return err
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		err = tx.Where("player_id=? AND committed=? AND abandoned_at=0", id, false).Order("created_at").First(&existing).Error
		if err == nil {
			if existing.InstanceID != bossUUID {
				return errors.New("请先完成上一场世界首领战斗")
			}
			out, err = p12WorldBossBattleStartProto(&existing)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		cfg, definition, err := p12WorldBossConfig(tx)
		if err != nil {
			return err
		}
		if !cfg.Enabled {
			return errors.New(constUtil.P12WorldBossBattleClosed)
		}
		cycle, definition, err := p12WorldBossCycle(tx, cfg, definition, now)
		if err != nil {
			return err
		}
		if cycle.ID != bossUUID || cycle.State != constUtil.P12WorldBossActive {
			return errors.New("世界首领尚未开放或实例已过期")
		}
		_, birth, err := p12WorldBossProjection(*definition, cycle.Level, cycle.PositionID)
		if err != nil {
			return err
		}
		point, err := fieldPoint(birth["pos"])
		if err != nil {
			return err
		}
		// 具名本地接触半径10米，沿当前真实野外位置校验，禁止跨图远程开战。
		if game.PositionLoading || game.Position.GetScene() != birth["scene"] || game.Field == nil || game.Field.Scene != birth["scene"] || game.Field.Side == nil || game.Field.Side.GetCurHp() <= 0 || game.Field.Side.GetState() != 0 || !p8Near(game.Position.GetCurrent(), &p8GatherConfig{Point: point, Radius: 10}) {
			return errors.New("请在首领所在野外地图靠近首领，并保持可战斗状态")
		}
		if game.Field.Fight != nil && !game.Field.Fight.Settled || game.Run != nil && !game.Run.Complete {
			return errors.New("请先完成当前战斗")
		}
		if err = s.ensureNoActiveDailyCopy(tx, id, now); err != nil {
			return err
		}
		instance, err := s.p12WorldBossBattleInstanceLocked(tx, cycle, definition)
		if err != nil {
			return err
		}
		if instance.CurrentHP == 0 {
			return errors.New("世界首领已被击杀")
		}
		free, err := strconv.ParseInt(definition.Settings["WorldBossFreeTimes"], 10, 64)
		if err != nil || free != 5 || definition.Settings["WorldBossRecoverTime"] != "15" {
			return errors.New("世界首领原免费次数定义无效")
		}
		var participation db.P12WorldBossBattleParticipation
		err = tx.First(&participation, "player_id=?", id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			participation = db.P12WorldBossBattleParticipation{PlayerID: id, Remaining: free, RecoveredAt: now.UnixMilli()}
		} else if err != nil {
			return err
		}
		interval := int64((15 * time.Minute) / time.Millisecond)
		if participation.Remaining < free {
			steps := (now.UnixMilli() - participation.RecoveredAt) / interval
			if steps > 0 {
				participation.Remaining = min(free, participation.Remaining+steps)
				participation.RecoveredAt += steps * interval
			}
		}
		if participation.Remaining <= 0 {
			return errors.New("世界首领免费挑战次数不足")
		}
		if participation.Remaining == free {
			participation.RecoveredAt = now.UnixMilli()
		}
		participation.Remaining--
		if err = tx.Save(&participation).Error; err != nil {
			return err
		}
		attacker, err := s.battlePlayer(player, game)
		if err != nil {
			return err
		}
		for _, hero := range attacker.Formation {
			for _, hp := range game.Field.Side.BattlePosHps {
				if hero.GetPose() == hp.GetBattlePos() {
					hero.Inst.CurHp = proto.Float64(hero.GetProp().GetHp() * hp.GetHpPercent())
				}
			}
		}
		attacker.Inst.Anger = proto.Float64(game.Field.Side.GetAnger())
		boostIDs, err := s.p12WorldBossApplyBoosts(tx, cycle.ID, id, attacker)
		if err != nil {
			return err
		}
		monster, _, err := s.p12WorldBossBattleMonster(definition, cycle.Level)
		if err != nil {
			return err
		}
		seed := now.UnixNano() & math.MaxInt32
		input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: attacker, Monster: monster, Seed: seed, MaxRound: 40, BattleType: 18, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1), AppliedProperties: map[string][]string{"localWorldBossPolicy": {constUtil.P12WorldBossBattlePolicy, constUtil.P12WorldBossBattleSource}, "worldBossPurchasedBoosts": boostIDs}})
		if err != nil {
			return err
		}
		terminal, err := s.computeP12WorldBossCertifiedFinalHP(ctx, input)
		if err != nil {
			return err
		}
		if terminal.Outcome == nil || (terminal.Outcome.Winner != 0 && terminal.Outcome.Winner != 1) {
			return errors.New("本地首领计算未产生有效胜败")
		}
		damage, err := p12WorldBossBattleDamage(input, terminal)
		if err != nil {
			return err
		}
		_, pos, err := p12WorldBossProjection(*definition, cycle.Level, cycle.PositionID)
		if err != nil {
			return err
		}
		rank, err := p12WorldBossBattleRank(tx, instance.ID, id)
		if err != nil {
			return err
		}
		// 原bosshpForwardLab/bosshpNowLab自己乘100，协议传共享剩余HP的0..1比例。
		out = &pb.SCWorldBossFight{FightId: proto.String(uuid.NewString()), RandSeed: proto.Int64(seed), BattlePlayer: attacker, BattleMonsterList: []*pb.BattleMonster{monster}, FightScene: proto.String(pos["scene"]), Code: proto.Int32(terminal.Outcome.Winner + 1), Maxround: proto.Int32(40), Damange: proto.Int32(int32(min(damage, int64(math.MaxInt32)))), DamageforwardPercent: proto.Float64(float64(instance.CurrentHP) / float64(instance.TotalHP)), DamagelaterPercent: proto.Float64(float64(instance.CurrentHP-min(damage, instance.CurrentHP)) / float64(instance.TotalHP)), ForwardRank: proto.Int32(rank.GetMyDamageInfo().GetRank()), LaterRank: proto.Int32(rank.GetMyDamageInfo().GetRank()), PlayerSide: proto.Clone(game.Field.Side).(*pb.PlayerSide)}
		raw, err := proto.Marshal(out)
		if err != nil {
			return err
		}
		frozen, err := json.Marshal(input)
		if err != nil {
			return err
		}
		final, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		var sourceStamp *int64
		if stamp > 0 {
			sourceStamp = &stamp
		}
		if err = tx.Create(&db.P12WorldBossBattleReceipt{FightID: out.GetFightId(), PlayerID: id, Stamp: sourceStamp, InstanceID: instance.ID, PolicyName: constUtil.P12WorldBossBattlePolicy, InputJSON: string(frozen), InputHash: p6Digest(frozen), TerminalJSON: string(final), TerminalHash: p6Digest(final), StartProto: raw, StartHash: p6Digest(raw), PredictedDamage: damage, CreatedAt: now.UnixMilli()}).Error; err != nil {
			return err
		}
		game.Field.Side.State = proto.Int32(6)
		return saveProfile(tx, &profile, player, game)
	})
	return out, err
}

// FinishP12WorldBossBattle 仅信任冻结预计算，以实例行锁将正常结束/跳过提交为一次入账。
func (s *GameService) FinishP12WorldBossBattle(ctx context.Context, id int64, session []byte, fightID string) (*model.P12WorldBossFinishResponse, error) {
	if !s.P12WorldBossBattleReady {
		return nil, errors.New(constUtil.P12WorldBossBattleClosed)
	}
	return s.finishP12WorldBossBattleAt(ctx, id, session, fightID, time.Now())
}

func (s *GameService) finishP12WorldBossBattleAt(ctx context.Context, id int64, session []byte, fightID string, now time.Time) (*model.P12WorldBossFinishResponse, error) {
	var response *model.P12WorldBossFinishResponse
	var killNotice *pb.SCPublicMessage
	expired := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		profile, player, err := p10VoiceAuth(tx, id, session)
		if err != nil {
			return err
		}
		_, game, err := decodeProfile(profile)
		if err != nil {
			return err
		}
		var receipt db.P12WorldBossBattleReceipt
		if err := tx.First(&receipt, "fight_id=? AND player_id=?", fightID, id).Error; err != nil {
			return err
		}
		start, err := p12WorldBossBattleStartProto(&receipt)
		if err != nil {
			return err
		}
		var instance db.P12WorldBossBattleInstance
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, "id=?", receipt.InstanceID).Error; err != nil {
			return err
		}
		// 先锁实例再锁回执；初次普通读取只定位实例，当前读用于后续幂等判断。
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&receipt, "fight_id=? AND player_id=?", fightID, id).Error; err != nil {
			return err
		}
		if receipt.AbandonedAt != 0 {
			return errors.New("该首领战斗已放弃或跨期结束，不再入账")
		}
		if instance.PolicyName != receipt.PolicyName || instance.TotalHP <= 0 || instance.CurrentHP < 0 || instance.CurrentHP > instance.TotalHP || receipt.PredictedDamage < 0 {
			return errors.New(constUtil.P12WorldBossBattleInvalid)
		}
		already := receipt.Committed
		if !already {
			var cycle db.P12WorldBossCycle
			if err = tx.First(&cycle, "id=?", receipt.InstanceID).Error; err != nil {
				return err
			}
			if now.UnixMilli() >= cycle.EndsAt {
				if err = p12WorldBossAbandonLocked(tx, &receipt, game, "cycle-expired", now); err != nil {
					return err
				}
				if err = saveProfile(tx, profile, player, game); err != nil {
					return err
				}
				expired = true
				return nil
			}
			var terminal model.CombatFinalHPOutcome
			if receipt.TerminalHash != p6Digest([]byte(receipt.TerminalJSON)) {
				return errors.New(constUtil.P12WorldBossBattleInvalid)
			}
			if err = json.Unmarshal([]byte(receipt.TerminalJSON), &terminal); err != nil {
				return err
			}
			var input model.FrozenBattleInput
			if err = json.Unmarshal([]byte(receipt.InputJSON), &input); err != nil {
				return err
			}
			predicted, err := p12WorldBossBattleDamage(&input, &terminal)
			if err != nil || predicted != receipt.PredictedDamage {
				return errors.New(constUtil.P12WorldBossBattleInvalid)
			}
			if game.Field == nil || game.Field.Side == nil {
				return errors.New(constUtil.P12WorldBossBattleInvalid)
			}
			// 查询在锁内读取已提交累计；预计算不会提前增加排行。
			before, err := p12WorldBossBattleRank(tx, instance.ID, id)
			if err != nil {
				return err
			}
			receipt.AppliedDamage = min(receipt.PredictedDamage, instance.CurrentHP)
			beforeHP := instance.CurrentHP
			instance.CurrentHP -= receipt.AppliedDamage
			if instance.CurrentHP == 0 && receipt.AppliedDamage > 0 && instance.KillerID == 0 {
				instance.KillerID = id
				instance.KilledAt = now.UnixMilli()
				killNotice = p12WorldBossKillNotice(player.GetName())
			}
			receipt.Committed = true
			receipt.FinishHP = instance.CurrentHP
			receipt.FinishedAt = now.UnixMilli()
			if err = tx.Save(&instance).Error; err != nil {
				return err
			}
			if err = tx.Save(&receipt).Error; err != nil {
				return err
			}
			after, err := p12WorldBossBattleRank(tx, instance.ID, id)
			if err != nil {
				return err
			}
			start.Damange = proto.Int32(int32(min(receipt.AppliedDamage, int64(math.MaxInt32))))
			start.ForwardRank = proto.Int32(before.GetMyDamageInfo().GetRank())
			start.LaterRank = proto.Int32(after.GetMyDamageInfo().GetRank())
			start.DamageforwardPercent = proto.Float64(float64(beforeHP) / float64(instance.TotalHP))
			start.DamagelaterPercent = proto.Float64(float64(instance.CurrentHP) / float64(instance.TotalHP))
			start.PlayerSide, err = p12WorldBossBattleTerminalSide(game.Field.Side, &terminal)
			if err != nil {
				return err
			}
			game.Field.Side = proto.Clone(start.PlayerSide).(*pb.PlayerSide)
			if err = saveProfile(tx, profile, player, game); err != nil {
				return err
			}
			receipt.FinishProto, err = proto.Marshal(start)
			if err != nil {
				return err
			}
			receipt.FinishHash = p6Digest(receipt.FinishProto)
			if err = tx.Save(&receipt).Error; err != nil {
				return err
			}
		}
		if len(receipt.FinishProto) == 0 || receipt.FinishHash != p6Digest(receipt.FinishProto) {
			return errors.New(constUtil.P12WorldBossBattleInvalid)
		}
		response = &model.P12WorldBossFinishResponse{StartBase64: base64.StdEncoding.EncodeToString(receipt.FinishProto), AppliedDamage: receipt.AppliedDamage, CurrentHP: receipt.FinishHP, TotalHP: instance.TotalHP, AlreadyCommitted: already}
		return nil
	})
	if err == nil && expired {
		return nil, errors.New("本轮首领已结束，未提交战斗已放弃且没有扣血")
	}
	if err == nil {
		// 只发送本事务新产生的死亡跃迁；回滚、预计算、重复Finish及归零后的其他战斗均不广播。
		s.publishP12WorldBossKill(context.WithoutCancel(ctx), killNotice)
	}
	return response, err
}

func p12WorldBossBattleTerminalSide(previous *pb.PlayerSide, terminal *model.CombatFinalHPOutcome) (*pb.PlayerSide, error) {
	side := proto.Clone(previous).(*pb.PlayerSide)
	side.BattlePosHps = nil
	var current, maximum float64
	for _, actor := range terminal.FinalActors {
		if actor.Camp != 0 {
			continue
		}
		if actor.MaxHP <= 0 || !combatFinite(actor.CurrentHP) || !combatFinite(actor.MaxHP) {
			return nil, errors.New(constUtil.P12WorldBossBattleInvalid)
		}
		hp := math.Max(0, math.Min(actor.CurrentHP, actor.MaxHP))
		current += hp
		maximum += actor.MaxHP
		part := &pb.BattlePosHp{BattlePos: proto.Int32(actor.Pose), HpPercent: proto.Float64(hp / actor.MaxHP)}
		side.BattlePosHps = append(side.BattlePosHps, part)
		switch actor.Pose {
		case 3:
			side.BattlePos3 = part.HpPercent
		case 7:
			side.BattlePos7 = part.HpPercent
		case 9:
			side.BattlePos9 = part.HpPercent
		case 13:
			side.BattlePos13 = part.HpPercent
		}
	}
	if maximum <= 0 {
		return nil, errors.New(constUtil.P12WorldBossBattleInvalid)
	}
	side.CurHp, side.MaxHp, side.State = proto.Float64(current/maximum), proto.Float64(1), proto.Int32(0)
	if current == 0 {
		side.State = proto.Int32(5)
	}
	side.Anger = proto.Float64(terminal.Outcome.Attacker.GetInst().GetAnger())
	return side, nil
}

// HasPendingP12WorldBossBattle 用于共享玩法入口禁止未结束首领战斗时开启其他战斗。
func HasPendingP12WorldBossBattle(ctx context.Context, id int64) (bool, error) {
	var count int64
	err := db.GetIns().WithContext(ctx).Model(&db.P12WorldBossBattleReceipt{}).Where("player_id=? AND committed=? AND abandoned_at=0", id, false).Count(&count).Error
	return count > 0, err
}

// ExpireP12WorldBossPendingLocked 由已持角色锁且已解码的调用者使用，直接更新同一game对象。
// 跨期仅放弃未提交战斗，不产生伤害或奖励，不退已用次数；调用者负责保存profile。
func (s *GameService) ExpireP12WorldBossPendingLocked(tx *gorm.DB, player *pb.Player, game *model.GameState, now time.Time) (bool, error) {
	if !s.P12WorldBossBattleReady {
		return false, nil
	}
	var receipt db.P12WorldBossBattleReceipt
	err := tx.Where("player_id=? AND committed=? AND abandoned_at=0", player.GetPlayerId(), false).First(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var cycle db.P12WorldBossCycle
	if err = tx.First(&cycle, "id=?", receipt.InstanceID).Error; err != nil {
		return false, err
	}
	if now.UnixMilli() < cycle.EndsAt {
		return false, nil
	}
	if err = p12WorldBossAbandonLocked(tx, &receipt, game, "cycle-expired", now); err != nil {
		return false, err
	}
	return true, nil
}

func p12WorldBossAbandonLocked(tx *gorm.DB, receipt *db.P12WorldBossBattleReceipt, game *model.GameState, reason string, now time.Time) error {
	if receipt.Committed {
		return errors.New("已结算的首领战斗不能放弃")
	}
	if receipt.AbandonedAt != 0 {
		return nil
	}
	start, err := p12WorldBossBattleStartProto(receipt)
	if err != nil {
		return err
	}
	// 预计算尚未修改野外HP，恢复开战前快照即可释放本地state6占用。
	if game.Field != nil && game.Field.Side != nil && game.Field.Side.GetState() == 6 {
		game.Field.Side = proto.Clone(start.PlayerSide).(*pb.PlayerSide)
	}
	receipt.AbandonedAt, receipt.AbandonReason = now.UnixMilli(), reason
	return tx.Save(receipt).Error
}

// ResumeP12WorldBossBattle 无需点击场景实体即可恢复掉线战斗；过期仅解除占用。
func (s *GameService) ResumeP12WorldBossBattle(ctx context.Context, id int64, session []byte) (*pb.SCWorldBossFight, string, error) {
	return s.resumeP12WorldBossBattleAt(ctx, id, session, time.Now())
}

func (s *GameService) resumeP12WorldBossBattleAt(ctx context.Context, id int64, session []byte, now time.Time) (*pb.SCWorldBossFight, string, error) {
	if !s.P12WorldBossBattleReady {
		return nil, "", errors.New(constUtil.P12WorldBossBattleClosed)
	}
	var start *pb.SCWorldBossFight
	var scene string
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		profile, player, err := p10VoiceAuth(tx, id, session)
		if err != nil {
			return err
		}
		_, game, err := decodeProfile(profile)
		if err != nil {
			return err
		}
		changed, err := s.ExpireP12WorldBossPendingLocked(tx, player, game, now)
		if err != nil {
			return err
		}
		if changed {
			return saveProfile(tx, profile, player, game)
		}
		var receipt db.P12WorldBossBattleReceipt
		err = tx.Where("player_id=? AND committed=? AND abandoned_at=0", id, false).First(&receipt).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		start, err = p12WorldBossBattleStartProto(&receipt)
		if err != nil {
			return err
		}
		var cycle db.P12WorldBossCycle
		if err = tx.First(&cycle, "id=?", receipt.InstanceID).Error; err != nil {
			return err
		}
		if cycle.DefinitionHash != p6Digest([]byte(cycle.DefinitionJSON)) {
			return errors.New(constUtil.P12WorldBossBattleInvalid)
		}
		var definition p12WorldBossDefinition
		if err = json.Unmarshal([]byte(cycle.DefinitionJSON), &definition); err != nil {
			return err
		}
		_, position, err := p12WorldBossProjection(definition, cycle.Level, cycle.PositionID)
		if err != nil {
			return err
		}
		scene = position["scene"]
		if scene == "" || start.GetFightScene() != scene || game.Position.GetScene() != scene || game.Field == nil || game.Field.Scene != scene || game.PositionLoading {
			return errors.New("待恢复首领战斗与角色当前场景不一致，请放弃旧战斗后重试")
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return start, scene, nil
}

// AbandonP12WorldBossBattle 主动放弃不会提交预计算，重试保留同一放弃事实。
func (s *GameService) AbandonP12WorldBossBattle(ctx context.Context, id int64, session []byte, fightID string) error {
	if !s.P12WorldBossBattleReady {
		return errors.New(constUtil.P12WorldBossBattleClosed)
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		profile, player, err := p10VoiceAuth(tx, id, session)
		if err != nil {
			return err
		}
		_, game, err := decodeProfile(profile)
		if err != nil {
			return err
		}
		var receipt db.P12WorldBossBattleReceipt
		if err = tx.First(&receipt, "fight_id=? AND player_id=?", fightID, id).Error; err != nil {
			return err
		}
		if err = p12WorldBossAbandonLocked(tx, &receipt, game, "player-abandoned", time.Now()); err != nil {
			return err
		}
		return saveProfile(tx, profile, player, game)
	})
}

func p12WorldBossPurchasedIDs(tx *gorm.DB, instance string, id int64) ([]string, error) {
	var rows []db.P12WorldBossBoost
	if err := tx.Where("instance_id=? AND player_id=?", instance, id).Order("property_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	var ids []string
	for _, row := range rows {
		ids = append(ids, row.PropertyID)
	}
	return ids, nil
}

func (s *GameService) p12WorldBossApplyBoosts(tx *gorm.DB, instance string, id int64, party *pb.BattlePlayer) ([]string, error) {
	var rows []db.P12WorldBossBoost
	if err := tx.Where("instance_id=? AND player_id=?", instance, id).Order("property_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.applyP12WorldBossBoostRows(party, rows)
}

// applyP12WorldBossBoostRows 复用已购买账本的冻结效果；诊断导出也调用此处，避免手写另一份属性变换。
// rows仍由真实购买事务产生，调用者不能用本函数建立购买资格或发放战斗奖励。
func (s *GameService) applyP12WorldBossBoostRows(party *pb.BattlePlayer, rows []db.P12WorldBossBoost) ([]string, error) {
	var ids []string
	for _, row := range rows {
		ids = append(ids, row.PropertyID)
		for _, hero := range party.Formation {
			switch row.Kind {
			case 0:
				parts := strings.Split(row.Property, ",")
				if len(parts) != 2 {
					return nil, errors.New("世界首领属性加成快照无效")
				}
				value, err := strconv.ParseFloat(parts[1], 64)
				if err != nil || value != 80 {
					return nil, errors.New("世界首领属性加成快照无效")
				}
				switch parts[0] {
				case "hit":
					hero.Prop.Hit = proto.Float64(hero.Prop.GetHit() + value)
				case "cri":
					hero.Prop.Cri = proto.Float64(hero.Prop.GetCri() + value)
				default:
					return nil, errors.New("世界首领属性加成类型无效")
				}
			case 1:
				if row.Property != "tal_3163" && row.Property != "tal_3165" || s.table("Talent", "Id", row.Property) == nil {
					return nil, errors.New("世界首领天赋加成快照无效")
				}
				hero.TalentList = append(hero.TalentList, row.Property)
			default:
				return nil, errors.New("世界首领加成类型无效")
			}
		}
	}
	return ids, nil
}

// PurchaseP12WorldBossProp 沿原150钻加成接口扣费，每周期每种仅一次，之后真实进入冻结战斗。
func (s *GameService) PurchaseP12WorldBossProp(ctx context.Context, id int64, propertyID string, now time.Time) (*pb.SCWorldBossPropPurchase, error) {
	var response *pb.SCWorldBossPropPurchase
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		if err = p12WorldBossBattlePolicy(tx); err != nil {
			return err
		}
		var policy db.P12WorldBossBattlePolicy
		if err = tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.EconomyEnabled || policy.EconomySource != constUtil.P12WorldBossEconomySource {
			return errors.New("本地首领增益购买尚未开放")
		}
		cfg, def, err := p12WorldBossConfig(tx)
		if err != nil {
			return err
		}
		if !cfg.Enabled {
			return errors.New(constUtil.P12WorldBossBattleClosed)
		}
		cycle, def, err := p12WorldBossCycle(tx, cfg, def, now)
		if err != nil {
			return err
		}
		if cycle.State != constUtil.P12WorldBossActive {
			return errors.New("本轮首领尚未开放")
		}
		instance, err := s.p12WorldBossBattleInstanceLocked(tx, cycle, def)
		if err != nil {
			return err
		}
		if instance.CurrentHP == 0 {
			return errors.New("本轮首领已结束")
		}
		var owned db.P12WorldBossBoost
		err = tx.First(&owned, "instance_id=? AND player_id=? AND property_id=?", cycle.ID, id, propertyID).Error
		if err == nil {
			ids, err := p12WorldBossPurchasedIDs(tx, cycle.ID, id)
			response = &pb.SCWorldBossPropPurchase{PropPurchased: ids}
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var pending int64
		if err = tx.Model(&db.P12WorldBossBattleReceipt{}).Where("player_id=? AND committed=? AND abandoned_at=0", id, false).Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return errors.New("请先结束当前首领战斗再购买加成")
		}
		var definition map[string]string
		for _, row := range def.Tables["WorldBossPropadd"] {
			if row["id"] == propertyID {
				definition = row
			}
		}
		if definition == nil || definition["cost"] != "diamond,150" {
			return errors.New("世界首领原加成定义不存在")
		}
		kind, err := strconv.ParseInt(definition["type"], 10, 64)
		if err != nil || kind < 0 || kind > 1 {
			return errors.New("世界首领原加成类型无效")
		}
		if player.GetDiamond() < 150 {
			return errors.New("钻石不足")
		}
		owned = db.P12WorldBossBoost{InstanceID: cycle.ID, PlayerID: id, PropertyID: propertyID, Property: definition["prop"], Kind: kind, Paid: 150, CreatedAt: now.UnixMilli()}
		// 先校验该队伍与购买后子集的已证范围，避免扣钻后才在开战时拒绝未知机制。
		var prospective []db.P12WorldBossBoost
		if err = tx.Where("instance_id=? AND player_id=?", cycle.ID, id).Find(&prospective).Error; err != nil {
			return err
		}
		prospective = append(prospective, owned)
		sort.Slice(prospective, func(i, j int) bool { return prospective[i].PropertyID < prospective[j].PropertyID })
		party, err := s.battlePlayer(player, game)
		if err != nil {
			return err
		}
		ids, err := s.applyP12WorldBossBoostRows(party, prospective)
		if err != nil {
			return err
		}
		monster, _, err := s.p12WorldBossBattleMonster(def, cycle.Level)
		if err != nil {
			return err
		}
		input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: party, Monster: monster, Seed: 0, MaxRound: 40, BattleType: 18, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1), AppliedProperties: map[string][]string{"worldBossPurchasedBoosts": ids}})
		if err != nil {
			return err
		}
		if _, err = p12WorldBossNativeProofForInput(input); err != nil {
			return err
		}
		if err = tx.Create(&owned).Error; err != nil {
			return err
		}
		player.Diamond = proto.Int64(player.GetDiamond() - 150)
		// 同轮同属性只有一笔真实购买，重开页面或重复请求保留原消费事实。
		source := "worldboss:boost:" + cycle.ID + ":" + strconv.FormatInt(id, 10) + ":" + propertyID
		if err = s.RecordP1Cost(tx, id, source, []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(int32(owned.Paid))}}, now); err != nil {
			return err
		}
		if err = saveProfile(tx, &profile, player, game); err != nil {
			return err
		}
		ids, err = p12WorldBossPurchasedIDs(tx, cycle.ID, id)
		response = &pb.SCWorldBossPropPurchase{PropPurchased: ids, Cost: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(150)}}}
		return err
	})
	return response, err
}

// p12WorldBossDrawReward 将原每个物品的权重按实际总和独立抽取一次，冻结所抽实际数量。
// 这是具名本地发奖选择，不把原权重补成1000，也不把可能ID预览当成固定奖。
func p12WorldBossDrawReward(content string) (string, error) {
	if !strings.Contains(content, ":") {
		if _, err := p1RewardResources(content); err != nil {
			return "", err
		}
		return content, nil
	}
	var rewards []string
	for _, part := range strings.Split(content, "|") {
		pair := strings.Split(part, ":")
		if len(pair) != 2 || pair[0] == "" {
			return "", errors.New("首领奖池格式无效")
		}
		var choices []struct{ Quantity, Weight int64 }
		var total int64
		for _, choice := range strings.Split(pair[1], ";") {
			values := strings.Split(choice, ",")
			if len(values) != 2 {
				return "", errors.New("首领奖池选项无效")
			}
			quantity, e1 := strconv.ParseInt(values[0], 10, 32)
			weight, e2 := strconv.ParseInt(values[1], 10, 32)
			if e1 != nil || e2 != nil || quantity < 0 || weight < 0 || total > math.MaxInt64-weight {
				return "", errors.New("首领奖池权重无效")
			}
			choices = append(choices, struct{ Quantity, Weight int64 }{quantity, weight})
			total += weight
		}
		if total <= 0 {
			return "", errors.New("首领奖池总权重为空")
		}
		draw, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return "", err
		}
		cursor := draw.Int64()
		for _, choice := range choices {
			if cursor < choice.Weight {
				if choice.Quantity > 0 {
					rewards = append(rewards, pair[0]+","+strconv.FormatInt(choice.Quantity, 10))
				}
				break
			}
			cursor -= choice.Weight
		}
	}
	result := strings.Join(rewards, ";")
	if result == "" {
		return "", nil
	}
	_, err := p1RewardResources(result)
	return result, err
}

// p12WorldBossRewardsLocked 仅给当前已持角色锁的人投递，避免跨玩家profile锁；离线者下次查询补领。
func (s *GameService) p12WorldBossRewardsLocked(tx *gorm.DB, id int64, now time.Time) error {
	var policy db.P12WorldBossBattlePolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Enabled || !policy.RewardsEnabled {
		return nil
	}
	if policy.EconomySource != constUtil.P12WorldBossEconomySource {
		return errors.New("世界首领奖励缺少具名本地来源")
	}
	var instanceIDs []string
	if err := tx.Model(&db.P12WorldBossBattleReceipt{}).Where("player_id=? AND committed=?", id, true).Distinct("instance_id").Pluck("instance_id", &instanceIDs).Error; err != nil {
		return err
	}
	sort.Strings(instanceIDs)
	for _, instanceID := range instanceIDs {
		var cycle db.P12WorldBossCycle
		if err := tx.First(&cycle, "id=?", instanceID).Error; err != nil {
			return err
		}
		var instance db.P12WorldBossBattleInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, "id=?", instanceID).Error; err != nil {
			return err
		}
		if instance.CurrentHP > 0 && now.UnixMilli() < cycle.EndsAt {
			continue
		}
		closedAt := cycle.EndsAt
		if instance.KilledAt > 0 {
			closedAt = instance.KilledAt
		}
		expires := closedAt + 7*86400000
		if now.UnixMilli() >= expires {
			continue
		}
		var def p12WorldBossDefinition
		if cycle.DefinitionHash != p6Digest([]byte(cycle.DefinitionJSON)) {
			return errors.New(constUtil.P12WorldBossBattleInvalid)
		}
		if err := json.Unmarshal([]byte(cycle.DefinitionJSON), &def); err != nil {
			return err
		}
		rank, err := p12WorldBossBattleRank(tx, instanceID, id)
		if err != nil {
			return err
		}
		kinds := []int64{0}
		if position := rank.GetMyDamageInfo().GetRank(); position > 0 {
			kind := int64(1)
			if position == 1 {
				kind = 4
			} else if position <= 3 {
				kind = 3
			} else if position <= 10 {
				kind = 2
			}
			kinds = append(kinds, kind)
		}
		if instance.KillerID == id {
			kinds = append(kinds, 5)
		}
		for _, kind := range kinds {
			var prior db.P12WorldBossAward
			err := tx.First(&prior, "instance_id=? AND player_id=? AND kind=?", instanceID, id, kind).Error
			if err == nil {
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			var definition map[string]string
			for _, row := range def.Tables["WorldBossReward"] {
				if row["level"] == strconv.FormatInt(cycle.Level, 10) && row["type"] == strconv.FormatInt(kind, 10) {
					definition = row
				}
			}
			if definition == nil {
				return errors.New("首领奖档缺失")
			}
			reward, err := p12WorldBossDrawReward(definition["reward"])
			if err != nil {
				return err
			}
			award := db.P12WorldBossAward{InstanceID: instanceID, PlayerID: id, Kind: kind, PolicyName: constUtil.P12WorldBossEconomyPolicy, Reward: reward, ExpiresAt: expires}
			if reward != "" {
				delivery, err := s.EnqueueRewardNotice(tx, id, "worldboss:"+instanceID+":"+strconv.FormatInt(kind, 10), "世界首领奖励", "本地首领奖励：参与、伤害排名与最后一击按周期结算，奖励在领奖中心领取。", reward, expires)
				if err != nil {
					return err
				}
				award.DeliveryID = delivery.ID
			}
			if err = tx.Create(&award).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// SyncP12WorldBossRewards 单独事务投递各已终结周期，避免当前活动锁与跨周期锁交叉。
func (s *GameService) SyncP12WorldBossRewards(ctx context.Context, id int64) error {
	if !s.P12WorldBossBattleReady {
		return nil
	}
	// 默认关闭时只读开关，不开启事务或抢占角色锁；原登录/领奖查询无需扫描历史回执。
	var policy db.P12WorldBossBattlePolicy
	if err := db.GetIns().WithContext(ctx).Select("enabled,rewards_enabled").First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Enabled || !policy.RewardsEnabled {
		return nil
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		return s.p12WorldBossRewardsLocked(tx, id, time.Now())
	})
}

// p12WorldBossBattleEntity 返回原场景协议需要的共享实体，原周期定义仍为唯一出生来源。
func p12WorldBossBattleEntity(tx *gorm.DB, cycle *db.P12WorldBossCycle) (*pb.WorldBoss, error) {
	var instance db.P12WorldBossBattleInstance
	err := tx.First(&instance, "id=?", cycle.ID).Error
	if err != nil {
		return nil, err
	}
	var def p12WorldBossDefinition
	if cycle.DefinitionHash != p6Digest([]byte(cycle.DefinitionJSON)) {
		return nil, errors.New(constUtil.P12WorldBossBattleInvalid)
	}
	if err = json.Unmarshal([]byte(cycle.DefinitionJSON), &def); err != nil {
		return nil, err
	}
	leader, pos, err := p12WorldBossProjection(def, cycle.Level, cycle.PositionID)
	if err != nil {
		return nil, err
	}
	point, err := fieldPoint(pos["pos"])
	if err != nil {
		return nil, err
	}
	angle, err := strconv.ParseFloat(pos["BornAngle"], 64)
	if err != nil {
		return nil, err
	}
	state := int32(1)
	if instance.CurrentHP == 0 {
		state = 3
	}
	return &pb.WorldBoss{Uuid: proto.String(cycle.ID), BossId: proto.String("boss_001"), CurrentPoint: point, Scene: proto.String(pos["scene"]), State: proto.Int32(state), CurHp: proto.Float64(float64(instance.CurrentHP)), TotalHp: proto.Float64(float64(instance.TotalHP)), HeroId: proto.String(leader["HeroId"]), BornAngle: proto.Float64(angle), Level: proto.Int32(int32(cycle.Level)), Name: proto.String(leader["name"])}, nil
}

// p12WorldBossBattleRank 仅投影实际提交回执；未提交预计算始终不进入伤害榜。
func p12WorldBossBattleRank(tx *gorm.DB, instanceID string, id int64) (*pb.SCWorldBossRank, error) {
	var instance db.P12WorldBossBattleInstance
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, "id=?", instanceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &pb.SCWorldBossRank{BossHpPercent: proto.Float64(1), State: proto.Int32(1)}, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []struct {
		PlayerID int64
		Damage   int64
	}
	// 当前读绕过MySQL RR旧快照；复合索引使此锁只涉及该实例已提交回执。
	var receipts []db.P12WorldBossBattleReceipt
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("player_id,applied_damage").Where("instance_id=? AND committed=?", instanceID, true).Find(&receipts).Error; err != nil {
		return nil, err
	}
	totals := map[int64]int64{}
	for _, receipt := range receipts {
		if receipt.AppliedDamage > 0 {
			totals[receipt.PlayerID] += receipt.AppliedDamage
		}
	}
	for playerID, damage := range totals {
		rows = append(rows, struct {
			PlayerID int64
			Damage   int64
		}{playerID, damage})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Damage != rows[j].Damage {
			return rows[i].Damage > rows[j].Damage
		}
		return rows[i].PlayerID < rows[j].PlayerID
	})
	state := int32(1)
	if instance.CurrentHP == 0 {
		state = 2
	}
	out := &pb.SCWorldBossRank{BossHpPercent: proto.Float64(float64(instance.CurrentHP) / float64(instance.TotalHP)), State: proto.Int32(state)}
	for index, row := range rows {
		var profile db.Profile
		if err = tx.First(&profile, row.PlayerID).Error; err != nil {
			return nil, err
		}
		player, _, err := decodeProfile(&profile)
		if err != nil {
			return nil, err
		}
		info := &pb.WorldBossDamageInfo{Damage: proto.Float64(float64(row.Damage)), PlayerName: proto.String(player.GetName()), Rank: proto.Int32(int32(index + 1))}
		out.DamageInfos = append(out.DamageInfos, info)
		if row.PlayerID == id {
			out.MyDamageInfo = info
		}
		if row.PlayerID == instance.KillerID {
			out.LastDamageInfo = info
		}
	}
	return out, nil
}
