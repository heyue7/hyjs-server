package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
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

type p12WantedDefinition struct {
	Tables             map[string][]map[string]string `json:"tables"`
	Settings           map[string]string              `json:"settings"`
	SupportedWantedIDs []string                       `json:"supportedWantedIds"`
	SelectionRule      string                         `json:"selectionRule,omitempty"`
	Rule               string                         `json:"rule"`
	RewardRule         string                         `json:"rewardRule"`
	RewardSource       string                         `json:"rewardSource"`
}

func (s *GameService) LoadP12WantedTables(root string) error {
	for _, name := range []string{"WantedEnemy", "WantedLevel", "WantedTimeLimit", "WantedSpecialTime", "WantedDamageRank", "WantedPerformanceRank"} {
		if err := s.loadP7Table(root, name); err != nil {
			return err
		}
	}
	return nil
}

// InitializeP12Wanted 采用用户已定同级/最近低级与原难度权重，保留旧冻结实例。
func (s *GameService) InitializeP12Wanted(ctx context.Context) error {
	def := p12WantedDefinition{Tables: map[string][]map[string]string{}, Settings: map[string]string{}, SelectionRule: constUtil.P12WantedCatalogSelectionRule, Rule: constUtil.P12WantedLocalRule, RewardRule: constUtil.P12WantedRewardRule, RewardSource: constUtil.P12WantedRewardSource}
	for _, name := range []string{"WantedEnemy", "WantedLevel", "WantedTimeLimit", "WantedSpecialTime", "WantedDamageRank", "WantedPerformanceRank"} {
		if len(s.Tables[name]) == 0 {
			return errors.New(constUtil.P12WantedInvalid)
		}
		def.Tables[name] = s.Tables[name]
	}
	for _, name := range []string{"WantedMissionLevel", "MaxWantedAttTimes", "WantedAttRecoveryTime", "WantedAttMultiples"} {
		row := s.table("GameConfigure", "StaticName", name)
		if row == nil {
			return errors.New(constUtil.P12WantedInvalid)
		}
		def.Settings[name] = row["NameValue"]
	}
	if def.Settings["WantedMissionLevel"] != "35" || def.Settings["MaxWantedAttTimes"] != "10" || def.Settings["WantedAttRecoveryTime"] != "1" || def.Settings["WantedAttMultiples"] != "2.5" {
		return errors.New(constUtil.P12WantedInvalid)
	}
	ids, err := p12WantedCatalogIDs(def)
	if err != nil {
		return err
	}
	def.SupportedWantedIDs = ids
	if !p12WantedDefinitionSelectionValid(def) {
		return errors.New(constUtil.P12WantedInvalid)
	}
	if _, _, _, err := s.p12WantedMonster(def, "1", nil); err != nil {
		return err
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err = tx.AutoMigrate(&db.P12WantedPolicy{}, &db.P12WantedInstance{}, &db.P12WantedDiscovery{}, &db.P12WantedFight{}, &db.P12WantedRewardClaim{}); err != nil {
		return err
	}
	if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P12WantedPolicy{ID: 1, DefinitionJSON: string(raw), DefinitionHash: p6Digest(raw), Source: constUtil.P12WantedLocalSource}).Error; err != nil {
		return err
	}
	return tx.Transaction(func(locked *gorm.DB) error {
		var prior db.P12WantedPolicy
		if err := locked.Clauses(clause.Locking{Strength: "UPDATE"}).First(&prior, 1).Error; err != nil {
			return err
		}
		upgrade, err := p12WantedDefaultCatalogUpgrade(&dbWantedCatalogPolicyView{Source: prior.Source, Hash: prior.DefinitionHash, JSON: prior.DefinitionJSON}, def)
		if err != nil || !upgrade {
			return err
		}
		return locked.Model(&prior).Updates(map[string]any{"definition_json": string(raw), "definition_hash": p6Digest(raw)}).Error
	})
}

func p12WantedConfig(tx *gorm.DB) (*db.P12WantedPolicy, p12WantedDefinition, error) {
	var (
		policy db.P12WantedPolicy
		def    p12WantedDefinition
	)
	if err := tx.First(&policy, 1).Error; err != nil {
		return nil, def, err
	}
	if policy.Source != constUtil.P12WantedLocalSource || policy.DefinitionHash != p6Digest([]byte(policy.DefinitionJSON)) || json.Unmarshal([]byte(policy.DefinitionJSON), &def) != nil || def.Rule != constUtil.P12WantedLocalRule || def.RewardRule != constUtil.P12WantedRewardRule || def.RewardSource != constUtil.P12WantedRewardSource || !p12WantedDefinitionSelectionValid(def) {
		return nil, def, errors.New(constUtil.P12WantedInvalid)
	}
	return &policy, def, nil
}

func p12WantedPeriod(def p12WantedDefinition, now time.Time) (map[string]string, error) {
	hour := now.In(time.FixedZone("UTC+8", 8*3600)).Hour()
	var found map[string]string
	for _, row := range def.Tables["WantedSpecialTime"] {
		parts := strings.Split(row["StartTime"], "-")
		if len(parts) != 2 {
			return nil, errors.New(constUtil.P12WantedInvalid)
		}
		first, e1 := strconv.Atoi(parts[0])
		last, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil || first < 0 || last > 23 || first > last {
			return nil, errors.New(constUtil.P12WantedInvalid)
		}
		if first <= hour && hour <= last {
			if found != nil {
				return nil, errors.New(constUtil.P12WantedInvalid)
			}
			found = row
		}
	}
	if found == nil {
		return nil, errors.New(constUtil.P12WantedInvalid)
	}
	return found, nil
}

