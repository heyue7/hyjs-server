package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeP13UnionFightTower creates no publication and enables no attacks.
func (s *GameService) InitializeP13UnionFightTower(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightTowerSource{}, &db.P13UnionFightTowerState{}, &db.P13UnionFightTowerHit{})
}
func p13UFTowerError() error { return errors.New(constUtil.P13UnionFightTowerConflict) }
func p13UFTowerSourceHash(row *db.P13UnionFightTowerState) string {
	copy := *row
	copy.SourceHash, copy.StateHash = "", ""
	copy.Active = false
	copy.NextFireAt, copy.PendingReleaseAt, copy.PendingTargetGUID, copy.Sequence, copy.LastResolvedAt = 0, 0, 0, 0, 0
	copy.PendingTargetKind = ""
	p13UFTowerClearLaunch(&copy)
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFTowerStateHash(row *db.P13UnionFightTowerState) string {
	copy := *row
	copy.StateHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFTowerHitHash(row *db.P13UnionFightTowerHit) string {
	copy := *row
	copy.HitHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFTowerValid(row *db.P13UnionFightTowerState) bool {
	if row == nil || row.MatchID == "" || row.TowerID < 1 || row.TowerID > 6 || row.TowerGUID != p13UFFieldGUID(row.MatchID, "tower", strconv.FormatInt(row.TowerID, 10)) || row.GuardGUID <= 0 || (row.Camp != 0 && row.Camp != 1) || !combatFinite(float64(row.X)) || !combatFinite(float64(row.Y)) || !combatFinite(float64(row.Z)) || row.Range != 1.4 || (row.PlayerDamageRule != "" && (row.PlayerDamageRule != constUtil.P13UnionFightTowerPlayerDamageRule || row.PlayerAttack != 65 || row.HPPercent != 5) || row.PlayerDamageRule == "" && row.PlayerAttack != 0) || row.HPPercent < 1 || row.HPPercent > 100 || row.CooldownMillis != constUtil.P13UnionFightTowerCooldownMillis || row.ReleaseMillis != constUtil.P13UnionFightTowerReleaseMillis || row.AnimationMillis != constUtil.P13UnionFightTowerAnimationMillis || row.ReviveSeconds != constUtil.P13UnionFightTowerReviveSeconds || len(row.MatchHash) != 64 || strings.TrimSpace(row.Version) == "" || strings.TrimSpace(row.Source) == "" || row.PublishedAt <= 0 || row.SourceHash != p13UFTowerSourceHash(row) || row.StateHash != p13UFTowerStateHash(row) || row.NextFireAt <= 0 || row.Sequence < 0 || row.LastResolvedAt < 0 {
		return false
	}
	if !p13UFTowerLaunchValid(row) {
		return false
	}
	if row.PendingReleaseAt == 0 {
		return row.PendingTargetGUID == 0 && row.PendingTargetKind == ""
	}
	return row.PendingReleaseAt > 0 && row.PendingTargetGUID > 0 && row.Sequence > 0 && (row.PendingTargetKind == "player" || row.PendingTargetKind == "car") && row.NextFireAt == row.PendingReleaseAt
}

// p13UFTowerBuild freezes the original six entities and only pre-start named damage.
// The field's projection is reused, so flat Re maps do not regain original Y.
func (s *GameService) p13UFTowerBuild(tx *gorm.DB, match *db.P13UnionFightMatch, buildings *pb.SCUFBuildingRefresh, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightTowerSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Order("tower_id").Limit(7).Find(&sources).Error; err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}
	if len(sources) > 6 || match.StartAt <= 0 || len(match.SnapshotHash) != 64 {
		return p13UFTowerError()
	}
	if original.table("GameConfigure", "StaticName", "UnionFightReviveTime")["NameValue"] != constUtil.P13UnionFightTowerReviveSeconds {
		return p13UFTowerError()
	}
	towers := map[int64]*pb.InstUFTower{}
	for _, tower := range buildings.TowerInfos {
		if towers[int64(tower.GetId())] != nil {
			return p13UFTowerError()
		}
		towers[int64(tower.GetId())] = tower
	}
	rows := make([]db.P13UnionFightTowerState, 0, len(sources))
	for _, source := range sources {
		tower := towers[source.TowerID]
		if tower == nil || source.TowerID < 1 || source.TowerID > 6 || source.HPPercent != 5 || source.PlayerAttack != 65 || source.PlayerDamageRule != constUtil.P13UnionFightTowerPlayerDamageRule || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || source.PublishedAt >= match.StartAt || strings.TrimSpace(source.Version) == "" || strings.TrimSpace(source.Source) == "" {
			return p13UFTowerError()
		}
		cfg := original.table("UnionFightTower", "UFTId", strconv.FormatInt(source.TowerID, 10))
		guard := tower.GetGuard()
		if cfg == nil || cfg["Range"] != "1.4" || cfg["AttackCountDown"] != "5" || cfg["AttackDuration"] != "1" || cfg["AttackRelease"] != "0.6" || guard == nil || guard.GetUFGuardId() != cfg["UFGuardId"] || strconv.FormatInt(int64(guard.GetCamp()), 10) != cfg["Camp"] {
			return p13UFTowerError()
		}
		point, err := fieldPoint(cfg["Position"])
		if err != nil {
			return err
		}
		if matchProjection := guard.GetInitPosition(); matchProjection != nil && matchProjection.GetY() == 0 {
			point.Y = proto.Float32(0)
		}
		row := db.P13UnionFightTowerState{MatchID: match.ID, TowerGUID: tower.GetGuid(), TowerID: source.TowerID, GuardGUID: guard.GetGuid(), Camp: int64(guard.GetCamp()), X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), Range: 1.4, HPPercent: source.HPPercent, PlayerAttack: source.PlayerAttack, PlayerDamageRule: source.PlayerDamageRule, CooldownMillis: 5000, ReleaseMillis: 600, AnimationMillis: 1000, ReviveSeconds: constUtil.P13UnionFightTowerReviveSeconds, MatchHash: match.SnapshotHash, Version: source.Version, PublishedAt: source.PublishedAt, Source: constUtil.P13UnionFightTowerSource + ";publication=" + source.Source, Active: tower.GetState() == 0, NextFireAt: match.StartAt}
		row.SourceHash = p13UFTowerSourceHash(&row)
		row.StateHash = p13UFTowerStateHash(&row)
		if !p13UFTowerValid(&row) {
			return p13UFTowerError()
		}
		rows = append(rows, row)
	}
	return tx.Create(&rows).Error
}

// p13UFTowerDamagePlayer preserves the legacy v1 percent proof byte contract.
// New publications use the single bound actor physical rule below.
func p13UFTowerDamagePlayer(before *pb.InstUFPlayer, percent int64) (*pb.InstUFPlayer, int64, error) {
	if before == nil || percent < 1 || percent > 100 || (before.GetState() != 0 && before.GetState() != 2 && before.GetState() != 5) || before.GetDeadNum() < 0 || before.GetDeadNum() == math.MaxInt32 {
		return nil, 0, p13UFTowerError()
	}
	after := proto.Clone(before).(*pb.InstUFPlayer)
	fight := after.GetFightInfo()
	if fight == nil || fight.GetGuid() != before.GetGuid() || len(fight.Index) < 1 || len(fight.Index) > 4 || len(fight.Index) != len(fight.HpPos) || len(fight.Index) != len(fight.MaxHpPos) {
		return nil, 0, p13UFTowerError()
	}
	seen := map[int32]bool{}
	alive, remaining := false, false
	for i, pose := range fight.Index {
		hp, maxHP := fight.HpPos[i], fight.MaxHpPos[i]
		if seen[pose] || pose < 0 || !combatFinite(hp) || !combatFinite(maxHP) || maxHP <= 0 || hp < 0 || hp > maxHP {
			return nil, 0, p13UFTowerError()
		}
		seen[pose] = true
		if hp > 0 {
			alive = true
			fight.HpPos[i] = math.Max(0, hp-maxHP*float64(percent)/100)
		}
		remaining = remaining || fight.HpPos[i] > 0
	}
	if !alive {
		return nil, 0, p13UFTowerError()
	}
	if remaining {
		return after, 0, nil
	}
	after.DeadNum = proto.Int32(after.GetDeadNum() + 1)
	revive := int64(min(int(after.GetDeadNum()), 8) * 5)
	after.State = proto.Int32(3)
	after.LeftTime = proto.Float32(float32(revive))
	after.TotalTime = proto.Float32(float32(revive))
	return after, revive, nil
}
func p13UFTowerDamageCar(before *pb.InstUFCariot, percent int64) (*pb.InstUFCariot, error) {
	if before == nil || percent < 1 || percent > 100 || !combatFinite(before.GetHp()) || !combatFinite(before.GetMaxHp()) || before.GetMaxHp() <= 0 || before.GetHp() <= 0 || before.GetHp() > before.GetMaxHp() {
		return nil, p13UFTowerError()
	}
	after := proto.Clone(before).(*pb.InstUFCariot)
	after.Hp = proto.Float64(math.Max(0, before.GetHp()-before.GetMaxHp()*float64(percent)/100))
	return after, nil
}

const p13UFTowerCancelWorld = "world-terminal-cutoff"

// Cutoff cancellations preserve launch evidence but have no impact observation
// and no HP effect. The mixed ledger also binds CutoffAt to its actual terminal.
func p13UFTowerCutoffVerify(row *db.P13UnionFightTowerHit, tower *db.P13UnionFightTowerState) error {
	if row != nil && row.OccupancyID != "" {
		if row.Status != constUtil.P13UnionFightTowerHitCanceled || row.CancelReason != p13UFTowerCancelWorld || row.CutoffAt <= 0 || (row.HitAt <= row.CutoffAt && row.DueAt <= row.CutoffAt) || row.AppliedAt < row.HitAt || row.HitHash != p13UFTowerHitHash(row) {
			return p13UFTowerError()
		}
		pending := *row
		pending.Status = constUtil.P13UnionFightTowerHitPending
		pending.CancelReason = ""
		pending.CutoffAt = 0
		pending.AppliedAt = 0
		pending.HitHash = p13UFTowerHitHash(&pending)
		return p13UFTowerPhysicalHitVerify(&pending, tower)
	}
	if row == nil || tower == nil || !p13UFTowerValid(tower) || row.Status != constUtil.P13UnionFightTowerHitCanceled || row.CancelReason != p13UFTowerCancelWorld || row.CutoffAt <= 0 || row.HitAt <= row.CutoffAt || row.DueAt != row.HitAt || row.AppliedAt < row.HitAt || row.MatchID != tower.MatchID || row.TowerGUID != tower.TowerGUID || row.TowerSourceHash != tower.SourceHash || row.TargetGUID <= 0 || row.Sequence < 1 || row.Sequence > tower.Sequence || row.ImmutableHash != p13UFTowerImmutableHash(row) || row.HitHash != p13UFTowerHitHash(row) || len(row.Before) != 0 || len(row.After) != 0 || row.BeforeHash != "" || row.AfterHash != "" || row.DeadPlayerID != 0 || row.ReviveSeconds != 0 || row.ReviveAt != 0 || len(row.ReleaseFrame) != 0 || row.ReleaseFrameHash != "" || len(row.CancelFrame) != 0 || row.CancelFrameHash != "" || row.CancelLeftAt != 0 || row.CoalescedInto != "" || row.Damage != 0 || row.OccupancyID != "" || row.OccupancyKind != "" || row.OccupancyInputHash != "" || row.OccupancyDueAt != 0 {
		return p13UFTowerError()
	}
	if row.TargetKind != "car" && row.TargetKind != "player" {
		return p13UFTowerError()
	}
	if tower.PlayerDamageRule != "" && row.TargetKind == "player" {
		var hero pb.BattleHero
		var capture pb.InstUFPlayer
		if row.FrozenActorHash != p6Digest(row.FrozenActor) || row.CaptureFrameHash != p6Digest(row.CaptureFrame) || proto.Unmarshal(row.FrozenActor, &hero) != nil || proto.Unmarshal(row.CaptureFrame, &capture) != nil || hero.GetGuid() != row.ActorGUID || int64(hero.GetPose()) != row.ActorPose || capture.GetGuid() != row.TargetGUID || int64(capture.GetDeadNum()) != row.LifeOrdinal {
			return p13UFTowerError()
		}
		if row.LaunchOccupancyID != "" {
			if capture.GetState() != 1 || len(row.LaunchOccupancyID) != 64 || len(row.LaunchOccupancyInputHash) != 64 || row.LaunchOccupancyDueAt <= 0 || (row.LaunchOccupancyKind != constUtil.P13UnionFightTowerOccupancyOrdinary && row.LaunchOccupancyKind != constUtil.P13UnionFightTowerOccupancyGuard) {
				return p13UFTowerError()
			}
		} else if capture.GetState() == 1 || row.LaunchOccupancyKind != "" || row.LaunchOccupancyInputHash != "" || row.LaunchOccupancyDueAt != 0 {
			return p13UFTowerError()
		}
	} else if len(row.FrozenActor) != 0 || row.FrozenActorHash != "" || len(row.CaptureFrame) != 0 || row.CaptureFrameHash != "" || row.ActorGUID != "" || row.ActorPose != 0 || row.LifeOrdinal != 0 || row.LaunchOccupancyKind != "" || row.LaunchOccupancyID != "" || row.LaunchOccupancyInputHash != "" || row.LaunchOccupancyDueAt != 0 {
		return p13UFTowerError()
	}
	return nil
}

// p13UFTowerHitVerify is a pure replay of a small business effect, not P6 payload.
func p13UFTowerHitVerify(row *db.P13UnionFightTowerHit, tower *db.P13UnionFightTowerState) error {
	if row != nil && row.CancelReason == p13UFTowerCancelWorld {
		return p13UFTowerCutoffVerify(row, tower)
	}
	if row != nil && row.CutoffAt != 0 {
		return p13UFTowerError()
	}
	if row != nil && row.Status != "" {
		return p13UFTowerPhysicalHitVerify(row, tower)
	}
	if row != nil && (len(row.ReleaseFrame) != 0 || row.ReleaseFrameHash != "" || row.LaunchOccupancyKind != "" || row.LaunchOccupancyID != "" || row.LaunchOccupancyInputHash != "" || row.LaunchOccupancyDueAt != 0 || row.HitAt != 0 || row.ActorGUID != "" || row.ActorPose != 0 || row.LifeOrdinal != 0 || row.Damage != 0 || row.OccupancyKind != "" || row.OccupancyID != "" || row.OccupancyInputHash != "" || row.OccupancyDueAt != 0 || row.ImmutableHash != "" || len(row.FrozenActor) != 0 || row.FrozenActorHash != "" || len(row.CaptureFrame) != 0 || row.CaptureFrameHash != "" || row.CancelReason != "" || len(row.CancelFrame) != 0 || row.CancelFrameHash != "" || row.CoalescedInto != "" || row.CancelLeftAt != 0) {
		return p13UFTowerError()
	}
	if tower != nil && tower.PlayerDamageRule != "" {
		return p13UFTowerError()
	}
	return p13UFTowerLegacyHitVerify(row, tower)
}
func p13UFTowerLegacyHitVerify(row *db.P13UnionFightTowerHit, tower *db.P13UnionFightTowerState) error {
	if !p13UFTowerValid(tower) || row == nil || row.MatchID != tower.MatchID || row.TowerGUID != tower.TowerGUID || row.Sequence < 1 || row.Sequence > tower.Sequence || row.TowerSourceHash != tower.SourceHash || row.BeforeHash != p6Digest(row.Before) || row.AfterHash != p6Digest(row.After) || row.HitHash != p13UFTowerHitHash(row) || row.DueAt <= 0 || row.AppliedAt < row.DueAt || !p13UFTowerInRange(tower, row.X, row.Y, row.Z) {
		return p13UFTowerError()
	}
	switch row.TargetKind {
	case "player":
		before, after := &pb.InstUFPlayer{}, &pb.InstUFPlayer{}
		if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil || before.GetGuid() != row.TargetGUID || before.GetCamp() == int32(tower.Camp) {
			return p13UFTowerError()
		}
		expected, revive, err := p13UFTowerDamagePlayer(before, tower.HPPercent)
		if err != nil {
			return p13UFTowerError()
		}
		if revive > 0 {
			expected.LeftTime = proto.Float32(float32(max(int64(0), row.DueAt+revive*1000-row.AppliedAt)) / 1000)
		}
		if !proto.Equal(expected, after) || row.ReviveSeconds != revive {
			return p13UFTowerError()
		}
		if revive == 0 {
			if row.DeadPlayerID != 0 || row.ReviveAt != 0 {
				return p13UFTowerError()
			}
		} else if row.DeadPlayerID != row.TargetGUID || row.ReviveAt != row.DueAt+revive*1000 {
			return p13UFTowerError()
		}
	case "car":
		before, after := &pb.InstUFCariot{}, &pb.InstUFCariot{}
		if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil || before.GetGuid() != row.TargetGUID || before.GetCamp() == int32(tower.Camp) || row.DeadPlayerID != 0 || row.ReviveSeconds != 0 || row.ReviveAt != 0 {
			return p13UFTowerError()
		}
		expected, err := p13UFTowerDamageCar(before, tower.HPPercent)
		if err != nil || !proto.Equal(expected, after) {
			return p13UFTowerError()
		}
	default:
		return p13UFTowerError()
	}
	return nil
}
func p13UFTowerInRange(tower *db.P13UnionFightTowerState, x, y, z float32) bool {
	dx, dy, dz := float64(x-tower.X), float64(y-tower.Y), float64(z-tower.Z)
	d := dx*dx + dy*dy + dz*dz
	return combatFinite(d) && d <= tower.Range*tower.Range
}

type p13UFTowerTarget struct {
	kind                                           string
	guid, camp                                     int64
	x, y, z                                        float32
	player                                         *db.P13UnionFightFieldPlayer
	frame                                          *pb.InstUFPlayer
	factory                                        *db.P13UnionFightFactoryState
	car                                            *pb.InstUFCariot
	generation                                     *db.P13UnionFightCarGeneration
	occupancyKind, occupancyID, occupancyInputHash string
	occupancyDueAt                                 int64
	occupancyShots                                 map[int64]string
}

func p13UFTowerSelect(tower *db.P13UnionFightTowerState, targets []*p13UFTowerTarget) *p13UFTowerTarget {
	var chosen *p13UFTowerTarget
	best := math.Inf(1)
	for _, target := range targets {
		if target.camp == tower.Camp || !p13UFTowerInRange(tower, target.x, target.y, target.z) {
			continue
		}
		if target.kind == "player" && (target.frame == nil || target.player.LeftAt != 0 || (target.frame.GetState() != 0 && target.frame.GetState() != 2 && target.frame.GetState() != 5 && !(target.frame.GetState() == 1 && tower.PlayerDamageRule == constUtil.P13UnionFightTowerPlayerDamageRule))) {
			continue
		}
		if target.kind == "car" && (target.car == nil || target.car.GetHp() <= 0) {
			continue
		}
		dx, dy, dz := float64(target.x-tower.X), float64(target.y-tower.Y), float64(target.z-tower.Z)
		distance := dx*dx + dy*dy + dz*dz
		if chosen == nil || target.kind == "car" && chosen.kind != "car" || target.kind == chosen.kind && (distance < best || distance == best && target.guid < chosen.guid) {
			chosen = target
			best = distance
		}
	}
	return chosen
}

// p13UFTowerPlayers locks the indexed match directory once and resolves live
// membership/session metadata in one two-JOIN read, never one query per target.
func (s *GameService) p13UFTowerPlayers(tx *gorm.DB, match *db.P13UnionFightMatch) ([]*p13UFTowerTarget, error) {
	var actors []db.P13UnionFightFieldPlayer
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=? AND left_at=0", match.ID).Order("player_id").Limit(constUtil.P13UnionFightTowerPlayerLimit + 1).Find(&actors).Error; err != nil {
		return nil, err
	}
	if len(actors) > constUtil.P13UnionFightTowerPlayerLimit {
		return nil, p13UFTowerError()
	}
	ids := make([]int64, 0, len(actors))
	for _, actor := range actors {
		ids = append(ids, actor.PlayerID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var live []p13UFFieldLiveRow
	scene := "json_extract(p.game,'$.position.scene')"
	if tx.Dialector.Name() == "mysql" {
		scene = "JSON_UNQUOTE(" + scene + ")"
	}
	if err := tx.Table("p13_union_fight_field_players AS a").Select("a.player_id,m.guild_id AS current_guild_id,p.session AS profile_session,"+scene+" AS actual_scene").Joins("JOIN profiles AS p ON p.id=a.player_id").Joins("JOIN p13_guild_members AS m ON m.player_id=a.player_id").Where("a.match_id=? AND a.player_id IN ?", match.ID, ids).Scan(&live).Error; err != nil {
		return nil, err
	}
	directory := map[int64]p13UFFieldLiveRow{}
	for _, row := range live {
		directory[row.PlayerID] = row
	}
	targets := make([]*p13UFTowerTarget, 0, len(actors))
	for i := range actors {
		actor := &actors[i]
		row, ok := directory[actor.PlayerID]
		sink, online := p10ClientSinks.Load(p10ClientKey{s.p10ClientOwner(), actor.PlayerID})
		expectedGuild := match.LeftGuildID
		if actor.Camp == 1 {
			expectedGuild = match.RightGuildID
		}
		if !ok || !online || actor.SessionHash == "" || actor.GuildID != expectedGuild || row.CurrentGuildID != actor.GuildID || row.ActualScene != constUtil.P13UnionFightFieldTarget || actor.SessionHash != p6Digest(row.ProfileSession) || !bytes.Equal(row.ProfileSession, sink.(*p10ClientSink).session) {
			continue
		}
		frame, err := p13UFFieldReadActor(actor)
		if err != nil {
			return nil, err
		}
		targets = append(targets, &p13UFTowerTarget{kind: "player", guid: actor.PlayerID, camp: actor.Camp, x: actor.X, y: actor.Y, z: actor.Z, player: actor, frame: frame})
	}
	return targets, nil
}

// p13UFTowerCars preloads all four factories and bounded generation/state maps.
// It shares the existing car identity, immutable snapshot and mutable SHA rules.
func (s *GameService) p13UFTowerCars(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService) ([]*p13UFTowerTarget, error) {
	var factories []db.P13UnionFightFactoryState
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=?", match.ID).Order("guid").Limit(5).Find(&factories).Error; err != nil {
		return nil, err
	}
	if len(factories) > 4 {
		return nil, p13UFTowerError()
	}
	if len(factories) == 0 {
		return nil, nil
	}
	factoryMap := map[int64]*db.P13UnionFightFactoryState{}
	for i := range factories {
		row := &factories[i]
		if _, err := p13UFFactoryFrame(row); err != nil {
			return nil, err
		}
		factoryMap[row.GUID] = row
	}
	var generations []db.P13UnionFightCarGeneration
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=?", match.ID).Order("car_guid").Limit(constUtil.P13UnionFightTowerCarLimit + 1).Find(&generations).Error; err != nil {
		return nil, err
	}
	if len(generations)+len(factories) > constUtil.P13UnionFightTowerCarLimit {
		return nil, p13UFTowerError()
	}
	var cycles []db.P13UnionFightFactoryCycleOffer
	var motions []db.P13UnionFightCarMotionOffer
	var positions []db.P13UnionFightCarMotionState
	var attacks []db.P13UnionFightCarAttackState
	var attackOffers []db.P13UnionFightCarAttackOffer
	if err := tx.Where("match_id=?", match.ID).Limit(5).Find(&cycles).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("match_id=?", match.ID).Limit(5).Find(&motions).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("match_id=?", match.ID).Limit(constUtil.P13UnionFightTowerCarLimit + 1).Find(&positions).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("match_id=?", match.ID).Limit(constUtil.P13UnionFightTowerCarLimit + 1).Find(&attacks).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("match_id=?", match.ID).Limit(5).Find(&attackOffers).Error; err != nil {
		return nil, err
	}
	if len(cycles) > 4 || len(motions) > 4 || len(attackOffers) > 4 || len(positions) > constUtil.P13UnionFightTowerCarLimit || len(attacks) > constUtil.P13UnionFightTowerCarLimit {
		return nil, p13UFTowerError()
	}
	cycleMap := map[int64]*db.P13UnionFightFactoryCycleOffer{}
	for i := range cycles {
		cycleMap[cycles[i].FactoryGUID] = &cycles[i]
	}
	motionMap := map[int64]*db.P13UnionFightCarMotionOffer{}
	for i := range motions {
		motionMap[motions[i].FactoryGUID] = &motions[i]
	}
	positionMap := map[int64]*db.P13UnionFightCarMotionState{}
	for i := range positions {
		positionMap[positions[i].FactoryGUID] = &positions[i]
	}
	attackMap := map[int64]*db.P13UnionFightCarAttackState{}
	for i := range attacks {
		attackMap[attacks[i].FactoryGUID] = &attacks[i]
	}
	offerMap := map[int64]*db.P13UnionFightCarAttackOffer{}
	for i := range attackOffers {
		offerMap[attackOffers[i].FactoryGUID] = &attackOffers[i]
	}
	candidates := make([]*p13UFTowerTarget, 0, len(factories)+len(generations))
	for i := range factories {
		if factories[i].CompletedAt > 0 {
			candidates = append(candidates, &p13UFTowerTarget{factory: &factories[i]})
		}
	}
	for i := range generations {
		row := &generations[i]
		actual := factoryMap[row.FactoryGUID]
		cycle := cycleMap[row.FactoryGUID]
		if actual == nil || cycle == nil || row.Generation < 2 || row.Generation > 128 || row.BornAt < match.StartAt || row.BornAt >= match.EndAt || row.SourceHash != p13UFCarGenerationHash(row) || row.CarHash != p6Digest(row.CarFrame) || row.StateHash != p13UFCarGenerationStateHash(row) || cycle.SourceHash != p13UFFactoryCycleOfferHash(cycle) || row.OfferHash != cycle.SourceHash || row.Generation > cycle.MaxGenerations {
			return nil, p13UFTowerError()
		}
		frozen := &db.P13UnionFightFactoryState{}
		if json.Unmarshal(row.FactorySnapshot, frozen) != nil {
			return nil, p13UFTowerError()
		}
		if _, err := p13UFFactoryFrame(frozen); err != nil {
			return nil, err
		}
		if frozen.MatchID != match.ID || frozen.GUID != row.FactoryGUID || frozen.CompletedAt != row.BornAt || frozen.SourceHash != actual.SourceHash || frozen.ChariotHP != actual.ChariotHP || frozen.Camp != actual.Camp || frozen.FactoryID != actual.FactoryID {
			return nil, p13UFTowerError()
		}
		if err := p13UFCarGenerationClockValid(row, frozen, cycle); err != nil {
			return nil, err
		}
		frozen.CarGeneration = row.Generation
		frozen.CarFrame = append([]byte(nil), row.CarFrame...)
		frozen.CarHash = row.CarHash
		frozen.StateHash = p13UFFactoryStateDigest(frozen)
		if row.CarGUID != p13UFCarInstanceGUID(frozen) {
			return nil, p13UFTowerError()
		}
		candidates = append(candidates, &p13UFTowerTarget{factory: frozen, generation: row})
	}
	targets := make([]*p13UFTowerTarget, 0, len(candidates))
	for _, target := range candidates {
		row := target.factory
		car, err := p13UFCarItemRead(row)
		if err != nil {
			return nil, err
		}
		if car.GetHp() <= 0 {
			continue
		}
		cfg := original.table("UnionFightFactory", "UFFId", strconv.FormatInt(row.FactoryID, 10))
		if cfg == nil || cfg["Camp"] != strconv.FormatInt(row.Camp, 10) || cfg["CariotId"] != strconv.FormatInt(int64(car.GetCariotId()), 10) {
			return nil, p13UFTowerError()
		}
		point, err := fieldPoint(cfg["CariotBornPos"])
		if err != nil {
			return nil, err
		}
		motion := motionMap[row.GUID]
		attackOffer := offerMap[row.GUID]
		if motion != nil {
			points, total, err := p13UFCarMotionPath(motion)
			if err != nil {
				return nil, err
			}
			if attackOffer != nil {
				if attackOffer.SourceHash != p13UFCarAttackOfferHash(attackOffer) || attackOffer.MotionHash != motion.SourceHash || attackOffer.StopDistance < 0 || attackOffer.StopDistance > total {
					return nil, p13UFTowerError()
				}
				total = attackOffer.StopDistance
			}
			point = proto.Clone(points[0]).(*pb.PointNew)
			moving := positionMap[p13UFCarStateGUID(row)]
			if moving != nil {
				if moving.StateHash != p13UFCarMotionStateHash(moving) || moving.OfferHash != motion.SourceHash || moving.CarGUID != car.GetGuid() || moving.LastAt < row.CompletedAt || !combatFinite(moving.Distance) || moving.Distance < 0 || moving.Distance > total || moving.Arrived != (moving.Distance == total) {
					return nil, p13UFTowerError()
				}
				point = p13UFCarMotionAt(points, moving.Distance)
			}
		} else if attackOffer != nil {
			return nil, p13UFTowerError()
		}
		if attacking := attackMap[p13UFCarStateGUID(row)]; attacking != nil {
			if attackOffer == nil || attacking.StateHash != p13UFCarAttackStateHash(attacking) || attacking.OfferHash != attackOffer.SourceHash || attacking.CarGUID != car.GetGuid() || attacking.Hits < 0 || attacking.PendingReleaseAt < 0 || attacking.NextAttackAt < row.CompletedAt {
				return nil, p13UFTowerError()
			}
			if attacking.Completed {
				continue
			}
		}
		target.kind = "car"
		target.guid = car.GetGuid()
		target.camp = row.Camp
		target.car = car
		target.x, target.y, target.z = point.GetX(), point.GetY(), point.GetZ()
		targets = append(targets, target)
	}
	return targets, nil
}

