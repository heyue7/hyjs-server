package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitGems 建立独立生活技能状态及回执；运营规则仅首次创建，重启保留调整。
func (s *GameService) InitGems(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := tx.AutoMigrate(&db.GemCraftState{}, &db.GemMutationReceipt{}, &db.GemRule{}); err != nil {
			return err
		}
		// 用户确认的临时产率仅首次创建，已有运营调整继续保留。
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.GemRule{ID: 1, Enabled: true, CrystalPerBatch: 1, Source: constUtil.GemDefaultRuleSource}).Error
	})
}

// gemMutation 锁定同一玩家后完成库存、等级和回执，全部业务失败随事务回滚。
func (s *GameService) gemMutation(ctx context.Context, id int64, opcode int32, stamp int64, request, response proto.Message, apply func(*gorm.DB, *pb.Player, *model.GameState, *db.GemCraftState) error) error {
	if stamp <= 0 {
		return errors.New("宝石请求缺少有效时间戳")
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		var receipt db.GemMutationReceipt
		err := tx.Where("player_id = ? AND opcode = ?", id, opcode).Take(&receipt).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && stamp <= receipt.Timestamp {
			if stamp < receipt.Timestamp {
				return errors.New("宝石请求已过期")
			}
			prior := request.ProtoReflect().Type().New().Interface()
			if proto.Unmarshal(receipt.Request, prior) != nil || !proto.Equal(prior, request) {
				return errors.New("宝石请求与回执不匹配")
			}
			if err = proto.Unmarshal(receipt.Response, response); err != nil {
				return err
			}
			if _, carriesEquipment := response.(interface{ GetEquip() *pb.Equipment }); !carriesEquipment {
				return nil
			}
			// Original mount/change/unmount callbacks replace the complete gear.
			// Retain the audit, but reject a grown, moved or removed old body.
			_, current, err := decodeProfileForCalendarUpdate(tx, &row)
			if err != nil {
				return err
			}
			return s.validateP5GrowthReceipt(current, response)
		}
		var state db.GemCraftState
		if err = tx.Where("player_id = ?", id).Take(&state).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			state = db.GemCraftState{PlayerID: id, Level: 1}
		} else if err != nil {
			return err
		}
		if state.Level < 1 || state.Level > 10 || state.Exp < 0 {
			return errors.New("冶炼等级存档无效")
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if err = s.PrepareP5GuildPrayerProperties(ctx, tx, player, game, p13UFNow(ctx)); err != nil {
			return err
		}
		before, err := s.P8GemTransferSnapshot(game, request)
		if err != nil {
			return err
		}
		if err = apply(tx, player, game, &state); err != nil {
			return err
		}
		if err = s.P8RecordGemTransfer(game, before); err != nil {
			return err
		}
		if opcode == constUtil.GemMountOpcode || opcode == constUtil.GemMountChangeOpcode || opcode == constUtil.GemUnmountOpcode {
			if err = s.P8RefreshCraftPower(player, game); err != nil {
				return err
			}
		}
		if err = saveProfile(tx, &row, player, game); err != nil {
			return err
		}
		if err = tx.Save(&state).Error; err != nil {
			return err
		}
		rawRequest, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		rawResponse, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		return tx.Save(&db.GemMutationReceipt{PlayerID: id, Opcode: opcode, Timestamp: stamp, Request: rawRequest, Response: rawResponse}).Error
	})
}

