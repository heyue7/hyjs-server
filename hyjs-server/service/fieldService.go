package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/elog"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// fieldConfigInt 使用 APK 的数值配置；缺配置时中止业务，避免产生虚构默认值。
func (s *GameService) fieldConfigInt(key string) (int64, error) {
	v, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", key)["NameValue"], 10, 32)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("Missing field configuration: %s", key)
	}
	return v, nil
}

func fieldToken() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// fieldWeighted 按表中的正整数权重抽一个档位，零数量档位仍参与抽取。
func fieldWeighted(text string) (string, error) {
	var (
		values  []string
		weights []int64
		total   int64
	)
	for _, part := range strings.Split(text, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 || pair[0] == "" {
			return "", errors.New("Invalid field weight table")
		}
		weight, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || weight <= 0 || total > math.MaxInt32-weight {
			return "", errors.New("Invalid field weight")
		}
		values = append(values, pair[0])
		weights = append(weights, weight)
		total += weight
	}
	draw, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return "", err
	}
	value := draw.Int64()
	for i, weight := range weights {
		if value < weight {
			return values[i], nil
		}
		value -= weight
	}
	return "", errors.New("Empty field weight table")
}

func fieldPoint(text string) (*pb.PointNew, error) {
	parts := strings.Split(text, ",")
	if len(parts) != 3 {
		return nil, errors.New("Invalid field position")
	}
	values := make([]float32, 3)
	for i, part := range parts {
		v, err := strconv.ParseFloat(part, 32)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("Invalid field position")
		}
		values[i] = float32(v)
	}
	return &pb.PointNew{X: proto.Float32(values[0]), Y: proto.Float32(values[1]), Z: proto.Float32(values[2])}, nil
}

func (s *GameService) fieldLeader(monID string) map[string]string {
	for _, row := range s.Tables["FieldMonster"] {
		if row["MonId"] == monID && row["TeamLeader"] == "1" {
			return row
		}
	}
	return nil
}

func (s *GameService) fieldSpawn(posID string) (*pb.SideMonster, error) {
	row := s.table("FieldMonsterPos", "monsterPosId", posID)
	if row == nil {
		return nil, errors.New("Missing field monster position")
	}
	monID, err := fieldWeighted(row["monsterIdProbs"])
	if err != nil {
		return nil, err
	}
	leader := s.fieldLeader(monID)
	if leader == nil {
		return nil, errors.New("Missing field monster leader")
	}
	point, err := fieldPoint(row["BornPos"])
	if err != nil {
		return nil, err
	}
	uuid, err := fieldToken()
	if err != nil {
		return nil, err
	}
	return &pb.SideMonster{Uuid: proto.String(uuid), MonsterPosId: proto.String(posID), MonId: proto.String(monID), HeroId: proto.String(leader["HeroId"]), CurrentPoint: point, State: proto.Int32(1), CurHp: proto.Float64(1)}, nil
}

func (s *GameService) fieldSceneRow(scene string) map[string]string {
	if !constUtil.P8OriginalFieldScene(scene) {
		return nil
	}
	row := s.table("Field", "FieldId", scene)
	if row == nil || row["FieldMap"] == "" || row["BattleMap"] == "" || tableNumber(row, "levelLimit") < 1 {
		return nil
	}
	return row
}

func (s *GameService) P8FieldSceneAvailable(scene string) bool {
	row := s.fieldSceneRow(scene)
	return constUtil.P8PackagedFieldScene(scene) && row != nil && row["adminEnabled"] != "false"
}

// fieldSelectScene archives only area-local objects. Changing maps preserves
// the player's HP, fatigue and shared daily reward counter.
func (s *GameService) fieldSelectScene(g *model.GameState, scene string) error {
	if s.fieldSceneRow(scene) == nil || !s.P8FieldSceneAvailable(scene) {
		return errors.New(constUtil.P8FieldSceneUnavailable)
	}
	if g.Field == nil {
		g.Field = &model.FieldState{Scene: scene, RespawnAt: map[string]int64{}}
		return nil
	}
	f := g.Field
	if f.Scene == scene {
		return nil
	}
	if f.Fight != nil && !f.Fight.Settled {
		return errors.New(constUtil.P8FieldSceneStateConflict)
	}
	if f.Areas == nil {
		f.Areas = map[string]*model.FieldAreaState{}
	}
	if s.fieldSceneRow(f.Scene) != nil {
		f.Areas[f.Scene] = &model.FieldAreaState{Monsters: f.Monsters, Bosses: f.Bosses, BossHP: f.BossHP, RespawnAt: f.RespawnAt, Fight: f.Fight}
	}
	area := f.Areas[scene]
	if area == nil {
		area = &model.FieldAreaState{RespawnAt: map[string]int64{}}
	}
	f.Scene, f.Monsters, f.Bosses, f.BossHP, f.RespawnAt, f.Fight = scene, area.Monsters, area.Bosses, area.BossHP, area.RespawnAt, area.Fight
	delete(f.Areas, scene)
	return nil
}