// p12WantedMonster 按原GroupIndex保留全部波次、技能和部位；当前HP只来自实例存档。
func (s *GameService) p12WantedMonster(def p12WantedDefinition, wantedID string, current map[string]float64) ([]*pb.BattleMonster, []*pb.RebelMonster, float64, error) {
	groups := map[int][]*pb.BattleHero{}
	var (
		view  []*pb.RebelMonster
		total float64
	)
	seen := map[string]bool{}
	for _, row := range def.Tables["WantedEnemy"] {
		if row["WantedID"] != wantedID {
			continue
		}
		group, e1 := strconv.Atoi(row["GroupIndex"])
		pose, e2 := strconv.ParseInt(row["Location"], 10, 32)
		if e1 != nil || e2 != nil || group < 1 || pose < 1 || pose > 15 || seen[row["WantedEnemyId"]] {
			return nil, nil, 0, errors.New(constUtil.P12WantedInvalid)
		}
		seen[row["WantedEnemyId"]] = true
		actor, err := s.actor(row, "wanted-"+strings.TrimPrefix(row["WantedEnemyId"], "wanted_"), int32(pose), true)
		if err != nil || actor.GetProp().GetHp() <= 0 {
			return nil, nil, 0, errors.New(constUtil.P12WantedInvalid)
		}
		hp := actor.GetProp().GetHp()
		if current != nil {
			value, ok := current[row["WantedEnemyId"]]
			if !ok || !combatFinite(value) || value < 0 || value > hp {
				return nil, nil, 0, errors.New(constUtil.P12WantedInvalid)
			}
			actor.Inst.CurHp = proto.Float64(value)
		}
		groups[group] = append(groups[group], actor)
		view = append(view, &pb.RebelMonster{WantedEnemyId: proto.String(row["WantedEnemyId"]), Hp: proto.Float64(actor.GetInst().GetCurHp())})
		total += hp
	}
	if len(groups) == 0 || current != nil && len(current) != len(seen) {
		return nil, nil, 0, errors.New(constUtil.P12WantedInvalid)
	}
	keys := make([]int, 0, len(groups))
	for group := range groups {
		keys = append(keys, group)
	}
	sort.Ints(keys)
	waves := make([]*pb.BattleMonster, 0, len(keys))
	for index, group := range keys {
		if group != index+1 {
			return nil, nil, 0, errors.New(constUtil.P12WantedInvalid)
		}
		waves = append(waves, &pb.BattleMonster{Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}, Formation: groups[group]})
	}
	return waves, view, total, nil
}

func p12WantedDefinitionRow(def p12WantedDefinition, wantedID string) (map[string]string, map[string]string, error) {
	var task, limit map[string]string
	for _, row := range def.Tables["WantedLevel"] {
		if row["WantedID"] == wantedID {
			task = row
		}
	}
	if task == nil {
		return nil, nil, errors.New(constUtil.P12WantedInvalid)
	}
	for _, row := range def.Tables["WantedTimeLimit"] {
		if row["Difficulty"] == task["Difficulty"] {
			limit = row
		}
	}
	if limit == nil {
		return nil, nil, errors.New(constUtil.P12WantedInvalid)
	}
	return task, limit, nil
}

func (s *GameService) p12WantedView(instance *db.P12WantedInstance) (*pb.RebelInfo, error) {
	var (
		def p12WantedDefinition
		hp  map[string]float64
	)
	if instance.DefinitionHash != p6Digest([]byte(instance.DefinitionJSON)) || json.Unmarshal([]byte(instance.DefinitionJSON), &def) != nil || json.Unmarshal([]byte(instance.MonsterHPJSON), &hp) != nil {
		return nil, errors.New(constUtil.P12WantedInvalid)
	}
	_, monsters, total, err := s.p12WantedMonster(def, instance.WantedID, hp)
	if err != nil || total != instance.MaxHP {
		return nil, errors.New(constUtil.P12WantedInvalid)
	}
	current := float64(0)
	for _, monster := range monsters {
		current += monster.GetHp()
	}
	if current != instance.CurrentHP {
		return nil, errors.New(constUtil.P12WantedInvalid)
	}
	state := int32(0)
	if instance.SharedAt > 0 {
		state = 1
	}
	return &pb.RebelInfo{Id: proto.String(instance.ID), Finder: proto.Int64(instance.FinderID), FinderName: proto.String(instance.FinderName), RunTime: proto.Int64(instance.ExpiresAt), Hp: proto.Float64(current), MaxHP: proto.Float64(total), Level: proto.Int32(int32(instance.Level)), WantedId: proto.String(instance.WantedID), Difficulty: proto.Int32(int32(instance.Difficulty)), State: proto.Int32(state), Monsters: monsters}, nil
}

