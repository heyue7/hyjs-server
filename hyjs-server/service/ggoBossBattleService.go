package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/elog"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) ggoBossCheck(ctx context.Context, id int64, request *pb.CSSideBossCheck) ([]proto.Message, error) {
	result := &pb.SCSideBossCheck{Code: proto.Int32(2003), Monster: &pb.SideBoss{Uuid: proto.String(request.GetUuid()), State: proto.Int32(3), CurHp: proto.Float64(0)}}
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		profile, err := ggoNativeProfileSession(tx, id)
		if err != nil {
			return err
		}
		if AdminProfileServerID(*profile) != s.OriginalServerID() {
			return constUtil.ErrAdminPermission
		}
		if g.GGOField == nil || g.Position.GetScene() != constUtil.GGOFieldScene || s.GGOFieldConfig == nil {
			return nil
		}
		if err := ggoCheckClientBuildLocked(tx, id, profile.Session, s.GGOFieldConfig); err != nil {
			return err
		}
		a := &ggoFieldActor{Service: s, Row: profile, Player: p, Game: g, Now: time.Now().UnixMilli()}
		if err = ggoCurrentPresence(tx, a, profile.Session); err != nil {
			return err
		}
		row, event, boss, _, err := s.ggoBossCurrent(tx, s.GGOFieldConfig, a.Now, false, int64(a.Presence.Line))
		if err != nil {
			return err
		}
		if event.ID != request.GetUuid() || a.Now < event.StartsAtMS || a.Now >= event.EndsAtMS || row.KillerID != 0 {
			return nil
		}
		result.Code = proto.Int32(1)
		result.Monster = boss
		return nil
	})
	return []proto.Message{result}, err
}

// Only the desert event builds one enemy. Other ordinary boss formations keep
// their original four-member contract and original wire messages.
func (s *GameService) ggoBossMonster(event *ggoBossEvent, currentHP float64) (*pb.BattleMonster, error) {
	base := s.fieldBossLeaderInScene(constUtil.GGOFieldBossID, constUtil.GGOFieldScene)
	hero := s.table("Hero", "id", event.Boss.HeroID)
	if base == nil || hero == nil || hero["resName"] == "" {
		return nil, errors.New("沙漠首领原模型或技能缺失")
	}
	row := make(map[string]string, len(base))
	for key, value := range base {
		row[key] = value
	}
	row["HeroId"] = event.Boss.HeroID
	row["Level"] = fmt.Sprint(event.Boss.Level)
	row["Hp"] = fmt.Sprint(event.Boss.HP)
	row["Attack"] = fmt.Sprint(event.Boss.Attack)
	row["PhyDefend"] = fmt.Sprint(event.Boss.PhyDefend)
	row["MagicDefend"] = fmt.Sprint(event.Boss.MagicDefend)
	for _, key := range []string{"SkillId1", "SkillId2", "UltSkillId"} {
		row[key] = hero[key]
	}
	for i := 0; i < 6; i++ {
		delete(row, fmt.Sprintf("Talent%d", i))
	}
	actor, err := s.actor(row, ggoBossGUID(event.ID), 13, true)
	if err != nil {
		return nil, err
	}
	if !combatFinite(currentHP) || currentHP <= 0 || currentHP > actor.GetProp().GetHp() {
		return nil, errors.New("沙漠首领当前生命无效")
	}
	actor.Inst.CurHp = proto.Float64(currentHP)
	return &pb.BattleMonster{MonsterId: proto.Int64(1), ServerId: proto.Int32(1), TaskId: proto.String(constUtil.GGOFieldBossID), GroupIndex: proto.Int32(1), Formation: []*pb.BattleHero{actor}, Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}, nil
}