// refreshField 保存单人本地六张原野外实例。多人共享怪物和 PK 尚未接入。
func (s *GameService) refreshField(id int64, g *model.GameState) error {
	if g.Field == nil {
		scene := constUtil.P8FirstFieldScene
		if s.fieldSceneRow(g.Position.GetScene()) != nil {
			scene = g.Position.GetScene()
		}
		if err := s.fieldSelectScene(g, scene); err != nil {
			return err
		}
	}
	f := g.Field
	if s.fieldSceneRow(f.Scene) == nil {
		return errors.New(constUtil.P8FieldSceneUnavailable)
	}
	if scene := g.Position.GetScene(); s.fieldSceneRow(scene) != nil && scene != f.Scene {
		if err := s.fieldSelectScene(g, scene); err != nil {
			return err
		}
	}
	if f.RespawnAt == nil {
		f.RespawnAt = map[string]int64{}
	}
	// 日期按本地服北京时间计算，日切仅重置奖励计数和生活技能疲劳记录。
	day := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02")
	if f.Side == nil {
		f.Side = &pb.PlayerSide{PlayerId: proto.Int64(id), State: proto.Int32(0), Anger: proto.Float64(0), MaxHp: proto.Float64(1), CurHp: proto.Float64(1)}
	}
	if f.RewardDay != day {
		reset := strings.Split(s.table("GameConfigure", "StaticName", "LifeSkillTirednessReset")["NameValue"], ",")
		if len(reset) != 2 || reset[0] != "tiredness" {
			return errors.New("Missing field tiredness reset configuration")
		}
		tiredness, err := strconv.ParseInt(reset[1], 10, 32)
		if err != nil || tiredness < 0 {
			return errors.New("Invalid field tiredness reset configuration")
		}
		f.RewardDay, f.RewardKills = day, 0
		f.Side.Tiredness = proto.Int32(int32(tiredness))
		f.Side.MaxTiredness = proto.Int32(int32(tiredness))
		f.Side.TiredNessLastRefreshDate = proto.String(day)
	}
	row := s.table("Field", "FieldId", f.Scene)
	if row == nil {
		return errors.New("Missing field map configuration")
	}
	if revision := row["adminRevision"]; revision != "" && (f.Fight == nil || f.Fight.Settled) {
		version, _ := strconv.ParseInt(revision[:15], 16, 64)
		if f.RespawnAt["__admin_revision"] != version {
			f.Monsters = nil
			// Monster configuration revisions must preserve an independently
			// defeated boss's original respawn deadline.
			deadlines := map[string]int64{"__admin_revision": version}
			for _, boss := range f.Bosses {
				if due := f.RespawnAt[boss.GetUuid()]; due > 0 {
					deadlines[boss.GetUuid()] = due
				}
			}
			f.RespawnAt = deadlines
		}
	}
	if len(f.Monsters) == 0 {
		for _, pos := range strings.Split(row["MonsterPoints"], ";") {
			if pos == "" {
				continue
			}
			monster, err := s.fieldSpawn(pos)
			if err != nil {
				return err
			}
			f.Monsters = append(f.Monsters, monster)
		}
	}
	now := time.Now().UnixMilli()
	if f.Fight != nil && !f.Fight.Settled {
		minutes, err := s.fieldConfigInt("SideMonsterFightKeepMaxTime")
		if err != nil {
			return err
		}
		if now-f.Fight.StartedAt >= minutes*60*1000 {
			// 超时仅释放战斗占用，旧 fightId 失效且不会产生战斗奖励。
			releaseFieldBossFight(g)
			if monster := fieldFind(g, f.Fight.MonsterUUID); monster != nil {
				monster.State = proto.Int32(1)
			}
			f.Fight = nil
			if f.Side.GetCurHp() > 0 {
				f.Side.State = proto.Int32(0)
			}
		}
	}
	for i, monster := range f.Monsters {
		if due := f.RespawnAt[monster.GetUuid()]; due > 0 && due <= now {
			replacement, err := s.fieldSpawn(monster.GetMonsterPosId())
			if err != nil {
				return err
			}
			delete(f.RespawnAt, monster.GetUuid())
			f.Monsters[i] = replacement
		}
	}
	return s.refreshFieldBosses(f, row)
}