// DiscoverP12Wanted 由已成功提交的原副本/扫荡事务调用；失败和重发不得重新抽发现。
func (s *GameService) DiscoverP12Wanted(tx *gorm.DB, id int64, player *pb.Player, game *model.GameState, businessSource string, now time.Time) (*pb.RebelInfo, *pb.ResultObject, error) {
	if !p1SourceValid(businessSource) || !(strings.HasPrefix(businessSource, "copy-complete:") || strings.HasPrefix(businessSource, "copy-sweep:")) {
		return nil, nil, errors.New(constUtil.P12WantedInvalid)
	}
	policy, def, err := p12WantedConfig(tx)
	if err != nil {
		return nil, nil, err
	}
	if !policy.Enabled || player.GetLevel() < 35 {
		return nil, &pb.ResultObject{}, nil
	}
	key := p6Digest([]byte(businessSource))
	var old db.P12WantedDiscovery
	err = tx.First(&old, "player_id=? AND source_key=?", id, key).Error
	if err == nil {
		if old.BusinessSource != businessSource {
			return nil, nil, errors.New(constUtil.P12WantedInvalid)
		}
		if old.InstanceID == "" {
			return nil, &pb.ResultObject{}, nil
		}
		var instance db.P12WantedInstance
		if err = tx.First(&instance, "id=?", old.InstanceID).Error; err != nil {
			return nil, nil, err
		}
		view, e := s.p12WantedView(&instance)
		// 原副本自己的完整回执保存第一次Gain；此处重查只返回关联，不再发发现钻。
		return view, &pb.ResultObject{}, e
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	period, err := p12WantedPeriod(def, now)
	if err != nil {
		return nil, nil, err
	}
	odds, err := strconv.ParseInt(period["Odd"], 10, 64)
	if err != nil || odds < 0 || odds > 100 {
		return nil, nil, errors.New(constUtil.P12WantedInvalid)
	}
	record := db.P12WantedDiscovery{PlayerID: id, SourceKey: key, BusinessSource: businessSource, CreatedAt: now.UnixMilli(), Draw: -1, Odds: odds, Reason: "no-match", Source: constUtil.P12WantedLocalSource}
	var active int64
	if err = tx.Model(&db.P12WantedInstance{}).Where("finder_id=? AND server_id=? AND state=? AND created_at<=? AND expires_at>?", id, s.OriginalServerID(), constUtil.P12WantedActive, now.UnixMilli(), now.UnixMilli()).Count(&active).Error; err != nil {
		return nil, nil, err
	}
	if active > 0 {
		record.Reason = "active-owner-cap"
		return nil, &pb.ResultObject{}, tx.Create(&record).Error
	}
	draw, err := rand.Int(rand.Reader, big.NewInt(100))
	if err != nil {
		return nil, nil, err
	}
	record.Draw = draw.Int64()
	if record.Draw >= odds {
		return nil, &pb.ResultObject{}, tx.Create(&record).Error
	}
	// Historical frozen definitions retain their original single-ID choice.
	wantedID := def.SupportedWantedIDs[0]
	if def.SelectionRule == constUtil.P12WantedCatalogSelectionRule {
		wantedID, err = p12WantedRandomCatalog(def, player.GetLevel())
		if err != nil {
			return nil, nil, err
		}
	}
	task, limit, err := p12WantedDefinitionRow(def, wantedID)
	if err != nil {
		return nil, nil, err
	}
	minutes, e1 := strconv.ParseInt(limit["TimeLimit"], 10, 64)
	level, e2 := strconv.ParseInt(task["Level"], 10, 64)
	difficulty, e3 := strconv.ParseInt(task["Difficulty"], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || minutes < 1 || minutes > 1440 {
		return nil, nil, errors.New(constUtil.P12WantedInvalid)
	}
	_, monsters, total, err := s.p12WantedMonster(def, wantedID, nil)
	if err != nil {
		return nil, nil, err
	}
	hps := map[string]float64{}
	for _, monster := range monsters {
		hps[monster.GetWantedEnemyId()] = monster.GetHp()
	}
	rawHP, err := json.Marshal(hps)
	if err != nil {
		return nil, nil, err
	}
	instance := db.P12WantedInstance{ID: uuid.NewString(), ServerID: s.OriginalServerID(), FinderID: id, FinderName: player.GetName(), WantedID: wantedID, Level: level, Difficulty: difficulty, State: constUtil.P12WantedActive, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Duration(minutes) * time.Minute).UnixMilli(), CurrentHP: total, MaxHP: total, MonsterHPJSON: string(rawHP), DefinitionJSON: policy.DefinitionJSON, DefinitionHash: policy.DefinitionHash}
	if err = tx.Create(&instance).Error; err != nil {
		return nil, nil, err
	}
	gain, err := s.GrantFixedReward(player, game, limit["AcceptReward"])
	if err != nil {
		return nil, nil, err
	}
	record.InstanceID, record.Reward, record.Reason = instance.ID, limit["AcceptReward"], "discovered"
	if err = tx.Create(&record).Error; err != nil {
		return nil, nil, err
	}
	if err = s.RecordP1Event(tx, id, "wanted:find:"+instance.ID, "wanted_found", 1, now); err != nil {
		return nil, nil, err
	}
	view, err := s.p12WantedView(&instance)
	return view, gain, err
}

func p12WantedRecover(player *pb.Player, now time.Time) error {
	if player.GetWantedAttTimes() < 0 || player.GetWantedAttTime() < 0 {
		return errors.New(constUtil.P12WantedInvalid)
	}
	if player.GetWantedAttTime() == 0 {
		player.WantedAttTime = proto.Int64(now.UnixMilli())
		player.WantedAttTimes = proto.Int32(max(10, player.GetWantedAttTimes()))
		return nil
	}
	if now.UnixMilli() < player.GetWantedAttTime() {
		return errors.New(constUtil.P12WantedInvalid)
	}
	if player.GetWantedAttTimes() >= 10 {
		player.WantedAttTime = proto.Int64(now.UnixMilli())
		return nil
	}
	steps := (now.UnixMilli() - player.GetWantedAttTime()) / 3600000
	if steps > 0 {
		player.WantedAttTimes = proto.Int32(int32(min(10, int64(player.GetWantedAttTimes())+steps)))
		player.WantedAttTime = proto.Int64(player.GetWantedAttTime() + steps*3600000)
	}
	return nil
}

// p12WantedProfile preserves the original profile lock and commits successful Wanted mutations.
// p3Profile intentionally delegates saving to each domain callback.
func (s *GameService) p12WantedProfile(ctx context.Context, id int64, fn func(*gorm.DB, *db.Profile, *pb.Player, *model.GameState) error) error {
	return s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState) error {
		if err := fn(tx, row, player, game); err != nil {
			return err
		}
		return saveProfile(tx, row, player, game)
	})
}