// GemCraftLevel 供登录的 SCCookingLevel 接线使用，旧存档从 1 级和零经验开始。
func (s *GameService) GemCraftLevel(ctx context.Context, id int64) (int32, int32, error) {
	var state db.GemCraftState
	err := db.GetIns().WithContext(ctx).Where("player_id = ?", id).Take(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	if state.Level < 1 || state.Level > 10 || state.Exp < 0 {
		return 0, 0, errors.New("冶炼等级存档无效")
	}
	return state.Level, state.Exp, nil
}

func (s *GameService) gemGate(player *pb.Player, state *db.GemCraftState, row map[string]string, transaction ...*gorm.DB) error {
	if err := s.gemLevelGate(player, state, row); err != nil {
		return err
	}
	if row["SkillNeeded"] != "" {
		var tx *gorm.DB
		if len(transaction) > 0 {
			tx = transaction[0]
		}
		learned, err := P8HasLifeSkill(tx, player.GetPlayerId(), row["SkillNeeded"])
		if err != nil {
			return err
		}
		if !learned {
			return errors.New("该配方需要实际学习高级生活技能书")
		}
	}
	return nil
}

func (s *GameService) gemLevelGate(player *pb.Player, state *db.GemCraftState, row map[string]string) error {
	gate := tableNumber(s.table("GameConfigure", "StaticName", "OutdoorLevel"), "NameValue")
	if gate <= 0 || float64(player.GetLevel()) < gate {
		return errors.New("生活技能尚未开放")
	}
	if row == nil || tableNumber(row, "LevelLimit") < 1 || tableNumber(row, "LevelLimit") > float64(state.Level) {
		return errors.New("冶炼等级不足或配置无效")
	}
	return nil
}

func (s *GameService) gemAddExperience(state *db.GemCraftState, amount int64) error {
	if amount <= 0 || amount > math.MaxInt32 || int64(state.Exp)+amount > math.MaxInt32 {
		return errors.New("冶炼经验配置无效")
	}
	state.Exp += int32(amount)
	for state.Level < 10 {
		limit := int64(tableNumber(s.table("GemMakeLevelup", "level", strconv.Itoa(int(state.Level))), "exp"))
		if limit <= 0 {
			return errors.New("冶炼升级经验配置缺失")
		}
		if int64(state.Exp) < limit {
			break
		}
		state.Exp -= int32(limit)
		state.Level++
	}
	if state.Level == 10 {
		state.Exp = 0
	}
	return nil
}

// gemInventoryDelta 统一检查每类矿石的唯一堆叠和数量，避免负数或重复库存绕过扣费。
func (s *GameService) gemInventoryDelta(player *pb.Player, g *model.GameState, deltas map[string]int64) error {
	stacks := map[string]*pb.SimpleItem{}
	for _, item := range g.Gems {
		if item == nil || item.GetId() == "" || item.GetNum() < 0 || stacks[item.GetId()] != nil {
			return errors.New("宝石库存存档无效")
		}
		stacks[item.GetId()] = item
	}
	for key, delta := range deltas {
		if key == "coin" {
			if delta > 0 || player.GetCoin() < 0 || player.GetCoin()+delta < 0 {
				return errors.New("制作金币不足")
			}
			continue
		}
		if s.table("MineSkill", "MineItemID", key) == nil {
			return errors.New("宝石库存配置不存在")
		}
		balance := int64(stacks[key].GetNum()) + delta
		if balance < 0 || balance > math.MaxInt32 {
			return errors.New("宝石材料不足或库存溢出")
		}
	}
	for key, delta := range deltas {
		if key == "coin" {
			player.Coin = proto.Int64(player.GetCoin() + delta)
			continue
		}
		if stacks[key] == nil {
			stacks[key] = &pb.SimpleItem{Id: proto.String(key)}
			g.Gems = append(g.Gems, stacks[key])
		}
		stacks[key].Num = proto.Int32(int32(int64(stacks[key].GetNum()) + delta))
	}
	var remaining []*pb.SimpleItem
	for _, item := range g.Gems {
		if item.GetNum() > 0 {
			remaining = append(remaining, item)
		}
	}
	g.Gems = remaining
	return nil
}

func gemCosts(deltas map[string]int64) []*pb.BaseResource {
	var keys []string
	for key, n := range deltas {
		if n < 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var costs []*pb.BaseResource
	for _, key := range keys {
		costs = append(costs, &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(int32(-deltas[key]))})
	}
	return costs
}

func gemRewards(gains map[string]int64) *pb.ResultObject {
	var keys []string
	for key, n := range gains {
		if n > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := &pb.ResultObject{}
	for _, key := range keys {
		result.Simps = append(result.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(gains[key]))})
	}
	return result
}

