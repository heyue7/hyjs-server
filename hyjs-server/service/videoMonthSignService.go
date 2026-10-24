package service

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// SeedUserMonthSignCalendars installs only the two explicitly requested months.
// Existing operator calendars win; hidden days retain the configured defaults.
func (s *GameService) SeedUserMonthSignCalendars(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var base []db.MonthSignRule
		if err := tx.Order("day").Find(&base).Error; err != nil {
			return err
		}
		if !validMonthSignRows(base) {
			return nil
		}
		for _, month := range []string{"2026-10", "2026-11"} {
			rows := videoMonthSignRows(month, base)
			for _, row := range rows {
				supported, err := s.openServerRewardSupported(row.Reward)
				if err != nil {
					return err
				}
				if !supported {
					return constUtil.ErrAdminConfiguration
				}
			}
			raw, err := json.Marshal(rows)
			if err != nil {
				return err
			}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.MonthSignCalendar{MonthKey: month, Rules: raw}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func videoMonthSignRows(month string, base []db.MonthSignRule) []db.MonthSignRule {
	rows := append([]db.MonthSignRule(nil), base...)
	if len(rows) != 31 {
		return rows
	}
	var rewards []string
	var vip map[int32]int32
	switch month {
	case "2026-10":
		rewards = []string{"coin,50000", "frag_equip_018,16", "frag_equip_018,16", "item_002,4", "item_005,100", "diamond,100", "frag_equip_019,16", "frag_equip_019,16", "item_001,4", "item_007,60", "diamond,200", "frag_equip_020,16", "frag_equip_020,16", "item_013,15", "item_011,150", "diamond,300", "frag_equip_017,16", "frag_equip_017,16", "item_002,10", "item_001,10"}
		vip = map[int32]int32{5: 1, 9: 2, 13: 3, 17: 4}
	case "2026-11":
		rewards = []string{"coin,50000", "item_005,200", "item_022,5", "item_008,10", "item_022,5", "diamond,100", "item_005,200", "item_022,5", "item_008,10", "item_022,5", "diamond,200", "item_013,10", "item_022,10", "item_006,40", "item_022,15", "diamond,300", "item_013,10", "item_022,10", "item_006,40", "item_022,15"}
		vip = map[int32]int32{5: 2, 10: 4, 15: 6, 20: 8}
	default:
		return rows
	}
	for i, reward := range rewards {
		rows[i].Reward = reward
		rows[i].DoubleVIP = vip[rows[i].Day]
		rows[i].Source = "用户参考图前20档；第21档起沿用原奖历"
	}
	return rows
}
