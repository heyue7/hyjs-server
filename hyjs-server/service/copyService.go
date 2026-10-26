package service

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LoadTables 读取 APK 恢复的配置表，并在启动时校验数值格式。
func (s *GameService) LoadTables(root string) error {
	s.Tables = map[string][]map[string]string{}
	for _, name := range []string{"Guide", "Area", "Scene", "DailyCopy", "DailyCopyChapter", "DailyCopyEnemy", "CookingMakeLevelup", "SkillSpecialty", "SkillSpecialtyStrength", "DailyTaskRewardBox", "GemMakeLevelup", "ActivityNewOpen7Day", "SkillCast", "EquipDecompose", "EquipForgeOdds", "EquipForgeProp", "HeroAwakenAdd", "HeroAwakenCost", "HeroAwakenFuseLevel", "HeroAwakenFuseProp", "Skill", "HeroTrainingMaxMin", "HeroBreached", "Treasure", "TreasureStrengthen", "TreasureStrengthLevelExp", "TreasureRefine", "TreasureRefineLevel", "TreasureSkill", "Frag", "ItemUse", "OpenServerFund", "EveryoneBenefit", "ItemShopPrice", "FishSkill", "MineSkill", "Field", "FieldMonsterPos", "FieldMonster", "FieldBoss", "FieldBossPos", "EquipRefine", "EquipRefineExp", "EquipSkill", "EquipStar", "EquipStarSuitProp", "Formation", "FormationSkill", "Equip", "EquipExp", "EquipOutfit", "StrengthenMaster", "Hero", "Item", "HeroExp", "TalentAdvenced", "Talent", "Buff", "WorldSeed", "Payment", "VipPerks", "LuxuryEquip", "CopyChapter", "CopyMission", "CopyTask", "CopyEnemy", "DemoHero", "BootHero", "RecruitShowHero", "CopyExp", "UserExp", "StoryQuest", "GameConfigure"} {
		f, err := os.Open(filepath.Join(root, "Table"+name+".tsv"))
		if err != nil {
			return err
		}
		reader := csv.NewReader(f)
		reader.Comma = '\t'
		reader.FieldsPerRecord = -1
		rows, err := reader.ReadAll()
		f.Close()
		if err != nil {
			return err
		}
		if len(rows) < 5 {
			return fmt.Errorf("invalid table %s", name)
		}
		for _, row := range rows[5:] {
			if len(row) == 0 || row[0] == "" {
				continue
			}
			item := map[string]string{}
			for i, v := range row {
				if i < len(rows[1]) {
					item[rows[1][i]] = v
					if i < len(rows[2]) && v != "" && slices.Contains([]string{"int", "float", "double", "long"}, rows[2][i]) {
						if _, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err != nil {
							return fmt.Errorf("%s.%s: %w", name, rows[1][i], err)
						}
					}
				}
			}
			s.Tables[name] = append(s.Tables[name], item)
		}
	}
	return s.installCopyExperience73()
}

func (s *GameService) table(name, key, value string) map[string]string {
	if row, indexed := s.indexedTable(name, key, value); indexed {
		return row
	}
	for _, row := range s.Tables[name] {
		if row[key] == value {
			return row
		}
	}
	return nil
}
func tableNumber(row map[string]string, key string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(row[key]), 64)
	return v
}

func (s *GameService) actor(row map[string]string, guid string, pose int32, enemy bool) (*pb.BattleHero, error) {
	if row == nil {
		return nil, errors.New("Missing actor table")
	}
	keys := map[string]string{"att": "attBase", "hp": "hpBase", "pdef": "pdefBase", "mdef": "mdefBase"}
	id := row["id"]
	if enemy {
		keys = map[string]string{"att": "Attack", "hp": "Hp", "pdef": "PhyDefend", "mdef": "MagicDefend"}
		id = row["HeroId"]
	}
	prop := map[string]any{}
	for k, v := range keys {
		prop[k] = tableNumber(row, v)
	}
	for _, k := range []string{"hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce"} {
		prop[k] = tableNumber(row, k)
	}
	var talents []string
	for i := 0; i < 6; i++ {
		if v := row[fmt.Sprintf("Talent%d", i)]; v != "" {
			talents = append(talents, v)
		}
	}
	level := tableNumber(row, "Level")
	if level == 0 {
		level = 1
	}
	blood := tableNumber(row, "bloodNumber")
	if blood == 0 {
		blood = 1
	}
	chance := tableNumber(row, "SkillCastChance")
	if row["SkillCastChance"] == "" {
		chance = 100
	}
	msg, err := model.GameMessage("BattleHero", map[string]any{"id": id, "guid": guid, "pose": pose, "level": level, "prop": prop, "inst": map[string]any{"curHp": prop["hp"]}, "bloodNumber": blood, "skillOrder": strings.Join([]string{row["SkillId1"], row["SkillId2"], row["UltSkillId"]}, ","), "SkillCastChance": chance, "talentList": talents})
	if err != nil {
		return nil, err
	}
	return msg.(*pb.BattleHero), nil
}

