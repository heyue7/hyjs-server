package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// adminFieldCombatMember 将原表行投影为可编辑的受限成员数值；身份字段保持原值。
func adminFieldCombatMember(row map[string]string, idKey string, groupIndex int64) model.AdminFieldCombatMember {
	return model.AdminFieldCombatMember{
		ID:           row[idKey],
		HeroID:       row["HeroId"],
		Location:     int64(tableNumber(row, "Location")),
		GroupIndex:   groupIndex,
		Level:        int64(tableNumber(row, "Level")),
		Hp:           tableNumber(row, "Hp"),
		Attack:       tableNumber(row, "Attack"),
		PhyDefend:    tableNumber(row, "PhyDefend"),
		MagicDefend:  tableNumber(row, "MagicDefend"),
		Hit:          tableNumber(row, "hit"),
		Dodge:        tableNumber(row, "dodge"),
		Cri:          tableNumber(row, "cri"),
		AntiCri:      tableNumber(row, "antiCri"),
		CriFactor:    tableNumber(row, "criFactor"),
		DamageAdd:    tableNumber(row, "damageAdd"),
		DamageReduce: tableNumber(row, "damageReduce"),
	}
}

// adminFieldMonsterMembers 返回一个原怪物编队的全部波次成员，成员身份与波次保持原表。
func adminFieldMonsterMembers(rows []map[string]string, monsterID string) []model.AdminFieldCombatMember {
	members := []model.AdminFieldCombatMember{}
	for _, row := range rows {
		if row["MonId"] != monsterID {
			continue
		}
		members = append(members, adminFieldCombatMember(row, "MonsterId", int64(tableNumber(row, "GroupIndex"))))
	}
	return members
}

// adminFieldBossGroups 按本地图候选首领和原组号汇总全部成员与奖励。
func (s *GameService) adminFieldBossGroups(bossIDs map[string]bool) []model.AdminFieldBossGroup {
	groups := []model.AdminFieldBossGroup{}
	seen := map[string]bool{}
	for _, row := range s.Tables["FieldBoss"] {
		if !bossIDs[row["BossId"]] {
			continue
		}
		groupIndex := int64(tableNumber(row, "GroupIndex"))
		key := row["BossId"] + ":" + strconv.FormatInt(groupIndex, 10)
		if seen[key] {
			continue
		}
		seen[key] = true
		var leader map[string]string
		members := []model.AdminFieldCombatMember{}
		for _, member := range s.Tables["FieldBoss"] {
			if member["BossId"] != row["BossId"] || int64(tableNumber(member, "GroupIndex")) != groupIndex {
				continue
			}
			if member["TeamLeader"] == "1" {
				leader = member
			}
			members = append(members, adminFieldCombatMember(member, "Id", groupIndex))
		}
		if leader == nil {
			continue
		}
		var death model.AdminFieldBossGroup
		_ = json.Unmarshal([]byte(leader["adminDeathRules73"]), &death)
		groups = append(groups, model.AdminFieldBossGroup{
			DeathSettlement: death.DeathSettlement, RankDrops: death.RankDrops, SproutBonusBps: death.SproutBonusBps, SproutMax: death.SproutMax,
			ID:          row["BossId"],
			GroupIndex:  groupIndex,
			HeroID:      leader["HeroId"],
			RebornHours: int64(tableNumber(leader, "RebornTime")),
			Members:     members,
			Drops:       adminFieldDrops(leader["rewardGroupId"]),
			KillDrops:   adminFieldDrops(leader["KillReward"]),
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].ID != groups[j].ID {
			return groups[i].ID < groups[j].ID
		}
		return groups[i].GroupIndex < groups[j].GroupIndex
	})
	return groups
}

// adminFieldWorldBossMembers 返回原世界首领四成员基础数值；等级由周期等级与系数表决定。
func (s *GameService) adminFieldWorldBossMembers() []model.AdminFieldCombatMember {
	members := []model.AdminFieldCombatMember{}
	for _, row := range s.Tables["WorldBoss"] {
		member := adminFieldCombatMember(row, "Id", 1)
		if member.Level < 1 {
			member.Level = 1
		}
		members = append(members, member)
	}
	return members
}

