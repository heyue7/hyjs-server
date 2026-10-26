package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// fieldBossRows 只取原首领第一组；当前两张原图的首领各为一组四人。
func (s *GameService) fieldBossRows(bossID string) []map[string]string {
	return s.fieldBossRowsInScene(bossID, "")
}

func (s *GameService) fieldBossRowsInScene(bossID, scene string) []map[string]string {
	var rows []map[string]string
	for _, row := range s.adminFieldCombatRows("FieldBoss", "BossId", bossID, scene) {
		if row["BossId"] == bossID && row["GroupIndex"] == "1" {
			rows = append(rows, row)
		}
	}
	return rows
}

func (s *GameService) fieldBossLeader(bossID string) map[string]string {
	return s.fieldBossLeaderInScene(bossID, "")
}

func (s *GameService) fieldBossLeaderInScene(bossID, scene string) map[string]string {
	for _, row := range s.fieldBossRowsInScene(bossID, scene) {
		if row["TeamLeader"] == "1" {
			return row
		}
	}
	return nil
}

// fieldBossSpawn 按原 FieldBossPos 权重与出生点生成个人实例，初始满血。
func (s *GameService) fieldBossSpawn(posID string) (*pb.SideBoss, error) {
	pos := s.table("FieldBossPos", "BoxxPosId", posID)
	if pos == nil || pos["BornPos"] == "" || pos["bossIdProbs"] == "" {
		return nil, errors.New("缺少原野外首领出生点")
	}
	bossID, err := fieldWeighted(pos["bossIdProbs"])
	if err != nil {
		return nil, err
	}
	rows := s.fieldBossRowsInScene(bossID, pos["adminScene"])
	leader := s.fieldBossLeaderInScene(bossID, pos["adminScene"])
	if len(rows) != 4 || leader == nil {
		return nil, errors.New("原野外首领第一组必须为四成员")
	}
	point, err := fieldPoint(pos["BornPos"])
	if err != nil {
		return nil, err
	}
	uuid, err := fieldToken()
	if err != nil {
		return nil, err
	}
	nature := int32(tableNumber(pos, "nature"))
	return &pb.SideBoss{Uuid: proto.String(uuid), BossPosId: proto.String(posID), BossId: proto.String(bossID), CurrentPoint: point, State: proto.Int32(1), CurHp: proto.Float64(1), HeroId: proto.String(leader["HeroId"]), Nature: proto.Int32(nature)}, nil
}

// refreshFieldBosses 只为当前地图的原始点位补充实例，并按独立 uuid 重生。
func (s *GameService) refreshFieldBosses(f *model.FieldState, row map[string]string) error {
	if f.BossHP == nil {
		f.BossHP = map[string]map[string]float64{}
	}
	positions := []string{}
	for _, id := range strings.Split(row["bossPosIds"], ";") {
		if id != "" {
			positions = append(positions, id)
		}
	}
	// 关闭普通首领只撤下未进入战斗的实例，已接受战斗保留冻结输入与结算。
	if row["adminOrdinaryBossEnabled"] == "false" {
		kept := f.Bosses[:0]
		for _, boss := range f.Bosses {
			if boss.GetState() == 2 {
				kept = append(kept, boss)
				continue
			}
			delete(f.RespawnAt, boss.GetUuid())
			delete(f.BossHP, boss.GetUuid())
		}
		f.Bosses = kept
		return nil
	}
	if len(positions) == 0 {
		f.Bosses = nil
		return nil
	}
	now := time.Now().UnixMilli()
	for i, boss := range f.Bosses {
		if boss.GetState() == 2 {
			continue
		}
		due := f.RespawnAt[boss.GetUuid()]
		if due == 0 || due > now {
			continue
		}
		replacement, err := s.fieldBossSpawn(boss.GetBossPosId())
		if err != nil {
			return err
		}
		delete(f.RespawnAt, boss.GetUuid())
		delete(f.BossHP, boss.GetUuid())
		f.Bosses[i] = replacement
	}
	known := map[string]bool{}
	for _, boss := range f.Bosses {
		known[boss.GetBossPosId()] = true
	}
	for _, posID := range positions {
		if known[posID] {
			continue
		}
		boss, err := s.fieldBossSpawn(posID)
		if err != nil {
			return err
		}
		f.Bosses = append(f.Bosses, boss)
	}
	return nil
}

func fieldBossFind(f *model.FieldState, uuid string) *pb.SideBoss {
	for _, boss := range f.Bosses {
		if boss.GetUuid() == uuid {
			return boss
		}
	}
	return nil
}

