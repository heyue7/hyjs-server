package service

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"slices"
	"strconv"
	"strings"
)

// recruitGuidePlayer 将未领取伙伴的客户端断点指向原招募入口，保留持久化任务进度。
func (s *GameService) recruitGuidePlayer(player *pb.Player, game *model.GameState) *pb.Player {
	if player == nil || game == nil || NoviceGuideSkipped(game) || player.GetNoviceGuideSchedule() <= 1 || game.GuideRecruit != nil || !slices.Contains(game.Quests, "quest_1") {
		return player
	}
	heroID := s.table("GameConfigure", "StaticName", "GuideWarriorRecruitCostType2")["NameValue"]
	if heroID == "" {
		return player
	}
	// 早期存档可能已有伙伴但没有新回执，不再次展示待领取状态。
	for _, warrior := range game.Warriors {
		if warrior.GetId() == heroID {
			return player
		}
	}
	updated := proto.Clone(player).(*pb.Player)
	updated.NoviceGuideSchedule = proto.Int32(1)
	return updated
}

// resumeRecruitGuide 在角色快照出口续接未领取步骤，避免后续任务快照覆盖入口。
func (s *GameService) resumeRecruitGuide(ctx context.Context, messages []proto.Message) {
	var playerID int64
	for _, message := range messages {
		switch response := message.(type) {
		case *pb.SCPlayer:
			if response.GetPlayer().GetPlayerId() > 0 {
				playerID = response.GetPlayer().GetPlayerId()
			}
		case *pb.SCPlayerLogin:
			if response.GetPlayer().GetPlayerId() > 0 {
				playerID = response.GetPlayer().GetPlayerId()
			}
		}
	}
	if playerID == 0 {
		return
	}
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).First(&row, playerID).Error; err != nil {
		return
	}
	storedPlayer, game, err := decodeProfile(&row)
	if err != nil {
		return
	}
	// 旧存档已先领首章宝箱时，只续接精确断点并保存；不补发任何奖励。
	if ReconcileChapterRewardGuide(storedPlayer, game) {
		if err := updateProfile(ctx, playerID, func(current *pb.Player, currentGame *model.GameState) error {
			ReconcileChapterRewardGuide(current, currentGame)
			storedPlayer = proto.Clone(current).(*pb.Player)
			game = currentGame
			return nil
		}); err != nil {
			return
		}
	}
	for _, message := range messages {
		switch response := message.(type) {
		case *pb.SCPlayer:
			if NoviceGuideSkipped(game) && response.Player.GetNoviceGuideSchedule() < constUtil.NoviceGuideInitialCompleted {
				response.Player = proto.Clone(response.Player).(*pb.Player)
				response.Player.NoviceGuideSchedule = proto.Int32(constUtil.NoviceGuideInitialCompleted)
			}
			if response.Player.GetNoviceGuideSchedule() == constUtil.ChapterRewardGuidePending && storedPlayer.GetNoviceGuideSchedule() == constUtil.ChapterRewardGuideCompleted {
				response.Player = proto.Clone(response.Player).(*pb.Player)
				response.Player.NoviceGuideSchedule = proto.Int32(constUtil.ChapterRewardGuideCompleted)
			}
			response.Player = s.recruitGuidePlayer(response.Player, game)
		case *pb.SCPlayerLogin:
			if NoviceGuideSkipped(game) && response.Player.GetNoviceGuideSchedule() < constUtil.NoviceGuideInitialCompleted {
				response.Player = proto.Clone(response.Player).(*pb.Player)
				response.Player.NoviceGuideSchedule = proto.Int32(constUtil.NoviceGuideInitialCompleted)
			}
			if response.Player.GetNoviceGuideSchedule() == constUtil.ChapterRewardGuidePending && storedPlayer.GetNoviceGuideSchedule() == constUtil.ChapterRewardGuideCompleted {
				response.Player = proto.Clone(response.Player).(*pb.Player)
				response.Player.NoviceGuideSchedule = proto.Int32(constUtil.ChapterRewardGuideCompleted)
			}
			response.Player = s.recruitGuidePlayer(response.Player, game)
		}
	}
}

