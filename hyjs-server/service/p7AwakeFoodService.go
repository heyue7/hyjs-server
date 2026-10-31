package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The original two slots contain up to three items of each recoverable type.
// Quantities remain in the ordinary inventory until a successful use commits.
type p7AwakeFoodSlot struct {
	ID        string  `json:"id"`
	Initial   int32   `json:"initial"`
	Remaining int32   `json:"remaining"`
	Percent   float64 `json:"percent"`
}

// Resume the exact enemy actors from the previously accepted original-engine
// terminal. The immutable record and receipt remain the authority for every HP.
func p7AwakeContinuedEnemy(tx *gorm.DB, run *db.P7AwakeRun, fightID, monsterGUID string, before int64) (*pb.BattleMonster, error) {
	var fight db.P7AwakeFight
	if err := tx.Where("id=? AND run_id=? AND player_id=? AND monster_guid=?", fightID, run.ID, run.PlayerID, monsterGUID).First(&fight).Error; err != nil {
		return nil, errors.New("觉醒续战缺少前场记录")
	}
	if fight.Status != constUtil.P7AwakeFightAccepted || fight.AcceptedAt <= 0 || fight.AcceptedAt > before || p6Digest(fight.Start) != fight.StartHash || p6Digest(fight.EndRequest) != fight.EndHash || p6Digest(fight.Terminal) != fight.TerminalHash || p6Digest(fight.Reply) != fight.ReplyHash {
		return nil, errors.New("觉醒续战前场回执无效")
	}
	var record db.P6BattleRecord
	if err := tx.Where("id=? AND player_id=? AND source=? AND source_fight=?", fight.ReplayID, run.PlayerID, constUtil.P7AwakeBattleReplaySource, fight.ID).First(&record).Error; err != nil {
		return nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil || input == nil || input.Monster == nil || fight.InputHash != record.InputHash || record.Status != constUtil.P6BattleComputed || !record.Authoritative || p6Digest(record.Outcome) != record.OutcomeHash {
		return nil, errors.New("觉醒续战前场战斗来源无效")
	}
	start := &pb.SCAwakeCopyFightBegin{}
	if proto.Unmarshal(fight.Start, start) != nil || start.GetFightId() != fight.ID || !proto.Equal(start.BattleMonser, input.Monster) || !proto.Equal(start.BattlePlayer, input.Attacker) {
		return nil, errors.New("觉醒续战前场快照无效")
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal(fight.Terminal, &terminal) != nil || terminal.Outcome == nil || terminal.Outcome.Winner == 0 {
		return nil, errors.New("已获胜的觉醒节点不能续战")
	}
	outcome, err := json.Marshal(terminal.Outcome)
	if err != nil || !bytes.Equal(outcome, record.Outcome) {
		return nil, errors.New("觉醒续战终局与录像不一致")
	}
	enemy := proto.Clone(input.Monster).(*pb.BattleMonster)
	seen := map[string]bool{}
	alive := false
	for _, actual := range terminal.FinalActors {
		if actual.Camp != 1 {
			continue
		}
		if seen[actual.GUID] || math.IsNaN(actual.CurrentHP) || math.IsInf(actual.CurrentHP, 0) || actual.CurrentHP < 0 || actual.CurrentHP > actual.MaxHP {
			return nil, errors.New("觉醒续战怪物生命无效")
		}
		found := false
		for _, hero := range enemy.Formation {
			if hero.GetGuid() == actual.GUID && hero.GetInst() != nil && hero.GetProp() != nil && hero.GetProp().GetHp() == actual.MaxHP {
				hero.Inst.CurHp = proto.Float64(actual.CurrentHP)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("觉醒续战怪物身份不一致")
		}
		seen[actual.GUID] = true
		alive = alive || actual.CurrentHP > 0
	}
	if len(seen) != len(enemy.Formation) || !alive {
		return nil, errors.New("觉醒续战怪物生命不完整")
	}
	return enemy, nil
}

type p7AwakeFoodReceipt struct {
	Stamp    int64  `json:"stamp"`
	ItemID   string `json:"itemId"`
	Response []byte `json:"response"`
}

// The original callback treats a remaining quantity as a backpack debit. Only
// the new client has the scoped fix; bind this check to the authenticated login.
func p7AwakeRequireFoodClient(tx *gorm.DB, playerID int64) error {
	var profile db.Profile
	if err := tx.Select("id", "session").First(&profile, playerID).Error; err != nil {
		return err
	}
	var binding db.ClientUpdateSession
	if len(profile.Session) != 16 || tx.Where("player_id=?", playerID).First(&binding).Error != nil || binding.SessionHash != clientUpdateSessionHash(profile.Session) || binding.BuildCode < 2026103105 {
		return errors.New("请更新至1.0.91后使用觉醒料理")
	}
	return nil
}

func (s *GameService) p7AwakeFoodSelection(p *pb.Player, g *model.GameState, items []*pb.SimpleItem) ([]p7AwakeFoodSlot, error) {
	if len(items) > 2 {
		return nil, errors.New("最多携带两种恢复道具")
	}
	var slots []p7AwakeFoodSlot
	seen := map[string]bool{}
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.GetId()) != item.GetId() || item.GetId() == "" || seen[item.GetId()] || item.GetNum() < 1 || item.GetNum() > 3 {
			return nil, errors.New("觉醒携带道具或数量无效")
		}
		percent, err := s.p8FoodPercent(item.GetId())
		if err != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || p4ResourceBalance(p, g, item.GetId()) < int64(item.GetNum()) {
			return nil, errors.New("恢复道具不足或配置无效")
		}
		seen[item.GetId()] = true
		slots = append(slots, p7AwakeFoodSlot{ID: item.GetId(), Initial: item.GetNum(), Remaining: item.GetNum(), Percent: percent})
	}
	return slots, nil
}

func p7AwakeFoodItems(state *p7AwakeExplorationState) []*pb.SimpleItem {
	var items []*pb.SimpleItem
	for _, slot := range state.Food {
		items = append(items, &pb.SimpleItem{Id: proto.String(slot.ID), Num: proto.Int32(slot.Remaining)})
	}
	return items
}

func p7AwakeFoodSelectionMatches(slots []p7AwakeFoodSlot, items []*pb.SimpleItem) bool {
	if len(slots) != len(items) {
		return false
	}
	for i, slot := range slots {
		if items[i] == nil || slot.ID != items[i].GetId() || slot.Initial != items[i].GetNum() {
			return false
		}
	}
	return true
}

func p7AwakeValidateFood(state *p7AwakeExplorationState) error {
	if len(state.Food) > 2 || len(state.FoodReceipts) > 6 || state.FoodReadyAt < 0 {
		return errors.New("觉醒携带道具账本无效")
	}
	known := map[string]bool{}
	initial := map[string]int32{}
	for _, slot := range state.Food {
		if slot.ID == "" || known[slot.ID] || slot.Initial < 1 || slot.Initial > 3 || slot.Remaining < 0 || slot.Remaining > slot.Initial || math.IsNaN(slot.Percent) || math.IsInf(slot.Percent, 0) || slot.Percent <= 0 || slot.Percent > 100 {
			return errors.New("觉醒携带道具账本无效")
		}
		known[slot.ID] = true
		initial[slot.ID] = slot.Initial
	}
	seen := map[int64]bool{}
	used := map[string]int32{}
	var priorStamp int64
	for _, receipt := range state.FoodReceipts {
		if receipt.Stamp <= priorStamp || seen[receipt.Stamp] || !known[receipt.ItemID] || len(receipt.Response) == 0 {
			return errors.New("觉醒道具回执无效")
		}
		seen[receipt.Stamp] = true
		priorStamp = receipt.Stamp
		used[receipt.ItemID]++
		result := &pb.SCAwakeUseItem{}
		if proto.Unmarshal(receipt.Response, result) != nil || result.GetHpPercent() == "" || len(result.GetItems()) != 1 || result.Items[0].GetId() != receipt.ItemID || result.Items[0].GetNum() != initial[receipt.ItemID]-used[receipt.ItemID] {
			return errors.New("觉醒道具回执无效")
		}
	}
	if (len(state.FoodReceipts) == 0) != (state.FoodReadyAt == 0) {
		return errors.New("觉醒道具冷却账本无效")
	}
	for _, slot := range state.Food {
		if used[slot.ID] != slot.Initial-slot.Remaining {
			return errors.New("觉醒道具数量与回执不一致")
		}
	}
	return nil
}

func (s *GameService) p7AwakeUseFood(tx *gorm.DB, p *pb.Player, g *model.GameState, ledger *db.P7AwakeExploration, run *db.P7AwakeRun, frozen *p7AwakeFrozen, state *p7AwakeExplorationState, itemID string, stamp int64, now time.Time) (*pb.SCAwakeUseItem, error) {
	if stamp <= 0 || itemID == "" || stamp < ledger.EnterStamp {
		return nil, errors.New("觉醒道具请求已过期")
	}
	for _, receipt := range state.FoodReceipts {
		if receipt.Stamp == stamp {
			if receipt.ItemID != itemID {
				return nil, errors.New("觉醒道具请求与回执不一致")
			}
			result := &pb.SCAwakeUseItem{}
			if err := proto.Unmarshal(receipt.Response, result); err != nil {
				return nil, err
			}
			return result, nil
		}
		if receipt.Stamp > stamp {
			return nil, errors.New("觉醒道具请求已过期")
		}
	}
	if now.UnixMilli() >= run.EndAt || state.ActiveFight != "" || state.FoodReadyAt > now.UnixMilli() {
		return nil, errors.New("觉醒道具当前不可使用")
	}
	var slot *p7AwakeFoodSlot
	for i := range state.Food {
		if state.Food[i].ID == itemID {
			slot = &state.Food[i]
		}
	}
	if slot == nil || slot.Remaining <= 0 {
		return nil, errors.New("未携带该恢复道具或数量已用完")
	}
	seconds, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", "Awake_Item_Cd")["NameValue"], 10, 32)
	if err != nil || seconds <= 0 || seconds > 3600 {
		return nil, errors.New("觉醒恢复道具冷却配置无效")
	}
	actor := &pb.BattlePlayer{}
	actorRaw := state.CurrentActor
	if len(actorRaw) == 0 {
		actorRaw = run.Actor
	}
	if proto.Unmarshal(actorRaw, actor) != nil || len(actor.Formation) == 0 {
		return nil, errors.New("觉醒队伍生命存档无效")
	}
	var current, maximum float64
	canHeal := false
	for _, hero := range actor.Formation {
		if hero == nil || hero.GetInst() == nil || hero.GetProp() == nil {
			return nil, errors.New("觉醒队伍生命存档无效")
		}
		hp, cap := hero.GetInst().GetCurHp(), hero.GetProp().GetHp()
		if math.IsNaN(hp) || math.IsInf(hp, 0) || math.IsNaN(cap) || math.IsInf(cap, 0) || cap <= 0 || hp < 0 || hp > cap {
			return nil, errors.New("觉醒队伍生命存档无效")
		}
		current += hp
		maximum += cap
		canHeal = canHeal || hp > 0 && hp < cap
	}
	if current <= 0 || !canHeal || math.IsInf(current, 0) || math.IsInf(maximum, 0) {
		return nil, errors.New("队伍已阵亡或生命已满")
	}
	if strings.HasPrefix(itemID, "FishItem_") {
		err = s.cookingInventoryDelta(p, g, map[string]int64{itemID: -1})
	} else {
		err = deductPoolCost(p, g, &pb.BaseResource{Id: proto.String(itemID), Num: proto.Int32(1)})
	}
	if err != nil {
		return nil, err
	}
	for _, hero := range actor.Formation {
		cap := hero.GetProp().GetHp()
		if hero.GetInst().GetCurHp() > 0 {
			hero.Inst.CurHp = proto.Float64(math.Min(cap, hero.GetInst().GetCurHp()+cap*slot.Percent/100))
		}
	}
	state.CurrentActor, err = proto.Marshal(actor)
	if err != nil {
		return nil, err
	}
	slot.Remaining--
	state.FoodReadyAt = now.UnixMilli() + seconds*1000
	refresh, err := p7AwakeRefresh(run, frozen, state)
	if err != nil {
		return nil, err
	}
	// The native callback updates the selected slot from this remaining value.
	result := &pb.SCAwakeUseItem{HpPercent: proto.String(refresh.GetHpPercent()), Items: []*pb.SimpleItem{{Id: proto.String(itemID), Num: proto.Int32(slot.Remaining)}}}
	raw, err := proto.Marshal(result)
	if err != nil {
		return nil, err
	}
	state.FoodReceipts = append(state.FoodReceipts, p7AwakeFoodReceipt{Stamp: stamp, ItemID: itemID, Response: raw})
	if err = p7AwakeSaveProgress(tx, ledger, state); err != nil {
		return nil, err
	}
	return result, nil
}