// fieldBossProjection 只投影当前地图真实存在的个人首领实例。
func (s *GameService) fieldBossProjection(g *model.GameState) []proto.Message {
	if g.Field == nil || len(g.Field.Bosses) == 0 {
		return nil
	}
	return []proto.Message{&pb.SCSideBoss{Monsters: g.Field.Bosses}}
}

// fieldBossCountdownProjection uses the original seconds-based UI countdown.
// Clear absent nature slots too: the native labels survive map switches.
func fieldBossCountdownProjection(field *model.FieldState, now time.Time) []proto.Message {
	var deadlines [6]int64
	if field != nil {
		for _, boss := range field.Bosses {
			nature := boss.GetNature()
			if nature < 0 || nature >= int32(len(deadlines)) || boss.GetState() != 3 {
				continue
			}
			due := field.RespawnAt[boss.GetUuid()]
			if due > now.UnixMilli() && (deadlines[nature] == 0 || due < deadlines[nature]) {
				deadlines[nature] = due
			}
		}
	}
	messages := make([]proto.Message, 0, len(deadlines))
	for nature, due := range deadlines {
		remaining := int64(0)
		if due > now.UnixMilli() {
			remaining = (due - now.UnixMilli() + 999) / 1000
		}
		messages = append(messages, &pb.SCSideBossCountdown{Nature: proto.Int32(int32(nature)), Time: proto.Int64(remaining)})
	}
	return messages
}

func (s *GameService) fieldBossCheck(ctx context.Context, id int64, b *pb.CSSideBossCheck) ([]proto.Message, error) {
	if b.GetUuid() == "" {
		return nil, errors.New("缺少野外首领实例")
	}
	var result *pb.SCSideBossCheck
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
		if s.fieldSceneRow(g.Position.GetScene()) == nil || !s.P8FieldSceneAvailable(g.Position.GetScene()) {
			return errors.New(constUtil.P8FieldSceneUnavailable)
		}
		if err := s.refreshField(id, g); err != nil {
			return err
		}
		if err := s.refreshSharedFieldBosses55(tx, g); err != nil {
			return err
		}
		boss := fieldBossFind(g.Field, b.GetUuid())
		if boss == nil || boss.GetState() == 3 {
			// 原客户端2003分支仍按 uuid 移除旧模型。
			result = &pb.SCSideBossCheck{Code: proto.Int32(2003), Monster: &pb.SideBoss{Uuid: proto.String(b.GetUuid()), State: proto.Int32(3), CurHp: proto.Float64(0)}}
			return nil
		}
		result = &pb.SCSideBossCheck{Code: proto.Int32(1), Monster: boss}
		return nil
	})
	return []proto.Message{result}, err
}