func (s *GameService) ggoBossFightStart(ctx context.Context, id int64, request *pb.CSSidePveBossFightStart) ([]proto.Message, error) {
	var result *pb.SCSidePveBossFightStart
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		profile, err := ggoNativeProfileSession(tx, id)
		if err != nil {
			return err
		}
		if AdminProfileServerID(*profile) != s.OriginalServerID() {
			return constUtil.ErrAdminPermission
		}
		if g.Field == nil || g.Field.Side == nil || g.GGOField == nil {
			return errors.New("沙漠战斗状态不存在")
		}
		if err := ggoCheckClientBuildLocked(tx, id, profile.Session, s.GGOFieldEconomy); err != nil {
			return err
		}
		field := g.Field
		if field.Fight != nil && !field.Fight.Settled {
			fight := field.Fight
			if fight.GGOBoss != nil && fight.GGOBoss.EventID == request.GetUuid() && fight.BossStart != nil {
				result = proto.Clone(fight.BossStart).(*pb.SCSidePveBossFightStart)
				return nil
			}
			return ggoFieldError(constUtil.GGOFieldBusy, "已有战斗进行中")
		}
		cfg := s.GGOFieldConfig
		if cfg == nil || cfg.Runtime == nil || g.Position.GetScene() != constUtil.GGOFieldScene {
			return ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠搜打撤暂未开放")
		}
		now := time.Now().UnixMilli()
		if g.PositionLoading || field.Side.GetCurHp() <= 0 || g.Run != nil && !g.Run.Complete {
			return ggoFieldError(constUtil.GGOFieldBusy, "当前无法战斗")
		}
		if g.GGOField.ProtectionUntil > now {
			return ggoFieldError(constUtil.GGOFieldProtectionConfirmation, "确认解除入场保护后请再次操作")
		}
		if err = P12FieldPVPRequireIdle(tx, id, time.UnixMilli(now)); err != nil {
			return err
		}
		if err = s.ensureNoActiveDailyCopy(tx, id, time.UnixMilli(now)); err != nil {
			return err
		}
		if s.P8GatherReady {
			if err = s.P8CancelGather(tx, id, g); err != nil {
				return err
			}
		}
		if err = ggoInterruptExtractionLocked(tx, id, g, now); err != nil {
			return err
		}
		a := &ggoFieldActor{Service: s, Row: profile, Player: player, Game: g, Now: now}
		if err = ggoCurrentPresence(tx, a, profile.Session); err != nil {
			return err
		}
		shared, event, boss, hp, err := s.ggoBossCurrent(tx, cfg, now, true, int64(a.Presence.Line))
		if err != nil {
			return err
		}
		if event.ID != request.GetUuid() || now < event.StartsAtMS || now >= event.EndsAtMS || shared.KillerID != 0 {
			return errors.New("沙漠首领当前不可挑战")
		}
		if !p8Near(g.Position.GetCurrent(), &p8GatherConfig{Point: boss.CurrentPoint, Radius: event.Radius}) {
			return errors.New("请靠近沙漠首领")
		}
		_, held, err := ggoProtectedChips(g.GGOField.Chips, cfg.Runtime.ProtectedChips)
		if err != nil {
			return err
		}
		if held > 0 && (g.GGOField.EconomyVersion != event.EconomyVersion || g.GGOField.ExchangeValues != event.ExchangeValues) {
			return errors.New("请先结算旧版本芯片")
		}
		monster, err := s.ggoBossMonster(event, hp[ggoBossGUID(event.ID)])
		if err != nil {
			return err
		}
		party, err := s.battlePlayer(player, g)
		if err != nil || len(party.GetFormation()) == 0 {
			return errors.New("首领战斗需要可用编队")
		}
		for _, actor := range party.Formation {
			for _, saved := range field.Side.GetBattlePosHps() {
				if actor.GetPose() == saved.GetBattlePos() {
					actor.Inst.CurHp = proto.Float64(actor.GetProp().GetHp() * saved.GetHpPercent())
				}
			}
		}
		party.Inst.Anger = proto.Float64(field.Side.GetAnger())
		seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
		if err != nil {
			return err
		}
		input, err := s.freezeAndQualifyP6GenericMechanisms(tx.Statement.Context, model.FrozenBattleInput{Attacker: party, Monster: monster, Seed: seed.Int64(), MaxRound: int32(event.Boss.PartyRounds * 2), BattleType: constUtil.FieldBossBattleType, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
		if err != nil {
			return err
		}
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		token, err := fieldToken()
		if err != nil {
			return err
		}
		heal := tableNumber(s.table("GameConfigure", "StaticName", "Resurrection2Safe"), "NameValue")
		if !combatFinite(heal) || heal <= 0 || heal > 1 {
			return errors.New("沙漠死亡返回恢复值无效")
		}
		result = &pb.SCSidePveBossFightStart{FightId: proto.String(token), RandSeed: proto.Int64(input.Seed), BattlePlayer: input.Attacker, BattleMonsterList: []*pb.BattleMonster{input.Monster}, FightScene: proto.String(""), Code: proto.Int32(1), Maxround: proto.Int32(input.MaxRound)}
		compat := &pb.SCSidePveFightStart{FightId: result.FightId, RandSeed: result.RandSeed, BattlePlayer: result.BattlePlayer, BattleMonsterList: result.BattleMonsterList, FightScene: result.FightScene, Code: result.Code, Maxround: result.Maxround}
		limit, err := s.fieldBossRewardLimit(tx)
		if err != nil {
			return err
		}
		if limit < 0 || event.Boss.DailyLimit < limit {
			limit = event.Boss.DailyLimit
		}
		deathLossPercent := int64(50)
		if cfg.Rules.DeathLossPercent != nil {
			deathLossPercent = *cfg.Rules.DeathLossPercent
		}
		field.Fight = &model.FieldFight{StartedAt: now, BossUUID: event.ID, BossLeaderGUID: ggoBossGUID(event.ID), Start: compat, BossStart: result, BossInput: input, BossRewardDailyLimit: &limit, GGOBoss: &model.GGOFieldBossFight{InputHash: p6Digest(raw), Boss: proto.Clone(boss).(*pb.SideBoss), EventID: event.ID, Line: int64(a.Presence.Line), EndsAtMS: event.EndsAtMS, EconomyVersion: event.EconomyVersion, ExchangeValues: event.ExchangeValues, ProtectedChips: cfg.Runtime.ProtectedChips, DeathLossPercent: ggoInt64(deathLossPercent), SafeReturn: cfg.Runtime.SafeReturn, HealRatio: heal}}
		field.Side.State = proto.Int32(6)
		projection := proto.Clone(boss).(*pb.SideBoss)
		projection.State = proto.Int32(2)
		field.Bosses = []*pb.SideBoss{projection}
		// First participation binds the event, perk snapshot and shared daily cap.
		var member db.FieldBossContribution73
		err = tx.Where("server_id=? AND spawn_uuid=? AND player_id=?", s.OriginalServerID(), event.ID, id).Take(&member).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			sprouts, e := s.fieldBossSprouts73(tx, id, constUtil.GGOFieldScene, event.Boss.SproutMax)
			if e != nil {
				return e
			}
			rules, e := json.Marshal(event)
			if e != nil {
				return e
			}
			member = db.FieldBossContribution73{ServerID: s.OriginalServerID(), SpawnUUID: event.ID, PlayerID: id, Rules: rules, Sprouts: sprouts, DailyLimit: limit}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&member).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		return nil
	})
	return []proto.Message{result}, err
}