// gemMake 使用 APK 中完整 Material 配方，客户端仅指定成品和数量。
func (s *GameService) gemMake(ctx context.Context, id int64, request *pb.CSGemMake, stamp int64) ([]proto.Message, error) {
	result := &pb.SCGemMake{}
	err := s.gemMutation(ctx, id, constUtil.GemMakeOpcode, stamp, request, result, func(tx *gorm.DB, p *pb.Player, g *model.GameState, state *db.GemCraftState) error {
		row := s.table("MineSkill", "MineItemID", request.GetGemId())
		if request.GetNum() < 1 || request.GetNum() > constUtil.GemMaxBatch || row["Type"] != "magicGem" || row["Material"] == "" {
			return errors.New("宝石制作配方或数量无效")
		}
		if err := s.gemGate(p, state, row, tx); err != nil {
			return err
		}
		deltas := map[string]int64{}
		for _, entry := range strings.Split(row["Material"], ";") {
			pair := strings.Split(entry, ",")
			if len(pair) != 2 || deltas[pair[0]] != 0 {
				return errors.New("宝石制作材料配置无效")
			}
			n, err := strconv.ParseInt(pair[1], 10, 32)
			if err != nil || n <= 0 || n*int64(request.GetNum()) > math.MaxInt32 {
				return errors.New("宝石材料数量配置无效")
			}
			deltas[pair[0]] = -n * int64(request.GetNum())
		}
		cost := gemCosts(deltas)
		deltas[request.GetGemId()] += int64(request.GetNum())
		if err := s.gemInventoryDelta(p, g, deltas); err != nil {
			return err
		}
		add := int64(tableNumber(row, "MakingExp")) * int64(request.GetNum())
		if err := s.gemAddExperience(state, add); err != nil {
			return err
		}
		result.Cost, result.Res = cost, gemRewards(map[string]int64{request.GetGemId(): int64(request.GetNum())})
		result.Level, result.Exp, result.Add = proto.Int32(state.Level), proto.Int32(state.Exp), proto.Int32(int32(add))
		return nil
	})
	return []proto.Message{result}, err
}

func (s *GameService) gemHoles(equip *pb.Equipment) ([]int32, error) {
	row := s.table("Equip", "id", equip.GetEquipId())
	if row == nil || len(equip.GetGems()) > 4 {
		return nil, errors.New("宝石装备或孔位存档无效")
	}
	holes := make([]int32, 4)
	for i := range holes {
		holes[i] = int32(tableNumber(row, "hole"+strconv.Itoa(i+1)))
		if holes[i] < 0 || holes[i] > 6 {
			return nil, errors.New("装备孔位颜色配置无效")
		}
	}
	return holes, nil
}

// equipmentGemProperties 严格验证真实四孔颜色并加入原表 Value，供面板和战斗共用。
func (s *GameService) equipmentGemProperties(equip *pb.Equipment, values map[string]float64) error {
	holes, err := s.gemHoles(equip)
	if err != nil {
		return err
	}
	for i, key := range equip.GetGems() {
		if key == "" {
			continue
		}
		row := s.table("MineSkill", "MineItemID", key)
		color := int32(tableNumber(row, "Color"))
		if row["Type"] != "magicGem" || holes[i] == 0 || color < 1 || color > 7 || (color != 7 && color != holes[i]) {
			return errors.New("宝石与装备孔位颜色不匹配")
		}
		for _, entry := range strings.Split(row["Value"], ";") {
			pair := strings.Split(entry, ",")
			if len(pair) != 2 {
				return errors.New("宝石属性格式无效")
			}
			key := pair[0]
			if key != "att" && key != "hp" && key != "pdef" && key != "mdef" && key != "cri" && key != "antiCri" {
				return errors.New("宝石属性类型无效")
			}
			value, err := strconv.ParseFloat(pair[1], 64)
			if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return errors.New("宝石属性数值无效")
			}
			values[key] += value
		}
	}
	return nil
}