func (s *GameService) p12WantedList(ctx context.Context, id int64, now time.Time) ([]proto.Message, error) {
	out := &pb.SCGetRebels{}
	var playerOut *pb.Player
	err := s.p12WantedProfile(ctx, id, func(tx *gorm.DB, _ *db.Profile, player *pb.Player, game *model.GameState) error {
		policy, def, err := p12WantedConfig(tx)
		if err != nil {
			return err
		}
		if !policy.Enabled || player.GetLevel() < 35 {
			// The original strategy hub requests this list before Wanted unlocks.
			// Return an empty directory without granting attempts or combat eligibility.
			playerOut = proto.Clone(player).(*pb.Player)
			return nil
		}
		if err = p12WantedRecover(player, now); err != nil {
			return err
		}
		period, err := p12WantedPeriod(def, now)
		if err != nil {
			return err
		}
		for key, target := range map[string]**int32{"NormalAttackCost": &out.NormalWantedAttTimes, "RageAttackCost": &out.RageWantedAttTimes} {
			values, e := p3Resources(period[key])
			if e != nil || len(values) != 1 || values[0].GetId() != "wantedAttTimes" {
				return errors.New(constUtil.P12WantedInvalid)
			}
			*target = proto.Int32(values[0].GetNum())
		}
		out.NormalRage, out.Rage = proto.Float64(1), proto.Float64(2.5)
		_, own, performanceRank, damageRank, err := p12WantedDailyTotals(tx, s.OriginalServerID(), id, now)
		if err != nil {
			return err
		}
		out.Exploit, out.ExploitNum, out.MaxHurt, out.MaxHurtNum = proto.Int32(int32(min(own.Performance, math.MaxInt32))), proto.Int32(performanceRank), proto.Int32(int32(min(own.Damage, math.MaxInt32))), proto.Int32(damageRank)
		var instances []db.P12WantedInstance
		if err = tx.Where("server_id=? AND state=? AND created_at<=? AND expires_at>? AND (finder_id=? OR (shared_at>0 AND shared_at<=?))", s.OriginalServerID(), constUtil.P12WantedActive, now.UnixMilli(), now.UnixMilli(), id, now.UnixMilli()).Order("created_at,id").Find(&instances).Error; err != nil {
			return err
		}
		for index := range instances {
			view, e := s.p12WantedView(&instances[index])
			if e != nil {
				return e
			}
			out.Rebels = append(out.Rebels, view)
		}
		playerOut = proto.Clone(player).(*pb.Player)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{out, &pb.SCPlayer{Player: playerOut}}, nil
}

func (s *GameService) p12WantedShare(ctx context.Context, id int64, request *pb.CSShareRebel, now time.Time) ([]proto.Message, error) {
	err := s.p12WantedProfile(ctx, id, func(tx *gorm.DB, _ *db.Profile, player *pb.Player, _ *model.GameState) error {
		policy, _, err := p12WantedConfig(tx)
		if err != nil {
			return err
		}
		if !policy.Enabled || player.GetLevel() < 35 || request.GetRebelId() == "" {
			return errors.New(constUtil.P12WantedUnavailable)
		}
		var instance db.P12WantedInstance
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, "id=?", request.GetRebelId()).Error; err != nil {
			return err
		}
		if instance.FinderID != id || instance.ServerID != s.OriginalServerID() || instance.State != constUtil.P12WantedActive || instance.CreatedAt > now.UnixMilli() || instance.ExpiresAt <= now.UnixMilli() || instance.CurrentHP <= 0 || instance.CurrentHP >= instance.MaxHP {
			return errors.New(constUtil.P12WantedUnavailable)
		}
		if instance.SharedAt > 0 {
			return nil
		}
		var count int64
		if err = tx.Model(&db.P12WantedFight{}).Where("instance_id=? AND player_id=? AND committed=? AND applied_damage>0", instance.ID, id, true).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New(constUtil.P12WantedUnavailable)
		}
		instance.SharedAt = now.UnixMilli()
		return tx.Save(&instance).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCShareRebel{}}, nil
}

// BuildP12WantedStart 冻结出口保留原完整波次；独立type8数值准入由调用者验原证明。
func (s *GameService) BuildP12WantedStart(player *pb.Player, game *model.GameState, instance *db.P12WantedInstance, attackType int32, seed int64) (*pb.SCRebelFightStart, *model.FrozenBattleInput, error) {
	if attackType != 1 && attackType != 2 || seed < 0 || seed > math.MaxInt32 {
		return nil, nil, errors.New(constUtil.P12WantedInvalid)
	}
	var (
		def p12WantedDefinition
		hps map[string]float64
	)
	if instance.DefinitionHash != p6Digest([]byte(instance.DefinitionJSON)) || json.Unmarshal([]byte(instance.DefinitionJSON), &def) != nil || json.Unmarshal([]byte(instance.MonsterHPJSON), &hps) != nil {
		return nil, nil, errors.New(constUtil.P12WantedInvalid)
	}
	party, err := s.battlePlayer(player, game)
	if err != nil {
		return nil, nil, err
	}
	party.Inst.FirstOrder = nil
	waves, _, _, err := s.p12WantedMonster(def, instance.WantedID, hps)
	if err != nil {
		return nil, nil, err
	}
	task, _, err := p12WantedDefinitionRow(def, instance.WantedID)
	if err != nil {
		return nil, nil, err
	}
	factor := float64(1)
	if attackType == 2 {
		factor = 2.5
	}
	start := &pb.SCRebelFightStart{FightId: proto.String(uuid.NewString()), RandSeed: proto.Int64(seed), BattlePlayer: party, BattleMonsterList: waves, FightRoomId: proto.String(task["FightRoomId"]), DamageFactor: proto.Float64(factor), Maxround: proto.Int32(20)}
	if len(waves) != 1 {
		return nil, nil, errors.New("悬赏原多波已完整保留，当前FrozenBattleInput单波容器不能截断后授予证明")
	}
	proof := constUtil.P12WantedNativeProof
	for _, actor := range waves[0].Formation {
		if actor.Inst.GetCurHp() != actor.Prop.GetHp() {
			proof = constUtil.P12WantedPartialHPProof
		}
	}
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: party, Monster: waves[0], Seed: seed, MaxRound: 20, BattleType: 8, AttackerDamageFactor: proto.Float64(factor), DefenderDamageFactor: proto.Float64(1), AppliedProperties: map[string][]string{constUtil.P12WantedNativeScope: {proof}, "localWantedRule": {def.Rule, instance.ID, fmt.Sprint(attackType)}}})
	if err != nil {
		return nil, nil, err
	}
	if def.SelectionRule == constUtil.P12WantedCatalogSelectionRule {
		input.AppliedProperties[constUtil.P12WantedCatalogScope] = []string{def.SelectionRule, instance.WantedID}
	}
	proof, err = P12WantedProofForInput(input)
	if err != nil {
		return nil, nil, err
	}
	input.AppliedProperties[constUtil.P12WantedNativeScope] = []string{proof}
	return start, input, nil
}