// fieldBossMonster 以冻结的等级/表行重建第一组，跨次战斗从 BossHP 恢复每槽血量。
func (s *GameService) fieldBossMonster(uuid string, def *model.GameState, boss *pb.SideBoss) (*pb.BattleMonster, error) {
	rows := s.fieldBossRowsInScene(boss.GetBossId(), def.Field.Scene)
	if len(rows) != 4 {
		return nil, errors.New("原野外首领第一组必须为四成员")
	}
	monster := &pb.BattleMonster{MonsterId: proto.Int64(1), ServerId: proto.Int32(1), TaskId: proto.String(boss.GetBossId()), GroupIndex: proto.Int32(1), Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
	saved := def.Field.BossHP[uuid]
	for _, row := range rows {
		guid := "field-boss-" + uuid + "-" + row["Id"]
		actor, err := s.actor(row, guid, int32(tableNumber(row, "Location")), true)
		if err != nil {
			return nil, err
		}
		if hp, ok := saved[guid]; ok {
			if hp < 0 || hp > actor.GetProp().GetHp() {
				return nil, errors.New("原野外首领保存血量无效")
			}
			actor.Inst.CurHp = proto.Float64(hp)
		}
		monster.Formation = append(monster.Formation, actor)
	}
	return monster, nil
}

func fieldBossRewardText(raw string) (string, error) {
	parts := []string{}
	for _, item := range strings.Split(raw, "|") {
		if item == "" {
			continue
		}
		pair := strings.SplitN(item, ":", 2)
		if len(pair) != 2 {
			return "", errors.New("原野外首领奖励配置无效")
		}
		quantity, err := fieldWeighted(pair[1])
		if err != nil {
			return "", err
		}
		num, err := strconv.ParseInt(quantity, 10, 32)
		if err != nil || num < 0 {
			return "", errors.New("原野外首领奖励数量无效")
		}
		if num > 0 {
			parts = append(parts, pair[0]+","+strconv.FormatInt(num, 10))
		}
	}
	return strings.Join(parts, ";"), nil
}

// fieldBossFightStart 与野外小怪共用 FieldFight 占用，冻结完整表行、奖励和重生间隔。
func (s *GameService) fieldBossFightStart(ctx context.Context, id int64, b *pb.CSSidePveBossFightStart) ([]proto.Message, error) {
	if b.GetUuid() == "" {
		return nil, errors.New("缺少野外首领实例")
	}
	var result *pb.SCSidePveBossFightStart
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		if err := checkFieldBattleClient(tx, id, g.Position.GetScene()); err != nil {
			return err
		}
		if g.Run != nil && !g.Run.Complete {
			return errors.New("请先完成当前副本")
		}
		if s.fieldSceneRow(g.Position.GetScene()) == nil || !s.P8FieldSceneAvailable(g.Position.GetScene()) {
			return errors.New(constUtil.P8FieldSceneUnavailable)
		}
		if err := s.refreshField(id, g); err != nil {
			return err
		}
		if err := s.refreshSharedFieldBosses55(tx, g); err != nil {
			return err
		}
		f := g.Field
		if f.Fight != nil && !f.Fight.Settled {
			if f.Fight.BossStart != nil && f.Fight.BossUUID == b.GetUuid() {
				result = f.Fight.BossStart
				return nil
			}
			return errors.New("请先完成当前野外战斗")
		}
		// 关闭后只拒绝新的开战；上面已接受的同一战斗仍可重取冻结输入。
		if scene := s.fieldSceneRow(g.Position.GetScene()); scene == nil || scene["adminOrdinaryBossEnabled"] == "false" {
			return errors.New("普通首领已关闭刷新与开战")
		}
		boss := fieldBossFind(f, b.GetUuid())
		if boss == nil || boss.GetState() != 1 {
			return errors.New("野外首领不存在或已被击败")
		}
		if player.GetLevel() < int32(tableNumber(s.fieldSceneRow(f.Scene), "levelLimit")) {
			return errors.New(constUtil.P8FieldSceneLevel)
		}
		if err := s.ensureNoActiveDailyCopy(tx, id, time.Now()); err != nil {
			return err
		}
		if s.P8GatherReady {
			if err := s.P8CancelGather(tx, id, g); err != nil {
				return err
			}
		}
		if f.Side.GetCurHp() <= 0 {
			return errors.New("野外角色已倒下，复活功能尚未恢复")
		}
		party, err := s.battlePlayer(player, g)
		if err != nil || len(party.GetFormation()) == 0 {
			return errors.New("野外首领战斗需要可用编队")
		}
		for _, hero := range party.Formation {
			for _, hp := range f.Side.GetBattlePosHps() {
				if hero.GetPose() == hp.GetBattlePos() {
					hero.Inst.CurHp = proto.Float64(hero.GetProp().GetHp() * hp.GetHpPercent())
				}
			}
		}
		party.Inst.Anger = proto.Float64(f.Side.GetAnger())
		monster, err := s.fieldBossMonster(boss.GetUuid(), g, boss)
		if err != nil {
			return err
		}
		leader := s.fieldBossLeaderInScene(boss.GetBossId(), f.Scene)
		if leader == nil {
			return errors.New("原野外首领缺少队长行")
		}
		leaderGUID := "field-boss-" + boss.GetUuid() + "-" + leader["Id"]
		reward, err := fieldBossRewardText(leader["rewardGroupId"])
		if err != nil {
			return err
		}
		kill, err := fieldBossRewardText(leader["KillReward"])
		if err != nil {
			return err
		}
		hours, err := strconv.ParseInt(leader["RebornTime"], 10, 64)
		if err != nil || hours <= 0 {
			return errors.New("原野外首领重生小时配置无效")
		}
		seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
		if err != nil {
			return err
		}
		rounds, err := s.fieldConfigInt("FightMaxRoundNum")
		if err != nil {
			return err
		}
		frozen, err := s.freezeAndQualifyP6GenericMechanisms(ctx, model.FrozenBattleInput{Attacker: party, Monster: monster, Seed: seed.Int64(), MaxRound: int32(rounds), BattleType: constUtil.FieldBossBattleType, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
		if err != nil {
			return err
		}
		token, err := fieldToken()
		if err != nil {
			return err
		}
		result = &pb.SCSidePveBossFightStart{FightId: proto.String(token), RandSeed: proto.Int64(frozen.Seed), BattlePlayer: frozen.Attacker, BattleMonsterList: []*pb.BattleMonster{frozen.Monster}, FightScene: proto.String(""), Code: proto.Int32(1), Maxround: proto.Int32(frozen.MaxRound)}
		// 兼容现有小怪判定的 Start 字段保持同一fightId，BossStart另行标识首领结算入口。
		compatStart := &pb.SCSidePveFightStart{FightId: proto.String(token), RandSeed: proto.Int64(frozen.Seed), BattlePlayer: frozen.Attacker, BattleMonsterList: []*pb.BattleMonster{frozen.Monster}, FightScene: proto.String(""), Code: proto.Int32(1), Maxround: proto.Int32(frozen.MaxRound)}
		rewardLimit, err := s.fieldBossRewardLimit(tx)
		if err != nil {
			return err
		}
		f.Fight = &model.FieldFight{StartedAt: time.Now().UnixMilli(), BossUUID: boss.GetUuid(), BossLeaderGUID: leaderGUID, Start: compatStart, BossStart: result, BossInput: frozen, BossReward: reward, BossKill: kill, BossCooldownMS: hours * 3600 * 1000, BossRewardDailyLimit: &rewardLimit}
		rules, err := s.freezeFieldBossDeath73(tx, f.Scene, boss, leader)
		if err != nil {
			return err
		}
		f.Fight.BossDeathRules73 = rules
		if rules != nil && rules.DeathSettlement {
			sprouts, e := s.fieldBossSprouts73(tx, id, f.Scene, rules.SproutMax)
			if e != nil {
				return e
			}
			f.Fight.BossSprouts73 = sprouts
		}
		boss.State = proto.Int32(2)
		f.Side.State = proto.Int32(6)
		return nil
	})
	return []proto.Message{result}, err
}

// fieldBossFightEnd 只用原计算回放终局，客户端 result 仅作胜败映射校验。
func (s *GameService) fieldBossFightEnd(ctx context.Context, id int64, b *pb.CSSidePveBossFightEnd) ([]proto.Message, error) {
	var (
		result     *pb.SCSidePveBossFightEnd
		side       *pb.PlayerSide
		countdowns []proto.Message
		notice     *pb.SCPublicMessage
	)
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		if g.Field == nil {
			return errors.New("未知野外首领战斗")
		}
		if err := s.refreshField(id, g); err != nil {
			return err
		}
		if err := s.refreshSharedFieldBosses55(tx, g); err != nil {
			return err
		}
		f := g.Field
		// Retries project the saved deadline; they never restart the cooldown.
		defer func() { countdowns = fieldBossCountdownProjection(f, time.Now()) }()
		fight := f.Fight
		if fight == nil || fight.BossStart == nil || fight.BossUUID != b.GetUuid() || fight.BossStart.GetFightId() != b.GetFightId() {
			return errors.New("未知野外首领战斗")
		}
		side = f.Side
		if fight.Settled {
			if fight.BossResult == nil {
				return errors.New("缺少已结算野外首领结果")
			}
			result = fight.BossResult
			return nil
		}
		if g.Position.GetScene() != f.Scene {
			return errors.New("野外首领战斗场景不一致")
		}
		boss := fieldBossFind(f, fight.BossUUID)
		if boss == nil || boss.GetState() != 2 {
			return errors.New("野外首领不在战斗中")
		}
		if b.GetResult() != 0 && b.GetResult() != 1 || b.GetOptRecord() == nil {
			return errors.New("野外首领结算参数无效")
		}
		input, err := fieldBossFrozenInput(fight)
		if err != nil {
			return err
		}
		if _, err := s.ValidateP6GenericMechanisms(ctx, input); err != nil {
			return err
		}
		// Type12 records real player commands, just like the existing original
		// PVE consumer. Playback executes one queued command per tick, so its
		// rerecorded player clock can be one tick later. Keep the existing PVE
		// source-record contract: command identity/order, automatic NPC clocks,
		// final round and both HP totals are validated by the original driver.
		// The exact supplied record is also bound into the calculation digest.
		terminal, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, b.GetOptRecord())
		if err != nil {
			return err
		}
		if err = validateP6GenericMechanismTerminal(input, terminal); err != nil {
			return err
		}
		if terminal == nil || terminal.Outcome == nil || terminal.Outcome.Winner != 0 && terminal.Outcome.Winner != 1 {
			return errors.New("野外首领计算未产生有效胜败")
		}
		expected := int32(0)
		if terminal.Outcome.Winner == 0 {
			expected = 1
		}
		if b.GetResult() != expected {
			return errors.New("野外首领结算胜败与计算不一致")
		}

		expectedAttackers := map[string]*pb.BattleHero{}
		for _, hero := range input.Attacker.GetFormation() {
			expectedAttackers[hero.GetGuid()] = hero
		}
		seen := map[string]bool{}
		poses := map[int32]bool{}
		var total, maximum float64
		var hps []*pb.BattlePosHp
		for _, actor := range terminal.FinalActors {
			if actor.Camp != 0 {
				continue
			}
			hero := expectedAttackers[actor.GUID]
			if hero == nil || seen[actor.GUID] || poses[hero.GetPose()] || !combatFinite(actor.CurrentHP) || actor.MaxHP <= 0 {
				return errors.New("野外首领己方终局无效")
			}
			seen[actor.GUID], poses[hero.GetPose()] = true, true
			total += math.Max(0, actor.CurrentHP)
			maximum += actor.MaxHP
			hps = append(hps, &pb.BattlePosHp{BattlePos: hero.Pose, HpPercent: proto.Float64(math.Max(0, actor.CurrentHP) / actor.MaxHP)})
		}
		if len(seen) != len(expectedAttackers) || maximum <= 0 {
			return errors.New("野外首领己方终局成员不完整")
		}
		anger := input.Attacker.Inst.GetAnger()
		if terminal.Outcome.Attacker != nil && terminal.Outcome.Attacker.Inst != nil {
			anger = terminal.Outcome.Attacker.Inst.GetAnger()
		}
		side.BattlePosHps, side.Anger, side.CurHp, side.State = hps, proto.Float64(anger), proto.Float64(total/maximum), proto.Int32(0)
		if total == 0 {
			side.State = proto.Int32(5)
		}

		if f.BossHP == nil {
			f.BossHP = map[string]map[string]float64{}
		}
		f.BossHP[boss.GetUuid()] = map[string]float64{}
		allDead := true
		leaderGUID := fight.BossLeaderGUID
		if leaderGUID == "" {
			for _, row := range s.fieldBossRowsInScene(boss.GetBossId(), f.Scene) {
				if row["TeamLeader"] == "1" {
					leaderGUID = "field-boss-" + boss.GetUuid() + "-" + row["Id"]
				}
			}
		}
		leaderRatio := float64(0)
		bossActors := 0
		for _, actor := range terminal.FinalActors {
			if actor.Camp != 1 {
				continue
			}
			bossActors++
			hp := math.Max(0, actor.CurrentHP)
			f.BossHP[boss.GetUuid()][actor.GUID] = hp
			if hp > 0 {
				allDead = false
			}
			if actor.GUID == leaderGUID && actor.MaxHP > 0 {
				leaderRatio = hp / actor.MaxHP
			}
		}
		if bossActors != 4 {
			return errors.New("野外首领敌方终局成员不完整")
		}
		boss.CurHp = proto.Float64(leaderRatio)
		killReward := allDead
		sharedApplied, sharedKilled, err := s.applySharedFieldBoss55(tx, id, f, boss, input, f.BossHP[boss.GetUuid()], fight.BossCooldownMS)
		if err != nil {
			return err
		}
		if sharedApplied {
			killReward = sharedKilled
			allDead = boss.GetState() == 3
		}
		now := time.Now().UnixMilli()
		if allDead {
			boss.State = proto.Int32(3)
			if fight.BossCooldownMS <= 0 {
				return errors.New("缺少原野外首领重生配置")
			}
			if !sharedApplied {
				f.RespawnAt[boss.GetUuid()] = now + fight.BossCooldownMS
			}
			if sharedKilled || !s.SharedFieldBoss55Ready {
				name := "野外首领"
				if s.AdminCatalogueData != nil {
					hero := s.table("Hero", "id", boss.GetHeroId())
					if localized := s.AdminCatalogueData.DisplayNames[hero["name"]]; localized != "" {
						name = localized
					}
				}
				notice = &pb.SCPublicMessage{Pubmsg: &pb.PublicMessage{
					Type: proto.Int32(constUtil.P12WorldBossKillNoticeType), ScrollType: proto.Int32(constUtil.P12WorldBossKillNoticeScrollType),
					Msg: proto.String(fmt.Sprintf(constUtil.FieldBossKillNoticeFormat, p12NoticeName(player.GetName()), p12NoticeName(name))),
				}}
			}
		} else {
			boss.State = proto.Int32(1)
		}

		gain := &pb.ResultObject{}
		content := fight.BossReward
		deathMode := fight.BossDeathRules73 != nil && fight.BossDeathRules73.DeathSettlement
		if deathMode {
			content = ""
		}
		if !deathMode && killReward && fight.BossKill != "" {
			if content != "" {
				content += ";"
			}
			content += fight.BossKill
		}
		content, err = fieldBossResourceContent(ctx, f, fight, content, tx)
		if err != nil {
			return err
		}
		if content != "" {
			if gain, err = s.openServerGain(player, g, content); err != nil {
				return err
			}
		}
		result = &pb.SCSidePveBossFightEnd{Get: gain, FightResult: proto.Int32(b.GetResult()), Code: proto.Int32(1), Anger: proto.Float64(anger), State: side.State, Monster: proto.Clone(boss).(*pb.SideBoss)}
		for _, hp := range hps {
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
		fight.Settled, fight.BossResult = true, proto.Clone(result).(*pb.SCSidePveBossFightEnd)
		if s.P8LifeReady {
			if _, err := s.RefreshP8Survival(tx, player, g, time.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	pendingGain, collectErr := s.CollectFieldBossPendingRewards(ctx, id)
	if collectErr != nil {
		return nil, collectErr
	}
	if pendingGain != nil {
		if result.Get == nil {
			result.Get = &pb.ResultObject{}
		}
		p1AppendGain(result.Get, pendingGain)
	}
	// Only a newly committed kill publishes. Repeated/cold receipts return before
	// constructing this message and preserve the single reward and deadline.
	s.publishP12WorldBossKill(ctx, notice)
	return append([]proto.Message{result, &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{side}}}, countdowns...), nil
}

// fieldBossFrozenInput expands only the one battle being settled and checks the
// canonical bytes. Historical uncompressed saves retain the same validation.
func fieldBossFrozenInput(fight *model.FieldFight) (*model.FrozenBattleInput, error) {
	if fight == nil {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if fight.BossInput != nil {
		if len(fight.BossInputStored) != 0 {
			return nil, errors.New(constUtil.P6ReplayConflict)
		}
		return fight.BossInput, nil
	}
	if len(fight.BossInputStored) == 0 || fight.BossInputHash == "" {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	raw, err := decodeP6StoredInput(fight.BossInputStored)
	if err != nil || p6Digest(raw) != fight.BossInputHash {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	var input model.FrozenBattleInput
	if json.Unmarshal(raw, &input) != nil {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return &input, nil
}

// compactFieldBossState keeps only the input needed for pending settlement.
// Completed replies retain their original idempotent result and saved Boss HP.
func compactFieldBossState(game *model.GameState) error {
	if game.Field == nil {
		return nil
	}
	compact := func(fight *model.FieldFight) error {
		if fight == nil {
			return nil
		}
		if fight.Settled && fight.BossResult != nil {
			fight.BossInput = nil
			fight.BossInputStored = nil
			fight.BossInputHash = ""
			return nil
		}
		if fight.BossInput == nil {
			return nil
		}
		if len(fight.BossInputStored) != 0 {
			return errors.New(constUtil.P6ReplayConflict)
		}
		raw, err := json.Marshal(fight.BossInput)
		if err != nil {
			return err
		}
		stored, err := encodeP6StoredInput(raw)
		if err != nil {
			return err
		}
		fight.BossInputStored = stored
		fight.BossInputHash = p6Digest(raw)
		fight.BossInput = nil
		return nil
	}
	if err := compact(game.Field.Fight); err != nil {
		return err
	}
	for _, area := range game.Field.Areas {
		if area != nil {
			if err := compact(area.Fight); err != nil {
				return err
			}
		}
	}
	return nil
}

// releaseFieldBossFight 在断线/超时丢弃未提交战斗时恢复首领可见状态，不结算伤害或奖励。
func releaseFieldBossFight(g *model.GameState) {
	if g == nil || g.Field == nil || g.Field.Fight == nil || g.Field.Fight.Settled || g.Field.Fight.BossStart == nil {
		return
	}
	if boss := fieldBossFind(g.Field, g.Field.Fight.BossUUID); boss != nil && boss.GetState() == 2 {
		boss.State = proto.Int32(1)
	}
}