func (s *GameService) gemMountApply(p *pb.Player, g *model.GameState, guid, key string, pos int32, auto, replace bool) (*pb.Equipment, []*pb.BaseResource, *pb.ResultObject, error) {
	equip, err := s.equipmentRecord(g, guid)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = s.equipmentGemProperties(equip, map[string]float64{}); err != nil {
		return nil, nil, nil, err
	}
	gate := tableNumber(s.table("GameConfigure", "StaticName", "OutdoorLevel"), "NameValue")
	if gate <= 0 || float64(p.GetLevel()) < gate {
		return nil, nil, nil, errors.New("生活技能尚未开放")
	}
	holes, err := s.gemHoles(equip)
	if err != nil {
		return nil, nil, nil, err
	}
	row := s.table("MineSkill", "MineItemID", key)
	color := int32(tableNumber(row, "Color"))
	if row["Type"] != "magicGem" || color < 1 || color > 7 {
		return nil, nil, nil, errors.New("该物品不能镶嵌")
	}
	// 四位固定索引与 Equipment.gems 原数组保持一致；原客户端拖放使用自动孔位消息。
	for len(equip.Gems) < 4 {
		equip.Gems = append(equip.Gems, "")
	}
	if auto {
		pos = -1
		for i, hole := range holes {
			if hole != 0 && (color == 7 || color == hole) && equip.Gems[i] == "" {
				pos = int32(i)
				break
			}
		}
		if pos < 0 {
			for i, hole := range holes {
				if hole != 0 && (color == 7 || color == hole) {
					pos = int32(i)
					break
				}
			}
		}
	}
	if pos < 0 || pos > 3 || holes[pos] == 0 || (color != 7 && holes[pos] != color) {
		return nil, nil, nil, errors.New("没有可镶嵌该颜色的装备孔位")
	}
	old := equip.Gems[pos]
	if old == key {
		return nil, nil, nil, errors.New("该孔位已经镶嵌同种宝石")
	}
	if old != "" && !replace {
		return nil, nil, nil, errors.New("该宝石孔位已有宝石")
	}
	deltas := map[string]int64{key: -1}
	gains := map[string]int64{}
	if old != "" {
		deltas[old]++
		gains[old]++
	}
	if err = s.gemInventoryDelta(p, g, deltas); err != nil {
		return nil, nil, nil, err
	}
	equip.Gems[pos] = key
	if err = s.equipmentGemProperties(equip, map[string]float64{}); err != nil {
		return nil, nil, nil, err
	}
	return proto.Clone(equip).(*pb.Equipment), []*pb.BaseResource{{Id: proto.String(key), Num: proto.Int32(1)}}, gemRewards(gains), nil
}

func (s *GameService) gemMount(ctx context.Context, id int64, r *pb.CSGemMount, stamp int64) ([]proto.Message, error) {
	result := &pb.SCGemMount{}
	err := s.gemMutation(ctx, id, constUtil.GemMountOpcode, stamp, r, result, func(_ *gorm.DB, p *pb.Player, g *model.GameState, _ *db.GemCraftState) error {
		var err error
		result.Equip, result.Cost, _, err = s.gemMountApply(p, g, r.GetGuid(), r.GetGemId(), r.GetPos(), false, false)
		return err
	})
	return []proto.Message{result}, err
}

func (s *GameService) gemMountOrChange(ctx context.Context, id int64, r *pb.CSGemMountOrChange, stamp int64) ([]proto.Message, error) {
	result := &pb.SCGemMountOrChange{}
	err := s.gemMutation(ctx, id, constUtil.GemMountChangeOpcode, stamp, r, result, func(_ *gorm.DB, p *pb.Player, g *model.GameState, _ *db.GemCraftState) error {
		var err error
		result.Equip, result.Cost, result.Res, err = s.gemMountApply(p, g, r.GetGuid(), r.GetGemId(), 0, true, true)
		return err
	})
	return []proto.Message{result}, err
}