func (s *GameService) DispatchP12Wanted(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	now := LocalPlayNow(ctx)
	switch message := request.(type) {
	case *pb.CSGetRebels:
		out, err := s.p12WantedList(ctx, id, now)
		return out, true, err
	case *pb.CSShareRebel:
		out, err := s.p12WantedShare(ctx, id, message, now)
		return out, true, err
	case *pb.CSRebelFightStart:
		out, err := s.p12WantedStart(ctx, id, message, stamp, now)
		return out, true, err
	case *pb.CSRebelFightEnd:
		out, err := s.p12WantedEnd(ctx, id, message, now)
		return out, true, err
	case *pb.CSGetRebelRewardInfo:
		out, err := s.p12WantedDailyReward(ctx, id, 0, now)
		return out, true, err
	case *pb.CSGetRebelReward:
		if message.GetRebelRewardId() < 1 || message.GetRebelRewardId() > 2 {
			return nil, true, errors.New(constUtil.P12WantedInvalid)
		}
		out, err := s.p12WantedDailyReward(ctx, id, message.GetRebelRewardId(), now)
		return out, true, err
	}
	return nil, false, nil
}

func (s *GameService) p12WantedStart(ctx context.Context, id int64, request *pb.CSRebelFightStart, stamp int64, now time.Time) ([]proto.Message, error) {
	var out pb.SCRebelFightStart
	err := s.p12WantedProfile(ctx, id, func(tx *gorm.DB, _ *db.Profile, player *pb.Player, game *model.GameState) error {
		if request.GetRebelId() == "" || request.GetType() != 1 && request.GetType() != 2 {
			return errors.New(constUtil.P12WantedUnavailable)
		}
		var err error
		// 原正时间戳绑定首次Start；结束后的网络重发也不创建新战斗或再次扣次数。
		if stamp > 0 {
			var prior db.P12WantedFight
			found := tx.Where("player_id=? AND stamp=?", id, stamp).Order("created_at,id").First(&prior).Error
			if found == nil {
				if prior.AbandonedAt > 0 || prior.InstanceID != request.GetRebelId() || prior.AttackType != int64(request.GetType()) || prior.StartHash != p6Digest(prior.StartProto) || proto.Unmarshal(prior.StartProto, &out) != nil {
					return errors.New(constUtil.P12WantedInvalid)
				}
				return nil
			}
			if !errors.Is(found, gorm.ErrRecordNotFound) {
				return found
			}
		}
		var pending db.P12WantedFight
		found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND committed=? AND abandoned_at=0 AND created_at<=?", id, false, now.UnixMilli()).Order("created_at,id").First(&pending).Error
		if found == nil {
			var priorInstance db.P12WantedInstance
			if err = tx.First(&priorInstance, "id=?", pending.InstanceID).Error; err != nil {
				return err
			}
			if priorInstance.ExpiresAt > now.UnixMilli() && priorInstance.State == constUtil.P12WantedActive && priorInstance.CurrentHP > 0 {
				if pending.InstanceID != request.GetRebelId() || pending.AttackType != int64(request.GetType()) || pending.StartHash != p6Digest(pending.StartProto) || proto.Unmarshal(pending.StartProto, &out) != nil {
					return errors.New(constUtil.P12WantedInvalid)
				}
				return nil
			}
			// 原任务已逃跑/击杀时释放未提交槽；不扣共享HP、不补奖、不退已消耗次数。
			pending.AbandonedAt = now.UnixMilli()
			if err = tx.Save(&pending).Error; err != nil {
				return err
			}
		}
		if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
			return found
		}
		// 已付费的同包/仍存活pending在前段恢复；新场才执行当前政策和跨玩法互斥。
		policy, def, err := p12WantedConfig(tx)
		if err != nil {
			return err
		}
		if !policy.Enabled || player.GetLevel() < 35 || game.Run != nil && !game.Run.Complete || game.Field != nil && game.Field.Fight != nil && !game.Field.Fight.Settled {
			return errors.New(constUtil.P12WantedUnavailable)
		}
		if err = s.ensureNoActiveDailyCopy(tx, id, now); err != nil {
			return err
		}
		var instance db.P12WantedInstance
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, "id=?", request.GetRebelId()).Error; err != nil {
			return err
		}
		if instance.ServerID != s.OriginalServerID() || instance.FinderID != id && (instance.SharedAt == 0 || instance.SharedAt > now.UnixMilli()) || instance.State != constUtil.P12WantedActive || instance.CreatedAt > now.UnixMilli() || instance.ExpiresAt <= now.UnixMilli() || instance.CurrentHP <= 0 {
			return errors.New(constUtil.P12WantedUnavailable)
		}
		if err = p12WantedRecover(player, now); err != nil {
			return err
		}
		period, err := p12WantedPeriod(def, now)
		if err != nil {
			return err
		}
		costKey := "NormalAttackCost"
		if request.GetType() == 2 {
			costKey = "RageAttackCost"
		}
		costs, err := p3Resources(period[costKey])
		if err != nil || len(costs) != 1 || costs[0].GetId() != "wantedAttTimes" || costs[0].GetNum() < 1 || player.GetWantedAttTimes() < costs[0].GetNum() {
			return errors.New(constUtil.P12WantedUnavailable)
		}
		seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32+1))
		if err != nil {
			return err
		}
		start, input, err := s.BuildP12WantedStart(player, game, &instance, request.GetType(), seed.Int64())
		if err != nil {
			return err
		}
		if _, err = P12WantedProofForInput(input); err != nil {
			return err
		}
		// 验证独立type8原自动驱动，客户端winner/血量/伤害不参与计算。
		input.AppliedProperties["wantedSettlementIndices"] = []string{period["PerformanceIndex"], period["CoreIndex"], constUtil.P12WantedRewardRule}
		terminal, err := s.ComputeP12WantedProtocolFinalHP(ctx, input)
		if err != nil {
			return err
		}
		if terminal == nil || terminal.Outcome == nil || terminal.OriginalOperates == nil || len(terminal.FinalActors) != len(input.Attacker.Formation)+len(input.Monster.Formation) {
			return errors.New(constUtil.P12WantedInvalid)
		}
		rawInput, err := json.Marshal(input)
		if err != nil {
			return err
		}
		rawTerminal, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		rawStart, err := proto.Marshal(start)
		if err != nil {
			return err
		}
		if player.GetWantedAttTimes() >= 10 {
			player.WantedAttTime = proto.Int64(now.UnixMilli())
		}
		player.WantedAttTimes = proto.Int32(player.GetWantedAttTimes() - costs[0].GetNum())
		fight := db.P12WantedFight{ID: start.GetFightId(), PlayerID: id, InstanceID: instance.ID, Stamp: stamp, AttackType: int64(request.GetType()), Cost: int64(costs[0].GetNum()), CreatedAt: now.UnixMilli(), InputJSON: string(rawInput), InputHash: p6Digest(rawInput), TerminalJSON: string(rawTerminal), TerminalHash: p6Digest(rawTerminal), StartProto: rawStart, StartHash: p6Digest(rawStart), EndProto: []byte{}}
		if err = tx.Create(&fight).Error; err != nil {
			return err
		}
		proto.Merge(&out, start)
		return s.RecordP1Cost(tx, id, "wanted:start:"+fight.ID, costs, now)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{&out}, nil
}