// battleWarrior 沿用 UIHero 的等级、当前主干成长和已激活节点属性计算。
func (s *GameService) battleWarrior(player *pb.Player, warrior *pb.Warrior, game *model.GameState, slot *pb.FormationItem) (*pb.BattleHero, error) {
	row := s.table("Hero", "id", warrior.GetId())
	if row == nil || row["heroClass"] == "99" {
		return nil, errors.New("经验材料不能出战")
	}
	actor, err := s.actor(row, warrior.GetGuid(), slot.GetBattlePos(), false)
	if err != nil {
		return nil, err
	}
	var (
		level   = warrior.GetLevel()
		current map[string]string
		hasTree bool
		active  []map[string]string
		values  = map[string]float64{}
		percent = map[string]float64{}
		talents []string
	)
	if warrior.GetGuid() == player.GetHeroWarId() {
		level = player.GetLevel()
	}
	if level < 1 {
		level = 1
	}
	for _, node := range s.Tables["TalentAdvenced"] {
		if node["warId"] != warrior.GetId() {
			continue
		}
		hasTree = true
		if node["trunck"] == "1" {
			tier, parseErr := strconv.ParseInt(node["talentLevel"], 10, 32)
			if parseErr != nil {
				return nil, parseErr
			}
			if int32(tier) == warrior.GetTalent() {
				current = node
			}
			if int32(tier) <= warrior.GetTalent() {
				active = append(active, node)
			}
		} else if slices.Contains(warrior.GetTalentBranch(), node["talentLevel"]) {
			active = append(active, node)
		}
	}
	if current == nil {
		// 原绿色伙伴没有天赋树。UIHero 跳过 CurrentTalent 成长后继续
		// 基础属性与装备计算；有树却缺层或存档伪造节点仍须拒绝。
		baseOnly := !hasTree && row["heroClass"] == "0" && row["quality"] == "12" &&
			(row["SkillId1"] == "" && row["SkillId2"] == "" && row["UltSkillId"] == "" || p15GreenHeroSingleNormalSkill(row)) &&
			warrior.GetTalent() == 0 && len(warrior.GetTalentBranch()) == 0
		if !baseOnly {
			return nil, errors.New("Missing warrior talent growth")
		}
	}
	for _, key := range []string{"hp", "att", "pdef", "mdef"} {
		values[key] = tableNumber(row, key+"Base") + tableNumber(current, key+"Grow")*float64(level-1)
	}
	for _, key := range []string{"hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce"} {
		values[key] = tableNumber(row, key)
	}
	training := warrior.GetTraining()
	values["hp"] += float64(training.GetHp())
	values["att"] += float64(training.GetAtt())
	values["pdef"] += float64(training.GetPdef())
	values["mdef"] += float64(training.GetMdef())
	// 累加原表觉醒与融合属性，并保留客户端战斗使用的觉醒阶段和技能。
	if err := s.AddWarriorAwakeProperties(warrior, values, percent, &talents); err != nil {
		return nil, err
	}
	// UIHero 累加全部已激活节点的固定加成，成长系数取当前主干层。
	for _, node := range active {
		for _, key := range []string{"hp", "att", "pdef", "mdef"} {
			values[key] += tableNumber(node, key+"ConstAdd")
		}
		if talentID := node["talentId"]; talentID != "" {
			talents = append(talents, talentID)
			talent := s.table("Talent", "Id", talentID)
			if talent == nil {
				return nil, errors.New("Missing warrior talent")
			}
			// CaculateSkillProperty 仅把前三槽 Forever 自身 Buff 纳入面板。
			// 动态触发继续交给 APK 战斗模拟，并保留完整 talentList。
			for index := 0; index < 3; index++ {
				key := fmt.Sprintf("Buff%d", index)
				if talent[key] == "" || tableNumber(talent, key+"TriggerTimer") != 0 || tableNumber(talent, key+"Select") != 0 {
					continue
				}
				buff := s.table("Buff", "Id", talent[key])
				if buff == nil {
					return nil, errors.New("Missing warrior talent buff")
				}
				copyTalentProperty(values, percent, buff)
			}
		}
	}
	if err := s.AddFormationBonus(game, values, percent); err != nil {
		return nil, err
	}
	if err := s.AddSkillSpecialtyProperties(warrior, values); err != nil {
		return nil, err
	}
	if err := s.AddCookingProperties(warrior, values); err != nil {
		return nil, err
	}
	// 原 UIHeroFormation 将已解锁世界之种属性加到每位队员，再乘百分比。
	if err := s.addWorldSeedProperties(player, values); err != nil {
		return nil, err
	}
	equipment, err := s.equipmentFormationProperties(game, slot)
	if err != nil {
		return nil, err
	}
	for key, value := range map[string]float64{"hp": equipment.GetHp(), "att": equipment.GetAtt(), "pdef": equipment.GetPdef(), "mdef": equipment.GetMdef(), "hit": equipment.GetHit(), "dodge": equipment.GetDodge(), "cri": equipment.GetCri(), "antiCri": equipment.GetAntiCri(), "criFactor": equipment.GetCriFactor(), "damageAdd": equipment.GetDamageAdd(), "damageReduce": equipment.GetDamageReduce()} {
		values[key] += value
	}
	if err := s.equipmentFormationPercent(game, slot, percent); err != nil {
		return nil, err
	}
	if err := s.treasureFormationProperties(game, slot, values, percent); err != nil {
		return nil, err
	}
	breached := s.table("HeroBreached", "id", strconv.Itoa(int(warrior.GetBreached().GetLevel())))
	if breached == nil {
		return nil, errors.New("突破属性配置缺失")
	}
	for key, column := range map[string]string{"hp": "HpAddPercent", "att": "AttAddPercent", "pdef": "PDefAddPercent", "mdef": "MDefAddPercent"} {
		percent[key] += tableNumber(breached, column) / 100
	}
	gemValues, err := s.equipmentFormationGemProperties(game, slot)
	if err != nil {
		return nil, err
	}
	if err := s.AddP5StaticBonus(player, game, warrior, values, percent); err != nil {
		return nil, err
	}
	// Original ResetAttribute adds prayer to the combined percent before gems/pets.
	if err := AddP5GuildPrayerPercent(player, game, percent); err != nil {
		return nil, err
	}
	unionConst, err := P5GuildTechConstants(player, game)
	if err != nil {
		return nil, err
	}
	petValues, petTalents, _, err := s.P9GuardProperties(game, slot)
	if err != nil {
		return nil, err
	}
	for key := range petValues {
		if _, exists := values[key]; !exists {
			values[key] = 0
		}
	}
	prop := map[string]any{}
	for key, value := range values {
		value = value*(1+percent[key]) + unionConst[key] + gemValues[key] + petValues[key]
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return nil, errors.New("Invalid warrior property")
		}
		prop[key] = value
	}
	msg, err := model.GameMessage("BaseProp", prop)
	if err != nil {
		return nil, err
	}
	actor.Prop = msg.(*pb.BaseProp)
	actor.Inst.CurHp = proto.Float64(actor.GetProp().GetHp())
	actor.Level = proto.Int32(level)
	actor.TalentList = append(talents, petTalents...)
	actor.AwakeStage = proto.Int32(warrior.GetAwake().GetStage())
	// 新战斗传原真实突破等级；历史录像保持其冻结wire，不回填旧输入。
	actor.BreachedLevel = proto.Int32(warrior.GetBreached().GetLevel())
	order, awakened, err := s.P5BattleSkillTransport(warrior)
	if err != nil {
		return nil, err
	}
	actor.SkillOrder = proto.String(order)
	actor.AwakenSkill = proto.String(awakened)
	extraTalents, err := s.P5DynamicTalentIDs(game, warrior)
	if err != nil {
		return nil, err
	}
	actor.TalentList = append(actor.TalentList, extraTalents...)
	// 原模型加载器按真实佩戴武器ID选择外观，战斗属性与旧录像保持原合同。
	weaponID, err := s.battleWeaponID(game, slot)
	if err != nil {
		return nil, err
	}
	if weaponID != "" {
		actor.WeaponId = proto.String(weaponID)
	}
	return actor, nil
}

