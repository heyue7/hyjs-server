package service

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// adminFieldRuntimeSnapshot 固化某个基础 GameService 在某个区服、某个数据库连接上的
// 完整野外覆盖表。快照内容有界且写入后不再修改，运行消费者按只读方式使用。
type adminFieldRuntimeSnapshot struct {
	serverID    int64
	database    *gorm.DB
	sqlDatabase *sql.DB
	signature   [sha256.Size]byte
	tables      map[string][]map[string]string
}

// adminFieldDeploymentSignature 按全局有序的已提交行和全部影响字段计算内容签名。
// 每次请求仍读取完整行，因此外部 SQL 未更新 Revision 的实际内容变化同样可被发现。
func adminFieldDeploymentSignature(deployments []db.AdminFieldDeployment) [sha256.Size]byte {
	digest := sha256.New()
	var size [8]byte
	writeField := func(value string) {
		binary.LittleEndian.PutUint64(size[:], uint64(len(value)))
		digest.Write(size[:])
		digest.Write([]byte(value))
	}
	binary.LittleEndian.PutUint64(size[:], uint64(len(deployments)))
	digest.Write(size[:])
	for _, deployment := range deployments {
		// 区服、场景、模板、启用状态、冻结配置、修订和时间戳均参与签名。
		for _, value := range []string{
			strconv.FormatInt(deployment.ServerID, 10),
			deployment.Scene,
			deployment.TemplateID,
			strconv.FormatBool(deployment.Enabled),
			deployment.Configuration,
			strconv.FormatInt(deployment.Revision, 10),
			strconv.FormatInt(deployment.UpdatedAt, 10),
		} {
			writeField(value)
		}
	}
	var signature [sha256.Size]byte
	copy(signature[:], digest.Sum(nil))
	return signature
}