func (s *GameService) GGOFieldBossActive(ctx context.Context, auth model.GGOFieldAuth, session []byte) (*model.GGOFieldBossActiveResponse, error) {
	out := &model.GGOFieldBossActiveResponse{}
	err := s.ggoWithProfile(ctx, auth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		if a.Game.Field == nil || a.Game.Field.Fight == nil {
			return nil
		}
		fight := a.Game.Field.Fight
		if fight.Settled || fight.GGOBoss == nil || fight.BossStart == nil {
			return nil
		}
		frozen := fight.GGOBoss
		if frozen.InputHash == "" {
			return errors.New("首领冻结输入摘要缺失")
		}
		*out = model.GGOFieldBossActiveResponse{Active: true, FightID: fight.BossStart.GetFightId(), EventID: frozen.EventID, InputHash: frozen.InputHash, StartedAtMS: fight.StartedAt, EndsAtMS: frozen.EndsAtMS}
		return nil
	})
	return out, err
}

func (s *GameService) GGOFieldBossReceipt(ctx context.Context, request model.GGOFieldBossReceiptRequest, session []byte) (*model.GGOFieldBossBattleReceipt, error) {
	out := &model.GGOFieldBossBattleReceipt{State: "unknown", FightID: request.FightID}
	err := s.ggoWithProfile(ctx, request.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		var row db.GGOFieldReceipt
		err := tx.Where("player_id=? AND request_id=? AND operation=?", a.Row.Id, ggoSystemReceiptID("boss-fight", request.FightID), constUtil.GGOFieldBossFightOperation).Take(&row).Error
		if err == nil {
			if json.Unmarshal(row.Response, out) != nil || out.State != "settled" || out.FightID != request.FightID {
				return errors.New("首领战斗回执损坏")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if a.Game.Field != nil && a.Game.Field.Fight != nil {
			f := a.Game.Field.Fight
			if !f.Settled && f.GGOBoss != nil && f.BossStart != nil && f.BossStart.GetFightId() == request.FightID {
				out.State = "active"
				out.EventID = f.GGOBoss.EventID
				out.InputHash = f.GGOBoss.InputHash
			}
		}
		return nil
	})
	return out, err
}

func ggoBossEndHash(request *pb.CSSidePveBossFightEnd) (string, error) {
	raw, err := (proto.MarshalOptions{Deterministic: true}).Marshal(request)
	if err != nil {
		return "", err
	}
	return p6Digest(raw), nil
}

func ggoBossRecordedEnd(tx *gorm.DB, id int64, request *pb.CSSidePveBossFightEnd) (*pb.SCSidePveBossFightEnd, bool, error) {
	var row db.GGOFieldReceipt
	err := tx.Where("player_id=? AND request_id=? AND operation=?", id, ggoSystemReceiptID("boss-fight", request.GetFightId()), constUtil.GGOFieldBossFightOperation).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	hash, err := ggoBossEndHash(request)
	if err != nil {
		return nil, true, err
	}
	var saved model.GGOFieldBossBattleReceipt
	if json.Unmarshal(row.Response, &saved) != nil || saved.State != "settled" || saved.FightID != request.GetFightId() || saved.EventID != request.GetUuid() || saved.InputHash == "" || saved.EndRequestHash != hash || len(saved.NativeEnd) == 0 {
		return nil, true, ggoFieldError(constUtil.GGOFieldRequestConflict, "首领结果与原始回执不一致")
	}
	result := &pb.SCSidePveBossFightEnd{}
	if proto.Unmarshal(saved.NativeEnd, result) != nil || result.GetCode() != 1 {
		return nil, true, errors.New("首领原生结果回执损坏")
	}
	return result, true, nil
}

func (s *GameService) ggoBossApplyDamageLocked(tx *gorm.DB, id int64, fight *model.FieldFight, input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome, now int64) (*pb.SideBoss, float64, error) {
	frozen := fight.GGOBoss
	boss := proto.Clone(frozen.Boss).(*pb.SideBoss)
	line, err := ggoBossFrozenLine(frozen)
	if err != nil {
		return nil, 0, err
	}
	var row db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(tx.Clauses(clause.Locking{Strength: "UPDATE"}), s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, line).Take(&row).Error; err != nil {
		return nil, 0, err
	}
	// The live event can advance while an accepted original battle continues.
	// Finishing that battle still resolves the player; it cannot damage a replacement.
	if row.UUID != frozen.EventID || now >= frozen.EndsAtMS {
		boss.State = proto.Int32(3)
		boss.CurHp = proto.Float64(0)
		return boss, 0, nil
	}
	event, shared, hp, err := ggoBossDecode(&row)
	if err != nil {
		return nil, 0, err
	}
	if row.KillerID != 0 {
		return shared, 0, nil
	}
	if len(input.Monster.GetFormation()) != 1 {
		return nil, 0, errors.New("沙漠首领必须为单体")
	}
	actor := input.Monster.Formation[0]
	var final *model.CombatFinalActorHP
	for i := range terminal.FinalActors {
		candidate := &terminal.FinalActors[i]
		if candidate.Camp != 1 {
			continue
		}
		if final != nil || candidate.GUID != actor.GetGuid() {
			return nil, 0, errors.New("首领终局敌方成员无效")
		}
		final = candidate
	}
	if final == nil || !combatFinite(final.CurrentHP) || final.CurrentHP < 0 || final.MaxHP != actor.GetProp().GetHp() {
		return nil, 0, errors.New("首领终局血量无效")
	}
	key := ggoBossGUID(event.ID)
	damage := math.Max(0, actor.GetInst().GetCurHp()-final.CurrentHP)
	damage = math.Min(hp[key], damage)
	if !combatFinite(damage) {
		return nil, 0, errors.New("首领有效伤害无效")
	}
	if damage <= 0 {
		return shared, 0, nil
	}
	hp[key] = math.Max(0, hp[key]-damage)
	shared.CurHp = proto.Float64(hp[key] / float64(event.Boss.HP))
	var member db.FieldBossContribution73
	if err = tx.Where("server_id=? AND spawn_uuid=? AND player_id=?", s.OriginalServerID(), event.ID, id).Take(&member).Error; err != nil {
		return nil, 0, err
	}
	if member.Frozen || !combatFinite(member.Damage) || member.Damage < 0 || member.Damage > float64(event.Boss.HP)-damage {
		return nil, 0, errors.New("首领贡献状态无效")
	}
	if err = tx.Model(&member).Update("damage", member.Damage+damage).Error; err != nil {
		return nil, 0, err
	}
	if hp[key] == 0 {
		row.KillerID = id
		row.RespawnAt = event.EndsAtMS
		shared.State = proto.Int32(3)
		if err = s.ggoBossFreezeAwardsLocked(tx, event, id); err != nil {
			return nil, 0, err
		}
	}
	if err = saveSharedFieldBoss55(tx, &row, shared, hp); err != nil {
		return nil, 0, err
	}
	return shared, damage, nil
}

func ggoBossFrozenLine(frozen *model.GGOFieldBossFight) (int64, error) {
	if frozen.Line == 0 {
		return constUtil.GGOFieldBossStorageLine, nil
	}
	if frozen.Line < 0 || frozen.Line > math.MaxInt32 {
		return 0, errors.New("沙漠首领冻结线路无效")
	}
	return frozen.Line, nil
}

// Original 30085 finalizes one input and persists the exact 30086 before any
// asynchronous recipient delivery. Every later replay is independent of FieldFight.
func (s *GameService) ggoBossFightEnd(ctx context.Context, id int64, request *pb.CSSidePveBossFightEnd) ([]proto.Message, error) {
	var result *pb.SCSidePveBossFightEnd
	var side *pb.PlayerSide
	var session []byte
	replayed := false
	changed := false
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		profile, err := ggoNativeProfileSession(tx, id)
		if err != nil {
			return err
		}
		if AdminProfileServerID(*profile) != s.OriginalServerID() {
			return constUtil.ErrAdminPermission
		}
		session = append([]byte(nil), profile.Session...)
		cached, found, err := ggoBossRecordedEnd(tx, id, request)
		if err != nil {
			return err
		}
		if found {
			result = cached
			replayed = true
			return nil
		}
		if g.Field == nil || g.Field.Side == nil || g.Field.Fight == nil {
			return errors.New("首领战斗不存在")
		}
		fight := g.Field.Fight
		frozen := fight.GGOBoss
		if frozen == nil || frozen.Boss == nil || fight.BossStart == nil || fight.BossStart.GetFightId() != request.GetFightId() || frozen.EventID != request.GetUuid() || fight.Settled {
			return errors.New("首领战斗不匹配")
		}
		if request.GetOptRecord() == nil || request.GetResult() != 0 && request.GetResult() != 1 {
			return errors.New("首领战斗结束参数无效")
		}
		input, err := fieldBossFrozenInput(fight)
		if err != nil {
			return err
		}
		if input.BattleType != constUtil.FieldBossBattleType || len(input.Monster.GetFormation()) != 1 {
			return errors.New("沙漠首领冻结输入无效")
		}
		if _, err = s.ValidateP6GenericMechanisms(tx.Statement.Context, input); err != nil {
			return err
		}
		terminal, err := s.ComputeOriginalP6ProtocolFinalHP(tx.Statement.Context, input, request.GetOptRecord())
		if err != nil {
			// The original automatic driver can emit several presses at the same
			// instant, whereas PlaybackAI consumes at most one per tick. Preserve
			// strict playback for ordinary records and accept this alternate path
			// only when a fresh automatic calculation reproduces the entire record.
			if contextErr := tx.Statement.Context.Err(); contextErr != nil {
				return contextErr
			}
			if !strings.HasPrefix(err.Error(), constUtil.P6ReplayConflict+": 原录像终局或完整技能记录不符") &&
				!strings.HasPrefix(err.Error(), "录像操作不符合原技能时序:") {
				return err
			}
			originalErr := err
			automatic, autoErr := s.ComputeOriginalP6ProtocolFinalHP(tx.Statement.Context, input, nil)
			if contextErr := tx.Statement.Context.Err(); contextErr != nil {
				return contextErr
			}
			if errors.Is(autoErr, context.Canceled) || errors.Is(autoErr, context.DeadlineExceeded) {
				return autoErr
			}
			if autoErr != nil || automatic == nil || automatic.OriginalOperates == nil {
				return originalErr
			}
			marshal := proto.MarshalOptions{Deterministic: true}
			actualRecord, actualErr := marshal.Marshal(request.GetOptRecord())
			expectedRecord, expectedErr := marshal.Marshal(automatic.OriginalOperates)
			if actualErr != nil || expectedErr != nil || !bytes.Equal(actualRecord, expectedRecord) {
				return originalErr
			}
			terminal = automatic
		}
		if err = validateP6GenericMechanismTerminal(input, terminal); err != nil {
			return err
		}
		if terminal == nil || terminal.Outcome == nil || terminal.Outcome.Winner != 0 && terminal.Outcome.Winner != 1 {
			return errors.New("首领原计算未产生有效胜败")
		}
		expected := int32(0)
		if terminal.Outcome.Winner == 0 {
			expected = 1
		}
		if request.GetResult() != expected {
			return errors.New("首领结果与原计算不一致")
		}
		if err = ggoMonsterApplyTerminal(input.Attacker, g.Field.Side, terminal); err != nil {
			return err
		}
		side = g.Field.Side
		now := time.Now().UnixMilli()
		boss, damage, err := s.ggoBossApplyDamageLocked(tx, id, fight, input, terminal, now)
		if err != nil {
			return err
		}
		changed = damage > 0
		var lost [6]int64
		// Reaching the ten-own-turn limit with surviving heroes is a contribution,
		// not a player death. Only an actually defeated party loses carried chips.
		if side.GetCurHp() <= 0 {
			lost, err = s.ggoPVEDeathLocked(tx, id, player, g, &model.GGOFieldMonsterFight{FightID: request.GetFightId(), EconomyVersion: frozen.EconomyVersion, ExchangeValues: frozen.ExchangeValues, ProtectedChips: frozen.ProtectedChips, DeathLossPercent: frozen.DeathLossPercent, SafeReturn: frozen.SafeReturn, HealRatio: frozen.HealRatio}, now)
			if err != nil {
				return err
			}
		}
		result = &pb.SCSidePveBossFightEnd{Get: &pb.ResultObject{}, FightResult: proto.Int32(request.GetResult()), Code: proto.Int32(1), Anger: proto.Float64(side.GetAnger()), State: side.State, Monster: boss}
		for _, part := range side.GetBattlePosHps() {
			result.BattlePos = append(result.BattlePos, part.GetBattlePos())
			result.Hps = append(result.Hps, part.GetHpPercent())
			switch part.GetBattlePos() {
			case 3:
				result.Hp3 = part.HpPercent
			case 7:
				result.Hp7 = part.HpPercent
			case 9:
				result.Hp9 = part.HpPercent
			case 13:
				result.Hp13 = part.HpPercent
			}
		}
		native, err := (proto.MarshalOptions{Deterministic: true}).Marshal(result)
		if err != nil {
			return err
		}
		hash, err := ggoBossEndHash(request)
		if err != nil {
			return err
		}
		revision := int64(0)
		if g.GGOField != nil {
			revision = g.GGOField.Revision
		}
		receipt := model.GGOFieldBossBattleReceipt{State: "settled", FightID: request.GetFightId(), EventID: frozen.EventID, InputHash: frozen.InputHash, EndRequestHash: hash, NativeEnd: native, Damage: damage, Lost: lost, TerminalRevision: revision}
		key := request.GetFightId()
		if err = ggoSaveReceipt(tx, id, ggoSystemReceiptID("boss-fight", key), constUtil.GGOFieldBossFightOperation, hash, frozen.EconomyVersion, &key, [6]int64{}, 0, now, receipt); err != nil {
			return err
		}
		fight.Settled = true
		fight.BossResult = proto.Clone(result).(*pb.SCSidePveBossFightEnd)
		g.Field.Bosses = []*pb.SideBoss{proto.Clone(boss).(*pb.SideBoss)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := []proto.Message{result}
	if replayed {
		return out, nil
	}
	if side != nil {
		out = append(out, &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{side}})
	}
	if changed {
		// Payload-free hints also refresh a player who changed line mid-fight;
		// each recipient reads its own authorized line when it resnapshots.
		s.ggoNotifyFieldChange(ctx, 0)
	}
	// Award collection owns only this recipient profile and is retried by the
	// existing snapshot/login path if delivery fails after the battle committed.
	gain, claimErr := s.CollectFieldBossPendingRewardsWithSession(ctx, id, session)
	if claimErr != nil {
		elog.Warn("沙漠首领战斗已结算，奖励待重试", zap.Int64("playerId", id), zap.Error(claimErr))
	} else if gain != nil {
		out = append(out, &pb.SCAddRewardsToPlayer{Result: gain})
	}
	return out, nil
}