// addWorldSeedProperties 对齐 GetWorldSeedData 的列表前缀与 SetLevel 的 type=1 筛选。
func (s *GameService) addWorldSeedProperties(player *pb.Player, values map[string]float64) error {
	limit := int(player.GetCurrentWorldSeedId())
	if limit <= 0 {
		return nil
	}
	rows := s.Tables["WorldSeed"]
	if len(rows) == 0 {
		return errors.New("世界之种属性配置缺失")
	}
	for index, row := range rows {
		if index >= limit {
			break
		}
		// 道具节点和主角品质节点继续由原业务处理，不计入全队面板属性。
		if row["type"] != "1" {
			continue
		}
		pair := strings.Split(row["get"], ",")
		if len(pair) != 2 {
			return errors.New("世界之种属性格式无效")
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(pair[1]), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return errors.New("世界之种属性数值无效")
		}
		key := strings.TrimSpace(pair[0])
		switch key {
		case "hp", "att", "pdef", "mdef":
			values[key] += value
		default:
			return errors.New("世界之种属性类型无效")
		}
	}
	return nil
}

// copyTalentProperty 对齐 UITalent.GetBufferAddProp 的固定值与百分比字段。
func copyTalentProperty(values, percent map[string]float64, buff map[string]string) {
	var (
		factor = buff["Factor"]
		value  = tableNumber(buff, "Args1")
	)
	switch factor {
	case "hp", "att", "pdef", "mdef", "hit", "dodge", "cri", "criFactor", "antiCri", "damageAdd", "damageReduce":
		values[factor] += value
	case "def":
		values["pdef"] += value
		values["mdef"] += value
	case "perhp", "peratt", "perpdef", "permdef":
		percent[strings.TrimPrefix(factor, "per")] += value * 0.01
	case "patt":
		percent["att"] += value * 0.01
	case "perdef":
		percent["pdef"] += value * 0.01
		percent["mdef"] += value * 0.01
	}
}

func (s *GameService) battlePlayer(player *pb.Player, game *model.GameState) (*pb.BattlePlayer, error) {
	if err := s.PrepareFormation(game); err != nil {
		return nil, err
	}
	var actors []*pb.BattleHero
	for _, slot := range game.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetPos() != 0 {
				continue
			}
			for _, warrior := range game.Warriors {
				if warrior.GetGuid() == thing.GetGuid() {
					a, err := s.battleWarrior(player, warrior, game, slot)
					if err != nil {
						return nil, err
					}
					actors = append(actors, a)
				}
			}
		}
	}
	msg, err := model.GameMessage("BattlePlayer", map[string]any{"playerId": player.GetPlayerId(), "serverId": s.OriginalServerID(), "level": player.GetLevel(), "nickName": player.GetName(), "formation": actors, "inst": map[string]any{"anger": 0}})
	if err != nil {
		return nil, err
	}
	return msg.(*pb.BattlePlayer), nil
}