func (s *GameService) gemUnmount(ctx context.Context, id int64, r *pb.CSGemUnmount, stamp int64) ([]proto.Message, error) {
	result := &pb.SCGemUnmount{}
	err := s.gemMutation(ctx, id, constUtil.GemUnmountOpcode, stamp, r, result, func(_ *gorm.DB, p *pb.Player, g *model.GameState, _ *db.GemCraftState) error {
		equip, err := s.equipmentRecord(g, r.GetGuid())
		if err != nil {
			return err
		}
		if err = s.equipmentGemProperties(equip, map[string]float64{}); err != nil {
			return err
		}
		if r.GetPos() < 0 || r.GetPos() >= int32(len(equip.Gems)) || equip.Gems[r.GetPos()] == "" {
			return errors.New("指定孔位没有宝石")
		}
		key := equip.Gems[r.GetPos()]
		if err = s.gemInventoryDelta(p, g, map[string]int64{key: 1}); err != nil {
			return err
		}
		equip.Gems[r.GetPos()] = ""
		result.Equip, result.Res = proto.Clone(equip).(*pb.Equipment), gemRewards(map[string]int64{key: 1})
		return nil
	})
	return []proto.Message{result}, err
}

func gemRoll(limit int64) (int64, error) {
	if limit <= 0 {
		return 0, errors.New("提炼概率配置无效")
	}
	v, err := rand.Int(rand.Reader, big.NewInt(limit))
	if err != nil {
		return 0, err
	}
	return v.Int64(), nil
}

// gemWeightedCount 恢复原表 id:n,weight 格式，其中零产出也是一个真实结果。
func gemWeightedCount(spec string) (string, int64, error) {
	parts := strings.Split(spec, ":")
	if len(parts) != 2 || parts[0] == "" {
		return "", 0, errors.New("提炼产出配置无效")
	}
	var total int64
	weights := map[int64]int64{}
	var counts []int64
	for _, entry := range strings.Split(parts[1], ";") {
		pair := strings.Split(entry, ",")
		if len(pair) != 2 {
			return "", 0, errors.New("提炼权重配置无效")
		}
		n, err := strconv.ParseInt(pair[0], 10, 32)
		if err != nil || n < 0 {
			return "", 0, errors.New("提炼产出数量无效")
		}
		w, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || w <= 0 || weights[n] != 0 {
			return "", 0, errors.New("提炼权重配置無效")
		}
		counts = append(counts, n)
		weights[n] = w
		total += w
	}
	roll, err := gemRoll(total)
	if err != nil {
		return "", 0, err
	}
	for _, n := range counts {
		if roll < weights[n] {
			return parts[0], n, nil
		}
		roll -= weights[n]
	}
	return "", 0, errors.New("提炼概率无法命中")
}

