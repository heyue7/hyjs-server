package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"
)

type p13UFFieldSnapshot struct {
	MapProjection     string               `json:",omitempty"`
	ShopDistance      *float64             `json:",omitempty"`
	ResourceConfig    *p13UFResourceConfig `json:",omitempty"`
	Buildings         []byte
	LeftSpawn         *pb.PointNew
	RightSpawn        *pb.PointNew
	Minimum           [3]float32
	Maximum           [3]float32
	PreparationMillis int64
	MoveTolerance     float64
	MoveSpeed         float32
	KickMillis        int64
	PlayerLimit       int32
	Source            string
}

type p13UFOriginalCacheKey struct {
	ConfigurationHash, ClientVersion, EngineVersion string
	ServerID                                        int64
}

var p13UFOriginalCache = struct {
	sync.Mutex
	Entries map[p13UFOriginalCacheKey]*GameService
	Order   []p13UFOriginalCacheKey
}{Entries: make(map[p13UFOriginalCacheKey]*GameService)}

// p13UFFieldPrewarm 在监听前解析活跃冻结版本，首次HTTP不在公会锁内重复构造全原表。
func (s *GameService) p13UFFieldPrewarm(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	var policy db.P13UnionFightFieldPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}
	var gate db.P13GuildPolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return err
	}
	var enrollment db.P13UnionFightPolicy
	if err := tx.First(&enrollment, 1).Error; err != nil {
		return err
	}
	if !gate.ManagementEnabled || !enrollment.EnrollmentEnabled || !enrollment.MatchEnabled {
		return nil
	}
	var matches []db.P13UnionFightMatch
	if err := tx.Where("end_at>=?", p13UFNow(ctx).UnixMilli()).Order("start_at").Limit(constUtil.P13UnionFightOriginalCacheLimit).Find(&matches).Error; err != nil {
		return err
	}
	for _, match := range matches {
		if _, err := s.p13UFFieldOriginal(&match, tx); err != nil {
			return err
		}
	}
	return nil
}

func p13UFNow(ctx context.Context) time.Time {
	if clock, ok := ctx.Value(constUtil.P13UnionFightClockContextKey{}).(func() time.Time); ok {
		return clock()
	}
	return LocalPlayNow(ctx)
}