func (s *GameService) tasks(mission string) []map[string]string {
	var rows []map[string]string
	for _, row := range s.Tables["CopyTask"] {
		if row["MissionId"] == mission {
			rows = append(rows, row)
		}
	}
	return rows
}

// copyMissionPassed 按具体关卡读取历史，后续首通助阵不受其他关卡影响。
func copyMissionPassed(g *model.GameState, chapter, mission string) bool {
	for _, info := range g.Copy.GetPassedCopyInfo() {
		if info.GetChapterId() == chapter && info.GetMissionId() == mission && info.GetStar() > 0 {
			return true
		}
	}
	return false
}

// copyMissionOpen 按原表全局关卡顺序检查前一关及当前 OpenId，跨章不重置前置。
func (s *GameService) copyMissionOpen(g *model.GameState, mission map[string]string) bool {
	if mission == nil || s.table("CopyChapter", "ChapterId", mission["ChapterId"]) == nil {
		return false
	}
	if quest := mission["OpenId"]; quest != "" && !slices.Contains(g.Quests, quest) {
		return false
	}
	var previous map[string]string
	for _, row := range s.Tables["CopyMission"] {
		if row["MissionId"] == mission["MissionId"] {
			return previous == nil || copyMissionPassed(g, previous["ChapterId"], previous["MissionId"])
		}
		previous = row
	}
	return false
}

// refreshCopyProgress 补齐全部原关卡和章节，保留旧成绩与每章已领取标记。
func (s *GameService) refreshCopyProgress(g *model.GameState) {
	if g.Copy == nil {
		g.Copy = &pb.SCCopyPassedInfo{}
	}
	stars := map[string]int32{}
	for _, mission := range s.Tables["CopyMission"] {
		var info *pb.CopySimpleInfo
		for _, saved := range g.Copy.GetPassedCopyInfo() {
			if saved.GetChapterId() == mission["ChapterId"] && saved.GetMissionId() == mission["MissionId"] {
				info = saved
				break
			}
		}
		if info == nil {
			info = &pb.CopySimpleInfo{ChapterId: proto.String(mission["ChapterId"]), MissionId: proto.String(mission["MissionId"]), Star: proto.Int32(0), ChallengeTimes: proto.Int32(0)}
			g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, info)
		}
		info.IsOpen = proto.Int32(0)
		if s.copyMissionOpen(g, mission) || info.GetStar() > 0 {
			info.IsOpen = proto.Int32(1)
			g.Copy.CurrentChapterId = proto.String(mission["ChapterId"])
			g.Copy.CurrentMissionId = proto.String(mission["MissionId"])
		}
		stars[mission["ChapterId"]] += info.GetStar()
	}
	for _, row := range s.Tables["CopyChapter"] {
		var chapter *pb.CopyChapterInfo
		for _, info := range g.Copy.GetPassedChapterInfo() {
			if info.GetChapterId() == row["ChapterId"] {
				chapter = info
				break
			}
		}
		if chapter == nil {
			chapter = &pb.CopyChapterInfo{ChapterId: proto.String(row["ChapterId"])}
			g.Copy.PassedChapterInfo = append(g.Copy.PassedChapterInfo, chapter)
		}
		chapter.Star = proto.Int32(stars[row["ChapterId"]])
	}
}

// refreshCopyChallengeDay 以本地测试区的北京时间日期恢复每日挑战次数。
func (s *GameService) refreshCopyChallengeDay(g *model.GameState) {
	day := time.Now().UTC().Add(8 * time.Hour).Format("2006-01-02")
	if g.CopyChallengeDay != "" && g.CopyChallengeDay != day {
		for _, info := range g.Copy.GetPassedCopyInfo() {
			info.ChallengeTimes = proto.Int32(0)
		}
	}
	// 旧存档缺少日期时保留已有次数，首次读取记录当天日期。
	g.CopyChallengeDay = day
}

func (s *GameService) copyPassedInfo(ctx context.Context, id int64) ([]proto.Message, error) {
	var result *pb.SCCopyPassedInfo
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
		s.refreshCopyProgress(g)
		if err := s.refreshCopyCalendar(ctx, tx, id, g, time.Now()); err != nil {
			return err
		}
		result = g.Copy
		return nil
	})
	return []proto.Message{result}, err
}

func (s *GameService) copyCurrentInfo(ctx context.Context, id int64) ([]proto.Message, error) {
	var result *pb.SCCopyCurrentInfo
	err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		s.refreshCopyProgress(g)
		result = &pb.SCCopyCurrentInfo{CurrentChapterId: g.Copy.CurrentChapterId, CurrentMissionId: g.Copy.CurrentMissionId}
		return nil
	})
	return []proto.Message{result}, err
}