// gemRefine 按五个矿石一批提炼，随机结果冻结在回执中，原表决定魔石、粉尘和七彩产出。
func (s *GameService) gemRefine(ctx context.Context, id int64, r *pb.CSMineRefine, stamp int64) ([]proto.Message, error) {
	result := &pb.SCMineRefine{}
	err := s.gemMutation(ctx, id, constUtil.GemRefineOpcode, stamp, r, result, func(tx *gorm.DB, p *pb.Player, g *model.GameState, state *db.GemCraftState) error {
		row := s.table("MineSkill", "MineItemID", r.GetMineId())
		if row["Type"] != "stone" || r.GetNum() < 5 || r.GetNum() > constUtil.GemMaxBatch || r.GetNum()%5 != 0 {
			return errors.New("提炼必须选择5的倍数矿石")
		}
		if err := s.gemGate(p, state, row, tx); err != nil {
			return err
		}
		var rule db.GemRule
		if err := tx.Take(&rule, 1).Error; err != nil {
			return err
		}
		if !rule.Enabled || rule.CrystalPerBatch < 0 || rule.CrystalPerBatch > 100 {
			return errors.New("本地提炼规则无效或未启用")
		}
		gains := map[string]int64{}
		for batch := int32(0); batch < r.GetNum()/5; batch++ {
			roll, err := gemRoll(1000000)
			if err != nil {
				return err
			}
			var total int64
			var magic string
			for _, entry := range strings.Split(row["ExtractMagicStone"], ";") {
				pair := strings.Split(entry, ",")
				if len(pair) != 2 {
					return errors.New("魔石提炼配置无效")
				}
				prob, err := strconv.ParseFloat(pair[0], 64)
				if err != nil || prob <= 0 || prob > 1 {
					return errors.New("魔石提炼概率无效")
				}
				weight := int64(math.Round(prob * 1000000))
				if roll >= total && roll < total+weight {
					magic = pair[1]
				}
				total += weight
			}
			if total != 1000000 || magic == "" {
				return errors.New("魔石提炼概率总和无效")
			}
			gains[magic]++
			key, n, err := gemWeightedCount(row["ExtractPowder"])
			if err != nil {
				return err
			}
			gains[key] += n
			if row["ExtractGem"] != "" {
				key, n, err = gemWeightedCount(row["ExtractGem"])
				if err != nil {
					return err
				}
				gains[key] += n
			}
			// 原 APK 未给六色晶石的服务端分布，使用可审计、可关闭的本地 SQL 策略。
			for crystal := int32(0); crystal < rule.CrystalPerBatch; crystal++ {
				color, err := gemRoll(6)
				if err != nil {
					return err
				}
				gains[fmt.Sprintf("MineItem_%03d", 13+color)]++
			}
		}
		deltas := map[string]int64{r.GetMineId(): -int64(r.GetNum())}
		for key, n := range gains {
			deltas[key] += n
		}
		if err := s.gemInventoryDelta(p, g, deltas); err != nil {
			return err
		}
		// 客户端显示 selectedNum / 5 * MakingExp，经验按批次累加。
		add := int64(r.GetNum()/5) * int64(tableNumber(row, "MakingExp"))
		if err := s.gemAddExperience(state, add); err != nil {
			return err
		}
		result.Cost = []*pb.BaseResource{{Id: proto.String(r.GetMineId()), Num: proto.Int32(r.GetNum())}}
		result.Res = gemRewards(gains)
		result.Level, result.Exp, result.Add = proto.Int32(state.Level), proto.Int32(state.Exp), proto.Int32(int32(add))
		return nil
	})
	return []proto.Message{result}, err
}

// gemCombineApply 按原客户端的目标 ID 恢复同色升级；三合一普通魔石，六合一七彩 7→10。
func (s *GameService) gemCombineApply(p *pb.Player, g *model.GameState, state *db.GemCraftState, target string, count int32) ([]*pb.BaseResource, *pb.ResultObject, error) {
	row := s.table("MineSkill", "MineItemID", target)
	if row["Type"] != "magicGem" || row["Compound"] != "1" {
		return nil, nil, errors.New("该宝石不能由低级宝石合成")
	}
	if err := s.gemLevelGate(p, state, row); err != nil {
		return nil, nil, err
	}
	color, level := tableNumber(row, "Color"), tableNumber(row, "Level")
	previous := level - 1
	if count == 6 {
		// Original high-tab colour 7 selects source level + 3. The
		// recovered catalogue contains both 7→10 and 10→13 targets.
		if color != 7 || (level != 10 && level != 13) {
			return nil, nil, errors.New("六合一仅支持原表七彩宝石")
		}
		previous = level - 3
	} else if count != 3 || color < 1 || color > 6 {
		return nil, nil, errors.New("三合一需要普通同色宝石")
	}
	var source map[string]string
	for _, candidate := range s.Tables["MineSkill"] {
		if candidate["Type"] == "magicGem" && tableNumber(candidate, "Color") == color && tableNumber(candidate, "Level") == previous {
			if source != nil {
				return nil, nil, errors.New("同色宝石配置重复")
			}
			source = candidate
		}
	}
	if source == nil || source["composeMeterial"] != "1" {
		return nil, nil, errors.New("合成原料宝石配置缺失")
	}
	key := source["MineItemID"]
	if err := s.gemInventoryDelta(p, g, map[string]int64{key: -int64(count), target: 1}); err != nil {
		return nil, nil, err
	}
	return []*pb.BaseResource{{Id: proto.String(key), Num: proto.Int32(count)}}, gemRewards(map[string]int64{target: 1}), nil
}