func (s *GameService) fieldTransfer(ctx context.Context, id int64, b *pb.CSTransferScene) ([]proto.Message, error) {
	var resolved *pb.PointNew
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		if s.P12FieldPVPReady {
			allowed, err := ShouldAllowP12PrisonTransfer(tx, id, b.GetTarget(), b.GetPoint(), time.Now())
			if err != nil {
				return err
			}
			if !allowed {
				if err = CheckP12FieldPVPAction(tx, id, time.Now()); err != nil {
					return err
				}
			}
			if !allowed && s.fieldSceneRow(g.Position.GetScene()) != nil && g.Field != nil && g.Field.Side != nil && (g.Field.Side.GetState() == constUtil.P8FieldDead || g.Field.Side.GetCurHp() <= 0) {
				return errors.New("死亡状态不能转场，请先完成原复活")
			}
		}
		if g.Field != nil {
			if err := s.refreshField(id, g); err != nil {
				return err
			}
		}
		if g.Run != nil && !g.Run.Complete {
			return errors.New("Leave the active copy before changing scene")
		}
		if g.Field != nil && g.Field.Fight != nil && !g.Field.Fight.Settled {
			return errors.New("Finish the active field battle before changing scene")
		}
		if s.P8GatherReady {
			if err := s.P8CancelGather(tx, id, g); err != nil {
				return err
			}
		}
		var point *pb.PointNew
		switch b.GetTarget() {
		case "field_001", "field_002", "field_003", "field_004", "field_005", "field_006":
			row := s.fieldSceneRow(b.GetTarget())
			if row == nil || !s.P8FieldSceneAvailable(b.GetTarget()) {
				return errors.New(constUtil.P8FieldSceneUnavailable)
			}
			if player.GetLevel() < int32(tableNumber(row, "levelLimit")) {
				return errors.New(constUtil.P8FieldSceneLevel)
			}
			if err := s.fieldSelectScene(g, b.GetTarget()); err != nil {
				return err
			}
			var err error
			point, err = fieldPoint(row["Resurrection"])
			if err != nil {
				return err
			}
			g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(b.GetTarget()), Current: point}
			if err := s.refreshField(id, g); err != nil {
				return err
			}
		case constUtil.P13GuildRoomTarget:
			if err := s.P13GuildRoomEnter(tx, id, player); err != nil {
				return err
			}
			var err error
			point, err = s.P13GuildRoomEntryPoint(b.GetPoint())
			if err != nil {
				return err
			}
		case "town", "town_main":
			point = b.GetPoint()
			if point == nil {
				point = &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}
			}
		default:
			return errors.New("This field scene is not available yet")
		}
		g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(b.GetTarget()), Current: point}
		g.PositionLoading = true
		resolved = proto.Clone(point).(*pb.PointNew)
		if s.P8LifeReady {
			if _, err := s.RefreshP8Survival(tx, player, g, time.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		// 仅记录成功转场的非敏感空间坐标，追踪原请求到最终入口的映射。
		elog.Info("scene transfer committed", zap.Int64("playerId", id), zap.String("scene", b.GetTarget()),
			zap.Float32s("requestedPoint", []float32{b.GetPoint().GetX(), b.GetPoint().GetY(), b.GetPoint().GetZ()}),
			zap.Float32s("resolvedPoint", []float32{resolved.GetX(), resolved.GetY(), resolved.GetZ()}))
	}
	return []proto.Message{&pb.SCTransferScene{Result: proto.Int32(1)}}, err
}