// p13UFTowerStoreCar writes the already locked generation without a per-hit read.
func p13UFTowerStoreCar(tx *gorm.DB, target *p13UFTowerTarget) error {
	raw, err := proto.Marshal(target.car)
	if err != nil {
		return err
	}
	target.factory.CarFrame = raw
	target.factory.CarHash = p6Digest(raw)
	if target.generation == nil {
		frame := &pb.InstUFCariotFactory{}
		if target.factory.SourceHash != p13UFFactoryDigest(target.factory) || target.factory.FrameHash != p6Digest(target.factory.Frame) || proto.Unmarshal(target.factory.Frame, frame) != nil || frame.GetGuid() != target.factory.GUID {
			return p13UFTowerError()
		}
		return p13UFFactorySave(tx, target.factory, frame)
	}
	row := target.generation
	row.CarFrame = raw
	row.CarHash = p6Digest(raw)
	row.StateHash = p13UFCarGenerationStateHash(row)
	return tx.Model(&db.P13UnionFightCarGeneration{}).Where("match_id=? AND car_guid=?", row.MatchID, row.CarGUID).Updates(map[string]interface{}{"car_frame": row.CarFrame, "car_hash": row.CarHash, "state_hash": row.StateHash}).Error
}

func p13UFTowerClear(row *db.P13UnionFightTowerState, now int64) {
	row.PendingTargetGUID = 0
	p13UFTowerClearLaunch(row)
	row.PendingTargetKind = ""
	row.PendingReleaseAt = 0
	row.LastResolvedAt = now
	row.NextFireAt = now + row.CooldownMillis - row.ReleaseMillis
}