// InitializeP13UnionFightField 保存实际地图与角色生命周期，默认没有可开启的本地策略参数。
func (s *GameService) InitializeP13UnionFightField(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13UnionFightFieldPolicy{}, &db.P13UnionFightField{}, &db.P13UnionFightFieldPlayer{}); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13UnionFightFieldPolicy{ID: 1, Source: constUtil.P13UnionFightFieldSource}).Error; err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightFocus(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightChat(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightResource(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightTransfer(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightShop(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightFactory(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightItem(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightGateItem(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightCarItem(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightCarMotion(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightCarAttack(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightSpeedItem(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightHealAura(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightRevivePoint(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightGuardAudit(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightBattleAudit(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightPlayerBattle(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightWarDrum(ctx); err != nil {
		return err
	}
	return s.p13UFFieldPrewarm(ctx)
}

func p13UFFieldGUID(match, kind, id string) int64 {
	digest := sha256.Sum256([]byte(match + "/" + kind + "/" + id))
	return int64(binary.BigEndian.Uint64(digest[:8]) >> 1)
}

func (s *GameService) p13UFFieldOriginal(match *db.P13UnionFightMatch, tx *gorm.DB) (*GameService, error) {
	date, err := time.ParseInLocation("2006-01-02", match.Day, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil {
		return nil, err
	}
	var entry db.P13UnionFightEntry
	if err = tx.First(&entry, "guild_id=? AND day=?", match.LeftGuildID, date.AddDate(0, 0, -1).Format("2006-01-02")).Error; err != nil {
		return nil, err
	}
	if p6Digest(entry.FrozenTables) != match.ConfigurationHash || entry.ClientVersion != match.ClientVersion || entry.EngineVersion != match.EngineVersion {
		return nil, errors.New("公会战原实例配置版本损坏")
	}
	// 先校验本次真实数据库字节，再按完整版本命中只读缓存；不因缓存跳过损坏检测。
	key := p13UFOriginalCacheKey{match.ConfigurationHash, match.ClientVersion, match.EngineVersion, s.ServerID}
	p13UFOriginalCache.Lock()
	defer p13UFOriginalCache.Unlock()
	if original, ok := p13UFOriginalCache.Entries[key]; ok {
		return original, nil
	}
	var tables map[string][]map[string]string
	if err = json.Unmarshal(entry.FrozenTables, &tables); err != nil {
		return nil, err
	}
	original := &GameService{Tables: tables, ServerID: s.ServerID}
	if len(p13UFOriginalCache.Order) >= constUtil.P13UnionFightOriginalCacheLimit {
		oldest := p13UFOriginalCache.Order[0]
		delete(p13UFOriginalCache.Entries, oldest)
		p13UFOriginalCache.Order = p13UFOriginalCache.Order[1:]
	}
	p13UFOriginalCache.Entries[key] = original
	p13UFOriginalCache.Order = append(p13UFOriginalCache.Order, key)
	return original, nil
}

// p13UFFieldBuild 从本场冻结原表构造全部固定建筑，运营明确的非权威初始HP写入来源。
func (s *GameService) p13UFFieldBuild(tx *gorm.DB, match *db.P13UnionFightMatch, policy *db.P13UnionFightFieldPolicy, now time.Time) (*p13UFFieldSnapshot, error) {
	var saved db.P13UnionFightField
	err := tx.First(&saved, "match_id=?", match.ID).Error
	if err == nil {
		if p6Digest(saved.Snapshot) != saved.SnapshotHash {
			return nil, errors.New("公会战原地图快照损坏")
		}
		var snapshot p13UFFieldSnapshot
		if err = json.Unmarshal(saved.Snapshot, &snapshot); err != nil {
			return nil, err
		}
		if err := p13UFFieldProjection(&snapshot); err != nil {
			return nil, err
		}
		return &snapshot, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if !p13UFMapProjectionKnown(policy.MapProjection) || policy.PreparationMillis <= 0 || policy.PreparationMillis > 86400000 || policy.InitialGuardHP <= 0 || policy.InitialGateHP <= 0 || math.IsNaN(policy.InitialGuardHP) || math.IsInf(policy.InitialGuardHP, 0) || math.IsNaN(policy.InitialGateHP) || math.IsInf(policy.InitialGateHP, 0) || policy.MoveTolerance < 0 || policy.MoveTolerance > 50 || math.IsNaN(policy.MoveTolerance) || math.IsInf(policy.MoveTolerance, 0) || policy.Source == "" {
		return nil, errors.New("公会战具名本地场景策略尚未完整发布")
	}
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return nil, err
	}
	snapshot := &p13UFFieldSnapshot{MapProjection: policy.MapProjection, PreparationMillis: policy.PreparationMillis, MoveTolerance: policy.MoveTolerance, Source: constUtil.P13UnionFightFieldSource + "；发布：" + policy.Source}
	rate, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightPlayerMoveSpeed")["NameValue"], 32)
	if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return nil, errors.New("公会战原移动速度配置缺失")
	}
	snapshot.MoveSpeed = float32(rate)
	kick, err := strconv.ParseInt(original.table("GameConfigure", "StaticName", "UnionFightForceKickOut")["NameValue"], 10, 64)
	if err != nil || kick < 0 || kick > 3600 {
		return nil, errors.New("公会战原结束退场时长缺失")
	}
	snapshot.KickMillis = kick * 1000
	firstLimit, err := strconv.ParseInt(original.table("GameConfigure", "StaticName", "UnionFighterNum")["NameValue"], 10, 32)
	if err != nil || firstLimit <= 0 || firstLimit > 100 {
		return nil, errors.New("公会战原首发容量缺失")
	}
	snapshot.PlayerLimit = int32(firstLimit * 2)
	for i := 0; i < 3; i++ {
		snapshot.Minimum[i] = math.MaxFloat32
		snapshot.Maximum[i] = -math.MaxFloat32
	}
	for _, name := range []string{"UnionFightBaseCamp", "UnionFightGate", "UnionFightTower", "UnionFightFactory", "UnionFightRevivePoint", "UnionFightNpc", "UnionFightGuard"} {
		for _, row := range original.Tables[name] {
			point, err := fieldPoint(row["Position"])
			if err != nil {
				return nil, err
			}
			for i, value := range []float32{point.GetX(), point.GetY(), point.GetZ()} {
				snapshot.Minimum[i] = min(snapshot.Minimum[i], value)
				snapshot.Maximum[i] = max(snapshot.Maximum[i], value)
			}
		}
	}
	for side, id := range []int64{policy.LeftRevivePointID, policy.RightRevivePointID} {
		row := original.table("UnionFightRevivePoint", "UFRPId", strconv.FormatInt(id, 10))
		if row == nil || row["InitCamp"] != strconv.Itoa(side) || row["IsCanOccupy"] != "0" {
			return nil, errors.New("公会战本地入口须指向本阵营原固定复活点")
		}
		point, err := fieldPoint(row["Position"])
		if err != nil {
			return nil, err
		}
		if snapshot.MapProjection == constUtil.P13UnionFightMapProjectionReFlat {
			point.Y = proto.Float32(0)
		}
		if side == 0 {
			snapshot.LeftSpawn = point
		} else {
			snapshot.RightSpawn = point
		}
	}
	buildings := &pb.SCUFBuildingRefresh{}
	guard := func(id string, camp int32, bind int64) (*pb.InstUFGuard, error) {
		if id == "" {
			return nil, nil
		}
		row := original.table("UnionFightGuard", "UFGuardId", id)
		if row == nil {
			return nil, errors.New("原建筑绑定守卫缺失")
		}
		point, err := fieldPoint(row["Position"])
		if err != nil {
			return nil, err
		}
		guid := p13UFFieldGUID(match.ID, "guard", id)
		return &pb.InstUFGuard{Guid: proto.Int64(guid), UFGuardId: proto.String(id), Camp: proto.Int32(camp), BindGuid: proto.Int64(bind), State: proto.Int32(0), InitPosition: point, FightInfo: &pb.InstFightable{Guid: proto.Int64(guid), Index: []int32{0}, HpPos: []float64{policy.InitialGuardHP}, MaxHpPos: []float64{policy.InitialGuardHP}, Anger: proto.Float64(0)}}, nil
	}
	for _, row := range original.Tables["UnionFightBaseCamp"] {
		id, err := strconv.ParseInt(row["UFBId"], 10, 32)
		if err != nil {
			return nil, err
		}
		camp, err := strconv.ParseInt(row["Camp"], 10, 32)
		if err != nil {
			return nil, err
		}
		guid := p13UFFieldGUID(match.ID, "base", row["UFBId"])
		unit, err := guard(row["UFGuardId"], int32(camp), guid)
		if err != nil {
			return nil, err
		}
		buildings.BaseCampInfos = append(buildings.BaseCampInfos, &pb.InstUFBaseCamp{Guid: proto.Int64(guid), Id: proto.Int32(int32(id)), Camp: proto.Int32(int32(camp)), Guard: unit})
	}
	for _, row := range original.Tables["UnionFightGate"] {
		id, err := strconv.ParseInt(row["UFGId"], 10, 32)
		if err != nil {
			return nil, err
		}
		camp, err := strconv.ParseInt(row["Camp"], 10, 32)
		if err != nil {
			return nil, err
		}
		buildings.GateInfos = append(buildings.GateInfos, &pb.InstUFGate{Guid: proto.Int64(p13UFFieldGUID(match.ID, "gate", row["UFGId"])), Id: proto.Int32(int32(id)), Camp: proto.Int32(int32(camp)), Level: proto.Int32(1), Hp: proto.Float64(policy.InitialGateHP), MaxHp: proto.Float64(policy.InitialGateHP)})
	}
	for _, row := range original.Tables["UnionFightTower"] {
		id, err := strconv.ParseInt(row["UFTId"], 10, 32)
		if err != nil {
			return nil, err
		}
		camp, err := strconv.ParseInt(row["Camp"], 10, 32)
		if err != nil {
			return nil, err
		}
		guid := p13UFFieldGUID(match.ID, "tower", row["UFTId"])
		unit, err := guard(row["UFGuardId"], int32(camp), guid)
		if err != nil {
			return nil, err
		}
		buildings.TowerInfos = append(buildings.TowerInfos, &pb.InstUFTower{Guid: proto.Int64(guid), Id: proto.Int32(int32(id)), Level: proto.Int32(1), State: proto.Int32(0), Guard: unit})
	}
	for _, row := range original.Tables["UnionFightFactory"] {
		id, err := strconv.ParseInt(row["UFFId"], 10, 32)
		if err != nil {
			return nil, err
		}
		camp, err := strconv.ParseInt(row["Camp"], 10, 32)
		if err != nil {
			return nil, err
		}
		guid := p13UFFieldGUID(match.ID, "factory", row["UFFId"])
		unit, err := guard(row["UFGuardId"], int32(camp), guid)
		if err != nil {
			return nil, err
		}
		open := int32(1)
		state := int32(0)
		if row["IsField"] == "1" {
			open = 0
			state = 2
		}
		buildings.FactoryInfos = append(buildings.FactoryInfos, &pb.InstUFCariotFactory{Guid: proto.Int64(guid), FactoryId: proto.Int32(int32(id)), IsOpen: proto.Int32(open), State: proto.Int32(state), Guard: unit})
	}
	for _, row := range original.Tables["UnionFightRevivePoint"] {
		id, err := strconv.ParseInt(row["UFRPId"], 10, 32)
		if err != nil {
			return nil, err
		}
		camp, err := strconv.ParseInt(row["InitCamp"], 10, 32)
		if err != nil {
			return nil, err
		}
		buildings.PointInfos = append(buildings.PointInfos, &pb.InstUFRevivePoint{Guid: proto.Int64(p13UFFieldGUID(match.ID, "revive", row["UFRPId"])), Id: proto.Int32(int32(id)), Camp: proto.Int32(int32(camp)), LeftOccupyProgress: proto.Float64(0), RightOccupyProgress: proto.Float64(0)})
	}
	for camp, raw := range [][]byte{match.LeftPreview, match.RightPreview} {
		preview := &pb.UFInfoPreView{}
		if err = proto.Unmarshal(raw, preview); err != nil {
			return nil, err
		}
		guildID := match.LeftGuildID
		if camp == 1 {
			guildID = match.RightGuildID
		}
		if preview.GetUnionId() != guildID {
			return nil, errors.New("公会战原两阵营身份损坏")
		}
		var guild db.P13Guild
		if err = tx.First(&guild, "id=?", guildID).Error; err != nil {
			return nil, err
		}
		buildings.UnionInfos = append(buildings.UnionInfos, &pb.InstUFUnion{UnionId: proto.String(guildID), ServerId: preview.ServerId, UnionFlag: proto.Int32(guild.Flag), UnionName: preview.UnionName, Camp: proto.Int32(int32(camp)), UnionFightPower: preview.FightPower, UnionResource: &pb.InstUFResource{ResourceNum: proto.Float64(0)}, UnionContro: proto.Float64(0)})
	}
	shopDistance, err := s.p13UFShopBuild(tx, match, original, now)
	if err != nil {
		return nil, err
	}
	snapshot.ShopDistance = &shopDistance
	if err = s.p13UFFactoryBuild(tx, match, buildings, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFCarMotionBuild(tx, match, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFItemBuild(tx, match, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFGateItemBuild(tx, match, buildings, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFCarAttackBuild(tx, match, buildings, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFCarItemBuild(tx, match, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFSpeedBuild(tx, match, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFHealAuraBuild(tx, match, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFBattleAuditBuild(tx, match, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFPlayerBattleBuild(tx, match, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFRevivePointBuild(tx, match, buildings, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFGuardAuditBuild(tx, match, buildings, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFDrumBuild(tx, match, original, now); err != nil {
		return nil, err
	}
	if err = s.p13UFResourceBuild(tx, match, snapshot, original, now); err != nil {
		return nil, err
	}
	snapshot.Buildings, err = proto.Marshal(buildings)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if err = tx.Create(&db.P13UnionFightField{MatchID: match.ID, Snapshot: raw, SnapshotHash: p6Digest(raw), CreatedAt: now.UnixMilli(), Source: snapshot.Source}).Error; err != nil {
		return nil, err
	}
	return snapshot, nil
}

func p13UFFieldFighter(tx *gorm.DB, match *db.P13UnionFightMatch, id int64, guild string) (*db.P13UnionFightFighter, int64, error) {
	if p6Digest(match.Roster) != match.SnapshotHash {
		return nil, 0, errors.New("公会战本场名单快照损坏")
	}
	var roster p13UFMatchRoster
	if err := json.Unmarshal(match.Roster, &roster); err != nil {
		return nil, 0, err
	}
	fighters := roster.Left
	camp := int64(0)
	if guild == match.RightGuildID {
		fighters = roster.Right
		camp = 1
	} else if guild != match.LeftGuildID {
		return nil, 0, errors.New("角色不属于本场双方公会")
	}
	for _, fighter := range fighters {
		if fighter.PlayerID == id && fighter.AppliedAt > 0 && fighter.FirstOn {
			frozen, err := p13UFReadMatchFighter(tx, fighter)
			if err != nil {
				return nil, 0, err
			}
			return frozen, camp, nil
		}
	}
	return nil, 0, errors.New("原本场25首发资格不足，替补实际替换待恢复")
}

func p13UFFieldPlayerFrame(fighter *db.P13UnionFightFighter, camp int64, server int64, mainGUID string) (*pb.InstUFPlayer, error) {
	battle := &pb.BattlePlayer{}
	if proto.Unmarshal(fighter.Battle, battle) != nil || battle.GetPlayerId() != fighter.PlayerID || int64(battle.GetServerId()) != server || len(battle.Formation) == 0 {
		return nil, errors.New("公会战原角色联合身份损坏")
	}
	actors := append([]*pb.BattleHero(nil), battle.Formation...)
	sort.Slice(actors, func(i, j int) bool { return actors[i].GetPose() < actors[j].GetPose() })
	var main *pb.BattleHero
	for _, actor := range actors {
		if actor.GetGuid() == mainGUID {
			main = actor
			break
		}
	}
	if main == nil {
		return nil, errors.New("公会战冻结阵容缺少实际主角GUID")
	}
	if math.IsNaN(battle.GetInst().GetAnger()) || math.IsInf(battle.GetInst().GetAnger(), 0) || battle.GetInst().GetAnger() < 0 {
		return nil, errors.New("公会战冻结AP面板无效")
	}
	fight := &pb.InstFightable{Guid: proto.Int64(fighter.PlayerID), Anger: proto.Float64(battle.GetInst().GetAnger()), FightPower: proto.Float64(float64(fighter.FightPower))}
	seen := map[int32]bool{}
	for _, actor := range actors {
		hp := actor.GetProp().GetHp()
		cur := actor.GetInst().GetCurHp()
		if seen[actor.GetPose()] || hp <= 0 || math.IsNaN(hp) || math.IsInf(hp, 0) || cur < 0 || cur > hp || math.IsNaN(cur) || math.IsInf(cur, 0) {
			return nil, errors.New("公会战冻结Pose/HP/AP面板无效")
		}
		seen[actor.GetPose()] = true
		fight.Index = append(fight.Index, actor.GetPose())
		fight.HpPos = append(fight.HpPos, cur)
		fight.MaxHpPos = append(fight.MaxHpPos, hp)
	}
	return &pb.InstUFPlayer{Guid: proto.Int64(fighter.PlayerID), ServerId: proto.Int32(int32(server)), HeroId: main.Id, WeaponId: main.WeaponId, NickName: proto.String(fighter.Name), State: proto.Int32(0), Camp: proto.Int32(int32(camp)), UnionAuthority: proto.Int32(fighter.Role), FightInfo: fight, OwnRes: &pb.InstUFResource{ResourceNum: proto.Float64(0)}, OwnToken: proto.Float64(0), OwnScore: proto.Float64(0), IsAuto: proto.Int32(0)}, nil
}

func p13UFMapProjectionKnown(projection string) bool {
	return projection == "" || projection == constUtil.P13UnionFightMapProjectionReFlat
}

func p13UFFieldProjection(snapshot *p13UFFieldSnapshot) error {
	if snapshot == nil || !p13UFMapProjectionKnown(snapshot.MapProjection) || snapshot.MapProjection == constUtil.P13UnionFightMapProjectionReFlat && (snapshot.LeftSpawn == nil || snapshot.RightSpawn == nil || snapshot.LeftSpawn.GetY() != 0 || snapshot.RightSpawn.GetY() != 0) {
		return errors.New("公会战冻结地图投影损坏")
	}
	return nil
}

func p13UFFieldActorProjection(actor *db.P13UnionFightFieldPlayer, snapshot *p13UFFieldSnapshot) error {
	if err := p13UFFieldProjection(snapshot); err != nil {
		return err
	}
	if actor == nil || actor.MapProjection != snapshot.MapProjection || actor.MapProjection == constUtil.P13UnionFightMapProjectionReFlat && actor.Y != 0 {
		return errors.New("公会战角色与冻结地图投影不一致")
	}
	return nil
}

// Readers outside Current validate the existing immutable field without
// rebuilding it from a changed live policy or rewriting historical actors.
func p13UFFieldStoredProjection(tx *gorm.DB, scope string) (*p13UFFieldSnapshot, error) {
	var saved db.P13UnionFightField
	if err := tx.First(&saved, "match_id=?", scope).Error; err != nil {
		return nil, err
	}
	var snapshot p13UFFieldSnapshot
	if saved.SnapshotHash != p6Digest(saved.Snapshot) || json.Unmarshal(saved.Snapshot, &snapshot) != nil {
		return nil, errors.New("公会战原地图快照损坏")
	}
	if err := p13UFFieldProjection(&snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func p13UFFieldReadActor(actor *db.P13UnionFightFieldPlayer) (*pb.InstUFPlayer, error) {
	if actor == nil || !p13UFMapProjectionKnown(actor.MapProjection) || actor.MapProjection == constUtil.P13UnionFightMapProjectionReFlat && actor.Y != 0 {
		return nil, errors.New("公会战角色投影或平面高度损坏")
	}
	if p6Digest(actor.Frame) != actor.FrameHash || p6Digest(actor.Battle) != actor.BattleHash {
		return nil, errors.New("公会战持久角色状态损坏")
	}
	frame := &pb.InstUFPlayer{}
	if proto.Unmarshal(actor.Frame, frame) != nil || frame.GetGuid() != actor.PlayerID || int64(frame.GetCamp()) != actor.Camp {
		return nil, errors.New("公会战原角色实例身份损坏")
	}
	return frame, nil
}

func p13UFFieldSaveActor(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer) error {
	raw, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	actor.Frame = raw
	actor.FrameHash = p6Digest(raw)
	return tx.Save(actor).Error
}

func (s *GameService) p13UFFieldNeedPolicy(tx *gorm.DB) (*db.P13UnionFightFieldPolicy, error) {
	var guild db.P13GuildPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&guild, 1).Error; err != nil {
		return nil, err
	}
	var enroll db.P13UnionFightPolicy
	if err := tx.First(&enroll, 1).Error; err != nil {
		return nil, err
	}
	var policy db.P13UnionFightFieldPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return nil, err
	}
	if !guild.ManagementEnabled || !enroll.EnrollmentEnabled || !enroll.MatchEnabled || !policy.Enabled {
		return nil, errors.New("公会战真实场景尚未开放")
	}
	return &policy, nil
}

func (s *GameService) p13UFFieldEnter(ctx context.Context, id int64, now time.Time) ([]proto.Message, error) {
	_, day, stage, _ := p13UFClock(now)
	if stage == constUtil.P13UnionFightStageMatch {
		return nil, errors.New("原公会战匹配窗口尚未结束")
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		policy, err := s.p13UFFieldNeedPolicy(tx)
		if err != nil {
			return err
		}
		var row db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if s.P12FieldPVPReady || s.P12WantedReady {
			if err = s.ensureNoActiveDailyCopy(tx, id, now); err != nil {
				return err
			}
		}
		if game.Run != nil && !game.Run.Complete || game.Field != nil && game.Field.Fight != nil && !game.Field.Fight.Settled {
			return errors.New("原现有对局尚未结束，不能进入公会战")
		}
		member, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		var matches []db.P13UnionFightMatch
		if err = tx.Where("(left_guild_id=? OR right_guild_id=?) AND end_at>?", guild.ID, guild.ID, now.UnixMilli()).Order("start_at").Find(&matches).Error; err != nil {
			return err
		}
		var selected *db.P13UnionFightMatch
		for i := range matches {
			if policy.PreparationMillis > 0 && now.UnixMilli() >= matches[i].StartAt-policy.PreparationMillis {
				selected = &matches[i]
				break
			}
		}
		if selected == nil {
			return errors.New("本会没有处于已发布准备或开赛窗口的真实对局")
		}
		if selected.Day != day {
			return errors.New("本场已发布对局不属于当前原比赛日")
		}
		fighter, camp, err := p13UFFieldFighter(tx, selected, id, guild.ID)
		if err != nil {
			return err
		}
		snapshot, err := s.p13UFFieldBuild(tx, selected, policy, now)
		if err != nil {
			return err
		}
		if now.UnixMilli() < selected.StartAt-snapshot.PreparationMillis {
			return errors.New("本场冻结的准备窗口尚未开始")
		}
		var actor db.P13UnionFightFieldPlayer
		err = tx.First(&actor, "match_id=? AND player_id=?", selected.ID, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			frame, err := p13UFFieldPlayerFrame(fighter, camp, s.OriginalServerID(), player.GetHeroWarId())
			if err != nil {
				return err
			}
			frame.UnionAuthority = proto.Int32(min(fighter.Role, member.Role))
			if fighter.AutoFight {
				frame.IsAuto = proto.Int32(1)
			}
			point := snapshot.LeftSpawn
			if camp == 1 {
				point = snapshot.RightSpawn
			}
			actor = db.P13UnionFightFieldPlayer{MapProjection: snapshot.MapProjection, MatchID: selected.ID, PlayerID: id, GuildID: guild.ID, Camp: camp, Battle: append([]byte(nil), fighter.Battle...), BattleHash: fighter.SnapshotHash, X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), JoinedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(), Source: snapshot.Source}
			if err = p13UFFieldSaveActor(tx, &actor, frame); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if _, err = p13UFFieldReadActor(&actor); err != nil {
				return err
			}
		}
		if err = p13UFFieldActorProjection(&actor, snapshot); err != nil {
			return err
		}
		// 每个角色只有一场在线状态；原帧重复转场继续使用已持久位置，不重造HP/AP或物品权益。
		closed := tx.Model(&db.P13UnionFightFieldPlayer{}).Where("player_id=? AND match_id<>? AND left_at=0", id, selected.ID).Updates(map[string]any{"left_at": now.UnixMilli(), "session_hash": ""})
		if closed.Error != nil {
			return closed.Error
		}
		actor.LeftAt = 0
		if actor.SessionHash != p6Digest(row.Session) {
			actor.SessionHash = ""
		}
		actor.UpdatedAt = now.UnixMilli()
		if err = tx.Save(&actor).Error; err != nil {
			return err
		}
		if s.P8GatherReady {
			if err = s.P8CancelGather(tx, id, game); err != nil {
				return err
			}
		}
		// 同名fight_union换轮仍加载新的实际实例；旧轮排队移动必须等待新Current握手。
		if game.Position.GetScene() != constUtil.P13UnionFightFieldTarget || closed.RowsAffected > 0 {
			game.PositionLoading = true
		}
		game.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(constUtil.P13UnionFightFieldTarget), Current: &pb.PointNew{X: proto.Float32(actor.X), Y: proto.Float32(actor.Y), Z: proto.Float32(actor.Z)}}
		return saveProfile(tx, &row, player, game)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCTransferScene{Result: proto.Int32(1)}}, nil
}

func (s *GameService) p13UFFieldCurrent(tx *gorm.DB, id int64, now time.Time) (*db.P13UnionFightFieldPlayer, *db.P13UnionFightMatch, *p13UFFieldSnapshot, *pb.InstUFPlayer, error) {
	_, err := s.p13UFFieldNeedPolicy(tx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var actor db.P13UnionFightFieldPlayer
	if err = tx.First(&actor, "player_id=? AND left_at=0", id).Error; err != nil {
		return nil, nil, nil, nil, errors.New("角色尚未进入实际公会战实例")
	}
	member, _, err := p13GuildNeedMember(tx, id)
	if err != nil || member.GuildID != actor.GuildID {
		return nil, nil, nil, nil, errors.New("角色已退出本场真实公会")
	}
	var match db.P13UnionFightMatch
	if err = tx.First(&match, "id=?", actor.MatchID).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	fighter, camp, err := p13UFFieldFighter(tx, &match, id, actor.GuildID)
	if err != nil || camp != actor.Camp {
		return nil, nil, nil, nil, errors.New("本场首发资格已失效")
	}
	// Entry creates the immutable map before its actor. Current readers must
	// reject a missing map instead of rebuilding it from a changed live policy.
	snapshot, err := p13UFFieldStoredProjection(tx, match.ID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if now.UnixMilli() < match.StartAt-snapshot.PreparationMillis {
		return nil, nil, nil, nil, errors.New("原本场准备窗口尚未开始")
	}
	if now.UnixMilli() >= match.EndAt+snapshot.KickMillis {
		return nil, nil, nil, nil, errors.New("原本场公会战退场时限已到")
	}
	if err = p13UFFieldActorProjection(&actor, snapshot); err != nil {
		return nil, nil, nil, nil, err
	}
	frame, err := p13UFFieldReadActor(&actor)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	frame.UnionAuthority = proto.Int32(min(fighter.Role, member.Role))
	if _, err = p13UFItemProject(tx, match.ID, id, frame, now); err != nil {
		return nil, nil, nil, nil, err
	}
	if changed, speedErr := p13UFSpeedProject(tx, match.ID, id, frame, now); speedErr != nil {
		return nil, nil, nil, nil, speedErr
	} else if changed {
		if err = p13UFFieldSaveActor(tx, &actor, frame); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	if changed, auraErr := p13UFHealAuraProject(tx, match.ID, id, frame, now); auraErr != nil {
		return nil, nil, nil, nil, auraErr
	} else if changed {
		if err = p13UFFieldSaveActor(tx, &actor, frame); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	if err = p13UFPlayerBattleProject(tx, &actor, frame, now); err != nil {
		return nil, nil, nil, nil, err
	}
	return &actor, &match, snapshot, frame, nil
}

func p13UFFieldState(match *db.P13UnionFightMatch, now time.Time) (int32, int64, int64) {
	if now.UnixMilli() < match.StartAt {
		return 1, match.StartAt - now.UnixMilli(), match.StartAt - match.CreatedAt
	}
	if now.UnixMilli() < match.EndAt {
		return 2, match.EndAt - now.UnixMilli(), match.EndAt - match.StartAt
	}
	return 3, 0, match.EndAt - match.StartAt
}

// p13UFFieldFrames 返回原切房/初始化帧，模型/地图来自本场持久快照，不从当前改装重算HP/AP。
func (s *GameService) p13UFFieldFrames(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var messages []proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, snapshot, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		position := &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(constUtil.P13UnionFightFieldTarget), Current: &pb.PointNew{X: proto.Float32(actor.X), Y: proto.Float32(actor.Y), Z: proto.Float32(actor.Z)}}
		battle := &pb.BattlePlayer{}
		if proto.Unmarshal(actor.Battle, battle) != nil {
			return errors.New("公会战冻结等级面板损坏")
		}
		move := &pb.MovePlayer{Position: position, Info: &pb.MovePlayerInfo{PlayerId: proto.Int64(id), ServerId: frame.ServerId, PlayerName: frame.NickName, Level: battle.Level, WarId: frame.HeroId, PlayerSide: &pb.PlayerSide{PlayerId: proto.Int64(id), Kills: frame.KillNum}}}
		switch request.(type) {
		case *pb.CSCurrentPosition:
			messages = append(messages, &pb.SCCurrentPosition{Self: move, Limit: proto.Int32(snapshot.PlayerLimit)})
		case *pb.CSLoginScene:
			messages = append(messages, &pb.SCLoginScene{Self: move, Limit: proto.Int32(snapshot.PlayerLimit)})
		}
		buildings := &pb.SCUFBuildingRefresh{}
		if proto.Unmarshal(snapshot.Buildings, buildings) != nil {
			return errors.New("公会战原建筑快照损坏")
		}
		if err = s.p13UFResourceOverlayCamp(tx, match.ID, buildings); err != nil {
			return err
		}
		if err = p13UFRevivePointApply(tx, match.ID, buildings); err != nil {
			return err
		}
		if err = s.p13UFFactoryOverlay(tx, match.ID, buildings); err != nil {
			return err
		}
		if err = p13UFGateItemOverlay(tx, match.ID, buildings); err != nil {
			return err
		}
		state, left, total := p13UFFieldState(match, now)
		if state == 1 {
			total = snapshot.PreparationMillis
		}
		transfer := &pb.SCUnionFightTransfer{UnionFightId: proto.String(match.ID), State: proto.Int32(state), LeftTime: proto.Int64(left), TotalTime: proto.Int64(total), Player: frame, PlayerPos: position.Current, BaseCampInfos: buildings.BaseCampInfos, FactoryInfos: buildings.FactoryInfos, GateInfos: buildings.GateInfos, TowerInfos: buildings.TowerInfos, RevivePointInfos: buildings.PointInfos, UnionInfos: buildings.UnionInfos}
		messages = append(messages, transfer)
		cars, err := s.p13UFFactoryCars(tx, match, nil)
		if err != nil {
			return err
		}
		messages = append(messages, cars...)
		drumFrames, drumErr := p13UFDrumFrames(tx, match.ID, nil, now)
		if drumErr != nil {
			return drumErr
		}
		messages = append(messages, drumFrames...)
		nodes, err := s.p13UFResourceFrames(tx, match.ID, nil)
		if err != nil {
			return err
		}
		messages = append(messages, nodes...)
		mining, err := s.p13UFResourceRecovery(tx, actor, now)
		if err != nil {
			return err
		}
		messages = append(messages, mining...)
		transferring, err := s.p13UFTransferRecovery(tx, actor, now)
		if err != nil {
			return err
		}
		messages = append(messages, transferring...)
		if actor.Hooked {
			messages = append(messages, &pb.SCUFOnHook{OnHook: proto.Int32(1)})
		}
		if _, loaded := request.(*pb.CSCurrentPosition); loaded {
			// 原新Room.OnLoadComplete后同TCP才发CurrentPosition；成功完整回包与释放旧场景移动门禁同事务。
			var row db.Profile
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
				return err
			}
			player, game, err := decodeProfileForCalendarUpdate(tx, &row)
			if err != nil {
				return err
			}
			if game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
				return errors.New("公会战Current握手的实际场景已改变")
			}
			if game.PositionLoading {
				game.PositionLoading = false
				return saveProfile(tx, &row, player, game)
			}
		}
		return nil
	})
	return messages, err
}

// P13UnionFightFieldNormalize 在原会员权限撤销时持久退场；事务由调用者持有角色锁。
func (s *GameService) P13UnionFightFieldNormalize(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, now time.Time) (bool, error) {
	if g.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
		return false, nil
	}
	member, err := p13GuildMember(tx, id)
	if err != nil {
		return false, err
	}
	var actor db.P13UnionFightFieldPlayer
	err = tx.First(&actor, "player_id=? AND left_at=0", id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	if err == nil && member.GuildID == actor.GuildID && member.GuildID != "" {
		return false, nil
	}
	if err == nil {
		if _, _, err = s.p13UFResourceCancel(tx, actor.MatchID, id, now); err != nil {
			return false, err
		}
		if err = s.p13UFTransferCancel(tx, actor.MatchID, id, now); err != nil {
			return false, err
		}
		if err = tx.Model(&actor).Updates(map[string]any{"left_at": now.UnixMilli(), "session_hash": ""}).Error; err != nil {
			return false, err
		}
		if _, err = p13UFDrumCancel(tx, actor.MatchID, id, now); err != nil {
			return false, err
		}
		if _, err = p13UFBuffScopeCancel(tx, actor.MatchID, id, now); err != nil {
			return false, err
		}
	}
	g.PositionLoading = true
	g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String("town_main"), Current: &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}}
	return true, nil
}

func (s *GameService) p13UFFieldMove(ctx context.Context, id int64, request *pb.CSMove, now time.Time) ([]proto.Message, error) {
	var moved *pb.PointNew
	var scope string
	var speed float32
	var canceledGUID int64
	ignored := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// 沿原公会锁→角色锁顺序先检查加载状态，旧Town排队点在地图/状态/限速验证前静默丢弃。
		var policy db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if g.PositionLoading {
			ignored = true
			return nil
		}
		actor, match, snapshot, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		if err = p13UFPlayerBattleMovement(tx, actor, frame); err != nil {
			return err
		}
		if now.UnixMilli() >= match.EndAt {
			return errors.New("原本场已结束，不能继续移动")
		}
		if frame.GetState() == 4 || frame.GetState() == 12 {
			return errors.New("原传送动画隐藏操控，完成前不能手动移动")
		}
		if actor.Hooked {
			return errors.New("原挂机状态不能提交手动移动")
		}
		point := request.GetPointNew()
		if point == nil {
			return errors.New("公会战移动目标缺失")
		}
		point = proto.Clone(point).(*pb.PointNew)
		if actor.MapProjection == constUtil.P13UnionFightMapProjectionReFlat {
			if math.IsNaN(float64(point.GetY())) || math.IsInf(float64(point.GetY()), 0) || math.Abs(float64(point.GetY())) > constUtil.P13UnionFightFlatYTolerance {
				return errors.New("公会战平面移动高度无效")
			}
			point.Y = proto.Float32(0)
		}
		for i, value := range []float32{point.GetX(), point.GetY(), point.GetZ()} {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || float64(value) < float64(snapshot.Minimum[i])-snapshot.MoveTolerance || float64(value) > float64(snapshot.Maximum[i])+snapshot.MoveTolerance {
				return errors.New("公会战移动超出原静态地图与发布余量")
			}
		}
		rate := float64(snapshot.MoveSpeed)
		if rate <= 0 {
			return errors.New("公会战原移动速度配置缺失")
		}
		dx := float64(point.GetX() - actor.X)
		dy := float64(point.GetY() - actor.Y)
		dz := float64(point.GetZ() - actor.Z)
		distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
		earned, currentRate, speedErr := p13UFSpeedMoveAllowance(tx, actor, rate, now.UnixMilli())
		if speedErr != nil {
			return speedErr
		}
		if actor.MovementDebt < 0 || math.IsNaN(actor.MovementDebt) || math.IsInf(actor.MovementDebt, 0) || distance > earned+max(float64(0), snapshot.MoveTolerance-actor.MovementDebt) {
			return errors.New("公会战移动超过真实服务时钟允许距离")
		}
		if g.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
			return errors.New("角色实际位置已经离开公会战")
		}
		guid, canceled, err := s.p13UFResourceCancel(tx, actor.MatchID, id, now)
		if err != nil {
			return err
		}
		if canceled {
			canceledGUID = guid
			frame.State, frame.LeftTime, frame.TotalTime = proto.Int32(0), proto.Float32(0), proto.Float32(0)
		}
		actor.X = point.GetX()
		actor.Y = point.GetY()
		actor.Z = point.GetZ()
		actor.UpdatedAt = now.UnixMilli()
		// 余量只作为累计移动债务使用，连续8Hz请求不能每次重新获得同一段免费距离。
		actor.MovementDebt = max(float64(0), actor.MovementDebt+distance-earned)
		if err = p13UFFieldSaveActor(tx, actor, frame); err != nil {
			return err
		}
		g.Position.Current = proto.Clone(point).(*pb.PointNew)
		if err = saveProfile(tx, &row, p, g); err != nil {
			return err
		}
		moved = proto.Clone(point).(*pb.PointNew)
		scope = actor.MatchID
		speed = float32(currentRate)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if ignored {
		return nil, nil
	}
	message := &pb.SCUFElementMove{Guid: []int64{id}, Pos: []*pb.PointNew{moved}, Speed: []float32{speed}}
	var response []proto.Message
	if canceledGUID > 0 {
		ended := &pb.SCUFPlayerCollectEnd{PlayerGuid: []int64{id}, ResGuid: []int64{canceledGUID}, IsResExhausted: []int32{0}}
		s.p13UFFieldBroadcast(ctx, scope, id, ended)
		response = append(response, ended)
	}
	s.p13UFFieldBroadcast(ctx, scope, id, message)
	return append(response, message), nil
}

func (s *GameService) p13UFFieldLeave(ctx context.Context, id int64, request *pb.CSTransferScene, now time.Time) ([]proto.Message, error) {
	messages, _, err := s.p13UFFieldLeaveExpected(ctx, id, request, now, "")
	return messages, err
}

// p13UFFieldLeaveExpected rechecks a queued worker's exact war under the same
// guild/profile locks as entering a round. A stale old-round kick has no effects.
func (s *GameService) p13UFFieldLeaveExpected(ctx context.Context, id int64, request *pb.CSTransferScene, now time.Time, expectedMatch string) ([]proto.Message, bool, error) {
	if request.GetTarget() != "town" && request.GetTarget() != "town_main" {
		return nil, false, errors.New("原公会战退场先返回主城")
	}
	var scope string
	left := false
	var canceledDrums []proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		var actor db.P13UnionFightFieldPlayer
		err = tx.First(&actor, "player_id=? AND left_at=0", id).Error
		if expectedMatch != "" && (errors.Is(err, gorm.ErrRecordNotFound) || err == nil && actor.MatchID != expectedMatch) {
			return nil
		}
		left = true
		if err == nil {
			scope = actor.MatchID
			_, _, err = s.p13UFResourceCancel(tx, actor.MatchID, id, now)
			if err != nil {
				return err
			}
			if err = s.p13UFTransferCancel(tx, actor.MatchID, id, now); err != nil {
				return err
			}
			if err != nil {
				return err
			}
			if err = tx.Model(&actor).Updates(map[string]any{"left_at": now.UnixMilli(), "session_hash": ""}).Error; err != nil {
				return err
			}
			canceledDrums, err = p13UFDrumCancel(tx, actor.MatchID, id, now)
			if err != nil {
				return err
			}
			buffOff, err := p13UFBuffScopeCancel(tx, actor.MatchID, id, now)
			if err != nil {
				return err
			}
			canceledDrums = append(canceledDrums, buffOff...)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		point := request.GetPoint()
		if point == nil {
			point = &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}
		}
		if g.Position.GetScene() != request.GetTarget() {
			g.PositionLoading = true
		}
		g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(request.GetTarget()), Current: proto.Clone(point).(*pb.PointNew)}
		return saveProfile(tx, &row, p, g)
	})
	if err != nil {
		return nil, false, err
	}
	if !left {
		return nil, false, nil
	}
	if scope != "" {
		s.p13UFFieldBroadcast(ctx, scope, id, &pb.SCUFPlayerDisappear{Guid: []int64{id}})
	}
	for _, canceled := range canceledDrums {
		s.p13UFFieldBroadcast(ctx, scope, id, canceled)
	}
	return append([]proto.Message{&pb.SCTransferScene{Result: proto.Int32(1)}}, canceledDrums...), true, nil
}

func (s *GameService) p13UFFieldLive(tx *gorm.DB, actor db.P13UnionFightFieldPlayer) (*pb.InstUFPlayer, bool) {
	value, ok := p10ClientSinks.Load(p10ClientKey{s.p10ClientOwner(), actor.PlayerID})
	if !ok || actor.LeftAt != 0 || actor.SessionHash == "" {
		return nil, false
	}
	sink := value.(*p10ClientSink)
	if p6Digest(sink.session) != actor.SessionHash {
		return nil, false
	}
	profile, err := AuthorizeAccount(tx.Statement.Context, actor.PlayerID, sink.session)
	if err != nil {
		return nil, false
	}
	_, game, err := decodeProfile(profile)
	if err != nil || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
		return nil, false
	}
	member, err := p13GuildMember(tx, actor.PlayerID)
	if err != nil || member.GuildID != actor.GuildID {
		return nil, false
	}
	snapshot, projectionErr := p13UFFieldStoredProjection(tx, actor.MatchID)
	if projectionErr != nil || p13UFFieldActorProjection(&actor, snapshot) != nil {
		return nil, false
	}
	frame, err := p13UFFieldReadActor(&actor)
	if err != nil {
		return nil, false
	}
	frame.UnionAuthority = proto.Int32(min(frame.GetUnionAuthority(), member.Role))
	return frame, true
}

type p13UFFieldLiveRow struct {
	db.P13UnionFightFieldPlayer
	CurrentGuildID string
	ProfileSession []byte
	ActualScene    string
	CurrentRole    int32
}

func (s *GameService) p13UFFieldPeers(tx *gorm.DB, scope string, exclude int64) ([]db.P13UnionFightFieldPlayer, []*pb.InstUFPlayer, error) {
	var rows []p13UFFieldLiveRow
	sceneExpression := "json_extract(p.game,'$.position.scene')"
	if tx.Dialector.Name() == "mysql" {
		sceneExpression = "JSON_UNQUOTE(" + sceneExpression + ")"
	}
	// 原移动每秒8次；一次联查会话/当前场景/会员，避免给每个旁观者读取整份装备背包存档。
	if err := tx.Table("p13_union_fight_field_players AS a").Select("a.*,m.guild_id AS current_guild_id,m.role AS current_role,p.session AS profile_session,"+sceneExpression+" AS actual_scene").Joins("JOIN profiles AS p ON p.id=a.player_id").Joins("JOIN p13_guild_members AS m ON m.player_id=a.player_id").Where("a.match_id=? AND a.player_id<>? AND a.left_at=0 AND a.session_hash<>?", scope, exclude, "").Order("a.player_id").Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	snapshot, projectionErr := p13UFFieldStoredProjection(tx, scope)
	if projectionErr != nil {
		return nil, nil, projectionErr
	}
	var actors []db.P13UnionFightFieldPlayer
	var frames []*pb.InstUFPlayer
	now := p13UFNow(tx.Statement.Context)
	for _, row := range rows {
		value, ok := p10ClientSinks.Load(p10ClientKey{s.p10ClientOwner(), row.PlayerID})
		if !ok || row.CurrentGuildID != row.GuildID || row.ActualScene != constUtil.P13UnionFightFieldTarget || row.SessionHash != p6Digest(row.ProfileSession) || !bytes.Equal(row.ProfileSession, value.(*p10ClientSink).session) {
			continue
		}
		if err := p13UFFieldActorProjection(&row.P13UnionFightFieldPlayer, snapshot); err != nil {
			return nil, nil, err
		}
		frame, err := p13UFFieldReadActor(&row.P13UnionFightFieldPlayer)
		if err != nil {
			return nil, nil, err
		}
		// Peer views derive the same remaining interval as the owner's Current.
		// No persisted old5s frame is rewritten or its countdown restarted.
		if err = p13UFPlayerBattleProject(tx, &row.P13UnionFightFieldPlayer, frame, now); err != nil {
			return nil, nil, err
		}
		frame.UnionAuthority = proto.Int32(min(frame.GetUnionAuthority(), row.CurrentRole))
		actors = append(actors, row.P13UnionFightFieldPlayer)
		frames = append(frames, frame)
	}
	return actors, frames, nil
}

func (s *GameService) p13UFFieldBroadcast(ctx context.Context, scope string, exclude int64, message proto.Message) {
	rows, _, err := s.p13UFFieldPeers(db.GetIns().WithContext(ctx), scope, exclude)
	if err != nil {
		return
	}
	for _, row := range rows {
		s.publishP10Private(ctx, row.PlayerID, message)
	}
}

// SyncP13UnionFightField 只同步同MatchID的真实已认证TCP，公共世界开关不改变战场可见范围。
func (s *GameService) SyncP13UnionFightField(ctx context.Context, id int64, movement bool) (bool, error) {
	var profile db.Profile
	err := db.GetIns().WithContext(ctx).First(&profile, id).Error
	if err != nil || profile.Game == nil {
		return false, nil
	}
	_, game, err := decodeProfile(&profile)
	if err != nil {
		return false, err
	}
	if game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
		return false, nil
	}
	value, ok := p10ClientSinks.Load(p10ClientKey{s.p10ClientOwner(), id})
	if !ok {
		return true, nil
	}
	sink := value.(*p10ClientSink)
	if _, err = AuthorizeAccount(ctx, id, sink.session); err != nil {
		return true, err
	}
	var current *db.P13UnionFightFieldPlayer
	var frame *pb.InstUFPlayer
	changed := false
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, _, _, live, err := s.p13UFFieldCurrent(tx, id, p13UFNow(ctx))
		if err != nil {
			return err
		}
		changed = actor.SessionHash != p6Digest(sink.session)
		actor.SessionHash = p6Digest(sink.session)
		updates := map[string]any{"session_hash": actor.SessionHash}
		if changed {
			actor.UpdatedAt = p13UFNow(ctx).UnixMilli()
			updates["updated_at"] = actor.UpdatedAt
		}
		if err = tx.Model(actor).Updates(updates).Error; err != nil {
			return err
		}
		current = actor
		frame = live
		return nil
	})
	if err != nil {
		return true, err
	}
	rows, frames, err := s.p13UFFieldPeers(db.GetIns().WithContext(ctx), current.MatchID, id)
	if err != nil {
		return true, err
	}
	if changed || !movement {
		s.p13UFFieldBroadcast(ctx, current.MatchID, id, &pb.SCUFPlayerAppear{Player: []*pb.InstUFPlayer{frame}, Local: []*pb.PointNew{{X: proto.Float32(current.X), Y: proto.Float32(current.Y), Z: proto.Float32(current.Z)}}})
		if len(frames) > 0 {
			positions := make([]*pb.PointNew, 0, len(rows))
			for _, row := range rows {
				positions = append(positions, &pb.PointNew{X: proto.Float32(row.X), Y: proto.Float32(row.Y), Z: proto.Float32(row.Z)})
			}
			s.publishP10Private(ctx, id, &pb.SCUFPlayer{Player: frames, Local: positions})
		}
	}
	return true, nil
}

// DisconnectP13UnionFightField 旧会话不能清除新连接的战场在线状态，持久HP/AP与位置继续保留。
func (s *GameService) DisconnectP13UnionFightField(ctx context.Context, id int64, session []byte) bool {
	var actor db.P13UnionFightFieldPlayer
	err := db.GetIns().WithContext(ctx).First(&actor, "player_id=? AND left_at=0 AND session_hash=?", id, p6Digest(session)).Error
	if err != nil {
		return false
	}
	if err = db.GetIns().WithContext(ctx).Model(&actor).Where("session_hash=?", p6Digest(session)).Update("session_hash", "").Error; err != nil {
		return true
	}
	s.p13UFFieldBroadcast(ctx, actor.MatchID, id, &pb.SCUFPlayerDisappear{Guid: []int64{id}})
	return true
}

func (s *GameService) p13UFFieldHook(ctx context.Context, id int64, request *pb.CSUFOnHook, now time.Time) ([]proto.Message, error) {
	var canceledGUID int64
	var scope string
	var auraOff []int64
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, _, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		if now.UnixMilli() >= match.EndAt {
			return errors.New("原本场已结束，不能改变挂机状态")
		}
		if frame.GetState() == 4 || frame.GetState() == 12 {
			return errors.New("原传送尚未完成，不能改变挂机状态")
		}
		if request.GetOnHook() == 1 {
			scope = actor.MatchID
			auraOff, err = p13UFHealAuraCancel(tx, actor.MatchID, id, now)
			if err != nil {
				return err
			}
			var canceled bool
			canceledGUID, canceled, err = s.p13UFResourceCancel(tx, actor.MatchID, id, now)
			if err != nil {
				return err
			}
			if canceled {
				scope = actor.MatchID
			}
		}
		actor.Hooked = request.GetOnHook() == 1
		return tx.Model(actor).Update("hooked", actor.Hooked).Error
	})
	if err != nil {
		return nil, err
	}
	var response []proto.Message
	if len(auraOff) > 0 {
		off := &pb.SCUFElementBuffOff{BuffGuid: auraOff}
		s.p13UFFieldBroadcast(ctx, scope, id, off)
		response = append(response, off)
	}
	if canceledGUID > 0 {
		ended := &pb.SCUFPlayerCollectEnd{PlayerGuid: []int64{id}, ResGuid: []int64{canceledGUID}, IsResExhausted: []int32{0}}
		s.p13UFFieldBroadcast(ctx, scope, id, ended)
		response = append(response, ended)
	}
	return append(response, &pb.SCUFOnHook{OnHook: proto.Int32(request.GetOnHook())}), nil
}

func (s *GameService) p13UFFieldElements(ctx context.Context, id int64, guids []int64, now time.Time) ([]proto.Message, error) {
	var messages []proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, snapshot, self, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		buildings := &pb.SCUFBuildingRefresh{}
		if proto.Unmarshal(snapshot.Buildings, buildings) != nil {
			return errors.New("公会战原建筑快照损坏")
		}
		if err = p13UFRevivePointApply(tx, match.ID, buildings); err != nil {
			return err
		}
		if err = s.p13UFFactoryOverlay(tx, match.ID, buildings); err != nil {
			return err
		}
		if err = p13UFGateItemOverlay(tx, match.ID, buildings); err != nil {
			return err
		}
		wanted := map[int64]bool{}
		for _, guid := range guids {
			wanted[guid] = true
		}
		selected := func(guid int64) bool { return len(wanted) == 0 || wanted[guid] }
		playerFrames := &pb.SCUFPlayer{}
		if selected(id) {
			playerFrames.Player = append(playerFrames.Player, self)
			playerFrames.Local = append(playerFrames.Local, &pb.PointNew{X: proto.Float32(actor.X), Y: proto.Float32(actor.Y), Z: proto.Float32(actor.Z)})
		}
		rows, frames, err := s.p13UFFieldPeers(tx, actor.MatchID, id)
		if err != nil {
			return err
		}
		for i, other := range rows {
			if selected(other.PlayerID) {
				playerFrames.Player = append(playerFrames.Player, frames[i])
				playerFrames.Local = append(playerFrames.Local, &pb.PointNew{X: proto.Float32(other.X), Y: proto.Float32(other.Y), Z: proto.Float32(other.Z)})
			}
		}
		if len(playerFrames.Player) > 0 {
			messages = append(messages, playerFrames)
		}
		base := &pb.SCUFBaseCamp{}
		for _, unit := range buildings.BaseCampInfos {
			if selected(unit.GetGuid()) {
				base.BaseCamp = append(base.BaseCamp, unit)
			} else if unit.GetGuard() != nil && selected(unit.GetGuard().GetGuid()) {
				base.BaseCamp = append(base.BaseCamp, unit)
			}
		}
		if len(base.BaseCamp) > 0 {
			messages = append(messages, base)
		}
		gates := &pb.SCUFGate{}
		for _, unit := range buildings.GateInfos {
			if selected(unit.GetGuid()) {
				gates.Gate = append(gates.Gate, unit)
			}
		}
		if len(gates.Gate) > 0 {
			messages = append(messages, gates)
		}
		towers := &pb.SCUFTower{}
		for _, unit := range buildings.TowerInfos {
			if selected(unit.GetGuid()) {
				towers.Tower = append(towers.Tower, unit)
			} else if unit.GetGuard() != nil && selected(unit.GetGuard().GetGuid()) {
				towers.Tower = append(towers.Tower, unit)
			}
		}
		if len(towers.Tower) > 0 {
			messages = append(messages, towers)
		}
		for _, unit := range buildings.FactoryInfos {
			if selected(unit.GetGuid()) || unit.GetGuard() != nil && selected(unit.GetGuard().GetGuid()) {
				messages = append(messages, &pb.SCUFCariotFactory{Factory: unit})
			}
		}
		points := &pb.SCUFRevivePoint{}
		for _, unit := range buildings.PointInfos {
			if selected(unit.GetGuid()) {
				points.Point = append(points.Point, unit)
			}
		}
		if len(points.Point) > 0 {
			messages = append(messages, points)
		}
		nodes, err := s.p13UFResourceFrames(tx, actor.MatchID, wanted)
		if err != nil {
			return err
		}
		messages = append(messages, nodes...)
		cars, err := s.p13UFFactoryCars(tx, match, wanted)
		if err != nil {
			return err
		}
		messages = append(messages, cars...)
		drumFrames, drumErr := p13UFDrumFrames(tx, match.ID, wanted, now)
		if drumErr != nil {
			return drumErr
		}
		messages = append(messages, drumFrames...)
		if len(messages) == 0 {
			return errors.New("本实例不存在请求的原实体GUID")
		}
		return nil
	})
	return messages, err
}

// TickP13UnionFightField 真实时间只推进场景Ready/Fight/End并原4秒退场，不写默认胜负或奖励。
func (s *GameService) TickP13UnionFightField(ctx context.Context, now time.Time) error {
	var failures []error
	for _, maintain := range []func(context.Context, time.Time) error{
		s.AdvanceP13UnionFightPlayerBattles,
		s.TickP13UnionFightTransfer,
		s.TickP13UnionFightResource,
		s.TickP13UnionFightFactory,
		s.TickP13UnionFightCarMotion,
		s.TickP13UnionFightCarAttack,
		s.TickP13UnionFightSpeedItem,
		s.TickP13UnionFightHealAura,
		s.TickP13UnionFightWarDrum,
		s.TickP13UnionFightRevivePoint,
		s.TickP13UnionFightFocus,
	} {
		if err := maintain(ctx, now); err != nil {
			failures = append(failures, err)
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
	}
	if err := s.p13UFFieldLifecycle(ctx, now); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

// p13UFFieldLifecycle runs even when an unrelated pending calculation is damaged.
func (s *GameService) p13UFFieldLifecycle(ctx context.Context, now time.Time) error {
	tx := db.GetIns().WithContext(ctx)
	var policy db.P13UnionFightFieldPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return err
	}
	var rows []db.P13UnionFightFieldPlayer
	if err := tx.Where("left_at=0").Order("player_id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		var match db.P13UnionFightMatch
		if err := tx.First(&match, "id=?", row.MatchID).Error; err != nil {
			return err
		}
		var saved db.P13UnionFightField
		if err := tx.First(&saved, "match_id=?", row.MatchID).Error; err != nil {
			return err
		}
		if p6Digest(saved.Snapshot) != saved.SnapshotHash {
			return errors.New("公会战生命周期原地图快照损坏")
		}
		var snapshot p13UFFieldSnapshot
		if err := json.Unmarshal(saved.Snapshot, &snapshot); err != nil {
			return err
		}
		if err := p13UFFieldActorProjection(&row, &snapshot); err != nil {
			return err
		}
		member, err := p13GuildMember(tx, row.PlayerID)
		if err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err = tx.First(&gate, 1).Error; err != nil {
			return err
		}
		var enrollment db.P13UnionFightPolicy
		if err = tx.First(&enrollment, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled || !gate.ManagementEnabled || !enrollment.EnrollmentEnabled || !enrollment.MatchEnabled || member.GuildID != row.GuildID || now.UnixMilli() >= match.EndAt+snapshot.KickMillis {
			_, left, err := s.p13UFFieldLeaveExpected(ctx, row.PlayerID, &pb.CSTransferScene{Target: proto.String("town_main")}, now, row.MatchID)
			if err != nil {
				return err
			}
			if left {
				s.publishP10Private(ctx, row.PlayerID, &pb.SCUFKickPlayer{})
			}
			continue
		}
		state, left, total := p13UFFieldState(&match, now)
		if state == 1 {
			total = snapshot.PreparationMillis
		}
		s.publishP10Private(ctx, row.PlayerID, &pb.SCUnionFightUpdateState{UnionFightId: proto.String(match.ID), State: proto.Int32(state), LeftTime: proto.Int64(left), TotalTime: proto.Int64(total)})
	}
	return nil
}

// ServeP13UnionFightField 绑定服务ctx每秒检查生命周期，关闭服务时保留同场持久状态供恢复。
func (s *GameService) ServeP13UnionFightField(ctx context.Context) error {
	if err := s.TickP13UnionFightField(ctx, p13UFNow(ctx)); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	ticker := time.NewTicker(constUtil.P13UnionFightFieldTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.TickP13UnionFightField(ctx, p13UFNow(ctx)); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// DispatchP13UnionFightField 对原共享转场/位置/移动只处理实际fight_union，不接收客户端指定房间身份。
func (s *GameService) DispatchP13UnionFightField(ctx context.Context, id int64, request proto.Message) ([]proto.Message, bool, error) {
	now := p13UFNow(ctx)
	if _, ok := request.(*pb.CSUFChatCache); ok {
		messages, err := s.p13UFChatHistory(ctx, id, now)
		return messages, true, err
	}
	if transfer, ok := request.(*pb.CSTransferScene); ok && transfer.GetTarget() == constUtil.P13UnionFightFieldTarget {
		messages, err := s.p13UFFieldEnter(ctx, id, now)
		if err == nil {
			_, err = s.SyncP13UnionFightField(ctx, id, false)
		}
		return messages, true, err
	}
	switch request.(type) {
	case *pb.CSTransferScene, *pb.CSCurrentPosition, *pb.CSLoginScene, *pb.CSMove, *pb.CSMoveConfirm, *pb.CSUFInit, *pb.CSUFChatCache, *pb.CSUFOnHook, *pb.CSUFRefreshElement, *pb.CSUFRefreshElementArray:
	default:
		return nil, false, nil
	}
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, true, err
	}
	_, game, err := decodeProfile(&row)
	if err != nil {
		return nil, true, err
	}
	if game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
		switch request.(type) {
		case *pb.CSUFInit, *pb.CSUFChatCache, *pb.CSUFOnHook, *pb.CSUFRefreshElement, *pb.CSUFRefreshElementArray:
			return nil, true, errors.New("角色实际位置不在公会战实例")
		}
		return nil, false, nil
	}
	var messages []proto.Message
	switch body := request.(type) {
	case *pb.CSTransferScene:
		messages, err = s.p13UFFieldLeave(ctx, id, body, now)
	case *pb.CSCurrentPosition, *pb.CSLoginScene, *pb.CSUFInit, *pb.CSUFChatCache:
		messages, err = s.p13UFFieldFrames(ctx, id, request, now)
	case *pb.CSMove:
		messages, err = s.p13UFFieldMove(ctx, id, body, now)
	case *pb.CSMoveConfirm:
		messages, err = s.p13UFFieldFrames(ctx, id, &pb.CSUFInit{}, now)
	case *pb.CSUFOnHook:
		messages, err = s.p13UFFieldHook(ctx, id, body, now)
	case *pb.CSUFRefreshElement:
		messages, err = s.p13UFFieldElements(ctx, id, []int64{body.GetGuid()}, now)
	case *pb.CSUFRefreshElementArray:
		messages, err = s.p13UFFieldElements(ctx, id, body.GetGuid(), now)
	}
	return messages, true, err
}

// P13UnionFightFieldRevoke 配合现会员变更同事务保存退场，变更完成后的推送由After方法执行。
func (s *GameService) P13UnionFightFieldRevoke(tx *gorm.DB, id int64, now time.Time) error {
	var row db.Profile
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
		return err
	}
	p, g, err := decodeProfileForCalendarUpdate(tx, &row)
	if err != nil {
		return err
	}
	changed, err := s.P13UnionFightFieldNormalize(tx, id, p, g, now)
	if err != nil {
		return err
	}
	if changed {
		return saveProfile(tx, &row, p, g)
	}
	return nil
}

func (s *GameService) AfterP13UnionFightFieldRevoke(ctx context.Context, ids []int64, now time.Time) {
	for _, id := range ids {
		var actor db.P13UnionFightFieldPlayer
		// 只投递本次会员事务关闭的实例，历史战场结束不能令现在的主城/领地玩家收到旧Kick。
		if db.GetIns().WithContext(ctx).Where("player_id=? AND left_at=?", id, now.UnixMilli()).Order("left_at DESC").First(&actor).Error != nil {
			continue
		}
		s.p13UFFieldBroadcast(ctx, actor.MatchID, id, &pb.SCUFPlayerDisappear{Guid: []int64{id}})
		var revokedDrums []db.P13UnionFightWarDrumEffect
		if db.GetIns().WithContext(ctx).Where("match_id=? AND caster_id=? AND ended_at>0", actor.MatchID, id).Order("guid").Find(&revokedDrums).Error == nil {
			for _, drum := range revokedDrums {
				s.p13UFFieldBroadcast(ctx, actor.MatchID, id, &pb.SCUFTotemDestroy{Guid: proto.Int64(drum.GUID)})
			}
		}
		var revokedSpeed []db.P13UnionFightSpeedItemEffect
		var revokedAura []db.P13UnionFightHealAuraEffect
		var off []int64
		if db.GetIns().WithContext(ctx).Where("match_id=? AND (caster_id=? OR target_id=?) AND ended_at=?", actor.MatchID, id, id, now.UnixMilli()).Order("guid").Find(&revokedSpeed).Error == nil {
			for _, effect := range revokedSpeed {
				off = append(off, effect.GUID)
			}
		}
		if db.GetIns().WithContext(ctx).Where("match_id=? AND caster_id=? AND ended_at=?", actor.MatchID, id, now.UnixMilli()).Order("guid").Find(&revokedAura).Error == nil {
			for _, effect := range revokedAura {
				off = append(off, effect.GUID)
			}
		}
		if len(off) > 0 {
			s.p13UFFieldBroadcast(ctx, actor.MatchID, id, &pb.SCUFElementBuffOff{BuffGuid: off})
		}
		s.publishP10Private(ctx, id, &pb.SCUFKickPlayer{})
	}
}