func (s *GameService) copyEnter(ctx context.Context, id int64, b *pb.CSCopyEnter) ([]proto.Message, error) {
	mission := s.table("CopyMission", "MissionId", b.GetMissionId())
	if mission == nil || mission["ChapterId"] != b.GetChapterId() || s.table("CopyChapter", "ChapterId", b.GetChapterId()) == nil {
		return nil, errors.New("关卡或章节配置不存在")
	}
	var (
		result  *pb.SCCopyEnter
		updated *pb.Player
	)
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		if !s.copyMissionOpen(g, mission) {
			return errors.New("请先完成前一关和前置任务")
		}
		chapter := s.table("CopyChapter", "ChapterId", b.GetChapterId())
		if chapter == nil || player.GetLevel() < int32(tableNumber(chapter, "LevelLimit")) {
			return errors.New("角色等级尚未达到章节要求")
		}
		s.refreshCopyProgress(g)
		if err := s.refreshCopyCalendar(ctx, tx, id, g, time.Now()); err != nil {
			return err
		}
		// 一个角色只保留一个活动副本，切换关卡前先显式离开。
		if g.Run != nil && !g.Run.Complete && (g.Run.Chapter != b.GetChapterId() || g.Run.Mission != b.GetMissionId()) {
			return errors.New("请先离开当前副本")
		}
		newRun := g.Run == nil || g.Run.Complete
		if newRun {
			if err := s.ensureNoActiveDailyCopy(tx, id, time.Now()); err != nil {
				return err
			}
			for _, info := range g.Copy.GetPassedCopyInfo() {
				if info.GetChapterId() == b.GetChapterId() && info.GetMissionId() == b.GetMissionId() && info.GetChallengeTimes() >= int32(tableNumber(mission, "ChallengeCount")) {
					return errors.New("本关今日挑战次数已用完")
				}
			}
			cost := int32(tableNumber(mission, "PowerCost"))
			if cost < 0 || player.GetStamina() < cost {
				return errors.New("体力不足")
			}
			player.Stamina = proto.Int32(player.GetStamina() - cost)
			g.Run = &model.CopyRun{Chapter: b.GetChapterId(), Mission: b.GetMissionId(), Passed: []string{}}
		}
		var jobs []*pb.CopyJobInfo
		for _, row := range s.tasks(b.GetMissionId()) {
			if slices.Contains(g.Run.Passed, row["TaskId"]) {
				continue
			}
			// 缺少场景或敌人配置时回滚入场和体力扣费。
			hasEnemy := false
			for _, enemy := range s.Tables["CopyEnemy"] {
				hasEnemy = hasEnemy || enemy["TaskId"] == row["TaskId"]
			}
			if row["SceneName"] == "" || !hasEnemy {
				return errors.New("Missing battle resources")
			}
			job := new(pb.CopyJobInfo)
			fields := job.ProtoReflect().Descriptor().Fields()
			values := map[string]any{}
			for i := 0; i < fields.Len(); i++ {
				f := fields.Get(i)
				name := string(f.Name())
				key := strings.ToUpper(name[:1]) + name[1:]
				if row[key] == "" {
					continue
				}
				if f.Kind() == protoreflect.StringKind {
					values[name] = row[key]
				} else {
					values[name] = tableNumber(row, key)
				}
			}
			m, err := model.GameMessage("CopyJobInfo", values)
			if err != nil {
				return err
			}
			jobs = append(jobs, m.(*pb.CopyJobInfo))
		}
		if len(jobs) == 0 || mission["SceneName"] == "" {
			return errors.New("Mission has no remaining tasks or scene")
		}
		// Freeze the original scene boxes in the same entry transaction.
		boxes, err := s.CaptureCopyBoxes(tx, id, b.GetChapterId(), b.GetMissionId())
		if err != nil {
			return err
		}
		result = &pb.SCCopyEnter{TaskId: proto.String(jobs[0].GetTaskId()), JobList: jobs, BoxList: boxes, SceneName: proto.String(mission["SceneName"])}
		updated = player
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result, &pb.SCPlayer{Player: updated}}, nil
}