// TickP13UnionFightTower processes at most sixteen entities per invocation.
// Lock order is guild policy, ordered tower entities, then ordered target tables.
// A delayed worker resolves one previously launched shot and never catches up
// missed idle shots. Revalidation failure cancels without a synthetic Hit row.
func (s *GameService) TickP13UnionFightTower(ctx context.Context, now time.Time) error {
	var due []db.P13UnionFightTowerState
	tx := db.GetIns().WithContext(ctx)
	if err := tx.Select("match_id,tower_guid").Where("next_fire_at<=? AND (pending_release_at>0 OR match_id IN (?) OR match_id IN (?))", now.UnixMilli(), tx.Model(&db.P13UnionFightMatch{}).Select("id").Where("start_at<=? AND end_at>?", now.UnixMilli(), now.UnixMilli()), tx.Model(&db.P13UnionFightTowerHit{}).Select("match_id").Where("status=?", constUtil.P13UnionFightTowerHitPending)).Order("next_fire_at,match_id,tower_guid").Limit(constUtil.P13UnionFightTowerBatch).Find(&due).Error; err != nil {
		return err
	}
	groups := map[string][]int64{}
	scopes := []string{}
	for _, row := range due {
		if _, ok := groups[row.MatchID]; !ok {
			scopes = append(scopes, row.MatchID)
		}
		groups[row.MatchID] = append(groups[row.MatchID], row.TowerGUID)
	}
	sort.Strings(scopes)
	clocks, clockErr := p13UFWorldClockBatch(ctx, scopes, now)
	if clockErr != nil {
		return clockErr
	}
	for _, scope := range scopes {
		if clocks.paused(scope) {
			continue
		}
		var broadcasts []proto.Message
		private := map[int64][]proto.Message{}
		err := tx.Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			if closed, err := p13UFWorldSealed(tx, scope); err != nil {
				return err
			} else if closed {
				return nil
			}
			var policy db.P13UnionFightPolicy
			if err := tx.First(&policy, 1).Error; err != nil {
				return err
			}
			var field db.P13UnionFightFieldPolicy
			if err := tx.First(&field, 1).Error; err != nil {
				return err
			}
			if !gate.ManagementEnabled || !policy.EnrollmentEnabled || !policy.MatchEnabled || !field.Enabled {
				return nil
			}
			var match db.P13UnionFightMatch
			if err := tx.First(&match, "id=?", scope).Error; err != nil {
				return err
			}
			if now.UnixMilli() < match.StartAt {
				return nil
			}
			var guildCount int64
			if err := tx.Model(&db.P13Guild{}).Where("id IN ?", []string{match.LeftGuildID, match.RightGuildID}).Count(&guildCount).Error; err != nil {
				return err
			}
			if guildCount != 2 {
				return nil
			}
			var towers []db.P13UnionFightTowerState
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=? AND tower_guid IN ?", scope, groups[scope]).Order("tower_guid").Find(&towers).Error; err != nil {
				return err
			}
			if now.UnixMilli() >= clocks.end(&match) {
				for i := range towers {
					tower := &towers[i]
					var pending []db.P13UnionFightTowerHit
					if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=? AND tower_guid=? AND status=? AND (hit_at>? OR due_at>?)", scope, tower.TowerGUID, constUtil.P13UnionFightTowerHitPending, clocks.end(&match), clocks.end(&match)).Order("hit_at,sequence").Limit(128).Find(&pending).Error; err != nil {
						return err
					}
					for j := range pending {
						hit := &pending[j]
						if err := p13UFTowerHitVerify(hit, tower); err != nil {
							return err
						}
						oldHash := hit.HitHash
						hit.Status = constUtil.P13UnionFightTowerHitCanceled
						hit.CancelReason = p13UFTowerCancelWorld
						hit.CutoffAt = clocks.end(&match)
						hit.AppliedAt = now.UnixMilli()
						hit.HitHash = p13UFTowerHitHash(hit)
						if err := p13UFTowerHitVerify(hit, tower); err != nil {
							return err
						}
						saved := tx.Model(&db.P13UnionFightTowerHit{}).Where("match_id=? AND tower_guid=? AND sequence=? AND hit_hash=? AND status=?", scope, hit.TowerGUID, hit.Sequence, oldHash, constUtil.P13UnionFightTowerHitPending).Select("status,cancel_reason,cutoff_at,applied_at,hit_hash").Updates(hit)
						if saved.Error != nil {
							return saved.Error
						}
						if saved.RowsAffected != 1 {
							return p13UFTowerError()
						}
					}
				}
			}
			guardIDs := make([]int64, 0, len(towers))
			for i := range towers {
				row := &towers[i]
				if !p13UFTowerValid(row) || row.MatchHash != match.SnapshotHash || row.PublishedAt >= match.StartAt {
					return p13UFTowerError()
				}
				guardIDs = append(guardIDs, row.GuardGUID)
			}
			var guards []db.P13UnionFightGuardState
			if len(guardIDs) > 0 {
				if err := tx.Where("match_id=? AND guard_guid IN ?", scope, guardIDs).Find(&guards).Error; err != nil {
					return err
				}
			}
			guardMap := map[int64]*db.P13UnionFightGuardState{}
			for i := range guards {
				if _, err := p13UFGuardStateFrame(&guards[i]); err != nil {
					return err
				}
				guardMap[guards[i].GuardGUID] = &guards[i]
			}
			targets, err := s.p13UFTowerPlayers(tx, &match)
			if err != nil {
				return err
			}
			original, err := s.p13UFFieldOriginal(&match, tx)
			if err != nil {
				return err
			}
			cars, err := s.p13UFTowerCars(tx, &match, original)
			if err != nil {
				return err
			}
			needOccupancy := false
			for _, tower := range towers {
				needOccupancy = needOccupancy || tower.PlayerDamageRule == constUtil.P13UnionFightTowerPlayerDamageRule
			}
			if needOccupancy {
				if err = p13UFTowerOccupancies(tx, scope, targets); err != nil {
					return err
				}
			}
			targets = append(targets, cars...)
			targetMap := map[string]*p13UFTowerTarget{}
			for _, target := range targets {
				key := target.kind + ":" + strconv.FormatInt(target.guid, 10)
				if targetMap[key] != nil {
					return p13UFTowerError()
				}
				targetMap[key] = target
			}
			for i := range towers {
				tower := &towers[i]
				if tower.NextFireAt > now.UnixMilli() {
					continue
				}
				cutoff := clocks.end(&match)
				if tower.PendingReleaseAt > cutoff && tower.PendingReleaseAt <= now.UnixMilli() {
					hit := db.P13UnionFightTowerHit{MatchID: scope, TowerGUID: tower.TowerGUID, Sequence: tower.Sequence, TargetKind: tower.PendingTargetKind, TargetGUID: tower.PendingTargetGUID, Status: constUtil.P13UnionFightTowerHitCanceled, CancelReason: p13UFTowerCancelWorld, CutoffAt: cutoff, HitAt: tower.PendingReleaseAt, DueAt: tower.PendingReleaseAt, AppliedAt: now.UnixMilli(), TowerSourceHash: tower.SourceHash, X: tower.X, Y: tower.Y, Z: tower.Z}
					if tower.PendingTargetKind == "player" && tower.PlayerDamageRule != "" {
						hit.ActorGUID = tower.PendingActorGUID
						hit.ActorPose = tower.PendingActorPose
						hit.LifeOrdinal = tower.PendingTargetLife
						hit.FrozenActor = append([]byte(nil), tower.PendingFrozenActor...)
						hit.FrozenActorHash = tower.PendingFrozenActorHash
						hit.CaptureFrame = append([]byte(nil), tower.PendingLaunchFrame...)
						hit.CaptureFrameHash = tower.PendingLaunchFrameHash
						hit.LaunchOccupancyKind = tower.PendingOccupancyKind
						hit.LaunchOccupancyID = tower.PendingOccupancyID
						hit.LaunchOccupancyInputHash = tower.PendingOccupancyInputHash
						hit.LaunchOccupancyDueAt = tower.PendingOccupancyDueAt
					}
					hit.ImmutableHash = p13UFTowerImmutableHash(&hit)
					hit.HitHash = p13UFTowerHitHash(&hit)
					if err = p13UFTowerHitVerify(&hit, tower); err != nil {
						return err
					}
					if err = tx.Create(&hit).Error; err != nil {
						return err
					}
					p13UFTowerClear(tower, now.UnixMilli())
					tower.StateHash = p13UFTowerStateHash(tower)
					if err = tx.Save(tower).Error; err != nil {
						return err
					}
					continue
				}
				if now.UnixMilli() >= cutoff && tower.PendingReleaseAt == 0 {
					continue
				}
				active := true
				if guard := guardMap[tower.GuardGUID]; guard != nil {
					if guard.BindGUID != tower.TowerGUID || guard.Camp != tower.Camp || guard.BuildingKind != "tower" {
						return p13UFTowerError()
					}
					active = guard.DefeatedAt == 0
				}
				if !active {
					if tower.Active {
						broadcasts = append(broadcasts, &pb.SCUFTowerCrash{TowerGuid: proto.Int64(tower.TowerGUID)})
					}
					tower.Active = false
					p13UFTowerClear(tower, now.UnixMilli())
					tower.NextFireAt = now.UnixMilli() + tower.CooldownMillis
				} else if !tower.Active {
					tower.Active = true
					p13UFTowerClear(tower, now.UnixMilli())
					tower.NextFireAt = now.UnixMilli() + tower.CooldownMillis
					guard := guardMap[tower.GuardGUID]
					if guard == nil {
						return p13UFTowerError()
					}
					frame, err := p13UFGuardStateFrame(guard)
					if err != nil {
						return err
					}
					broadcasts = append(broadcasts, &pb.SCUFTowerRecovery{Tower: &pb.InstUFTower{Guid: proto.Int64(tower.TowerGUID), Id: proto.Int32(int32(tower.TowerID)), Level: proto.Int32(1), State: proto.Int32(0), Guard: frame}})
				} else if tower.PendingReleaseAt > 0 {
					if now.UnixMilli() < tower.PendingReleaseAt {
						continue
					}
					target := targetMap[tower.PendingTargetKind+":"+strconv.FormatInt(tower.PendingTargetGUID, 10)]
					newPlayer := tower.PlayerDamageRule == constUtil.P13UnionFightTowerPlayerDamageRule && tower.PendingTargetKind == "player"
					if newPlayer && target == nil {
						target = &p13UFTowerTarget{kind: "player", guid: tower.PendingTargetGUID, camp: 1 - tower.Camp, x: tower.X, y: tower.Y, z: tower.Z}
					}
					if newPlayer || target != nil && target.camp != tower.Camp && p13UFTowerInRange(tower, target.x, target.y, target.z) && (target.kind != "player" || target.frame.GetState() == 0 || target.frame.GetState() == 2 || target.frame.GetState() == 5 || target.frame.GetState() == 1 && tower.PlayerDamageRule == constUtil.P13UnionFightTowerPlayerDamageRule) && (target.kind != "car" || target.car.GetHp() > 0) {
						hit := db.P13UnionFightTowerHit{MatchID: scope, TowerGUID: tower.TowerGUID, Sequence: tower.Sequence, TargetKind: target.kind, TargetGUID: target.guid, DueAt: tower.PendingReleaseAt, AppliedAt: now.UnixMilli(), X: target.x, Y: target.y, Z: target.z, TowerSourceHash: tower.SourceHash}

						if tower.PlayerDamageRule != "" {
							hit.Status = constUtil.P13UnionFightTowerHitApplied
							hit.HitAt = tower.PendingReleaseAt
						}

						if target.kind == "player" {
							var after *pb.InstUFPlayer
							var revive int64
							if tower.PlayerDamageRule == "" {
								after, revive, err = p13UFTowerDamagePlayer(target.frame, tower.HPPercent)
								if err != nil {
									return err
								}
								hit.Before, _ = proto.Marshal(target.frame)
								if revive > 0 {
									hit.DeadPlayerID = target.guid
									hit.ReviveSeconds = revive
									hit.ReviveAt = hit.DueAt + revive*1000
									after.LeftTime = proto.Float32(float32(max(int64(0), hit.ReviveAt-now.UnixMilli())) / 1000)
								}
								hit.After, _ = proto.Marshal(after)
							} else {
								if err = p13UFTowerCapturePhysical(tx, &hit, tower, target); err != nil {
									return err
								}
								if hit.Status == constUtil.P13UnionFightTowerHitApplied {
									after, revive, err = p13UFTowerApplyFrozen(&hit, target.frame, now.UnixMilli())
									if err != nil {
										return err
									}
								}
							}
							if after != nil {
								target.frame = after
								target.player.UpdatedAt = now.UnixMilli()
								if err = p13UFFieldSaveActor(tx, target.player, after); err != nil {
									return err
								}
								broadcasts = append(broadcasts, p13UFTowerPlayerEffect(&hit, after, target.x, target.y, target.z)...)
								if revive > 0 {
									private[target.guid] = append(private[target.guid], &pb.SCUFPlayerDead{Guid: []int64{target.guid}, ReviveTime: []float64{float64(max(int64(0), hit.ReviveAt-now.UnixMilli())) / 1000}, ReviveItemId: []string{p13UFGuardReviveItem(after)}})
								}
							}
						} else {
							before := target.car
							after, err := p13UFTowerDamageCar(before, tower.HPPercent)
							if err != nil {
								return err
							}
							hit.Before, err = proto.Marshal(before)
							if err != nil {
								return err
							}
							hit.After, err = proto.Marshal(after)
							if err != nil {
								return err
							}
							target.car = after
							if err = p13UFTowerStoreCar(tx, target); err != nil {
								return err
							}
							broadcasts = append(broadcasts, &pb.SCUFElementTickExport{TargetGuid: []int64{target.guid}, SourceGuid: []int64{tower.TowerGUID}, HpDelta: []float64{after.GetHp() - before.GetHp()}}, &pb.SCUFCaropt{Cariot: []*pb.InstUFCariot{after}, Local: []*pb.PointNew{{X: proto.Float32(target.x), Y: proto.Float32(target.y), Z: proto.Float32(target.z)}}})
							if after.GetHp() == 0 {
								broadcasts = append(broadcasts, &pb.SCUFCariotCrash{Guid: after.Guid})
							}
						}
						if len(hit.Before) > 0 {
							hit.BeforeHash = p6Digest(hit.Before)
						}
						if len(hit.After) > 0 {
							hit.AfterHash = p6Digest(hit.After)
						}
						if hit.Status != "" && hit.ImmutableHash == "" {
							hit.ImmutableHash = p13UFTowerImmutableHash(&hit)
						}
						hit.HitHash = p13UFTowerHitHash(&hit)
						if err = p13UFTowerHitVerify(&hit, tower); err != nil {
							return err
						}
						if err = tx.Create(&hit).Error; err != nil {
							return err
						}
					}
					p13UFTowerClear(tower, now.UnixMilli())
				} else {
					target := p13UFTowerSelect(tower, targets)
					if target == nil || now.UnixMilli()+tower.ReleaseMillis > cutoff {
						tower.NextFireAt = now.UnixMilli() + tower.CooldownMillis
					} else {
						if tower.Sequence == math.MaxInt64 {
							return p13UFTowerError()
						}
						tower.Sequence++
						tower.PendingTargetGUID = target.guid
						tower.PendingTargetKind = target.kind
						tower.PendingReleaseAt = now.UnixMilli() + tower.ReleaseMillis
						tower.NextFireAt = tower.PendingReleaseAt
						if target.kind == "player" && tower.PlayerDamageRule == constUtil.P13UnionFightTowerPlayerDamageRule {
							if err = p13UFTowerFreezeLaunch(tower, target); err != nil {
								return err
							}
						}
						if target.kind == "car" {
							broadcasts = append(broadcasts, &pb.SCUFTowerAttackCariot{TowerGuid: proto.Int64(tower.TowerGUID), TargetGuid: proto.Int64(target.guid), ReleaseTime: proto.Float32(float32(tower.ReleaseMillis) / 1000)})
						} else {
							broadcasts = append(broadcasts, &pb.SCUFTowerAttackPlayer{TowerGuid: proto.Int64(tower.TowerGUID), TargetGuid: proto.Int64(target.guid), ReleaseTime: proto.Float32(float32(tower.ReleaseMillis) / 1000)})
						}
					}
				}
				tower.StateHash = p13UFTowerStateHash(tower)
				if err = tx.Save(tower).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if len(broadcasts) > 0 || len(private) > 0 {
			var recipients []db.P13UnionFightFieldPlayer
			if len(broadcasts) > 0 {
				var readErr error
				recipients, _, readErr = s.p13UFFieldPeers(db.GetIns().WithContext(ctx), scope, 0)
				if readErr != nil {
					return readErr
				}
			}
			if err := p13UFPublishBeforeAbort(ctx, scope, func() {
				for _, recipient := range recipients {
					for _, frame := range broadcasts {
						s.publishP10Private(ctx, recipient.PlayerID, frame)
					}
				}
				for player, frames := range private {
					for _, frame := range frames {
						s.publishP10Private(ctx, player, frame)
					}
				}
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// p13UFTowerProject only projects a matching latest environmental death. It
// never heals, revives or rewrites a later battle/item transition.
func p13UFTowerProject(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer, now time.Time) error {
	if frame.GetState() != 3 {
		return nil
	}
	var hit db.P13UnionFightTowerHit
	err := tx.Where("match_id=? AND target_kind=? AND target_guid=? AND dead_player_id=?", actor.MatchID, "player", actor.PlayerID, actor.PlayerID).Order("due_at DESC,tower_guid DESC,sequence DESC").First(&hit).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var tower db.P13UnionFightTowerState
	if err = tx.First(&tower, "match_id=? AND tower_guid=?", actor.MatchID, hit.TowerGUID).Error; err != nil {
		return err
	}
	if err = p13UFTowerHitVerify(&hit, &tower); err != nil {
		return err
	}
	bound := &pb.InstUFPlayer{}
	if proto.Unmarshal(hit.After, bound) != nil {
		return p13UFTowerError()
	}
	if frame.GetDeadNum() != bound.GetDeadNum() || !proto.Equal(frame.FightInfo, bound.FightInfo) {
		return nil
	}
	frame.LeftTime = proto.Float32(float32(max(int64(0), hit.ReviveAt-now.UnixMilli())) / 1000)
	frame.TotalTime = proto.Float32(float32(hit.ReviveSeconds))
	return nil
}

// p13UFTowerFrames restores only the still-pending projectile with remaining
// flight seconds. Callers may batch this once per match during scene refresh.
func p13UFTowerFrames(tx *gorm.DB, scope string, now time.Time) ([]proto.Message, error) {
	var rows []db.P13UnionFightTowerState
	if err := tx.Where("match_id=?", scope).Order("tower_guid").Limit(7).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 6 {
		return nil, p13UFTowerError()
	}
	frames := []proto.Message{}
	for i := range rows {
		row := &rows[i]
		if !p13UFTowerValid(row) {
			return nil, p13UFTowerError()
		}
		if !row.Active {
			frames = append(frames, &pb.SCUFTowerCrash{TowerGuid: proto.Int64(row.TowerGUID)})
			continue
		}
		if row.PendingReleaseAt <= now.UnixMilli() {
			continue
		}
		delay := float32(row.PendingReleaseAt-now.UnixMilli()) / 1000
		if row.PendingTargetKind == "car" {
			frames = append(frames, &pb.SCUFTowerAttackCariot{TowerGuid: proto.Int64(row.TowerGUID), TargetGuid: proto.Int64(row.PendingTargetGUID), ReleaseTime: proto.Float32(delay)})
		} else {
			frames = append(frames, &pb.SCUFTowerAttackPlayer{TowerGuid: proto.Int64(row.TowerGUID), TargetGuid: proto.Int64(row.PendingTargetGUID), ReleaseTime: proto.Float32(delay)})
		}
	}
	return frames, nil
}

// P13UnionFightTowerHitMeta is the payload-free complete directory contract.
// Pending and canceled entries are facts, never successful HP transitions.
type P13UnionFightTowerHitMeta struct {
	ReleaseFrameHash, LaunchOccupancyKind, LaunchOccupancyID, LaunchOccupancyInputHash string
	LaunchOccupancyDueAt                                                               int64
	MatchID, TargetKind, TowerSourceHash, ImmutableHash, HitHash                       string
	TowerGUID, Sequence, TargetGUID, HitAt, DueAt, AppliedAt                           int64
	Status, CancelReason, ActorGUID, FrozenActorHash, CaptureFrameHash                 string
	ActorPose, LifeOrdinal, CutoffAt                                                   int64
	Damage                                                                             float64
	OccupancyKind, OccupancyID, OccupancyInputHash                                     string
	OccupancyDueAt                                                                     int64
	BeforeHash, AfterHash, CancelFrameHash, CoalescedInto                              string
	CancelLeftAt, DeadPlayerID, ReviveSeconds, ReviveAt                                int64
}

func P13UnionFightTowerHitMetadata(row *db.P13UnionFightTowerHit) P13UnionFightTowerHitMeta {
	status, hitAt := row.Status, row.HitAt
	if status == "" {
		status = constUtil.P13UnionFightTowerHitLegacyApplied
		hitAt = row.DueAt
	}
	return P13UnionFightTowerHitMeta{CutoffAt: row.CutoffAt, ReleaseFrameHash: row.ReleaseFrameHash, LaunchOccupancyKind: row.LaunchOccupancyKind, LaunchOccupancyID: row.LaunchOccupancyID, LaunchOccupancyInputHash: row.LaunchOccupancyInputHash, LaunchOccupancyDueAt: row.LaunchOccupancyDueAt, MatchID: row.MatchID, TargetKind: row.TargetKind, TowerSourceHash: row.TowerSourceHash, ImmutableHash: row.ImmutableHash, HitHash: row.HitHash, TowerGUID: row.TowerGUID, Sequence: row.Sequence, TargetGUID: row.TargetGUID, HitAt: hitAt, DueAt: row.DueAt, AppliedAt: row.AppliedAt, Status: status, CancelReason: row.CancelReason, ActorGUID: row.ActorGUID, FrozenActorHash: row.FrozenActorHash, CaptureFrameHash: row.CaptureFrameHash, ActorPose: row.ActorPose, LifeOrdinal: row.LifeOrdinal, Damage: row.Damage, OccupancyKind: row.OccupancyKind, OccupancyID: row.OccupancyID, OccupancyInputHash: row.OccupancyInputHash, OccupancyDueAt: row.OccupancyDueAt, BeforeHash: row.BeforeHash, AfterHash: row.AfterHash, CancelFrameHash: row.CancelFrameHash, CoalescedInto: row.CoalescedInto, CancelLeftAt: row.CancelLeftAt, DeadPlayerID: row.DeadPlayerID, ReviveSeconds: row.ReviveSeconds, ReviveAt: row.ReviveAt}
}

func p13UFTowerImmutableHash(row *db.P13UnionFightTowerHit) string {
	copy := *row
	copy.ImmutableHash, copy.HitHash, copy.Status, copy.CancelReason = "", "", "", ""
	copy.AppliedAt, copy.DeadPlayerID, copy.ReviveSeconds, copy.ReviveAt = 0, 0, 0, 0
	copy.Before, copy.After, copy.CancelFrame = nil, nil, nil
	copy.BeforeHash, copy.AfterHash, copy.CancelFrameHash = "", "", ""
	copy.CancelLeftAt = 0
	copy.CutoffAt = 0
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// The scene has no persistent skill shield snapshot. This named local rule
// uses the original subtractive physical defense/reduction and max1 clamp,
// with no skill factor, crit, dodge or RNG. P6 skill shields remain in P6.
func p13UFTowerPhysicalDamage(hero *pb.BattleHero, attack int64) (float64, error) {
	if hero == nil || hero.Prop == nil || attack != 65 || !combatFinite(hero.Prop.GetPdef()) || !combatFinite(hero.Prop.GetDamageReduce()) {
		return 0, p13UFTowerError()
	}
	damage := math.Max(1, (float64(attack)-hero.Prop.GetPdef())*math.Max(0, 1-hero.Prop.GetDamageReduce()/100))
	if !combatFinite(damage) {
		return 0, p13UFTowerError()
	}
	return damage, nil
}
func p13UFTowerFreezeActor(actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer, tower *db.P13UnionFightTowerState) (*pb.BattleHero, float64, error) {
	if actor == nil || frame == nil || frame.FightInfo == nil || actor.BattleHash != p6Digest(actor.Battle) || frame.GetDeadNum() < 0 {
		return nil, 0, p13UFTowerError()
	}
	var party pb.BattlePlayer
	if proto.Unmarshal(actor.Battle, &party) != nil || party.GetPlayerId() != actor.PlayerID {
		return nil, 0, p13UFTowerError()
	}
	fight := frame.FightInfo
	if len(fight.Index) < 1 || len(fight.Index) > 4 || len(fight.Index) != len(fight.HpPos) || len(fight.Index) != len(fight.MaxHpPos) {
		return nil, 0, p13UFTowerError()
	}
	slot := -1
	seen := map[int32]bool{}
	for i, pose := range fight.Index {
		hp, maxHP := fight.HpPos[i], fight.MaxHpPos[i]
		if seen[pose] || pose < 1 || pose > 15 || !combatFinite(hp) || !combatFinite(maxHP) || hp < 0 || maxHP <= 0 || hp > maxHP {
			return nil, 0, p13UFTowerError()
		}
		seen[pose] = true
		if slot < 0 && hp > 0 {
			slot = i
		}
	}
	if slot < 0 {
		return nil, 0, p13UFTowerError()
	}
	for _, hero := range party.Formation {
		if hero.GetPose() == fight.Index[slot] {
			if hero.GetGuid() == "" || hero.Prop == nil || hero.Inst == nil || hero.GetProp().GetHp() != fight.MaxHpPos[slot] {
				return nil, 0, p13UFTowerError()
			}
			damage, err := p13UFTowerPhysicalDamage(hero, tower.PlayerAttack)
			return hero, damage, err
		}
	}
	return nil, 0, p13UFTowerError()
}

func p13UFTowerDamageBoundActor(before *pb.InstUFPlayer, hero *pb.BattleHero, damage float64) (*pb.InstUFPlayer, int64, error) {
	if before == nil || hero == nil || before.FightInfo == nil || before.GetState() == 3 || before.GetDeadNum() < 0 || before.GetDeadNum() == math.MaxInt32 || !combatFinite(damage) || damage < 1 {
		return nil, 0, p13UFTowerError()
	}
	after := proto.Clone(before).(*pb.InstUFPlayer)
	fight := after.FightInfo
	if len(fight.Index) != len(fight.HpPos) || len(fight.Index) != len(fight.MaxHpPos) {
		return nil, 0, p13UFTowerError()
	}
	found, remaining := false, false
	for i, pose := range fight.Index {
		if !combatFinite(fight.HpPos[i]) || !combatFinite(fight.MaxHpPos[i]) || fight.HpPos[i] < 0 || fight.MaxHpPos[i] <= 0 || fight.HpPos[i] > fight.MaxHpPos[i] {
			return nil, 0, p13UFTowerError()
		}
		if pose == hero.GetPose() {
			if found || fight.HpPos[i] <= 0 || fight.MaxHpPos[i] != hero.GetProp().GetHp() {
				return nil, 0, p13UFTowerError()
			}
			found = true
			fight.HpPos[i] = math.Max(0, fight.HpPos[i]-damage)
		}
		remaining = remaining || fight.HpPos[i] > 0
	}
	if !found {
		return nil, 0, p13UFTowerError()
	}
	if remaining {
		return after, 0, nil
	}
	after.DeadNum = proto.Int32(after.GetDeadNum() + 1)
	revive := int64(min(int(after.GetDeadNum()), 8) * 5)
	after.State = proto.Int32(3)
	after.LeftTime = proto.Float32(float32(revive))
	after.TotalTime = proto.Float32(float32(revive))
	return after, revive, nil
}

func p13UFTowerOccupancies(tx *gorm.DB, scope string, targets []*p13UFTowerTarget) error {
	ids := []int64{}
	directory := map[int64]*p13UFTowerTarget{}
	for _, target := range targets {
		if target.kind == "player" && target.frame.GetState() == 1 {
			ids = append(ids, target.guid)
			directory[target.guid] = target
		}
	}
	if len(ids) == 0 {
		return nil
	}
	type occupancy struct {
		ID, InputHash             string
		PlayerID, TargetID, DueAt int64
	}
	var ordinary, guard []occupancy
	if err := tx.Model(&db.P13UnionFightPlayerBattleAttempt{}).Select("id,input_hash,player_id,target_id,due_at").Where("match_id=? AND status=? AND (player_id IN ? OR target_id IN ?)", scope, constUtil.P13UFPlayerBattlePending, ids, ids).Limit(129).Scan(&ordinary).Error; err != nil {
		return err
	}
	if err := tx.Model(&db.P13UnionFightGuardBattleAttempt{}).Select("id,input_hash,player_id,due_at").Where("match_id=? AND status=? AND player_id IN ?", scope, constUtil.P13UFGuardPending, ids).Limit(129).Scan(&guard).Error; err != nil {
		return err
	}
	if len(ordinary) > 128 || len(guard) > 128 {
		return p13UFTowerError()
	}
	bind := func(id int64, row occupancy, kind string) error {
		target := directory[id]
		if target == nil {
			return nil
		}
		if target.occupancyID != "" || len(row.ID) != 64 || len(row.InputHash) != 64 || row.DueAt <= 0 {
			return p13UFTowerError()
		}
		target.occupancyKind, target.occupancyID, target.occupancyInputHash, target.occupancyDueAt = kind, row.ID, row.InputHash, row.DueAt
		return nil
	}
	for _, row := range ordinary {
		if err := bind(row.PlayerID, row, constUtil.P13UnionFightTowerOccupancyOrdinary); err != nil {
			return err
		}
		if err := bind(row.TargetID, row, constUtil.P13UnionFightTowerOccupancyOrdinary); err != nil {
			return err
		}
	}
	for _, row := range guard {
		if err := bind(row.PlayerID, row, constUtil.P13UnionFightTowerOccupancyGuard); err != nil {
			return err
		}
	}
	for _, target := range directory {
		if target.occupancyID == "" {
			return p13UFTowerError()
		}
	}
	return p13UFTowerOccupancyShots(tx, scope, targets)
}

func p13UFTowerCapturePhysical(tx *gorm.DB, hit *db.P13UnionFightTowerHit, tower *db.P13UnionFightTowerState, target *p13UFTowerTarget) error {
	resolvedAt := hit.AppliedAt
	if !p13UFTowerLaunchValid(tower) || tower.PendingActorGUID == "" || tower.PendingTargetGUID != hit.TargetGUID || tower.PendingReleaseAt != hit.HitAt || tower.Sequence != hit.Sequence {
		return p13UFTowerError()
	}
	var hero pb.BattleHero
	if proto.Unmarshal(tower.PendingFrozenActor, &hero) != nil {
		return p13UFTowerError()
	}
	damage, err := p13UFTowerPhysicalDamage(&hero, tower.PlayerAttack)
	if err != nil {
		return err
	}
	hit.ActorGUID, hit.ActorPose, hit.LifeOrdinal, hit.Damage = tower.PendingActorGUID, tower.PendingActorPose, tower.PendingTargetLife, damage
	hit.FrozenActor = append([]byte(nil), tower.PendingFrozenActor...)
	hit.FrozenActorHash = tower.PendingFrozenActorHash
	hit.CaptureFrame = append([]byte(nil), tower.PendingLaunchFrame...)
	hit.CaptureFrameHash = tower.PendingLaunchFrameHash
	hit.LaunchOccupancyKind, hit.LaunchOccupancyID, hit.LaunchOccupancyInputHash, hit.LaunchOccupancyDueAt = tower.PendingOccupancyKind, tower.PendingOccupancyID, tower.PendingOccupancyInputHash, tower.PendingOccupancyDueAt
	hit.Before, hit.After = []byte{}, []byte{}
	reason := ""
	if target == nil || target.player == nil || target.frame == nil {
		reason = constUtil.P13UnionFightTowerCancelUnavailable
	} else {
		hit.ReleaseFrame, err = proto.MarshalOptions{Deterministic: true}.Marshal(target.frame)
		if err != nil {
			return err
		}
		hit.ReleaseFrameHash = p6Digest(hit.ReleaseFrame)
		if target.frame.GetState() == 1 {
			hit.OccupancyKind, hit.OccupancyID, hit.OccupancyInputHash, hit.OccupancyDueAt = target.occupancyKind, target.occupancyID, target.occupancyInputHash, target.occupancyDueAt
			if hit.OccupancyID == "" {
				return p13UFTowerError()
			}
			hit.DueAt = hit.OccupancyDueAt
		}
		switch {
		case target.player.LeftAt > 0:
			reason = constUtil.P13UnionFightTowerCancelLeft
			hit.CancelLeftAt = target.player.LeftAt
		case int64(target.frame.GetDeadNum()) != hit.LifeOrdinal:
			reason = constUtil.P13UnionFightTowerCancelLife
		case !p13UFTowerInRange(tower, target.x, target.y, target.z) || target.camp == tower.Camp || (target.frame.GetState() != 0 && target.frame.GetState() != 1 && target.frame.GetState() != 2 && target.frame.GetState() != 5 && target.frame.GetState() != 3):
			reason = constUtil.P13UnionFightTowerCancelUnavailable
		default:
			var party pb.BattlePlayer
			if target.player.BattleHash != p6Digest(target.player.Battle) || proto.Unmarshal(target.player.Battle, &party) != nil || party.GetPlayerId() != hit.TargetGUID {
				return p13UFTowerError()
			}
			bound := false
			for _, currentHero := range party.Formation {
				if currentHero.GetGuid() == hit.ActorGUID && currentHero.GetPose() == int32(hit.ActorPose) {
					if !proto.Equal(currentHero, &hero) {
						return p13UFTowerError()
					}
					bound = true
					break
				}
			}
			if !bound {
				return p13UFTowerError()
			}
			live := false
			for i, pose := range target.frame.GetFightInfo().GetIndex() {
				if int64(pose) == hit.ActorPose && i < len(target.frame.FightInfo.HpPos) && target.frame.FightInfo.HpPos[i] > 0 {
					live = true
				}
			}
			if !live || target.frame.GetState() == 3 {
				reason = constUtil.P13UnionFightTowerCancelActor
			}
			if reason == "" && hit.LaunchOccupancyID != "" && hit.OccupancyID != "" && (hit.LaunchOccupancyID != hit.OccupancyID || hit.LaunchOccupancyKind != hit.OccupancyKind || hit.LaunchOccupancyInputHash != hit.OccupancyInputHash || hit.LaunchOccupancyDueAt != hit.OccupancyDueAt) {
				reason = constUtil.P13UnionFightTowerCancelOccupancy
			}
		}
	}
	if reason != "" {
		hit.Status, hit.CancelReason, hit.AppliedAt = constUtil.P13UnionFightTowerHitCanceled, reason, resolvedAt
		hit.CancelFrame = append([]byte(nil), hit.ReleaseFrame...)
		hit.CancelFrameHash = hit.ReleaseFrameHash
	} else if hit.OccupancyID != "" {
		hit.Status, hit.AppliedAt = constUtil.P13UnionFightTowerHitPending, 0
		if prior := target.occupancyShots[tower.TowerGUID]; prior != "" {
			hit.Status, hit.CancelReason, hit.CoalescedInto, hit.AppliedAt = constUtil.P13UnionFightTowerHitCanceled, constUtil.P13UnionFightTowerCancelCoalesced, prior, resolvedAt
		}

	}
	hit.ImmutableHash = p13UFTowerImmutableHash(hit)
	if hit.Status == constUtil.P13UnionFightTowerHitPending {
		if target.occupancyShots == nil {
			target.occupancyShots = map[int64]string{}
		}
		target.occupancyShots[tower.TowerGUID] = hit.ImmutableHash
	}
	return nil
}

func p13UFTowerApplyFrozen(hit *db.P13UnionFightTowerHit, before *pb.InstUFPlayer, now int64) (*pb.InstUFPlayer, int64, error) {
	var hero pb.BattleHero
	if proto.Unmarshal(hit.FrozenActor, &hero) != nil {
		return nil, 0, p13UFTowerError()
	}
	after, revive, err := p13UFTowerDamageBoundActor(before, &hero, hit.Damage)
	if err != nil {
		return nil, 0, err
	}
	hit.Status = constUtil.P13UnionFightTowerHitApplied
	hit.AppliedAt = now
	hit.Before, err = proto.MarshalOptions{Deterministic: true}.Marshal(before)
	if err != nil {
		return nil, 0, err
	}
	hit.After, err = proto.MarshalOptions{Deterministic: true}.Marshal(after)
	if err != nil {
		return nil, 0, err
	}
	hit.BeforeHash, hit.AfterHash = p6Digest(hit.Before), p6Digest(hit.After)
	if revive > 0 {
		hit.DeadPlayerID = hit.TargetGUID
		hit.ReviveSeconds = revive
		hit.ReviveAt = now + revive*1000
	}
	return after, revive, nil
}
func p13UFTowerPlayerEffect(hit *db.P13UnionFightTowerHit, after *pb.InstUFPlayer, x, y, z float32) []proto.Message {
	var before pb.InstUFPlayer
	_ = proto.Unmarshal(hit.Before, &before)
	delta := float64(0)
	for i, hp := range before.GetFightInfo().GetHpPos() {
		delta += after.FightInfo.HpPos[i] - hp
	}
	return []proto.Message{&pb.SCUFElementTickExport{TargetGuid: []int64{hit.TargetGUID}, SourceGuid: []int64{hit.TowerGUID}, HpDelta: []float64{delta}}, &pb.SCUFPlayer{Player: []*pb.InstUFPlayer{after}, Local: []*pb.PointNew{{X: proto.Float32(x), Y: proto.Float32(y), Z: proto.Float32(z)}}}}
}

func p13UFTowerPhysicalHitVerify(row *db.P13UnionFightTowerHit, tower *db.P13UnionFightTowerState) error {
	if !p13UFTowerValid(tower) || tower.PlayerDamageRule != constUtil.P13UnionFightTowerPlayerDamageRule || row.MatchID != tower.MatchID || row.TowerGUID != tower.TowerGUID || row.Sequence < 1 || row.Sequence > tower.Sequence || row.TowerSourceHash != tower.SourceHash || row.HitAt <= 0 || row.DueAt <= 0 || row.ImmutableHash != p13UFTowerImmutableHash(row) || row.HitHash != p13UFTowerHitHash(row) || !combatFinite(float64(row.X)) || !combatFinite(float64(row.Y)) || !combatFinite(float64(row.Z)) || row.Status != constUtil.P13UnionFightTowerHitCanceled && !p13UFTowerInRange(tower, row.X, row.Y, row.Z) {
		return p13UFTowerError()
	}
	if row.TargetKind == "car" {
		var before, after pb.InstUFCariot
		if row.Status != constUtil.P13UnionFightTowerHitApplied || row.DueAt != row.HitAt || row.AppliedAt < row.HitAt || row.OccupancyID != "" || row.ActorGUID != "" || row.Damage != 0 || row.CancelReason != "" || row.DeadPlayerID != 0 || row.ReviveSeconds != 0 || row.ReviveAt != 0 || row.BeforeHash != p6Digest(row.Before) || row.AfterHash != p6Digest(row.After) || proto.Unmarshal(row.Before, &before) != nil || proto.Unmarshal(row.After, &after) != nil || before.GetGuid() != row.TargetGUID || before.GetCamp() == int32(tower.Camp) {
			return p13UFTowerError()
		}
		expected, err := p13UFTowerDamageCar(&before, tower.HPPercent)
		if err != nil || !proto.Equal(expected, &after) {
			return p13UFTowerError()
		}
		return nil
	}
	if row.TargetKind != "player" || row.FrozenActorHash != p6Digest(row.FrozenActor) || row.CaptureFrameHash != p6Digest(row.CaptureFrame) {
		return p13UFTowerError()
	}
	var hero pb.BattleHero
	var capture pb.InstUFPlayer
	if proto.Unmarshal(row.FrozenActor, &hero) != nil || proto.Unmarshal(row.CaptureFrame, &capture) != nil || capture.GetGuid() != row.TargetGUID || capture.GetCamp() == int32(tower.Camp) || int64(capture.GetDeadNum()) != row.LifeOrdinal || hero.GetGuid() != row.ActorGUID || int64(hero.GetPose()) != row.ActorPose || capture.FightInfo == nil || len(capture.FightInfo.Index) != len(capture.FightInfo.HpPos) || len(capture.FightInfo.Index) != len(capture.FightInfo.MaxHpPos) {
		return p13UFTowerError()
	}
	damage, err := p13UFTowerPhysicalDamage(&hero, tower.PlayerAttack)
	if err != nil || damage != row.Damage {
		return p13UFTowerError()
	}
	if hero.GetGuid() == "" || len(capture.FightInfo.Index) < 1 || len(capture.FightInfo.Index) > 4 || (capture.GetState() != 0 && capture.GetState() != 1 && capture.GetState() != 2 && capture.GetState() != 5) {
		return p13UFTowerError()
	}
	seenPos := map[int32]bool{}
	for i, pose := range capture.FightInfo.Index {
		hp, maxHP := capture.FightInfo.HpPos[i], capture.FightInfo.MaxHpPos[i]
		if seenPos[pose] || pose < 1 || pose > 15 || !combatFinite(hp) || !combatFinite(maxHP) || hp < 0 || maxHP <= 0 || hp > maxHP {
			return p13UFTowerError()
		}
		seenPos[pose] = true
	}
	selected := false
	for i, hp := range capture.FightInfo.HpPos {
		if hp > 0 {
			selected = capture.FightInfo.Index[i] == hero.GetPose() && capture.FightInfo.MaxHpPos[i] == hero.GetProp().GetHp()
			break
		}
	}
	if !selected {
		return p13UFTowerError()
	}
	var release pb.InstUFPlayer
	if len(row.ReleaseFrame) > 0 {
		if row.ReleaseFrameHash != p6Digest(row.ReleaseFrame) || proto.Unmarshal(row.ReleaseFrame, &release) != nil || release.GetGuid() != row.TargetGUID {
			return p13UFTowerError()
		}
	} else if row.ReleaseFrameHash != "" || row.Status != constUtil.P13UnionFightTowerHitCanceled || row.CancelReason != constUtil.P13UnionFightTowerCancelUnavailable {
		return p13UFTowerError()
	}
	if row.LaunchOccupancyID != "" {
		if capture.GetState() != 1 || len(row.LaunchOccupancyID) != 64 || len(row.LaunchOccupancyInputHash) != 64 || row.LaunchOccupancyDueAt <= 0 || (row.LaunchOccupancyKind != constUtil.P13UnionFightTowerOccupancyOrdinary && row.LaunchOccupancyKind != constUtil.P13UnionFightTowerOccupancyGuard) {
			return p13UFTowerError()
		}
	} else if capture.GetState() == 1 || row.LaunchOccupancyKind != "" || row.LaunchOccupancyInputHash != "" || row.LaunchOccupancyDueAt != 0 {
		return p13UFTowerError()
	}
	if row.OccupancyID != "" {
		if (row.OccupancyKind != constUtil.P13UnionFightTowerOccupancyOrdinary && row.OccupancyKind != constUtil.P13UnionFightTowerOccupancyGuard) || len(row.OccupancyID) != 64 || len(row.OccupancyInputHash) != 64 || row.OccupancyDueAt <= 0 || row.DueAt != row.OccupancyDueAt || release.GetState() != 1 {
			return p13UFTowerError()
		}
	} else if row.OccupancyKind != "" || row.OccupancyInputHash != "" || row.OccupancyDueAt != 0 || row.DueAt != row.HitAt || release.GetState() == 1 {
		return p13UFTowerError()
	}
	switch row.Status {
	case constUtil.P13UnionFightTowerHitPending:
		if row.OccupancyID == "" || row.AppliedAt != 0 || row.CancelReason != "" || len(row.Before) != 0 || len(row.After) != 0 || row.BeforeHash != "" || row.AfterHash != "" || row.DeadPlayerID != 0 || row.ReviveAt != 0 || row.ReviveSeconds != 0 || len(row.CancelFrame) != 0 || row.CancelFrameHash != "" || row.CancelLeftAt != 0 || row.CoalescedInto != "" {
			return p13UFTowerError()
		}
	case constUtil.P13UnionFightTowerHitCanceled:
		if row.AppliedAt < row.HitAt || len(row.Before) != 0 || len(row.After) != 0 || row.BeforeHash != "" || row.AfterHash != "" || row.DeadPlayerID != 0 || row.ReviveAt != 0 || row.ReviveSeconds != 0 {
			return p13UFTowerError()
		}
		if row.CancelReason == constUtil.P13UnionFightTowerCancelCoalesced {
			if len(row.CoalescedInto) != 64 || len(row.CancelFrame) != 0 || row.CancelFrameHash != "" || row.CancelLeftAt != 0 {
				return p13UFTowerError()
			}
			return nil
		}
		if row.CancelReason == constUtil.P13UnionFightTowerCancelUnavailable && len(row.CancelFrame) == 0 {
			if len(row.ReleaseFrame) != 0 || row.CancelFrameHash != "" {
				return p13UFTowerError()
			}
			return nil
		}
		var canceled pb.InstUFPlayer
		if row.CoalescedInto != "" || row.CancelFrameHash != p6Digest(row.CancelFrame) || proto.Unmarshal(row.CancelFrame, &canceled) != nil || canceled.GetGuid() != row.TargetGUID {
			return p13UFTowerError()
		}
		if row.CancelReason != constUtil.P13UnionFightTowerCancelLeft && row.CancelLeftAt != 0 {
			return p13UFTowerError()
		}
		switch row.CancelReason {
		case constUtil.P13UnionFightTowerCancelUnavailable:
			if p13UFTowerInRange(tower, row.X, row.Y, row.Z) && canceled.GetCamp() != int32(tower.Camp) && (canceled.GetState() == 0 || canceled.GetState() == 1 || canceled.GetState() == 2 || canceled.GetState() == 5) {
				return p13UFTowerError()
			}
		case constUtil.P13UnionFightTowerCancelOccupancy:
			if row.LaunchOccupancyID == "" || row.OccupancyID == "" || row.LaunchOccupancyID == row.OccupancyID && row.LaunchOccupancyKind == row.OccupancyKind && row.LaunchOccupancyInputHash == row.OccupancyInputHash && row.LaunchOccupancyDueAt == row.OccupancyDueAt {
				return p13UFTowerError()
			}
		case constUtil.P13UnionFightTowerCancelLeft:
			if row.CancelLeftAt <= 0 {
				return p13UFTowerError()
			}
		case constUtil.P13UnionFightTowerCancelLife:
			if int64(canceled.GetDeadNum()) == row.LifeOrdinal {
				return p13UFTowerError()
			}
		case constUtil.P13UnionFightTowerCancelActor:
			live := false
			for i, pose := range canceled.GetFightInfo().GetIndex() {
				if pose == hero.GetPose() && i < len(canceled.GetFightInfo().GetHpPos()) && canceled.FightInfo.HpPos[i] > 0 {
					live = true
				}
			}
			if live && canceled.GetState() != 3 {
				return p13UFTowerError()
			}
		default:
			return p13UFTowerError()
		}
	case constUtil.P13UnionFightTowerHitApplied:
		var before, after pb.InstUFPlayer
		if row.AppliedAt < row.HitAt || row.AppliedAt < row.DueAt || row.CancelReason != "" || len(row.CancelFrame) != 0 || row.CancelFrameHash != "" || row.CancelLeftAt != 0 || row.CoalescedInto != "" || row.BeforeHash != p6Digest(row.Before) || row.AfterHash != p6Digest(row.After) || proto.Unmarshal(row.Before, &before) != nil || proto.Unmarshal(row.After, &after) != nil || before.GetGuid() != row.TargetGUID || int64(before.GetDeadNum()) != row.LifeOrdinal {
			return p13UFTowerError()
		}
		expected, revive, err := p13UFTowerDamageBoundActor(&before, &hero, row.Damage)
		if err != nil || !proto.Equal(expected, &after) || row.ReviveSeconds != revive {
			return p13UFTowerError()
		}
		if revive > 0 {
			if row.DeadPlayerID != row.TargetGUID || row.ReviveAt != row.AppliedAt+revive*1000 {
				return p13UFTowerError()
			}
		} else if row.DeadPlayerID != 0 || row.ReviveAt != 0 {
			return p13UFTowerError()
		}
	default:
		return p13UFTowerError()
	}
	return nil
}

// p13UFTowerDrain is called inside a verified P6 application transaction, before
// releasing that occupancy. The attempt.After remains pure P6; only the returned
// scene frame includes these deterministic environmental transitions.
func p13UFTowerDrain(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, scene *pb.InstUFPlayer, kind, id, inputHash string, dueAt int64, now time.Time) (*pb.InstUFPlayer, []proto.Message, error) {
	return p13UFEnvironmentDrain(tx, actor, scene, kind, id, inputHash, dueAt, now)
}

func p13UFTowerDrainRows(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, scene *pb.InstUFPlayer, kind, id, inputHash string, dueAt int64, now time.Time, rows []db.P13UnionFightTowerHit, directory map[int64]*db.P13UnionFightTowerState, party *pb.BattlePlayer) (*pb.InstUFPlayer, []proto.Message, error) {
	current := proto.Clone(scene).(*pb.InstUFPlayer)
	frames := []proto.Message{}
	for i := range rows {
		hit := &rows[i]
		tower := directory[hit.TowerGUID]
		if err := p13UFTowerHitVerify(hit, tower); err != nil {
			return nil, nil, err
		}
		if hit.OccupancyInputHash != inputHash || hit.OccupancyDueAt != dueAt {
			return nil, nil, p13UFTowerError()
		}
		oldHash := hit.HitHash
		reason := ""
		if actor.LeftAt > 0 {
			reason = constUtil.P13UnionFightTowerCancelLeft
		} else if int64(current.GetDeadNum()) != hit.LifeOrdinal {
			reason = constUtil.P13UnionFightTowerCancelLife
		} else {
			var frozen pb.BattleHero
			if proto.Unmarshal(hit.FrozenActor, &frozen) != nil {
				return nil, nil, p13UFTowerError()
			}
			bound := false
			for _, hero := range party.Formation {
				if hero.GetGuid() == hit.ActorGUID && hero.GetPose() == int32(hit.ActorPose) {
					if !proto.Equal(hero, &frozen) {
						return nil, nil, p13UFTowerError()
					}
					bound = true
					break
				}
			}
			if !bound {
				return nil, nil, p13UFTowerError()
			}
			alive := false
			for j, pose := range current.GetFightInfo().GetIndex() {
				if pose == int32(hit.ActorPose) && j < len(current.FightInfo.HpPos) && current.FightInfo.HpPos[j] > 0 {
					alive = true
				}
			}
			if !alive || current.GetState() == 3 {
				reason = constUtil.P13UnionFightTowerCancelActor
			}
		}
		if reason != "" {
			hit.Status, hit.CancelReason, hit.AppliedAt = constUtil.P13UnionFightTowerHitCanceled, reason, now.UnixMilli()
			hit.CancelFrame, _ = proto.MarshalOptions{Deterministic: true}.Marshal(current)
			hit.CancelFrameHash = p6Digest(hit.CancelFrame)
			if actor.LeftAt > 0 {
				hit.CancelLeftAt = actor.LeftAt
			}
		} else {
			after, revive, err := p13UFTowerApplyFrozen(hit, current, now.UnixMilli())
			if err != nil {
				return nil, nil, err
			}
			current = after
			frames = append(frames, p13UFTowerPlayerEffect(hit, current, actor.X, actor.Y, actor.Z)...)
			if revive > 0 {
				frames = append(frames, &pb.SCUFPlayerDead{Guid: []int64{actor.PlayerID}, ReviveTime: []float64{float64(revive)}, ReviveItemId: []string{p13UFGuardReviveItem(current)}})
			}
		}
		hit.HitHash = p13UFTowerHitHash(hit)
		if err := p13UFTowerHitVerify(hit, tower); err != nil {
			return nil, nil, err
		}
		result := tx.Model(&db.P13UnionFightTowerHit{}).Where("match_id=? AND tower_guid=? AND sequence=? AND status=? AND hit_hash=?", hit.MatchID, hit.TowerGUID, hit.Sequence, constUtil.P13UnionFightTowerHitPending, oldHash).Select("*").Updates(hit)
		if result.Error != nil {
			return nil, nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, nil, p13UFTowerError()
		}
	}
	return current, frames, nil
}

// The next capture/action gate must reject any pending hit for this life until
// its current occupancy has atomically drained. No HP/AP is modified here.
func p13UFTowerPendingLife(tx *gorm.DB, scope string, id int64, life int32) error {
	var pendingAura int64
	if err := tx.Model(&db.P13UnionFightHealAuraPending{}).Where("match_id=? AND player_id=? AND life_ordinal=? AND status=?", scope, id, life, constUtil.P13UFHealAuraPending).Limit(1).Count(&pendingAura).Error; err != nil {
		return err
	}
	if pendingAura > 0 {
		return errors.New("回复法阵环境事件正在结算")
	}

	var hit db.P13UnionFightTowerHit
	err := tx.Select("tower_guid").Where("match_id=? AND target_kind=? AND target_guid=? AND life_ordinal=? AND status=?", scope, "player", id, life, constUtil.P13UnionFightTowerHitPending).Limit(1).First(&hit).Error
	if err == nil {
		return errors.New(constUtil.P13UnionFightTowerPending)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var tower db.P13UnionFightTowerState
	err = tx.Select("tower_guid").Where("match_id=? AND player_damage_rule=? AND pending_target_kind=? AND pending_target_guid=? AND pending_target_life=? AND pending_release_at>0", scope, constUtil.P13UnionFightTowerPlayerDamageRule, "player", id, life).Limit(1).First(&tower).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New(constUtil.P13UnionFightTowerPending)
}

func p13UFTowerClearLaunch(row *db.P13UnionFightTowerState) {
	row.PendingTargetLife, row.PendingActorPose, row.PendingOccupancyDueAt = 0, 0, 0
	row.PendingActorGUID, row.PendingFrozenActorHash, row.PendingLaunchFrameHash = "", "", ""
	row.PendingFrozenActor, row.PendingLaunchFrame = nil, nil
	row.PendingOccupancyKind, row.PendingOccupancyID, row.PendingOccupancyInputHash = "", "", ""
}
func p13UFTowerLaunchValid(row *db.P13UnionFightTowerState) bool {
	blank := row.PendingTargetLife == 0 && row.PendingActorPose == 0 && row.PendingActorGUID == "" && len(row.PendingFrozenActor) == 0 && row.PendingFrozenActorHash == "" && len(row.PendingLaunchFrame) == 0 && row.PendingLaunchFrameHash == "" && row.PendingOccupancyKind == "" && row.PendingOccupancyID == "" && row.PendingOccupancyInputHash == "" && row.PendingOccupancyDueAt == 0
	if row.PlayerDamageRule == "" || row.PendingReleaseAt == 0 || row.PendingTargetKind != "player" {
		return blank
	}
	var actor pb.BattleHero
	var frame pb.InstUFPlayer
	if row.PendingTargetLife < 0 || row.PendingActorGUID == "" || row.PendingActorPose < 1 || row.PendingActorPose > 15 || row.PendingFrozenActorHash != p6Digest(row.PendingFrozenActor) || row.PendingLaunchFrameHash != p6Digest(row.PendingLaunchFrame) || proto.Unmarshal(row.PendingFrozenActor, &actor) != nil || proto.Unmarshal(row.PendingLaunchFrame, &frame) != nil || actor.GetGuid() != row.PendingActorGUID || int64(actor.GetPose()) != row.PendingActorPose || int64(frame.GetDeadNum()) != row.PendingTargetLife || frame.GetGuid() != row.PendingTargetGUID || frame.GetCamp() == int32(row.Camp) {
		return false
	}
	if frame.GetState() == 1 {
		return (row.PendingOccupancyKind == constUtil.P13UnionFightTowerOccupancyOrdinary || row.PendingOccupancyKind == constUtil.P13UnionFightTowerOccupancyGuard) && len(row.PendingOccupancyID) == 64 && len(row.PendingOccupancyInputHash) == 64 && row.PendingOccupancyDueAt > 0
	}
	return (frame.GetState() == 0 || frame.GetState() == 2 || frame.GetState() == 5) && row.PendingOccupancyID == "" && row.PendingOccupancyKind == "" && row.PendingOccupancyInputHash == "" && row.PendingOccupancyDueAt == 0
}
func p13UFTowerFreezeLaunch(tower *db.P13UnionFightTowerState, target *p13UFTowerTarget) error {
	hero, _, err := p13UFTowerFreezeActor(target.player, target.frame, tower)
	if err != nil {
		return err
	}
	tower.PendingActorGUID = hero.GetGuid()
	tower.PendingActorPose = int64(hero.GetPose())
	tower.PendingTargetLife = int64(target.frame.GetDeadNum())
	tower.PendingFrozenActor, err = proto.MarshalOptions{Deterministic: true}.Marshal(hero)
	if err != nil {
		return err
	}
	tower.PendingFrozenActorHash = p6Digest(tower.PendingFrozenActor)
	tower.PendingLaunchFrame, err = proto.MarshalOptions{Deterministic: true}.Marshal(target.frame)
	if err != nil {
		return err
	}
	tower.PendingLaunchFrameHash = p6Digest(tower.PendingLaunchFrame)
	if target.frame.GetState() == 1 {
		tower.PendingOccupancyKind, tower.PendingOccupancyID, tower.PendingOccupancyInputHash, tower.PendingOccupancyDueAt = target.occupancyKind, target.occupancyID, target.occupancyInputHash, target.occupancyDueAt
	}
	if !p13UFTowerLaunchValid(tower) {
		return p13UFTowerError()
	}
	return nil
}

// Preload only the current bounded occupancy directory, once per match. Full
// hit blobs stay on the verification/drain path instead of an inner tower read.
func p13UFTowerOccupancyShots(tx *gorm.DB, scope string, targets []*p13UFTowerTarget) error {
	ids, occupancies := []int64{}, []string{}
	directory := map[int64]*p13UFTowerTarget{}
	for _, target := range targets {
		if target.occupancyID != "" {
			ids = append(ids, target.guid)
			occupancies = append(occupancies, target.occupancyID)
			directory[target.guid] = target
			target.occupancyShots = map[int64]string{}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var rows []db.P13UnionFightTowerHit
	if err := tx.Select("tower_guid,target_guid,occupancy_kind,occupancy_id,occupancy_input_hash,occupancy_due_at,immutable_hash").Where("match_id=? AND target_kind=? AND target_guid IN ? AND occupancy_id IN ? AND status IN ?", scope, "player", ids, occupancies, []string{constUtil.P13UnionFightTowerHitPending, constUtil.P13UnionFightTowerHitApplied}).Limit(constUtil.P13UnionFightTowerPlayerLimit*6 + 1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) > constUtil.P13UnionFightTowerPlayerLimit*6 {
		return p13UFTowerError()
	}
	for _, row := range rows {
		target := directory[row.TargetGUID]
		if target == nil || target.occupancyKind != row.OccupancyKind || target.occupancyID != row.OccupancyID {
			continue
		}
		if row.OccupancyInputHash != target.occupancyInputHash || row.OccupancyDueAt != target.occupancyDueAt || len(row.ImmutableHash) != 64 || target.occupancyShots[row.TowerGUID] != "" {
			return p13UFTowerError()
		}
		target.occupancyShots[row.TowerGUID] = row.ImmutableHash
	}
	return nil
}