// AdminFieldCombatMemberAllowed 校验受限成员数值的实际战斗安全范围；身份匹配由调用方完成。
func AdminFieldCombatMemberAllowed(member model.AdminFieldCombatMember) bool {
	finite := func(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
	if member.Level < 1 || member.Level > constUtil.AdminFieldCombatMaxLevel || !finite(member.Hp) || !finite(member.Attack) || member.Hp <= 0 || member.Attack <= 0 || member.Hp > constUtil.AdminFieldCombatMaxBase || member.Attack > constUtil.AdminFieldCombatMaxBase {
		return false
	}
	for _, value := range []float64{member.PhyDefend, member.MagicDefend} {
		if !finite(value) || value < 0 || value > constUtil.AdminFieldCombatMaxBase {
			return false
		}
	}
	for _, value := range []float64{member.Hit, member.Dodge, member.Cri, member.AntiCri, member.CriFactor, member.DamageAdd, member.DamageReduce} {
		if !finite(value) || value < 0 || value > constUtil.AdminFieldCombatMaxRate {
			return false
		}
	}
	return true
}

// adminFieldApplyCombatMember 将已校验成员数值写入克隆行，原基础表保持不变。
func adminFieldApplyCombatMember(row map[string]string, member model.AdminFieldCombatMember) {
	row["Level"] = strconv.FormatInt(member.Level, 10)
	row["Hp"] = strconv.FormatFloat(member.Hp, 'f', -1, 64)
	row["Attack"] = strconv.FormatFloat(member.Attack, 'f', -1, 64)
	row["PhyDefend"] = strconv.FormatFloat(member.PhyDefend, 'f', -1, 64)
	row["MagicDefend"] = strconv.FormatFloat(member.MagicDefend, 'f', -1, 64)
	row["hit"] = strconv.FormatFloat(member.Hit, 'f', -1, 64)
	row["dodge"] = strconv.FormatFloat(member.Dodge, 'f', -1, 64)
	row["cri"] = strconv.FormatFloat(member.Cri, 'f', -1, 64)
	row["antiCri"] = strconv.FormatFloat(member.AntiCri, 'f', -1, 64)
	row["criFactor"] = strconv.FormatFloat(member.CriFactor, 'f', -1, 64)
	row["damageAdd"] = strconv.FormatFloat(member.DamageAdd, 'f', -1, 64)
	row["damageReduce"] = strconv.FormatFloat(member.DamageReduce, 'f', -1, 64)
}

func adminFieldNormalizeConfiguration(cfg *model.AdminFieldConfiguration, source model.AdminFieldConfiguration) {
	if cfg.OrdinaryBossEnabled == nil {
		enabled := true
		if source.OrdinaryBossEnabled != nil {
			enabled = *source.OrdinaryBossEnabled
		}
		cfg.OrdinaryBossEnabled = &enabled
	}
	sourceMonsters := map[string]model.AdminFieldMonsterGroup{}
	for _, group := range source.MonsterGroups {
		sourceMonsters[group.ID] = group
	}
	for i := range cfg.MonsterGroups {
		group := &cfg.MonsterGroups[i]
		if len(group.Members) == 0 {
			group.Members = append([]model.AdminFieldCombatMember(nil), sourceMonsters[group.ID].Members...)
		}
	}
	if len(cfg.OrdinaryBossGroups) == 0 {
		cfg.OrdinaryBossGroups = append([]model.AdminFieldBossGroup(nil), source.OrdinaryBossGroups...)
	} else {
		sourceGroups := map[string]model.AdminFieldBossGroup{}
		for _, group := range source.OrdinaryBossGroups {
			sourceGroups[group.ID+":"+fmt.Sprint(group.GroupIndex)] = group
		}
		for i := range cfg.OrdinaryBossGroups {
			group := &cfg.OrdinaryBossGroups[i]
			original := sourceGroups[group.ID+":"+fmt.Sprint(group.GroupIndex)]
			if group.HeroID == "" {
				group.HeroID = original.HeroID
			}
			if group.RebornHours == 0 {
				group.RebornHours = original.RebornHours
			}
			if len(group.Members) == 0 {
				group.Members = append([]model.AdminFieldCombatMember(nil), original.Members...)
			}
		}
	}
	if len(cfg.WorldBossMembers) == 0 {
		cfg.WorldBossMembers = append([]model.AdminFieldCombatMember(nil), source.WorldBossMembers...)
	}
}

// adminFieldWorldBossStatus 读取现有全服策略的出生地图和下一周期等级，不创建周期也不写库。
func (s *GameService) adminFieldWorldBossStatus(policy db.P12WorldBossPolicy) *model.AdminFieldWorldBossStatus {
	status := &model.AdminFieldWorldBossStatus{Enabled: policy.Enabled, Level: policy.Level, PositionID: policy.PositionID}
	var def p12WorldBossDefinition
	if json.Unmarshal([]byte(policy.DefinitionJSON), &def) == nil {
		for _, row := range def.Tables["WorldBossPos"] {
			if row["id"] == fmt.Sprint(policy.PositionID) {
				status.Scene = row["scene"]
				status.SceneName = s.adminFieldName(row["scene"])
				status.BossID = row["BossId"]
				break
			}
		}
		if status.BossID == "" {
			for _, row := range def.Tables["WorldBoss"] {
				if row["show"] == "1" || len(def.Tables["WorldBoss"]) == 1 {
					status.BossID = row["BossId"]
					break
				}
			}
		}
	}
	return status
}

// adminFieldCombatRows 以内部场景绑定选择覆盖行，原资源ID始终保留用于客户端协议。
// 未发布地图和旧存档沿用无场景标记的原行；同组的其他地图覆盖不可混入编队。
func (s *GameService) adminFieldCombatRows(table, key, id, scene string) []map[string]string {
	var original, scoped []map[string]string
	for _, row := range s.Tables[table] {
		if row[key] != id {
			continue
		}
		if row["adminScene"] == "" {
			original = append(original, row)
		} else if row["adminScene"] == scene {
			scoped = append(scoped, row)
		}
	}
	if len(scoped) > 0 {
		return scoped
	}
	return original
}

func (s *GameService) fieldLeaderInScene(monID, scene string) map[string]string {
	for _, row := range s.adminFieldCombatRows("FieldMonster", "MonId", monID, scene) {
		if row["TeamLeader"] == "1" {
			return row
		}
	}
	return nil
}