// recruitGuide 按 APK 新手配置发放固定角色，并保留原响应供网络重试使用。
func (s *GameService) recruitGuide(ctx context.Context, id int64) ([]proto.Message, error) {
	var (
		result  *pb.SCWarriorRecruitGuide
		updated *pb.Player
	)
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if g.GuideRecruit != nil {
			result = g.GuideRecruit
			updated = proto.Clone(player).(*pb.Player)
			return nil
		}
		if !slices.Contains(g.Quests, "quest_1") {
			return errors.New("请先完成首个任务并领取奖励")
		}
		heroId := s.table("GameConfigure", "StaticName", "GuideWarriorRecruitCostType2")["NameValue"]
		if heroId == "" || s.table("Hero", "id", heroId) == nil {
			return errors.New("Missing guide recruit configuration")
		}
		warrior, err := model.GameMessage("Warrior", map[string]any{"id": heroId, "guid": fmt.Sprintf("%d-guide-recruit", id), "level": 1, "exp": 0, "talent": 0, "training": map[string]any{}, "tempTraining": map[string]any{}, "breached": map[string]any{"level": 1}, "awake": map[string]any{}})
		if err != nil {
			return err
		}
		w := warrior.(*pb.Warrior)
		g.Warriors = append(g.Warriors, w)
		result = &pb.SCWarriorRecruitGuide{Get: &pb.ResultObject{Wars: []*pb.Warrior{w}}}
		g.GuideRecruit = result
		if player.GetNoviceGuideSchedule() < 2 {
			player.NoviceGuideSchedule = proto.Int32(2)
		}
		updated = proto.Clone(player).(*pb.Player)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 同步领取后的引导阶段，让可选引导包的手动招募入口进入正常招募。
	return []proto.Message{result, &pb.SCPlayer{Player: updated}}, nil
}

// PrepareWarriors 补齐早期存档的空突破状态，保留已有经验、时间和消耗记录。
func PrepareWarriors(game *model.GameState) {
	for _, warrior := range game.Warriors {
		breach := warrior.GetBreached()
		if breach.GetLevel() == 0 && breach.GetExp() == 0 && breach.GetLastAddExpTime() == "" && breach.GetCostRecord() == "" {
			if breach == nil {
				breach = &pb.Breached{}
				warrior.Breached = breach
			}
			breach.Level = proto.Int32(1)
		}
	}
}

// warriorTalent 按主干顺序与分支前置扣费，消耗材料、激活节点和新手断点原子保存。
func (s *GameService) warriorTalent(ctx context.Context, id int64, b *pb.CSWarriorTalent) ([]proto.Message, error) {
	var (
		result  = &pb.SCWarriorTalent{}
		updated *pb.Player
	)
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if err := s.warriorTalentApply(player, g, b, result); err != nil {
			return err
		}
		updated = proto.Clone(player).(*pb.Player)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result, &pb.SCPlayer{Player: updated}}, nil
}