func (s *GameService) p12WantedEnd(ctx context.Context, id int64, request *pb.CSRebelFightEnd, now time.Time) ([]proto.Message, error) {
	var out pb.SCRebelFightEnd
	err := s.p12WantedProfile(ctx, id, func(tx *gorm.DB, _ *db.Profile, player *pb.Player, game *model.GameState) error {
		if request.GetFightId() == "" || request.OptRecord == nil || request.BattlePlayer != nil {
			return errors.New(constUtil.P12WantedInvalid)
		}
		var fight db.P12WantedFight
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&fight, "id=? AND player_id=?", request.GetFightId(), id).Error; err != nil {
			return err
		}
		requestRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
		if err != nil {
			return err
		}
		if fight.Committed {
			if fight.EndRequestHash != p6Digest(requestRaw) {
				return errors.New(constUtil.P12WantedInvalid)
			}
			if fight.EndHash != p6Digest(fight.EndProto) || proto.Unmarshal(fight.EndProto, &out) != nil {
				return errors.New(constUtil.P12WantedInvalid)
			}
			if err := p12WantedRecordedReceipt(&fight, request); err != nil {
				return err
			}
			return nil
		}
		if fight.AbandonedAt > 0 || calendar75Adjusted(tx) && fight.CreatedAt > now.UnixMilli() || fight.InputHash != p6Digest([]byte(fight.InputJSON)) || fight.TerminalHash != p6Digest([]byte(fight.TerminalJSON)) || fight.StartHash != p6Digest(fight.StartProto) {
			return errors.New(constUtil.P12WantedInvalid)
		}
		var input model.FrozenBattleInput
		var terminal model.CombatFinalHPOutcome
		if json.Unmarshal([]byte(fight.InputJSON), &input) != nil || json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || ValidateP12WantedCertifiedReceipt(&input, &terminal) != nil {
			return errors.New(constUtil.P12WantedInvalid)
		}
		if !proto.Equal(request.OptRecord, terminal.OriginalOperates) {
			recorded, err := s.computeP12WantedRecorded(ctx, &input, request.OptRecord)
			if err != nil {
				return err
			}
			if err := p12WantedStoreRecorded(&fight, request, recorded); err != nil {
				return err
			}
			terminal = *recorded
		}
		winner := int32(0)
		if terminal.Outcome.Winner == 0 {
			winner = 1
		}
		if request.GetResult() != winner {
			return errors.New(constUtil.P12WantedInvalid)
		}
		var instance db.P12WantedInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, "id=?", fight.InstanceID).Error; err != nil {
			return err
		}
		if _, err = s.p12WantedView(&instance); err != nil {
			return err
		}
		var hp map[string]float64
		if json.Unmarshal([]byte(instance.MonsterHPJSON), &hp) != nil {
			return errors.New(constUtil.P12WantedInvalid)
		}
		_, _, beforeRank, _, err := p12WantedDailyTotals(tx, s.OriginalServerID(), id, now)
		if err != nil {
			return err
		}
		applied := float64(0)
		if now.UnixMilli() < instance.ExpiresAt && instance.State == constUtil.P12WantedActive {
			hp, applied, err = p12WantedApplyTerminalHP(&input, &terminal, hp)
			if err != nil {
				return err
			}
			instance.CurrentHP = 0
			for _, value := range hp {
				instance.CurrentHP += value
			}
			if instance.CurrentHP == 0 {
				instance.State, instance.KilledAt, instance.KillerID = constUtil.P12WantedKilled, now.UnixMilli(), id
			}
		} else if now.UnixMilli() >= instance.ExpiresAt && instance.State == constUtil.P12WantedActive {
			instance.State = constUtil.P12WantedExpired
		}
		indices := input.AppliedProperties["wantedSettlementIndices"]
		if len(indices) != 3 || indices[2] != constUtil.P12WantedRewardRule {
			return errors.New(constUtil.P12WantedInvalid)
		}
		performanceDiv, e1 := strconv.ParseInt(indices[0], 10, 64)
		scoreDiv, e2 := strconv.ParseInt(indices[1], 10, 64)
		if e1 != nil || e2 != nil || performanceDiv <= 0 || scoreDiv <= 0 || applied < 0 || !combatFinite(applied) || applied > float64(math.MaxInt32)*float64(min(performanceDiv, scoreDiv)) {
			return errors.New(constUtil.P12WantedInvalid)
		}
		performance, score := int64(math.Floor(applied/float64(performanceDiv))), int64(math.Floor(applied/float64(scoreDiv)))
		gain := &pb.ResultObject{}
		if score > 0 {
			gain, err = s.GrantFixedReward(player, game, "score,"+strconv.FormatInt(score, 10))
			if err != nil {
				return err
			}
		}
		fight.Committed, fight.CommittedAt, fight.AppliedDamage, fight.Performance, fight.Score = true, now.UnixMilli(), applied, performance, score
		if err = tx.Save(&fight).Error; err != nil {
			return err
		}
		_, _, afterRank, _, err := p12WantedDailyTotals(tx, s.OriginalServerID(), id, now)
		if err != nil {
			return err
		}
		out = pb.SCRebelFightEnd{Get: gain, Result: proto.Int32(winner), Damage: proto.Float64(applied), RecordAdd: proto.Int64(performance), ScoreAdd: proto.Int32(int32(score)), RankBefore: proto.Int32(beforeRank), RankNow: proto.Int32(afterRank), Cost: []*pb.BaseResource{{Id: proto.String("wantedAttTimes"), Num: proto.Int32(int32(fight.Cost))}}}
		encoded, err := proto.Marshal(&out)
		if err != nil {
			return err
		}
		fight.EndProto, fight.EndHash, fight.EndRequestHash = encoded, p6Digest(encoded), p6Digest(requestRaw)
		if err = tx.Save(&fight).Error; err != nil {
			return err
		}
		rawHP, err := json.Marshal(hp)
		if err != nil {
			return err
		}
		instance.MonsterHPJSON = string(rawHP)
		if err = tx.Save(&instance).Error; err != nil {
			return err
		}
		if err = s.RecordP1Event(tx, id, "wanted:attack:"+fight.ID, "wanted_attack", 1, now); err != nil {
			return err
		}
		if math.Floor(applied) > 0 {
			if err = s.RecordP1Event(tx, id, "wanted:damage:"+fight.ID, "wanted_damage", int64(math.Floor(applied)), now); err != nil {
				return err
			}
		}
		if instance.KillerID == id && instance.KilledAt == now.UnixMilli() {
			return s.RecordP1Event(tx, id, "wanted:kill:"+instance.ID, "wanted_kill", 1, now)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{&out}, nil
}

// p12WantedApplyTerminalHP clips each original actor's actual loss to its live shared HP.
// Concurrent late finishes cannot overdraw another body or award already consumed HP.
func p12WantedApplyTerminalHP(input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome, current map[string]float64) (map[string]float64, float64, error) {
	invalid := func() (map[string]float64, float64, error) {
		return nil, 0, errors.New(constUtil.P12WantedInvalid)
	}
	if input == nil || input.Monster == nil || terminal == nil || len(current) != len(input.Monster.Formation) || len(current) != 4 {
		return invalid()
	}
	actors := map[string]*pb.BattleHero{}
	for _, actor := range input.Monster.Formation {
		if actor == nil || actor.Prop == nil || actor.Inst == nil || actors[actor.GetGuid()] != nil {
			return invalid()
		}
		actors[actor.GetGuid()] = actor
	}
	seen := map[string]bool{}
	next := map[string]float64{}
	applied := float64(0)
	for _, final := range terminal.FinalActors {
		if final.Camp != 1 {
			continue
		}
		original := actors[final.GUID]
		key := "wanted_" + strings.TrimPrefix(final.GUID, "wanted-")
		live, ok := current[key]
		if original == nil || !ok || seen[key] || final.HeroID != original.GetId() || final.MaxHP != original.GetProp().GetHp() || !combatFinite(live) || live < 0 || live > original.GetProp().GetHp() || !combatFinite(final.CurrentHP) || final.CurrentHP < 0 || final.CurrentHP > original.GetProp().GetHp() {
			return invalid()
		}
		seen[key] = true
		// The local shared rule commits positive net HP loss; local healing cannot mint damage.
		damage := min(live, max(0, original.GetInst().GetCurHp()-final.CurrentHP))
		next[key] = live - damage
		applied += damage
	}
	if len(seen) != len(current) || !combatFinite(applied) {
		return invalid()
	}
	return next, applied, nil
}

type p12WantedStanding struct {
	PlayerID    int64
	Performance int64
	Damage      float64
}

func p12WantedDay(now time.Time) (string, time.Time, time.Time) {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	return start.Format("2006-01-02"), start, start.AddDate(0, 0, 1)
}

func p12WantedDailyTotals(tx *gorm.DB, serverID, id int64, now time.Time) ([]p12WantedStanding, p12WantedStanding, int32, int32, error) {
	_, start, end := p12WantedDay(now)
	var rows []p12WantedStanding
	err := tx.Table("p12_wanted_fights AS f").Select("f.player_id,COALESCE(SUM(f.performance),0) AS performance,COALESCE(MAX(f.applied_damage),0) AS damage").Joins("JOIN p12_wanted_instances AS i ON i.id=f.instance_id").Where("i.server_id=? AND f.committed=? AND f.committed_at>=? AND f.committed_at<?", serverID, true, start.UnixMilli(), end.UnixMilli()).Group("f.player_id").Scan(&rows).Error
	if err != nil {
		return nil, p12WantedStanding{}, 0, 0, err
	}
	own := p12WantedStanding{PlayerID: id}
	for _, row := range rows {
		if row.PlayerID == id {
			own = row
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Performance != rows[j].Performance {
			return rows[i].Performance > rows[j].Performance
		}
		return rows[i].PlayerID < rows[j].PlayerID
	})
	var performanceRank, damageRank int32
	for index, row := range rows {
		if row.PlayerID == id && row.Performance > 0 {
			performanceRank = int32(index + 1)
		}
	}
	damages := append([]p12WantedStanding{}, rows...)
	sort.Slice(damages, func(i, j int) bool {
		if damages[i].Damage != damages[j].Damage {
			return damages[i].Damage > damages[j].Damage
		}
		return damages[i].PlayerID < damages[j].PlayerID
	})
	for index, row := range damages {
		if row.PlayerID == id && row.Damage > 0 {
			damageRank = int32(index + 1)
		}
	}
	return rows, own, performanceRank, damageRank, nil
}

// p12WantedDailyReward 保留原运营目标入口，未知奖档按用户决定留空。
// 旧已领取记录只重放冻结回执，新请求不会依据旧硬编码再发score。
func (s *GameService) p12WantedDailyReward(ctx context.Context, id int64, rewardID int32, now time.Time) ([]proto.Message, error) {
	info := &pb.SCGetRebelRewardInfo{}
	claim := &pb.SCGetRebelReward{}
	err := s.p12WantedProfile(ctx, id, func(tx *gorm.DB, _ *db.Profile, player *pb.Player, _ *model.GameState) error {
		policy, _, err := p12WantedConfig(tx)
		if err != nil {
			return err
		}
		if rewardID == 0 {
			return nil
		}
		if !policy.Enabled || player.GetLevel() < 35 || rewardID < 0 {
			return errors.New(constUtil.P12WantedUnavailable)
		}
		if rewardID == 0 {
			return nil
		}
		day, _, _ := p12WantedDay(now)
		var prior db.P12WantedRewardClaim
		if err = tx.First(&prior, "player_id=? AND day=? AND reward_id=?", id, day, rewardID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(constUtil.P12WantedUnavailable)
			}
			return err
		}
		if prior.ResponseHash != p6Digest(prior.Response) || proto.Unmarshal(prior.Response, claim) != nil {
			return errors.New(constUtil.P12WantedInvalid)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if rewardID == 0 {
		return []proto.Message{info}, nil
	}
	return []proto.Message{claim}, nil
}

func p12WantedRankReward(def p12WantedDefinition, table, key, value string, rank int32) (string, error) {
	if rank <= 0 {
		return "", nil
	}
	for _, row := range def.Tables[table] {
		parts := strings.Split(row[key], "-")
		if len(parts) < 1 || len(parts) > 2 {
			return "", errors.New(constUtil.P12WantedInvalid)
		}
		first, err := strconv.ParseInt(parts[0], 10, 32)
		if err != nil {
			return "", err
		}
		last := first
		if len(parts) == 2 {
			last, err = strconv.ParseInt(parts[1], 10, 32)
			if err != nil {
				return "", err
			}
		}
		if int64(rank) >= first && int64(rank) <= last {
			if _, err = p3Resources(row[value]); err != nil {
				return "", err
			}
			return row[value], nil
		}
	}
	return "", nil
}

// SyncP12WantedRewards 在当前角色的P1事务补投已结束自然日两张独立榜，不锁其他角色。
func (s *GameService) SyncP12WantedRewards(tx *gorm.DB, id int64, now time.Time) error {
	policy, def, err := p12WantedConfig(tx)
	if err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}
	for offset := 1; offset <= 7; offset++ {
		day := now.AddDate(0, 0, -offset)
		dayKey, _, end := p12WantedDay(day)
		expires := end.AddDate(0, 0, 7).UnixMilli()
		if now.UnixMilli() >= expires {
			continue
		}
		_, _, performanceRank, damageRank, err := p12WantedDailyTotals(tx, s.OriginalServerID(), id, day)
		if err != nil {
			return err
		}
		for _, spec := range []struct {
			Kind, Table, Key, RewardKey string
			Rank                        int32
		}{
			{"performance", "WantedPerformanceRank", "PerformanceRank", "PerformanceRankReward", performanceRank},
			{"damage", "WantedDamageRank", "DamageRank", "DamageRankReward", damageRank},
		} {
			reward, err := p12WantedRankReward(def, spec.Table, spec.Key, spec.RewardKey, spec.Rank)
			if err != nil {
				return err
			}
			if reward == "" {
				continue
			}
			source := fmt.Sprintf("wanted:rank:%s:%s:%d:%d", dayKey, spec.Kind, s.OriginalServerID(), id)
			if _, err = s.EnqueueReward(tx, id, source, "悬赏每日排行奖励", fmt.Sprintf("%s %s榜第%d名；%s", dayKey, spec.Kind, spec.Rank, constUtil.P12WantedRewardRule), reward, expires); err != nil {
				return err
			}
		}
	}
	return nil
}
