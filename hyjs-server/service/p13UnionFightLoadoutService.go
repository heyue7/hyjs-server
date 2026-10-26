package service

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"time"
)

// p13UFSetRoundItems 使用原5携带上限核对双方场次配置，不把配置种类当作已消费库存。
func (s *GameService) p13UFSetRoundItems(tx *gorm.DB, id int64, guild string, state *model.GameState, request *pb.CSUFSetFightItems, now time.Time) error {
	var policy db.P13UnionFightPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.MatchEnabled {
		return errors.New("公会战真实匹配尚未开放")
	}
	calendar, err := p13UFCalendar(tx, now)
	if err != nil {
		return err
	}
	day, stage := calendar.MatchDay, calendar.Stage
	if stage == constUtil.P13UnionFightStageMatch {
		return errors.New("原公会战匹配窗口尚未结束")
	}
	var rows []db.P13UnionFightMatch
	if err := tx.Where("day=? AND (left_guild_id=? OR right_guild_id=?)", day, guild, guild).Order("round").Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != 2 {
		return errors.New("当前公会没有完整原两轮匹配")
	}
	var target db.P13UnionFightMatch
	choices := map[int32][]string{}
	for _, row := range rows {
		if row.Round == request.GetFightIndex() {
			target = row
		}
		if p6Digest(row.Roster) != row.SnapshotHash {
			return errors.New("公会战原冻结阵容损坏")
		}
		var roster p13UFMatchRoster
		if err := json.Unmarshal(row.Roster, &roster); err != nil {
			return err
		}
		fighters := roster.Left
		if row.RightGuildID == guild {
			fighters = roster.Right
		}
		found := false
		for _, fighter := range fighters {
			if fighter.PlayerID == id && fighter.AppliedAt > 0 {
				found = true
			}
		}
		if !found {
			return errors.New("角色不在原冻结报名名单")
		}
		var loadout db.P13UnionFightLoadout
		err := tx.First(&loadout, "match_id=? AND player_id=?", row.ID, id).Error
		if err == nil {
			var selected []string
			if err = json.Unmarshal([]byte(loadout.ItemIDs), &selected); err != nil {
				return err
			}
			choices[row.Round] = selected
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	if target.ID == "" || now.UnixMilli() >= target.StartAt {
		return errors.New("原本轮道具配置已截止")
	}
	choices[request.GetFightIndex()] = request.GetItemIds()
	// 原准备UI在首场选择相同种类时，次场可用数减5；实际场内带入与消费由后续战场负责。
	for round, items := range choices {
		for _, itemID := range items {
			if s.table("Item", "itemId", itemID)["category"] != "17" {
				return errors.New("原每场道具不属于公会战类别17")
			}
			original := s.table("UnionFightItem", "MainItemId", itemID)
			maximum, err := strconv.ParseInt(original["ItemCatchMaxNum"], 10, 32)
			if err != nil || maximum <= 0 {
				return errors.New("原每场道具携带上限缺失")
			}
			var owned int64
			for _, stack := range state.Items {
				if stack.GetId() == itemID {
					owned += int64(stack.GetNum())
				}
			}
			if round == 2 {
				for _, first := range choices[1] {
					if first == itemID {
						owned -= maximum
					}
				}
			}
			if owned <= 0 {
				return errors.New("原本轮道具可用库存不足")
			}
		}
	}
	encoded, err := json.Marshal(request.GetItemIds())
	if err != nil {
		return err
	}
	return tx.Save(&db.P13UnionFightLoadout{MatchID: target.ID, PlayerID: id, ItemIDs: string(encoded), PreparedAt: now.UnixMilli()}).Error
}