func (s *GameService) copyFightStart(ctx context.Context, id int64, b *pb.CSCopyFightStart) ([]proto.Message, error) {
	var result *pb.SCCopyFightStart
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		run := g.Run
		if run == nil || run.Complete || run.Chapter != b.GetChapterId() || run.Mission != b.GetMissionId() {
			return errors.New("Enter this mission before starting a fight")
		}
		task := s.table("CopyTask", "TaskId", b.GetTaskId())
		if task == nil || task["MissionId"] != run.Mission {
			return errors.New("Invalid battle task")
		}
		// Ordinary live map collisions choose the touched monster's own task.
		// Preserve scripted/demo sequencing and reject tasks already consumed.
		next := b.GetTaskId()
		if slices.Contains(run.Passed, next) {
			return errors.New("This encounter has already been completed")
		}
		// Boss胜利会触发原客户端的结束剧情，须在其余遭遇全部完成后开放。
		// 普通怪仍按玩家实际碰撞顺序处理，重连沿用本次付费Run的Passed。
		if task["IsBoss"] == "1" {
			for _, candidate := range s.tasks(run.Mission) {
				if candidate["TaskId"] != next && !slices.Contains(run.Passed, candidate["TaskId"]) {
					return errors.New("请先击败本关的小怪")
				}
			}
		}
		if task["BornType"] != "0" || task["IsDemoFight"] != "0" && task["IsDemoFight"] != "1" {
			return errors.New("Unsupported scripted encounter")
		}
		if task["IsDemoFight"] == "1" {
			first := ""
			for _, candidate := range s.tasks(run.Mission) {
				if !slices.Contains(run.Passed, candidate["TaskId"]) {
					first = candidate["TaskId"]
					break
				}
			}
			if first != next {
				return errors.New("Complete the previous encounter first")
			}
		}
		// One pending fight owns this paid run. A different collision cannot
		// replace its frozen input, while exact retry returns the saved frame.
		if run.Fight != nil && !run.Fight.Settled {
			if run.Fight.Task != next {
				return errors.New("Finish the active encounter first")
			}
			if run.Fight.Start == nil {
				return errors.New("Missing saved battle configuration")
			}
			result = run.Fight.Start
			return nil
		}
		if err := s.ensureNoActiveDailyCopy(tx, id, time.Now()); err != nil {
			return err
		}
		party, err := s.copyContinuityParty(player, g)
		if err != nil {
			return err
		}
		if len(party.GetFormation()) == 0 {
			return errors.New("请先安排出战伙伴")
		}
		groups := map[int][]*pb.BattleHero{}
		for _, row := range s.Tables["CopyEnemy"] {
			if row["TaskId"] != next {
				continue
			}
			a, err := s.actor(row, "monster-"+row["CopyEnemyId"], int32(tableNumber(row, "Location")), true)
			if err != nil {
				return err
			}
			index := int(tableNumber(row, "GroupIndex"))
			groups[index] = append(groups[index], a)
		}
		if len(groups) == 0 || task["SceneName"] == "" {
			return errors.New("Missing battle resources")
		}
		if task["IsDemoFight"] == "1" && !copyMissionPassed(g, run.Chapter, run.Mission) {
			var helpers []*pb.BattleHero
			occupied := map[int32]bool{}
			for _, row := range s.Tables["DemoHero"] {
				if row["TaskId"] == next {
					a, err := s.actor(row, "demo-"+row["DemoHeroId"], int32(tableNumber(row, "Weight")), true)
					if err != nil {
						return err
					}
					helpers = append(helpers, a)
					occupied[a.GetPose()] = true
				}
			}
			// 首通助阵保留原表位置，仅重排本场快照中发生冲突的玩家角色。
			var displaced []*pb.BattleHero
			for _, hero := range party.Formation {
				if occupied[hero.GetPose()] {
					displaced = append(displaced, hero)
				} else {
					occupied[hero.GetPose()] = true
				}
			}
			for _, hero := range displaced {
				found := false
				for _, pose := range []int32{2, 3, 4, 7, 8, 9, 12, 13, 14} {
					if !occupied[pose] {
						hero.Pose = proto.Int32(pose)
						occupied[pose] = true
						found = true
						break
					}
				}
				if !found {
					return errors.New("助阵角色超出战场位置")
				}
			}
			party.Formation = append(party.Formation, helpers...)
		}
		var monsters []any
		indexes := make([]int, 0, len(groups))
		for k := range groups {
			indexes = append(indexes, k)
		}
		sort.Ints(indexes)
		taskNum, _ := strconv.Atoi(next)
		for _, index := range indexes {
			monsters = append(monsters, map[string]any{"monsterId": taskNum*100 + index, "serverId": 1, "taskId": next, "groupIndex": index, "formation": groups[index], "inst": map[string]any{"anger": 0}})
		}
		token := make([]byte, 12)
		if _, err = rand.Read(token); err != nil {
			return err
		}
		seed, err := rand.Int(rand.Reader, big.NewInt(2147483647))
		if err != nil {
			return err
		}
		m, err := model.GameMessage("SCCopyFightStart", map[string]any{"fightId": hex.EncodeToString(token), "randSeed": seed.Int64(), "fightScene": task["SceneName"], "maxRound": 40, "battlePlayer": party, "battleMonsterList": monsters})
		if err != nil {
			return err
		}
		result = m.(*pb.SCCopyFightStart)
		if err := s.captureCopyDrops(tx, id, result.GetFightId(), task); err != nil {
			return err
		}
		run.Fight = &model.CopyFight{Task: next, Start: result}
		if err := s.CaptureOriginalCopyReplay(ctx, tx, id, result); err != nil {
			return err
		}
		return nil
	})
	return []proto.Message{result}, err
}