// fieldScene 把怪物和持久化血量附在原场景回复之后，客户端据此创建野外模型。
func (s *GameService) fieldScene(ctx context.Context, id int64, login bool) ([]proto.Message, error) {
	var messages []proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		// Login is a recovery query. CurrentPosition completes a control handshake,
		// and only the original fixed prison destination can bypass an active jail.
		if s.P12FieldPVPReady && !login {
			allowed, err := ShouldAllowP12PrisonPosition(tx, id, g.Position, time.Now())
			if err != nil {
				return err
			}
			if !allowed {
				if err = CheckP12FieldPVPAction(tx, id, time.Now()); err != nil {
					return err
				}
			}
		}
		forced, err := s.P13GuildRoomNormalize(tx, id, player, g)
		if err != nil {
			return err
		}
		isUnion := g.Position.GetScene() == constUtil.P13GuildRoomTarget
		limit := int32(100)
		if isUnion {
			if err = s.P13GuildRoomEnter(tx, id, player); err != nil {
				return err
			}
			_, guild, err := p13GuildNeedMember(tx, id)
			if err != nil {
				return err
			}
			limit, err = s.p13GuildCapacity(guild.Level)
			if err != nil {
				return err
			}
		}
		var side = &pb.PlayerSide{PlayerId: proto.Int64(id), Kills: proto.Int32(0)}
		isField := s.fieldSceneRow(g.Position.GetScene()) != nil
		if isField {
			base, configErr := p10SceneConfig(tx)
			if configErr != nil {
				return configErr
			}
			lineConfig, configErr := s.adminFieldSceneConfig(tx, g.Position.GetScene(), base)
			if configErr != nil {
				return configErr
			}
			if lineConfig != nil {
				limit = lineConfig.MaxPlayers
			}
			if err := s.refreshField(id, g); err != nil {
				return err
			}
			// 原客户端重连会重建野外房间，不会重放正在加载的小怪战斗。
			// 释放未结算占用，保留战前HP/怒气/疲劳与奖励计数，旧fightId随之失效。
			if login && g.Field.Fight != nil && !g.Field.Fight.Settled {
				releaseFieldBossFight(g)
				if monster := fieldFind(g, g.Field.Fight.MonsterUUID); monster != nil {
					monster.State = proto.Int32(1)
				}
				g.Field.Fight = nil
				if g.Field.Side.GetCurHp() > 0 {
					g.Field.Side.State = proto.Int32(0)
				}
			}
			side = g.Field.Side
		}
		if s.P12FieldPVPReady {
			if prison, ok := P12FieldPVPPrisonSide(player, g, time.Now()); ok {
				side = prison
			}
		}
		heroID, err := P5WorldHeroID(player, g)
		if err != nil {
			return err
		}
		self := &pb.MovePlayer{Position: g.Position, Info: &pb.MovePlayerInfo{ServerId: proto.Int32(int32(s.OriginalServerID())), PlayerId: proto.Int64(id), PlayerName: player.Name, Level: player.Level, WarId: proto.String(heroID), ServerName: proto.String("本地测试"), PlayerSide: side}}
		if s.P9PetsReady {
			pet, err := s.P9FollowPetInfo(g)
			if err != nil {
				return err
			}
			self.Info.PetId = proto.String(pet.GetPetId())
		}
		if login {
			messages = append(messages, &pb.SCLoginScene{Self: self, Limit: proto.Int32(limit)})
		} else {
			messages = append(messages, &pb.SCCurrentPosition{Self: self, Limit: proto.Int32(limit)})
			// 原客户端在新房间加载完成后通过TCP请求此帧。旧房间移动
			// 排在该请求前，先保留转场入口，握手后再接受新角色移动。
			g.PositionLoading = false
		}
		if isUnion {
			// 用公会持久建设事实恢复场景，避免重登退回未创建投影。
			building, err := s.P13GuildBuildingState(tx, id)
			if err != nil {
				return err
			}
			messages = append(messages, building, &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{side}})
		}
		if forced && !login {
			messages = append(messages, &pb.SCRefreshScene{})
		}
		if isField {
			messages = append(messages, &pb.SCSideMonster{Monsters: g.Field.Monsters}, &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{side}})
			messages = append(messages, s.fieldBossProjection(g)...)
			boss, err := s.p12WorldBossFieldProjection(tx, g.Position.GetScene(), time.Now())
			if err != nil {
				return err
			}
			messages = append(messages, boss...)
			if s.P8GatherReady {
				resources, err := s.RefreshP8Resources(tx, player, g, time.Now(), true)
				if err != nil {
					return err
				}
				messages = append(messages, resources...)
			}
			if s.P8LifeReady {
				recovery, err := s.RefreshP8Survival(tx, player, g, time.Now())
				if err != nil {
					return err
				}
				messages = append(messages, recovery...)
			}
		}
		return nil
	})
	return messages, err
}

func fieldFind(g *model.GameState, uuid string) *pb.SideMonster {
	if g.Field != nil {
		for _, monster := range g.Field.Monsters {
			if monster.GetUuid() == uuid {
				return monster
			}
		}
	}
	return nil
}

