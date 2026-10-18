package service

import "fmt"

// ApplyP15GreenHeroCompatibility 补齐用户批准的六个绿色角色原空战斗字段。
// 数据与客户端资源覆盖同源；保留原资质、基础属性、模型、肖像与空大招。
func (s *GameService) ApplyP15GreenHeroCompatibility() error {
	for _, pair := range [][2]string{{"hero_056", "monster_068"}, {"hero_057", "monster_070"}, {"hero_058", "monster_070"}, {"hero_059", "monster_070"}, {"hero_060", "monster_067"}, {"hero_061", "monster_070"}} {
		row, donor := s.table("Hero", "id", pair[0]), s.table("Hero", "id", pair[1])
		if row == nil || donor == nil || row["quality"] != "12" || row["heroClass"] != "0" {
			return fmt.Errorf("绿色角色兼容原表不符: %s", pair[0])
		}
		// 仅复用原单体普通攻击；不增加大招、天赋或任何基础成长属性。
		for _, key := range []string{"SkillId1", "BeFoundBone", "RushFlyTime", "RushFallTime", "JumpBackFlyTime", "JumpBackFallTime", "ImposState"} {
			if row[key] != "" && row[key] != donor[key] {
				return fmt.Errorf("绿色角色兼容字段已被修改: %s.%s", pair[0], key)
			}
			row[key] = donor[key]
		}
	}
	// 六绿碎片的临时兼容规则：十合一，零售价，避免回收货币循环。
	for _, pair := range [][2]string{{"hero_056", "monster_068"}, {"hero_057", "monster_070"}, {"hero_058", "monster_070"}, {"hero_059", "monster_070"}, {"hero_060", "monster_067"}, {"hero_061", "monster_070"}} {
		id := "frag_" + pair[0]
		if row := s.table("Frag", "id", id); row != nil {
			if row["target"] != pair[0] || row["num"] != "10" || row["price"] != "0" {
				return fmt.Errorf("绿色碎片兼容字段已被修改: %s", id)
			}
			continue
		}
		s.Tables["Frag"] = append(s.Tables["Frag"], map[string]string{"id": id, "target": pair[0], "num": "10", "price": "0"})
	}
	return nil
}

// p15GreenHeroSingleNormalSkill 只识别已批准六绿与固定原普攻，不扩大其他空技能角色。
func p15GreenHeroSingleNormalSkill(row map[string]string) bool {
	if row == nil || row["heroClass"] != "0" || row["quality"] != "12" || row["SkillId2"] != "" || row["UltSkillId"] != "" {
		return false
	}
	var expected string
	switch row["id"] {
	case "hero_056":
		expected = "1068_1"
	case "hero_060":
		expected = "1067_1"
	case "hero_057", "hero_058", "hero_059", "hero_061":
		expected = "1070_1"
	default:
		return false
	}
	return row["SkillId1"] == expected && row["ImposState"] == "0"
}