// copyFirstDrops 恢复表中只有一个数量档位且概率明确为 1000 的首次掉落。
// APK 只包含掉落展示和配置，Boss 多档概率、特殊装备抽取尚无服务端规则证据。
func (s *GameService) copyFirstDrops(tx *gorm.DB, id int64, player *pb.Player, g *model.GameState, fight *model.CopyFight) (*pb.ResultObject, []*pb.BaseResource, error) {
	gain := &pb.ResultObject{}
	if slices.Contains(g.CopyFirstPassedTasks, fight.Task) || copyMissionPassed(g, g.Run.Chapter, g.Run.Mission) {
		return gain, nil, nil
	}
	task := s.table("CopyTask", "TaskId", fight.Task)
	if task == nil || task["MissionId"] != g.Run.Mission {
		return nil, nil, errors.New("Missing first-drop task configuration")
	}
	var resources []*pb.BaseResource
	for partIndex, part := range strings.Split(task["FirstDropOdds"], "|") {
		if part == "" {
			continue
		}
		var (
			pair  = strings.Split(part, ":")
			entry []string
		)
		if len(pair) != 2 || strings.Contains(pair[1], ";") {
			continue
		}
		entry = strings.Split(pair[1], ",")
		if len(entry) != 2 || entry[1] != "1000" {
			continue
		}
		num, err := strconv.ParseInt(entry[0], 10, 32)
		if err != nil || num <= 0 {
			return nil, nil, errors.New("Invalid guaranteed first-drop quantity")
		}
		key := pair[0]
		switch {
		case strings.HasPrefix(key, "hero_exp_"):
			// 经验材料属于 Hero 类实例，独立 guid 供伙伴升级请求选择消耗。
			row := s.table("Hero", "id", key)
			if row == nil || row["heroClass"] != "99" {
				return nil, nil, errors.New("Missing experience-material configuration")
			}
			if int64(len(g.Warriors))+num > int64(player.GetWarriorBagSize()) {
				awarded, err := s.grantCopyBattleReward(tx, player, g, fmt.Sprintf("%s,%d", key, num), fmt.Sprintf("legacy-first:%d", partIndex), func(fit string) (*pb.ResultObject, error) {
					return s.openServerGain(player, g, fit)
				})
				if err != nil {
					return nil, nil, err
				}
				p1AppendGain(gain, awarded)
				for _, warrior := range awarded.GetWars() {
					resources = append(resources, &pb.BaseResource{Id: warrior.Id, Guid: warrior.Guid, Num: proto.Int32(1)})
				}
				continue
			}
			for i := int64(0); i < num; i++ {
				guid := fmt.Sprintf("%d-copy-%s-%d", id, fight.Start.GetFightId(), len(gain.GetWars()))
				m, err := model.GameMessage("Warrior", map[string]any{"id": key, "guid": guid, "level": 1, "exp": 0, "talent": 0, "training": map[string]any{}, "tempTraining": map[string]any{}, "breached": map[string]any{"level": 1}, "awake": map[string]any{}})
				if err != nil {
					return nil, nil, err
				}
				warrior := m.(*pb.Warrior)
				g.Warriors = append(g.Warriors, warrior)
				gain.Wars = append(gain.Wars, proto.Clone(warrior).(*pb.Warrior))
				resources = append(resources, &pb.BaseResource{Id: proto.String(key), Guid: proto.String(guid), Num: proto.Int32(1)})
			}
		case strings.HasPrefix(key, "item_"):
			if s.table("Item", "itemId", key) == nil {
				return nil, nil, errors.New("Missing first-drop item configuration")
			}
			var owned *pb.SimpleItem
			for _, item := range g.Items {
				if item.GetId() == key {
					owned = item
					break
				}
			}
			if owned == nil {
				g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(num))})
			} else {
				if int64(owned.GetNum()) > math.MaxInt32-num {
					return nil, nil, errors.New("First-drop item quantity overflow")
				}
				owned.Num = proto.Int32(owned.GetNum() + int32(num))
			}
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(num))})
			resources = append(resources, &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(int32(num))})
		default:
			// 装备实例化规则尚未恢复，保留表定义并等待完整的装备协议实现。
			continue
		}
	}
	return gain, resources, nil
}

// copyCompletionStars 沿 GM OfflineProgress.FightEndRequest 的已实现规则，
// 按最终出战队伍的伤亡扣星。该参考规则不改变战斗胜负或历史最高星。
func copyCompletionStars(party *pb.BattlePlayer) int32 {
	const maximumStars int32 = 5
	stars := maximumStars
	for _, hero := range party.GetFormation() {
		if hero.GetInst().GetCurHp() <= 0 {
			stars--
		}
	}
	return max(int32(1), stars)
}