// buildAdminFieldRuntimeTables 从基础服务原表构建一次完整覆盖表；调用方只在本服务
// 持有快照且未发布新内容时复用返回结果。
func (s *GameService) buildAdminFieldRuntimeTables(deployments []db.AdminFieldDeployment) (map[string][]map[string]string, error) {
	tables := make(map[string][]map[string]string, len(s.Tables))
	for k, v := range s.Tables {
		tables[k] = v
	}
	for _, name := range []string{"Field", "FieldResource", "FieldResourcePos", "FieldMonster", "FieldMonsterPos", "FieldBoss", "FieldBossPos", "WorldBossReward"} {
		rows := []map[string]string{}
		for _, r := range s.Tables[name] {
			copy := map[string]string{}
			for k, v := range r {
				copy[k] = v
			}
			rows = append(rows, copy)
		}
		tables[name] = rows
	}
	lookup := func(table, key, id string) map[string]string {
		for _, r := range tables[table] {
			if r[key] == id {
				return r
			}
		}
		return nil
	}
	// 共享原编队在地图之间复制内部场景行，未发布地图继续使用原表；先按全部地图点位统计引用。
	monsterUsage, bossUsage := map[string]int{}, map[string]int{}
	for _, field := range s.Tables["Field"] {
		monsterSeen, bossSeen := map[string]bool{}, map[string]bool{}
		for _, posID := range strings.Split(field["monsterPosIds"], ";") {
			if posID == "" {
				continue
			}
			if pos := lookup("FieldMonsterPos", "monsterPosId", posID); pos != nil {
				for _, choice := range adminFieldWeights(pos["monsterIdProbs"]) {
					if !monsterSeen[choice.ID] {
						monsterUsage[choice.ID]++
						monsterSeen[choice.ID] = true
					}
				}
			}
		}
		for _, posID := range strings.Split(field["bossPosIds"], ";") {
			if posID == "" {
				continue
			}
			if pos := lookup("FieldBossPos", "BoxxPosId", posID); pos != nil {
				for _, choice := range adminFieldWeights(pos["bossIdProbs"]) {
					if !bossSeen[choice.ID] {
						bossUsage[choice.ID]++
						bossSeen[choice.ID] = true
					}
				}
			}
		}
	}
	for _, deployment := range deployments {
		var cfg model.AdminFieldConfiguration
		if json.Unmarshal([]byte(deployment.Configuration), &cfg) != nil || cfg.Scene != deployment.Scene {
			return nil, constUtil.ErrAdminConfiguration
		}
		field := lookup("Field", "FieldId", cfg.Scene)
		if field == nil {
			return nil, constUtil.ErrAdminConfiguration
		}
		field["adminEnabled"] = strconv.FormatBool(deployment.Enabled)
		field["adminPK"] = strconv.FormatBool(cfg.AllowPK)
		if cfg.AllowPK {
			field["allSafe"] = "0"
		} else {
			field["allSafe"] = "1"
		}
		field["adminBossEnabled"] = strconv.FormatBool(cfg.BossEnabled)
		ordinaryEnabled := true
		if cfg.OrdinaryBossEnabled != nil {
			ordinaryEnabled = *cfg.OrdinaryBossEnabled
		}
		field["adminOrdinaryBossEnabled"] = strconv.FormatBool(ordinaryEnabled)
		field["adminRevision"] = adminVersion(deployment)
		field["levelLimit"] = fmt.Sprint(cfg.MinLevel)
		field["MineMax"], field["FishGroupMax"] = fmt.Sprint(cfg.MineCount), fmt.Sprint(cfg.FishCount)
		for _, spec := range []struct {
			key                   string
			points                []model.AdminFieldPoint
			table, idKey, weights string
		}{{"MinePoints", cfg.MinePoints, "FieldResourcePos", "posId", "resourceIdProbs"}, {"FishPoints", cfg.FishPoints, "FieldResourcePos", "posId", "resourceIdProbs"}, {"MonsterPoints", cfg.MonsterPoints, "FieldMonsterPos", "monsterPosId", "monsterIdProbs"}} {
			ids := []string{}
			for _, p := range spec.points {
				ids = append(ids, p.ID)
				if row := lookup(spec.table, spec.idKey, p.ID); row != nil {
					choices := append([]model.AdminFieldWeight{}, p.Choices...)
					if spec.table == "FieldResourcePos" {
						for i := range choices {
							choices[i].ID = choices[i].ID + "__" + cfg.Scene
						}
					}
					if spec.table == "FieldMonsterPos" {
						row["adminScene"] = cfg.Scene
					}
					row[spec.weights] = adminFieldWeightText(choices)
				}
			}
			field[spec.key] = strings.Join(ids, ";")
		}
		for _, g := range cfg.GatherGroups {
			if original := lookup("FieldResource", "id", g.ID); original != nil {
				row := map[string]string{}
				for k, v := range original {
					row[k] = v
				}
				row["id"] = g.ID + "__" + cfg.Scene
				row["adminOriginalID"] = g.ID
				tables["FieldResource"] = append(tables["FieldResource"], row)
				row["level"], row["gatherTime"], row["cdTime"], row["times"], row["cost"], row["rewardProbId"] = fmt.Sprint(g.Level), fmt.Sprint(g.GatherMS), fmt.Sprint(g.CooldownMS), fmt.Sprint(g.Times), fmt.Sprintf("tiredness,%d", g.Fatigue), adminFieldDropText(g.Drops)
			}
		}
		for _, g := range cfg.MonsterGroups {
			if monsterUsage[g.ID] > 1 {
				for _, original := range s.Tables["FieldMonster"] {
					if original["MonId"] != g.ID {
						continue
					}
					row := map[string]string{}
					for k, v := range original {
						row[k] = v
					}
					row["adminScene"] = cfg.Scene
					tables["FieldMonster"] = append(tables["FieldMonster"], row)
				}
			}
			for _, row := range tables["FieldMonster"] {
				if row["MonId"] != g.ID || monsterUsage[g.ID] > 1 && row["adminScene"] != cfg.Scene {
					continue
				}
				row["RebornTime"], row["rewardGroupId"] = fmt.Sprint(g.CooldownMS), adminFieldDropText(g.Drops)
				if err := adminFieldApplyGroupMembers(row, g.Members, "MonsterId"); err != nil {
					return nil, err
				}
			}
		}
		for _, posID := range strings.Split(field["bossPosIds"], ";") {
			if pos := lookup("FieldBossPos", "BoxxPosId", posID); pos != nil {
				pos["adminScene"] = cfg.Scene
			}
		}
		for _, g := range cfg.OrdinaryBossGroups {
			if bossUsage[g.ID] > 1 {
				for _, original := range s.Tables["FieldBoss"] {
					if original["BossId"] != g.ID || int64(tableNumber(original, "GroupIndex")) != g.GroupIndex {
						continue
					}
					row := map[string]string{}
					for k, v := range original {
						row[k] = v
					}
					row["adminScene"] = cfg.Scene
					tables["FieldBoss"] = append(tables["FieldBoss"], row)
				}
			}
			for _, row := range tables["FieldBoss"] {
				if row["BossId"] != g.ID || int64(tableNumber(row, "GroupIndex")) != g.GroupIndex || bossUsage[g.ID] > 1 && row["adminScene"] != cfg.Scene {
					continue
				}
				if row["TeamLeader"] == "1" {
					row["RebornTime"] = fmt.Sprint(g.RebornHours)
					row["rewardGroupId"], row["KillReward"] = adminFieldDropText(g.Drops), adminFieldDropText(g.KillDrops)
				}
				if err := adminFieldApplyGroupMembers(row, g.Members, "Id"); err != nil {
					return nil, err
				}
			}
		}
		// 世界首领配置随出生地图方案保存；当前策略未选该地图时保留原值。
		worldMembers, _ := json.Marshal(cfg.WorldBossMembers)
		field["adminWorldBossMembers"] = string(worldMembers)
		field["adminWorldBossLevel"] = ""
		if cfg.WorldBossLevel != nil {
			field["adminWorldBossLevel"] = fmt.Sprint(*cfg.WorldBossLevel)
		}
		// Boss definitions are supplied separately per original scene to new cycles.
		raw, _ := json.Marshal(cfg.BossRewards)
		field["adminBossRewards"] = string(raw)
	}
	return tables, nil
}

// adminFieldApplyGroupMembers 在成员集合存在时按原行ID写入数值；缺省成员继承原表。
func adminFieldApplyGroupMembers(row map[string]string, members []model.AdminFieldCombatMember, idKey string) error {
	if len(members) == 0 {
		return nil
	}
	for _, member := range members {
		if member.ID != row[idKey] {
			continue
		}
		if !AdminFieldCombatMemberAllowed(member) {
			return constUtil.ErrAdminConfiguration
		}
		adminFieldApplyCombatMember(row, member)
		return nil
	}
	return constUtil.ErrAdminConfiguration
}