// warriorTalentApply keeps the original 1003-node costs, prerequisites and
// real donor selection in one action shared by direct and receipted callers.
func (s *GameService) warriorTalentApply(player *pb.Player, g *model.GameState, b *pb.CSWarriorTalent, result *pb.SCWarriorTalent) error {
	var (
		owned  = map[string]*pb.Warrior{}
		active = map[string]bool{}
	)
	for _, w := range g.Warriors {
		owned[w.GetGuid()] = w
	}
	warrior := owned[b.GetGuid()]
	if warrior == nil {
		return errors.New("未拥有该角色")
	}
	hero := s.table("Hero", "id", warrior.GetId())
	if hero == nil || hero["heroClass"] == "99" {
		return errors.New("经验材料不能激活天赋")
	}
	nodes := map[string]map[string]string{}
	for _, row := range s.Tables["TalentAdvenced"] {
		if row["warId"] == warrior.GetId() {
			nodes[row["talentLevel"]] = row
		}
	}
	talent := nodes[b.GetTalent()]
	if talent == nil {
		return errors.New("当前天赋配置不存在")
	}
	var target int64
	if talent["trunck"] == "1" {
		var err error
		target, err = strconv.ParseInt(b.GetTalent(), 10, 32)
		if err != nil || target != int64(warrior.GetTalent())+1 {
			return errors.New("天赋已激活或不是下一层主干天赋")
		}
	} else if talent["trunck"] != "0" || slices.Contains(warrior.GetTalentBranch(), b.GetTalent()) {
		return errors.New("分支天赋已激活或配置无效")
	}
	// 前置引用的是同一英雄的 talentLevel，主干已达层数和分支已激活均须满足。
	for _, key := range []string{"parent", "parent2"} {
		parent := talent[key]
		if parent == "" {
			continue
		}
		node := nodes[parent]
		if node == nil {
			return errors.New("缺少天赋前置配置")
		}
		if node["trunck"] == "1" {
			tier, err := strconv.ParseInt(parent, 10, 32)
			if err != nil || tier > int64(warrior.GetTalent()) {
				return errors.New("请先激活前置主干天赋")
			}
		} else if !slices.Contains(warrior.GetTalentBranch(), parent) {
			return errors.New("请先激活前置分支天赋")
		}
	}
	level := warrior.GetLevel()
	if warrior.GetGuid() == player.GetHeroWarId() {
		// APK 主角的等级来自 Player，早期 Warrior 存档仍可能保留初始等级。
		level = player.GetLevel()
	}
	if float64(level) < tableNumber(talent, "levelLimit") {
		return errors.New("角色等级不足")
	}
	var (
		costs      []*pb.BaseResource
		expected   = map[string]int32{}
		items      = map[string]*pb.SimpleItem{}
		materialId string
		lastItemId string
	)
	for _, part := range strings.Split(talent["cost"], ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return errors.New("Invalid talent cost configuration")
		}
		num, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || num <= 0 || expected[pair[0]] != 0 {
			return errors.New("Invalid talent cost configuration")
		}
		expected[pair[0]] = int32(num)
		if strings.HasPrefix(pair[0], "hero_") {
			if materialId != "" || s.table("Hero", "id", pair[0]) == nil {
				return errors.New("Invalid talent hero cost configuration")
			}
			materialId = pair[0]
			continue
		}
		if pair[0] == "coin" {
			if player.GetCoin() < num {
				return errors.New("金币不足")
			}
		} else if strings.HasPrefix(pair[0], "item_") && s.table("Item", "itemId", pair[0]) != nil {
			lastItemId = pair[0]
			for _, item := range g.Items {
				if item.GetId() == pair[0] {
					items[pair[0]] = item
					break
				}
			}
			if items[pair[0]] == nil || int64(items[pair[0]].GetNum()) < num {
				return errors.New("天赋材料不足")
			}
		} else {
			return errors.New("Unsupported talent cost configuration")
		}
		costs = append(costs, &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(num))})
	}
	for _, formation := range g.Formation {
		for _, thing := range formation.GetThings() {
			active[thing.GetGuid()] = true
		}
	}
	var (
		seen       = map[string]bool{}
		candidates []*pb.Warrior
	)
	for _, cost := range b.GetCost() {
		if cost.GetGuid() != "" {
			material := owned[cost.GetGuid()]
			if materialId == "" || material == nil || seen[cost.GetGuid()] || material.GetId() != materialId || (cost.GetId() != materialId && cost.GetId() != warrior.GetId()) || cost.GetNum() < 0 || cost.GetNum() > 1 || material.GetGuid() == warrior.GetGuid() || material.GetGuid() == player.GetHeroWarId() || active[material.GetGuid()] || !talentMaterialUnused(material) {
				return errors.New("天赋材料未拥有、已培养、重复或已上阵")
			}
			seen[cost.GetGuid()] = true
			candidates = append(candidates, material)
			continue
		}
		if strings.HasPrefix(cost.GetId(), "hero_") || seen[cost.GetId()] || cost.GetNum() != expected[cost.GetId()] || cost.GetNum() <= 0 {
			return errors.New("天赋消耗与配置不一致")
		}
		seen[cost.GetId()] = true
	}
	// TalentDetail 只发送最后一种道具和全部可用英雄 GUID；实际扣费始终取完整配置。
	if (expected["coin"] > 0 && !seen["coin"]) || (lastItemId != "" && !seen[lastItemId]) || len(candidates) < int(expected[materialId]) {
		return errors.New("天赋消耗与配置不一致")
	}
	used := map[string]bool{}
	for _, material := range candidates[:int(expected[materialId])] {
		used[material.GetGuid()] = true
		costs = append(costs, &pb.BaseResource{Id: proto.String(material.GetId()), Guid: proto.String(material.GetGuid()), Num: proto.Int32(1)})
	}
	for _, cost := range costs {
		if cost.GetId() == "coin" {
			player.Coin = proto.Int64(player.GetCoin() - int64(cost.GetNum()))
		} else if cost.GetGuid() == "" {
			item := items[cost.GetId()]
			item.Num = proto.Int32(item.GetNum() - cost.GetNum())
		}
	}
	remaining := make([]*pb.Warrior, 0, len(g.Warriors)-len(used))
	for _, w := range g.Warriors {
		if !used[w.GetGuid()] {
			remaining = append(remaining, w)
		}
	}
	g.Warriors = remaining
	if talent["trunck"] == "1" {
		warrior.Talent = proto.Int32(int32(target))
	} else {
		warrior.TalentBranch = append(warrior.TalentBranch, b.GetTalent())
	}
	if warrior.GetGuid() == player.GetHeroWarId() && target == 1 && player.GetNoviceGuideSchedule() >= 5 && player.GetNoviceGuideSchedule() < 6 {
		player.NoviceGuideSchedule = proto.Int32(6)
	}
	result.Guid = proto.Clone(warrior).(*pb.Warrior)
	result.Cost = costs
	return nil
}