func (s *GameService) copyFightEnd(ctx context.Context, id int64, b *pb.CSCopyFightEnd) ([]proto.Message, error) {
	var (
		result   *pb.SCCopyFightEnd
		updated  *pb.Player
		progress *pb.SCCopyPassedInfo
	)
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		run := g.Run
		if run == nil || run.Fight == nil || run.Fight.Start == nil || run.Fight.Start.GetFightId() != b.GetFightId() {
			return errors.New("Unknown fight for this player")
		}
		fight := run.Fight
		benefitAt := time.Now()
		updated = player
		// 回复发送当前完整进度，跨日重试同一结算也能同步新的挑战次数。
		s.refreshCopyProgress(g)
		if err := s.refreshCopyCalendar(ctx, tx, id, g, benefitAt); err != nil {
			return err
		}
		progress = g.Copy
		if fight.Settled {
			if fight.Result == nil {
				return errors.New("Missing saved battle result")
			}
			// 原结算页会重新写入完整获奖实例；旧包只在实例及精灵状态仍吻合时恢复。
			if err := s.ValidateCopyAwardReceiptCurrent(tx, id, g, fight.Result); err != nil {
				return err
			}
			result = fight.Result
			return nil
		}
		// 单人战斗继续由客户端模拟，服务端校验会话、阵容和剩余血量。
		if (b.GetResult() != 0 && b.GetResult() != 1) || b.GetBattlePlayer().GetPlayerId() != id {
			return errors.New("Invalid fight owner")
		}
		expected := map[string]*pb.BattleHero{}
		for _, a := range fight.Start.GetBattlePlayer().GetFormation() {
			expected[a.GetGuid()] = a
		}
		observed := map[string]bool{}
		surviving := false
		for _, a := range b.GetBattlePlayer().GetFormation() {
			e := expected[a.GetGuid()]
			hp := a.GetInst().GetCurHp()
			if e == nil || observed[a.GetGuid()] || e.GetId() != a.GetId() || math.IsNaN(hp) || math.IsInf(hp, 0) || hp < 0 || hp > e.GetProp().GetHp() {
				return errors.New("Invalid battle lineup or remaining HP")
			}
			observed[a.GetGuid()] = true
			surviving = surviving || hp > 0
		}
		if len(observed) != len(expected) || b.GetResult() == 1 && !surviving {
			return errors.New("Invalid battle survivors")
		}
		acceptedParty, err := s.copyContinuityAccepted(run, b.GetBattlePlayer())
		if err != nil {
			return err
		}
		complete := false
		result = &pb.SCCopyFightEnd{IsCopyComplete: proto.Int32(0), Star: proto.Int32(0), FightResult: proto.Int32(b.GetResult()), Gain: &pb.ResultObject{}}
		if b.GetResult() == 1 {
			tx = beginCopyOverflow(tx, id, fight.Start.GetFightId())
			gain, resources, err := s.settleCopyFirstDrops(tx, id, player, g, fight)
			if err != nil {
				return err
			}
			ordinary, extraResources, err := s.settleCopyDrops(tx, id, player, g, fight)
			if err != nil {
				return err
			}
			p1AppendGain(gain, ordinary)
			resources = append(resources, extraResources...)
			// 首通与普通掉落福利均与冻结战斗保存使用同一事务。
			if s.P4Ready {
				bonus, extra, err := s.ApplyP4CopyDropBonus(tx, id, player, g, "copy:"+fight.Start.GetFightId(), copyOverflowBenefitBase(tx, gain), LocalPlayAt(ctx, benefitAt))
				if err != nil {
					return err
				}
				p1AppendGain(gain, bonus)
				resources = append(resources, extra...)
			}
			result.Gain = gain
			result.CurrentResource = resources
			if run.Gain == nil {
				run.Gain = &pb.ResultObject{}
			}
			// 中途战斗发送当次展示，最终结算发送整个副本累计获得的实例。
			p1AppendGain(run.Gain, gain)
			if !slices.Contains(g.CopyFirstPassedTasks, fight.Task) {
				g.CopyFirstPassedTasks = append(g.CopyFirstPassedTasks, fight.Task)
			}
			run.Passed = append(run.Passed, fight.Task)
			complete = true
			for _, task := range s.tasks(run.Mission) {
				complete = complete && slices.Contains(run.Passed, task["TaskId"])
			}
		}
		if complete {
			reward := s.table("CopyExp", "level", strconv.Itoa(int(player.GetLevel())))
			if reward == nil {
				return errors.New("Missing reward table")
			}
			coin := int64(tableNumber(reward, "coin"))
			exp := int64(tableNumber(reward, "exp"))
			var err error
			if s.P4Ready {
				coin, exp, err = s.ApplyP4CopyCurrencyBonus(tx, LocalPlayAt(ctx, benefitAt), coin, exp)
				if err != nil {
					return err
				}
			}
			coin, exp, err = s.ApplyCookingRewardBonus(tx, id, time.Now().UnixMilli(), coin, exp, 0)
			if err != nil {
				return err
			}
			// 奖励超出协议存储范围时回滚战斗进度和全部奖励，保留可重试状态。
			if coin < 0 || exp < 0 || coin > math.MaxInt32 || exp > math.MaxInt32 || player.GetCoin() > math.MaxInt64-coin || player.GetExp() > math.MaxInt64-exp {
				return errors.New("Battle reward balance overflow")
			}
			player.Coin = proto.Int64(player.GetCoin() + coin)
			s.addExperience(player, exp)
			// 完成记录按关卡更新，结算重试在 fight.Settled 分支返回已有结果。
			s.refreshCopyProgress(g)
			stars := copyCompletionStars(b.GetBattlePlayer())
			for _, info := range g.Copy.GetPassedCopyInfo() {
				if info.GetChapterId() == run.Chapter && info.GetMissionId() == run.Mission {
					info.Star = proto.Int32(max(info.GetStar(), stars))
					info.ChallengeTimes = proto.Int32(info.GetChallengeTimes() + 1)
				}
			}
			s.refreshCopyProgress(g)
			result.Gain = proto.Clone(run.Gain).(*pb.ResultObject)
			result.Gain.Simps = append(result.Gain.Simps, &pb.SimpleItem{Id: proto.String("coin"), Num: proto.Int32(int32(coin))}, &pb.SimpleItem{Id: proto.String("exp"), Num: proto.Int32(int32(exp))})
			result.IsCopyComplete = proto.Int32(1)
			result.Star = proto.Int32(stars)
			run.Complete = true
			if s.P9PetsReady {
				adds, err := s.ApplyP9CopyIntimacy(tx, id, player, g, "copy:"+fight.Start.GetFightId())
				if err != nil {
					return err
				}
				result.PetAdds = adds
			}
			if s.P12WantedReady {
				// 整关成功才按原时段概率抽取；已结算回执在前段返回，不重复发现或奖励。
				found, gain, err := s.DiscoverP12Wanted(tx, id, player, g, "copy-complete:"+fight.Start.GetFightId(), benefitAt)
				if err != nil {
					return err
				}
				p1AppendGain(result.Gain, gain)
				if found != nil {
					result.WantedId = proto.String(found.GetWantedId())
					result.WantedLevel = proto.Int32(found.GetLevel())
				}
				// 固定发现奖会克隆GameState；重绑活动Run，确保结算回执保存到当前存档。
				if g.Run == nil || g.Run.Fight == nil {
					return errors.New("Missing active copy after Wanted discovery")
				}
				run, fight, progress = g.Run, g.Run.Fight, g.Copy
			}
		}
		if err := s.FinishOriginalCopyReplay(ctx, tx, id, fight.Start, b); err != nil {
			return err
		}
		run.Party = acceptedParty
		fight.Settled = true
		fight.Result = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result, &pb.SCPlayer{Player: updated}, progress}, nil
}
