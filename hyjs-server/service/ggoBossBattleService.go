package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
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

// The frozen event selects the legacy single encounter or the original
// boss_003 formation, so retained old events keep their visible model.
func (s *GameService) ggoBossMonster(event *ggoBossEvent, currentHP float64) (*pb.BattleMonster, error) {
	return s.ggoBossMonsterWithHealth(event, currentHP, nil)
}

func (s *GameService) ggoBossMonsterWithHealth(event *ggoBossEvent, currentHP float64, sharedHP map[string]float64) (*pb.BattleMonster, error) {
	if event == nil || event.ID == "" || event.Boss.Level < 1 || event.Boss.HP < 1 || event.Boss.Attack < 1 || event.Boss.PhyDefend < 0 || event.Boss.MagicDefend < 0 {
		return nil, errors.New("沙漠首领事件参数无效")
	}
	if !combatFinite(currentHP) || currentHP <= 0 || currentHP > float64(event.Boss.HP) {
		return nil, errors.New("沙漠首领当前生命无效")
	}
	if event.Boss.HeroID == "hero_017" {
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
		actor.Inst.CurHp = proto.Float64(currentHP)
		return &pb.BattleMonster{MonsterId: proto.Int64(1), ServerId: proto.Int32(1), TaskId: proto.String(constUtil.GGOFieldBossID), GroupIndex: proto.Int32(1), Formation: []*pb.BattleHero{actor}, Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}, nil
	}
	if event.Boss.HeroID != "monster_047" {
		return nil, errors.New("沙漠首领事件模型无效")
	}
	rows := s.fieldBossRows(constUtil.GGOFieldBossID)
	if len(rows) != 4 {
		return nil, errors.New("沙漠首领原阵容缺失")
	}
	monster := &pb.BattleMonster{MonsterId: proto.Int64(1), ServerId: proto.Int32(1), TaskId: proto.String(constUtil.GGOFieldBossID), GroupIndex: proto.Int32(1), Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
	seen := map[string]bool{}
	for _, base := range rows {
		location := base["Location"]
		leader := location == "13"
		if seen[location] || (location != "13" && location != "7" && location != "9" && location != "3") ||
			(leader && (base["TeamLeader"] != "1" || base["HeroId"] != "monster_047")) ||
			(!leader && (base["TeamLeader"] != "0" || base["HeroId"] != "monster_033")) {
			return nil, errors.New("沙漠首领原阵容无效")
		}
		seen[location] = true
		hero := s.table("Hero", "id", base["HeroId"])
		if hero == nil || hero["resName"] == "" {
			return nil, errors.New("沙漠首领原模型缺失")
		}
		row := make(map[string]string, len(base))
		for key, value := range base {
			row[key] = value
		}
		guid := fmt.Sprintf("%s-%s", ggoBossGUID(event.ID), location)
		if leader {
			guid = ggoBossGUID(event.ID)
			row["Level"] = fmt.Sprint(event.Boss.Level)
			row["Hp"] = fmt.Sprint(event.Boss.HP)
			row["Attack"] = fmt.Sprint(event.Boss.Attack)
			row["PhyDefend"] = fmt.Sprint(event.Boss.PhyDefend)
			row["MagicDefend"] = fmt.Sprint(event.Boss.MagicDefend)
		} else {
			// Baseline used by the original-engine 15-party calibration; skill,
			// talents and all other properties continue to come from boss_003.
			row["Level"], row["Hp"], row["Attack"] = "60", "573045", "37500"
			row["PhyDefend"], row["MagicDefend"] = "7000", "7000"
		}
		actor, err := s.actor(row, guid, int32(tableNumber(row, "Location")), true)
		if err != nil {
			return nil, err
		}
		if leader {
			actor.Inst.CurHp = proto.Float64(currentHP)
		} else if saved, ok := sharedHP[guid]; ok {
			if !combatFinite(saved) || saved < 0 || saved > actor.Prop.GetHp() {
				return nil, errors.New("沙漠首领随从当前生命无效")
			}
			actor.Inst.CurHp = proto.Float64(saved)
		}
		monster.Formation = append(monster.Formation, actor)
	}
	return monster, nil
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
		monster, err := s.ggoBossMonsterWithHealth(event, hp[ggoBossGUID(event.ID)], hp)
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
			rulesEvent := *event
			rulesEvent.AttendantHP = nil
			rules, e := json.Marshal(rulesEvent)
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
			return s.ggoBossRewardStatusLocked(tx, a.Row.Id, out)
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

// A reported loss with an actual, bounded terminal record and zero attacker
// HP is a request to forfeit. Protobuf-net can omit the zero-valued fields.
// This report is never used to calculate boss damage or award eligibility.
func ggoBossRequestedForfeit(request *pb.CSSidePveBossFightEnd, maxRound int32) bool {
	if request == nil || request.GetResult() != 0 || request.OptRecord == nil || maxRound < 1 {
		return false
	}
	record := request.GetOptRecord()
	round, hp := record.GetFinalRoundNum(), record.GetAttackerTotalLeftHp()
	return round >= 1 && round <= maxRound+1 && combatFinite(hp) && hp == 0
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

// ggoBossLeaderDamage validates every frozen enemy and its terminal HP, while
// only the leader contributes to the line's shared event health.
func ggoBossLeaderDamage(fight *model.FieldFight, input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome) (float64, error) {
	damage, err := ggoBossActorDamage(fight, input, terminal)
	if err != nil {
		return 0, err
	}
	return damage[ggoBossGUID(fight.GGOBoss.EventID)], nil
}

func ggoBossActorDamage(fight *model.FieldFight, input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome) (map[string]float64, error) {
	if fight == nil || fight.GGOBoss == nil || fight.GGOBoss.EventID == "" || input == nil || input.Monster == nil || terminal == nil {
		return nil, errors.New("沙漠首领冻结战斗无效")
	}
	leaderGUID := ggoBossGUID(fight.GGOBoss.EventID)
	if fight.BossLeaderGUID != "" && fight.BossLeaderGUID != leaderGUID {
		return nil, errors.New("沙漠首领冻结标识无效")
	}
	formation := input.Monster.GetFormation()
	if len(formation) != 1 && len(formation) != 4 {
		return nil, errors.New("沙漠首领冻结阵容无效")
	}
	actors := make(map[string]*pb.BattleHero, len(formation))
	for _, actor := range formation {
		if actor == nil || actor.GetGuid() == "" || actor.Prop == nil || actor.Inst == nil || actors[actor.GetGuid()] != nil {
			return nil, errors.New("沙漠首领冻结成员无效")
		}
		maxHP, currentHP := actor.Prop.GetHp(), actor.Inst.GetCurHp()
		if !combatFinite(maxHP) || maxHP <= 0 || !combatFinite(currentHP) || currentHP < 0 || currentHP > maxHP {
			return nil, errors.New("沙漠首领冻结血量无效")
		}
		actors[actor.GetGuid()] = actor
	}
	leader := actors[leaderGUID]
	if leader == nil || leader.Inst.GetCurHp() <= 0 {
		return nil, errors.New("沙漠首领冻结主怪无效")
	}
	seen := make(map[string]bool, len(actors))
	damage := make(map[string]float64, len(actors))
	for _, final := range terminal.FinalActors {
		if final.Camp == 0 {
			continue
		}
		if final.Camp != 1 {
			return nil, errors.New("沙漠首领终局阵营无效")
		}
		actor := actors[final.GUID]
		if actor == nil || seen[final.GUID] || !combatFinite(final.MaxHP) || final.MaxHP != actor.Prop.GetHp() ||
			!combatFinite(final.CurrentHP) || final.CurrentHP < 0 || final.CurrentHP > final.MaxHP {
			return nil, errors.New("沙漠首领终局敌方成员无效")
		}
		seen[final.GUID] = true
		damage[final.GUID] = math.Max(0, actor.Inst.GetCurHp()-final.CurrentHP)
	}
	if len(seen) != len(actors) {
		return nil, errors.New("沙漠首领终局敌方成员缺失")
	}
	return damage, nil
}

// The remaining shared HP and each contributor's damage are accumulated in
// separate float64 operations. On an actual final hit, their exact binary sum
// can exceed the original maximum by a few representable steps. Accept at
// most two ULPs of that maximum and clamp only this terminal rounding residue.
func ggoBossContributionTotal(prior, damage, maximum float64, finalHit bool) (float64, error) {
	invalid := func() (float64, error) { return 0, errors.New("首领贡献状态无效") }
	if !combatFinite(prior) || !combatFinite(damage) || !combatFinite(maximum) || prior < 0 || damage <= 0 || maximum <= 0 || prior > maximum || damage > maximum {
		return invalid()
	}
	total := prior + damage
	if !combatFinite(total) {
		return invalid()
	}
	if prior <= maximum-damage && total <= maximum {
		return total, nil
	}
	if !finalHit {
		return invalid()
	}
	next := math.Nextafter(maximum, math.Inf(1))
	if !combatFinite(next) {
		return invalid()
	}
	// Compare the exact values represented by both operands. A rounded sum
	// alone can conceal an actual overflow beyond the permitted two ULPs.
	exact := new(big.Rat).Add(new(big.Rat).SetFloat64(prior), new(big.Rat).SetFloat64(damage))
	maxExact := new(big.Rat).SetFloat64(maximum)
	ulp := new(big.Rat).Sub(new(big.Rat).SetFloat64(next), maxExact)
	upper := new(big.Rat).Add(maxExact, new(big.Rat).Mul(ulp, big.NewRat(2, 1)))
	if exact.Cmp(upper) > 0 {
		return invalid()
	}
	return min(total, maximum), nil
}

func (s *GameService) ggoBossApplyDamageLocked(tx *gorm.DB, id int64, fight *model.FieldFight, input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome, now int64) (*pb.SideBoss, float64, error) {
	actorDamage, err := ggoBossActorDamage(fight, input, terminal)
	if err != nil {
		return nil, 0, err
	}
	frozen := fight.GGOBoss
	if frozen.Boss == nil {
		return nil, 0, errors.New("沙漠首领冻结投影无效")
	}
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
	if row.UUID != frozen.EventID {
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
	key := ggoBossGUID(event.ID)
	if event.Boss.HeroID == "monster_047" && len(hp) == 1 {
		// Missing keys are an old event without stored attendant history.
		for _, pose := range []string{"3", "7", "9"} {
			hp[key+"-"+pose] = 573045
		}
	}
	for guid := range actorDamage {
		if _, ok := hp[guid]; !ok {
			return nil, 0, errors.New("沙漠首领冻结成员与共享事件不匹配")
		}
	}
	changed := false
	var damage float64
	for guid, dealt := range actorDamage {
		applied := math.Min(hp[guid], dealt)
		if !combatFinite(applied) {
			return nil, 0, errors.New("首领有效伤害无效")
		}
		if applied > 0 {
			hp[guid] -= applied
			changed = true
		}
		if guid == key {
			damage = applied
		}
	}
	if !combatFinite(damage) {
		return nil, 0, errors.New("首领有效伤害无效")
	}
	if !changed {
		return shared, 0, nil
	}
	shared.CurHp = proto.Float64(hp[key] / float64(event.Boss.HP))
	if damage > 0 {
		var member db.FieldBossContribution73
		if err = tx.Where("server_id=? AND spawn_uuid=? AND player_id=?", s.OriginalServerID(), event.ID, id).Take(&member).Error; err != nil {
			return nil, 0, err
		}
		if member.Frozen {
			return nil, 0, errors.New("首领贡献状态无效")
		}
		total, err := ggoBossContributionTotal(member.Damage, damage, float64(event.Boss.HP), hp[key] == 0)
		if err != nil {
			return nil, 0, err
		}
		if err = tx.Model(&member).Update("damage", total).Error; err != nil {
			return nil, 0, err
		}
	}
	if hp[key] == 0 && damage > 0 {
		row.KillerID = id
		if now > math.MaxInt64-constUtil.GGOFieldBossRespawnMS {
			return nil, 0, errors.New("沙漠首领刷新时间溢出")
		}
		row.RespawnAt = now + constUtil.GGOFieldBossRespawnMS
		shared.State = proto.Int32(3)
		if err = s.ggoBossFreezeAwardsLocked(tx, event, id); err != nil {
			return nil, 0, err
		}
	}
	if err = saveGGOBossState(tx, &row, event, shared, hp); err != nil {
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
	var killNotice *pb.SCPublicMessage
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
		if request.GetResult() != 0 && request.GetResult() != 1 {
			return errors.New("首领战斗结束参数无效")
		}
		input, err := fieldBossFrozenInput(fight)
		if err != nil {
			return err
		}
		if input.BattleType != constUtil.FieldBossBattleType || (len(input.Monster.GetFormation()) != 1 && len(input.Monster.GetFormation()) != 4) {
			return errors.New("沙漠首领冻结输入无效")
		}
		if _, err = s.ValidateP6GenericMechanisms(tx.Statement.Context, input); err != nil {
			return err
		}
		terminal, expected, err := s.ggoBattleTerminal(tx.Statement.Context, input, request.GetOptRecord(), request.GetResult(), nil, id, false)
		if err != nil {
			return err
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
		// Under the shared event row lock, positive applied leader damage with
		// zero remaining HP is exactly the first durable KillerID transition.
		// The enclosing transaction still owns the receipt and profile writes.
		if damage > 0 && boss.GetState() == 3 && boss.GetCurHp() == 0 {
			killNotice = s.ggoBossKillNotice(player.GetName(), boss.GetHeroId())
		}
		var lost [6]int64
		// A surviving round-limit outcome keeps its contribution without death.
		// A client-reported zero-HP loss can also request a self-funded forfeit,
		// while damage and rewards remain derived from the server terminal.
		forfeit := side.GetCurHp() > 0 && ggoBossRequestedForfeit(request, input.MaxRound)
		defeated := side.GetCurHp() <= 0 || forfeit
		if defeated {
			lost, err = s.ggoPVEDeathLocked(tx, id, player, g, &model.GGOFieldMonsterFight{FightID: request.GetFightId(), EconomyVersion: frozen.EconomyVersion, ExchangeValues: frozen.ExchangeValues, ProtectedChips: frozen.ProtectedChips, DeathLossPercent: frozen.DeathLossPercent, SafeReturn: frozen.SafeReturn, HealRatio: frozen.HealRatio}, now)
			if err != nil {
				return err
			}
		}
		if forfeit {
			expected = 0
		}
		result = &pb.SCSidePveBossFightEnd{Get: &pb.ResultObject{}, FightResult: proto.Int32(expected), Code: proto.Int32(1), Anger: proto.Float64(side.GetAnger()), State: side.State, Monster: boss}
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
		receipt := model.GGOFieldBossBattleReceipt{State: "settled", FightID: request.GetFightId(), EventID: frozen.EventID, InputHash: frozen.InputHash, EndRequestHash: hash, NativeEnd: native, Damage: damage, Lost: lost, TerminalRevision: revision, Defeated: defeated, Forfeit: forfeit}
		if defeated {
			if g.Position == nil || g.Position.Current == nil || g.GGOField == nil || g.GGOField.PositionRevision < 1 {
				return errors.New("沙漠死亡返回位置缺失")
			}
			point := g.Position.Current
			x, y, z := float64(point.GetX()), float64(point.GetY()), float64(point.GetZ())
			receipt.SafeReturn = &model.AdminFieldGGOPosition{X: &x, Y: &y, Z: &z}
			receipt.PositionRevision = g.GGOField.PositionRevision
		}
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
	// Never replay after restart: a committed KillerID is the durable once
	// marker. Like the existing field marquee, a process exit between commit
	// and TCP send can omit this transient notice.
	s.publishP12WorldBossKill(context.WithoutCancel(ctx), killNotice)
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