func (s *GameService) fieldMonsterCheck(ctx context.Context, id int64, b *pb.CSSideMonsterCheck) ([]proto.Message, error) {
	var result *pb.SCSideMonsterCheck
	err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		if s.fieldSceneRow(g.Position.GetScene()) == nil || !s.P8FieldSceneAvailable(g.Position.GetScene()) {
			return errors.New(constUtil.P8FieldSceneUnavailable)
		}
		if err := s.refreshField(id, g); err != nil {
			return err
		}
		monster := fieldFind(g, b.GetUuid())
		result = &pb.SCSideMonsterCheck{Monster: monster, Code: proto.Int32(1)}
		if monster == nil || monster.GetState() == 3 {
			result.Code = proto.Int32(2003)
			if monster == nil {
				// 客户端在 2003 分支仍读取 uuid 来移除旧模型。
				result.Monster = &pb.SideMonster{Uuid: proto.String(b.GetUuid()), State: proto.Int32(3), CurHp: proto.Float64(0)}
			}
		}
		return nil
	})
	return []proto.Message{result}, err
}

// fieldHeartbeat 借用原 TCP 心跳推送重生，玩家停留原地也会看到新怪物。
func (s *GameService) fieldHeartbeat(ctx context.Context, id int64) ([]proto.Message, error) {
	var messages []proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		if s.fieldSceneRow(g.Position.GetScene()) == nil || g.Field == nil {
			return nil
		}
		var before strings.Builder
		for _, monster := range g.Field.Monsters {
			fmt.Fprintf(&before, "%s:%d;", monster.GetUuid(), monster.GetState())
		}
		if err := s.refreshField(id, g); err != nil {
			return err
		}
		if s.P8GatherReady {
			resources, err := s.RefreshP8Resources(tx, player, g, time.Now(), false)
			if err != nil {
				return err
			}
			messages = append(messages, resources...)
		}
		if s.P8LifeReady {
			recovery, err := s.RefreshP8Survival(tx, player, g, time.Now())
			if err != nil {
				return err
			}
			messages = append(messages, recovery...)
		}
		var after strings.Builder
		for _, monster := range g.Field.Monsters {
			fmt.Fprintf(&after, "%s:%d;", monster.GetUuid(), monster.GetState())
		}
		if before.String() != after.String() {
			messages = append(messages, &pb.SCSideMonster{Monsters: g.Field.Monsters}, &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{g.Field.Side}})
		}
		messages = append(messages, s.fieldBossProjection(g)...)
		boss, err := s.p12WorldBossFieldProjection(tx, g.Position.GetScene(), time.Now())
		if err != nil {
			return err
		}
		messages = append(messages, boss...)
		return nil
	})
	return messages, err
}

func (s *GameService) fieldFightStart(ctx context.Context, id int64, b *pb.CSSidePveFightStart) ([]proto.Message, error) {
	var result *pb.SCSidePveFightStart
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		if !s.P8FieldSceneAvailable(g.Position.GetScene()) || g.Run != nil && !g.Run.Complete {
			return errors.New("Enter the field before starting a battle")
		}
		if s.fieldSceneRow(g.Position.GetScene())["BattleMap"] != "map_forest_003" {
			return errors.New(constUtil.P8FieldSceneUnavailable)
		}
		if err := s.refreshField(id, g); err != nil {
			return err
		}
		f := g.Field
		if f.Fight != nil && !f.Fight.Settled {
			if f.Fight.BossStart != nil {
				return errors.New("Another field boss battle is active")
			}
			if f.Fight.MonsterUUID != b.GetMonsterId() {
				return errors.New("Another field battle is active")
			}
			result = f.Fight.Start
			return nil
		}
		if err := s.ensureNoActiveDailyCopy(tx, id, time.Now()); err != nil {
			return err
		}
		if s.P8GatherReady {
			if err := s.P8CancelGather(tx, id, g); err != nil {
				return err
			}
		}
		monster := fieldFind(g, b.GetMonsterId())
		if monster == nil || monster.GetState() != 1 {
			return errors.New("Field monster is unavailable")
		}
		if f.Side.GetCurHp() <= 0 {
			return errors.New("野外角色已倒下，复活功能尚未恢复")
		}
		party, err := s.battlePlayer(player, g)
		if err != nil || len(party.GetFormation()) == 0 {
			return errors.New("Field battle requires an available formation")
		}
		for _, hero := range party.Formation {
			for _, hp := range f.Side.GetBattlePosHps() {
				if hero.GetPose() == hp.GetBattlePos() {
					hero.Inst.CurHp = proto.Float64(hero.GetProp().GetHp() * hp.GetHpPercent())
				}
			}
		}
		party.Inst.Anger = proto.Float64(f.Side.GetAnger())
		groups := map[int32][]*pb.BattleHero{}
		for _, row := range s.Tables["FieldMonster"] {
			if row["MonId"] == monster.GetMonId() {
				hero, err := s.actor(row, "field-"+monster.GetUuid()+"-"+row["MonsterId"], int32(tableNumber(row, "Location")), true)
				if err != nil {
					return err
				}
				group := int32(tableNumber(row, "GroupIndex"))
				groups[group] = append(groups[group], hero)
			}
		}
		if len(groups) == 0 {
			return errors.New("Missing field battle group")
		}
		var indexes []int
		for group := range groups {
			indexes = append(indexes, int(group))
		}
		sort.Ints(indexes)
		var monsters []*pb.BattleMonster
		for _, group := range indexes {
			monsters = append(monsters, &pb.BattleMonster{MonsterId: proto.Int64(int64(group)), ServerId: proto.Int32(1), TaskId: monster.MonId, GroupIndex: proto.Int32(int32(group)), Formation: groups[int32(group)], Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}})
		}
		token, err := fieldToken()
		if err != nil {
			return err
		}
		seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
		if err != nil {
			return err
		}
		rounds, err := s.fieldConfigInt("FightMaxRoundNum")
		if err != nil {
			return err
		}
		// 原 BattleData.SetData(SCSidePveFightStart) 在 fightScene 为空时使用 Field.BattleMap。
		// 非空值会进入写死的 map_arena_001 分支，因此保留原野外地图选择路径。
		if s.table("Field", "FieldId", f.Scene)["BattleMap"] == "" {
			return errors.New("Missing field battle map")
		}
		result = &pb.SCSidePveFightStart{FightId: proto.String(token), RandSeed: proto.Int64(seed.Int64()), BattlePlayer: party, BattleMonsterList: monsters, FightScene: proto.String(""), Code: proto.Int32(1), Maxround: proto.Int32(int32(rounds))}
		if err := s.adminFieldFreezeBattle(tx, id, f.Scene, token, monster.GetMonId()); err != nil {
			return err
		}
		f.Fight = &model.FieldFight{MonsterUUID: monster.GetUuid(), Start: result, StartedAt: time.Now().UnixMilli()}
		monster.State = proto.Int32(2)
		f.Side.State = proto.Int32(6)
		return nil
	})
	return []proto.Message{result}, err
}

