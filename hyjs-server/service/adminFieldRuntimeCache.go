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
	for _, name := range []string{"Field", "FieldResource", "FieldResourcePos", "FieldMonster", "FieldMonsterPos", "WorldBossReward"} {
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
			for _, row := range tables["FieldMonster"] {
				if row["MonId"] == g.ID {
					row["RebornTime"], row["rewardGroupId"] = fmt.Sprint(g.CooldownMS), adminFieldDropText(g.Drops)
				}
			}
		}
		// Boss definitions are supplied separately per original scene to new cycles.
		raw, _ := json.Marshal(cfg.BossRewards)
		field["adminBossRewards"] = string(raw)
	}
	return tables, nil
}