// talentMaterialUnused 对齐 APK IsZeroHero，避免天赋消耗已培养的伙伴。
func talentMaterialUnused(w *pb.Warrior) bool {
	if w.GetLevel() != 1 || w.GetTalent() != 0 || len(w.GetTalentBranch()) != 0 || w.GetBreached().GetLevel() != 1 || w.GetAwake().GetStage() > 0 {
		return false
	}
	for _, training := range []*pb.Training{w.GetTraining(), w.GetTempTraining()} {
		if training.GetAtt() != 0 || training.GetHp() != 0 || training.GetPdef() != 0 || training.GetMdef() != 0 {
			return false
		}
	}
	return true
}

// warriorStrengthen 用实际拥有的闲置角色计算经验与金币，已消耗 GUID 不会再次扣费。
func (s *GameService) warriorStrengthen(ctx context.Context, id int64, b *pb.CSWarriorStrengthen) ([]proto.Message, error) {
	result := &pb.SCWarriorStrengthen{}
	var updated *pb.Player
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if err := s.warriorStrengthenApply(player, g, b, result); err != nil {
			return err
		}
		updated = proto.Clone(player).(*pb.Player)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result, &pb.SCPlayer{Player: updated}}, nil
}

// warriorStrengthenApply 保留原材料GUID、默认num=0、经验边界和成长金币规则。
func (s *GameService) warriorStrengthenApply(player *pb.Player, g *model.GameState, b *pb.CSWarriorStrengthen, result *pb.SCWarriorStrengthen) error {
	var (
		owned  = map[string]*pb.Warrior{}
		active = map[string]bool{}
		used   = map[string]bool{}
		gain   int64
		costs  []*pb.BaseResource
	)
	for _, w := range g.Warriors {
		owned[w.GetGuid()] = w
	}
	warrior := owned[b.GetGuid()]
	if warrior == nil || warrior.GetGuid() == player.GetHeroWarId() {
		return errors.New("仅支持升级拥有的伙伴角色")
	}
	hero := s.table("Hero", "id", warrior.GetId())
	if hero == nil || hero["heroClass"] == "99" {
		return errors.New("经验材料不能作为升级目标")
	}
	for _, formation := range g.Formation {
		for _, thing := range formation.GetThings() {
			active[thing.GetGuid()] = true
		}
	}
	if len(b.GetCost()) < 1 || len(b.GetCost()) > 5 {
		return errors.New("请添加 1-5 个升级材料角色")
	}
	for _, cost := range b.GetCost() {
		material := owned[cost.GetGuid()]
		if material == nil || material.GetId() != cost.GetId() || material.GetGuid() == b.GetGuid() || material.GetGuid() == player.GetHeroWarId() || active[material.GetGuid()] || used[material.GetGuid()] {
			return errors.New("升级材料未拥有、重复或已上阵")
		}
		// APK 仅发送材料 id/guid，num 默认 0；每个 GUID 始终代表一个角色。
		if cost.GetNum() < 0 || cost.GetNum() > 1 {
			return errors.New("升级材料数量无效")
		}
		row := s.table("Hero", "id", material.GetId())
		if row == nil || row["heroClass"] == "100" || row["heroClass"] == "101" || material.GetLevel() < 1 || material.GetExp() < 0 {
			return errors.New("Invalid material hero configuration")
		}
		provided, err := strconv.ParseInt(row["expBase"], 10, 64)
		if err != nil || provided <= 0 || provided > math.MaxInt64-material.GetExp() {
			return errors.New("Invalid material experience")
		}
		provided += material.GetExp()
		for level := int32(1); level < material.GetLevel(); level++ {
			threshold, err := s.warriorLevelExperience(row["quality"], level)
			if err != nil || provided > math.MaxInt64-threshold {
				return errors.New("Invalid material experience")
			}
			provided += threshold
		}
		if gain > math.MaxInt64-provided {
			return errors.New("Experience overflow")
		}
		gain += provided
		used[material.GetGuid()] = true
		costs = append(costs, &pb.BaseResource{Id: proto.String(material.GetId()), Guid: proto.String(material.GetGuid()), Num: proto.Int32(1)})
	}
	factor, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", "HeroStrengthCostExpToCoin")["NameValue"], 10, 64)
	if err != nil || factor <= 0 || gain > math.MaxInt32/factor {
		return errors.New("Invalid upgrade coin cost")
	}
	coin := gain * factor
	if player.GetCoin() < coin {
		return errors.New("金币不足")
	}
	if warrior.GetLevel() < 1 || warrior.GetExp() < 0 || warrior.GetExp() > math.MaxInt64-gain {
		return errors.New("Invalid target hero experience")
	}
	var maxLevel int32
	for _, row := range s.Tables["HeroExp"] {
		if row["quality"] == hero["quality"] {
			maxLevel++
		}
	}
	limit := min(player.GetLevel(), maxLevel)
	if limit < warrior.GetLevel() {
		return errors.New("伙伴等级已达到当前上限")
	}
	var (
		level = warrior.GetLevel()
		exp   = warrior.GetExp() + gain
	)
	for {
		threshold, err := s.warriorLevelExperience(hero["quality"], level)
		if err != nil {
			return err
		}
		if level == limit {
			if warrior.GetLevel() == limit && warrior.GetExp() >= threshold {
				return errors.New("伙伴经验已达到当前上限")
			}
			exp = min(exp, threshold)
			break
		}
		// 与 APK GetPredictLevelWithExp 保持严格大于当前门槛的升等边界。
		if exp <= threshold {
			break
		}
		exp -= threshold
		level++
	}
	warrior.Level = proto.Int32(level)
	warrior.Exp = proto.Int64(exp)
	player.Coin = proto.Int64(player.GetCoin() - coin)
	remaining := make([]*pb.Warrior, 0, len(g.Warriors)-len(used))
	for _, w := range g.Warriors {
		if !used[w.GetGuid()] {
			remaining = append(remaining, w)
		}
	}
	g.Warriors = remaining
	costs = append(costs, &pb.BaseResource{Id: proto.String("coin"), Num: proto.Int32(int32(coin))})
	if player.GetNoviceGuideSchedule() == 6 {
		for _, formation := range g.Formation {
			if formation.GetFormatPos() == 2 {
				for _, thing := range formation.GetThings() {
					if thing.GetGuid() == warrior.GetGuid() {
						player.NoviceGuideSchedule = proto.Int32(7)
					}
				}
			}
		}
	}
	*result = pb.SCWarriorStrengthen{Guid: proto.Clone(warrior).(*pb.Warrior), Cost: costs}
	return nil
}

// p5WarriorStrengthen 持久恢复已消耗材料GUID的提交回执，当前完整伙伴变化后旧回包失效。
func (s *GameService) p5WarriorStrengthen(ctx context.Context, id int64, b *pb.CSWarriorStrengthen, stamp int64) ([]proto.Message, error) {
	out := &pb.SCWarriorStrengthen{}
	return s.p5Mutation(ctx, id, constUtil.P5WarriorStrengthenOperation, stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		return s.warriorStrengthenApply(p, g, b, out)
	})
}

// warriorLevelExperience 使用资质与当前等级查询 APK 的下一等级经验门槛。
func (s *GameService) warriorLevelExperience(quality string, level int32) (int64, error) {
	for _, row := range s.Tables["HeroExp"] {
		if row["quality"] == quality && row["level"] == strconv.FormatInt(int64(level), 10) {
			exp, err := strconv.ParseInt(row["exp"], 10, 64)
			if err == nil && exp > 0 {
				return exp, nil
			}
			break
		}
	}
	return 0, errors.New("Missing hero level experience")
}