// fieldReward 每次整群胜利抽取队长的奖励一次，这是本地服结算规则。
func (s *GameService) fieldReward(id int64, player *pb.Player, g *model.GameState, monster *pb.SideMonster) (*pb.ResultObject, error) {
	gain := &pb.ResultObject{}
	limit, err := s.fieldConfigInt("MaxSideMonsterKillTimesReward")
	if err != nil {
		return nil, err
	}
	if int64(g.Field.RewardKills) >= limit {
		return gain, nil
	}
	leader := s.fieldLeader(monster.GetMonId())
	for _, part := range strings.Split(leader["rewardGroupId"], "|") {
		pair := strings.Split(part, ":")
		if len(pair) != 2 {
			return nil, errors.New("Invalid field reward configuration")
		}
		quantity, err := fieldWeighted(pair[1])
		if err != nil {
			return nil, err
		}
		num, err := strconv.ParseInt(quantity, 10, 32)
		if err != nil || num < 0 {
			return nil, errors.New("Invalid field reward quantity")
		}
		if num == 0 {
			continue
		}
		key := pair[0]
		switch {
		case key == "coin":
			if player.GetCoin() > math.MaxInt64-num {
				return nil, errors.New("Field coin balance overflow")
			}
			player.Coin = proto.Int64(player.GetCoin() + num)
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(num))})
		case strings.HasPrefix(key, "hero_exp_"):
			if s.table("Hero", "id", key)["heroClass"] != "99" {
				return nil, errors.New("Unknown field material")
			}
			if int64(len(g.Warriors))+num > int64(player.GetWarriorBagSize()) {
				return nil, errors.New("伙伴背包已满")
			}
			for i := int64(0); i < num; i++ {
				warrior := &pb.Warrior{Id: proto.String(key), Guid: proto.String(fmt.Sprintf("%d-field-%s-%s-%d", id, g.Field.Fight.Start.GetFightId(), key, i)), Level: proto.Int32(1), Exp: proto.Int64(0), Talent: proto.Int32(0), Breached: &pb.Breached{Level: proto.Int32(1)}, Training: &pb.Training{}, TempTraining: &pb.Training{}, Awake: &pb.Awake{}}
				g.Warriors = append(g.Warriors, warrior)
				gain.Wars = append(gain.Wars, warrior)
			}
		default:
			var inventory *[]*pb.SimpleItem
			switch {
			case s.table("Item", "itemId", key) != nil:
				inventory = &g.Items
			case s.table("MineSkill", "MineItemID", key) != nil:
				inventory = &g.Gems
			case s.table("FishSkill", "FishItemID", key) != nil:
				inventory = &g.Cooking
			}
			if inventory == nil {
				return nil, errors.New("Unsupported field drop: " + key)
			}
			var owned *pb.SimpleItem
			for _, item := range *inventory {
				if item.GetId() == key {
					owned = item
					break
				}
			}
			if owned == nil {
				*inventory = append(*inventory, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(num))})
			} else {
				if int64(owned.GetNum()) > math.MaxInt32-num {
					return nil, errors.New("Field item quantity overflow")
				}
				owned.Num = proto.Int32(owned.GetNum() + int32(num))
			}
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(num))})
		}
	}
	g.Field.RewardKills++
	return gain, nil
}