func (s *GameService) gemCombineThree(ctx context.Context, id int64, r *pb.CSGemCombineByThree, stamp int64) ([]proto.Message, error) {
	result := &pb.SCGemCombineByThree{}
	err := s.gemMutation(ctx, id, constUtil.GemCombineThreeOpcode, stamp, r, result, func(_ *gorm.DB, p *pb.Player, g *model.GameState, state *db.GemCraftState) error {
		var err error
		result.Cost, result.Res, err = s.gemCombineApply(p, g, state, r.GetGemId(), 3)
		return err
	})
	return []proto.Message{result}, err
}

func (s *GameService) gemCombineSix(ctx context.Context, id int64, r *pb.CSGemCombineBySix, stamp int64) ([]proto.Message, error) {
	result := &pb.SCGemCombineBySix{}
	err := s.gemMutation(ctx, id, constUtil.GemCombineSixOpcode, stamp, r, result, func(_ *gorm.DB, p *pb.Player, g *model.GameState, state *db.GemCraftState) error {
		var err error
		result.Cost, result.Res, err = s.gemCombineApply(p, g, state, r.GetGemId(), 6)
		return err
	})
	return []proto.Message{result}, err
}

// gemCombineTwo 原消息三个 ID 依次为两个异色原料与指定成品，全部同等级且不能选择原颜色。
func (s *GameService) gemCombineTwo(ctx context.Context, id int64, r *pb.CSGemCombineByTwo, stamp int64) ([]proto.Message, error) {
	result := &pb.SCGemCombineByTwo{}
	err := s.gemMutation(ctx, id, constUtil.GemCombineTwoOpcode, stamp, r, result, func(_ *gorm.DB, p *pb.Player, g *model.GameState, state *db.GemCraftState) error {
		if len(r.GetGemId()) != 3 {
			return errors.New("转换需要两个原料和一个指定成品")
		}
		rows := make([]map[string]string, 3)
		colors := map[string]bool{}
		for i, key := range r.GetGemId() {
			rows[i] = s.table("MineSkill", "MineItemID", key)
			row := rows[i]
			if row["Type"] != "magicGem" || tableNumber(row, "Color") < 1 || tableNumber(row, "Color") > 6 || colors[row["Color"]] {
				return errors.New("转换需要三种不同的普通宝石颜色")
			}
			colors[row["Color"]] = true
			if i > 0 && row["Level"] != rows[0]["Level"] {
				return errors.New("转换宝石等级必须一致")
			}
		}
		if err := s.gemLevelGate(p, state, rows[2]); err != nil {
			return err
		}
		if err := s.gemInventoryDelta(p, g, map[string]int64{r.GemId[0]: -1, r.GemId[1]: -1, r.GemId[2]: 1}); err != nil {
			return err
		}
		result.Cost = []*pb.BaseResource{{Id: proto.String(r.GemId[0]), Num: proto.Int32(1)}, {Id: proto.String(r.GemId[1]), Num: proto.Int32(1)}}
		result.Res = gemRewards(map[string]int64{r.GemId[2]: 1})
		return nil
	})
	return []proto.Message{result}, err
}