// fieldFightEnd 校验客户端真实战斗快照；客户端技能引擎的权威重放仍待恢复。
func (s *GameService) fieldFightEnd(ctx context.Context, id int64, b *pb.CSSidePveFightEnd) ([]proto.Message, error) {
	var (
		result *pb.SCSidePveFightEnd
		side   *pb.PlayerSide
	)
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		if g.Field == nil {
			return errors.New("Unknown field battle")
		}
		if err := s.refreshField(id, g); err != nil {
			return err
		}
		if g.Field.Fight == nil || g.Field.Fight.Start.GetFightId() != b.GetFightId() {
			return errors.New("Unknown field battle")
		}
		if g.Field.Fight.BossStart != nil {
			return errors.New("Active field boss must be settled through boss endpoint")
		}
		f := g.Field
		fight := f.Fight
		side = f.Side
		if fight.Settled {
			if fight.Result == nil {
				return errors.New("Missing saved field result")
			}
			if err := s.validateP5GrowthReceipt(g, fight.Result); err != nil {
				return err
			}
			result = fight.Result
			return nil
		}
		if g.Position.GetScene() != f.Scene {
			return errors.New("Field battle scene mismatch")
		}
		if err := s.refreshField(id, g); err != nil {
			return err
		}
		monster := fieldFind(g, fight.MonsterUUID)
		if monster == nil || monster.GetState() != 2 {
			return errors.New("Missing active field monster")
		}
		if b.GetBattlePlayer().GetPlayerId() != id || b.GetResult() != 0 && b.GetResult() != 1 {
			return errors.New("Invalid field battle owner or result")
		}
		expected := map[string]*pb.BattleHero{}
		for _, hero := range fight.Start.GetBattlePlayer().GetFormation() {
			expected[hero.GetGuid()] = hero
		}
		seen := map[string]bool{}
		poses := map[int32]bool{}
		var total, maximum float64
		var hps []*pb.BattlePosHp
		for _, hero := range b.GetBattlePlayer().GetFormation() {
			e := expected[hero.GetGuid()]
			hp := hero.GetInst().GetCurHp()
			if e == nil || seen[hero.GetGuid()] || poses[hero.GetPose()] || hero.GetPose() < 1 || hero.GetPose() > 15 || hero.GetId() != e.GetId() || math.IsNaN(hp) || math.IsInf(hp, 0) || hp < 0 || hp > e.GetProp().GetHp() {
				return errors.New("Invalid field formation or remaining HP")
			}
			seen[hero.GetGuid()], poses[hero.GetPose()] = true, true
			total += hp
			maximum += e.GetProp().GetHp()
			hps = append(hps, &pb.BattlePosHp{BattlePos: hero.Pose, HpPercent: proto.Float64(hp / e.GetProp().GetHp())})
		}
		if len(seen) != len(expected) || maximum <= 0 || b.GetResult() == 1 && total <= 0 {
			return errors.New("Invalid field survivors")
		}
		record := b.GetOptRecord()
		if record == nil || record.GetFinalRoundNum() < 0 || record.GetFinalRoundNum() > fight.Start.GetMaxround() || math.IsNaN(record.GetAttackerTotalLeftHp()) || math.IsInf(record.GetAttackerTotalLeftHp(), 0) || math.Abs(record.GetAttackerTotalLeftHp()-total) > math.Max(0.01, total*0.00001) || math.IsNaN(record.GetDefenderTotalLeftHp()) || math.IsInf(record.GetDefenderTotalLeftHp(), 0) || record.GetDefenderTotalLeftHp() < 0 || b.GetResult() == 1 && record.GetDefenderTotalLeftHp() != 0 {
			return errors.New("Invalid field operation record")
		}
		anger := b.GetBattlePlayer().GetInst().GetAnger()
		if math.IsNaN(anger) || math.IsInf(anger, 0) || anger < 0 {
			return errors.New("Invalid field anger")
		}
		result = &pb.SCSidePveFightEnd{Getn: &pb.ResultObject{}, FightResult: proto.Int32(b.GetResult()), Code: proto.Int32(1), Anger: proto.Float64(anger), State: proto.Int32(0), Monster: monster}
		if b.GetResult() == 1 {
			reward, cooldown, err := s.adminFieldBattleReward(tx, id, f.Scene, fight.Start.GetFightId(), monster.GetMonId())
			if err != nil {
				return err
			}
			frozenTables := make(map[string][]map[string]string, len(s.Tables))
			for k, v := range s.Tables {
				frozenTables[k] = v
			}
			frozenRows := []map[string]string{}
			for _, r := range s.Tables["FieldMonster"] {
				clone := map[string]string{}
				for k, v := range r {
					clone[k] = v
				}
				if r["MonId"] == monster.GetMonId() {
					clone["rewardGroupId"] = reward
				}
				frozenRows = append(frozenRows, clone)
			}
			frozenTables["FieldMonster"] = frozenRows
			rewardService := &GameService{Tables: frozenTables}
			gain, err := rewardService.fieldReward(id, player, g, monster)
			if err != nil {
				return err
			}
			result.Getn = gain
			monster.State, monster.CurHp = proto.Int32(3), proto.Float64(0)

			if cooldown <= 0 {
				return errors.New("Missing field respawn configuration")
			}
			f.RespawnAt[monster.GetUuid()] = time.Now().UnixMilli() + cooldown
		} else {
			monster.State = proto.Int32(1)
		}
		side.BattlePosHps, side.Anger, side.CurHp, side.State = hps, proto.Float64(anger), proto.Float64(total/maximum), proto.Int32(0)
		if total == 0 {
			side.State, result.State = proto.Int32(5), proto.Int32(5)
		}
		for _, hp := range hps {
			result.BattlePos = append(result.BattlePos, hp.GetBattlePos())
			result.Hps = append(result.Hps, hp.GetHpPercent())
			switch hp.GetBattlePos() {
			case 3:
				result.Hp3, side.BattlePos3 = hp.HpPercent, hp.HpPercent
			case 7:
				result.Hp7, side.BattlePos7 = hp.HpPercent, hp.HpPercent
			case 9:
				result.Hp9, side.BattlePos9 = hp.HpPercent, hp.HpPercent
			case 13:
				result.Hp13, side.BattlePos13 = hp.HpPercent, hp.HpPercent
			}
		}
		fight.Settled, fight.Result = true, proto.Clone(result).(*pb.SCSidePveFightEnd)
		if s.P8LifeReady {
			if _, err := s.RefreshP8Survival(tx, player, g, time.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	// CopyBalance 在展示结束时通过 AddResultObj 增加奖励，避免提前覆盖 Player 后再次加币。
	return []proto.Message{result, &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{side}}}, err
}

// fieldMove keeps movement, survival and gather updates in the original profile transaction.
func (s *GameService) fieldMove(ctx context.Context, id int64, b *pb.CSMove) error {
	return updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if s.P12FieldPVPReady {
			allowed, err := ShouldAllowP12PrisonTransfer(tx, id, g.Position.GetScene(), b.GetPointNew(), time.Now())
			if err != nil {
				return err
			}
			if !allowed {
				if err = CheckP12FieldPVPAction(tx, id, time.Now()); err != nil {
					return err
				}
			}
			if s.fieldSceneRow(g.Position.GetScene()) != nil && g.Field != nil && g.Field.Side != nil && (g.Field.Side.GetState() == constUtil.P8FieldDead || g.Field.Side.GetCurHp() <= 0) {
				return errors.New("死亡状态不能移动，请先完成原复活")
			}
		}
		if g.PositionLoading {
			return nil
		}
		if s.P8LifeReady && g.Position.GetScene() == "field_001" && g.Field != nil && g.Field.Side != nil && (g.Field.Side.GetState() == 5 || g.Field.Side.GetCurHp() <= 0) {
			return errors.New("死亡状态不能移动，请先复活")
		}
		if s.P8LifeReady {
			if _, err := s.RefreshP8Survival(tx, p, g, time.Now()); err != nil {
				return err
			}
		}
		if s.P8GatherReady {
			if err := s.P8ObserveGatherMove(tx, id, g, b.GetPointNew()); err != nil {
				return err
			}
		}
		g.Position.Current = b.GetPointNew()
		if s.P8LifeReady {
			if _, err := s.RefreshP8Survival(tx, p, g, time.Now()); err != nil {
				return err
			}
		}
		return nil
	})
}
