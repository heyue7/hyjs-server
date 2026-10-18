package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// InitializeAdminFields creates independent operations tables and preserves source tables.
func (s *GameService) InitializeAdminFields(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.AdminFieldTemplate{}, &db.AdminFieldDeployment{}, &db.AdminFieldBattleSnapshot{}); err != nil {
		return err
	}
	for _, row := range s.Tables["Field"] {
		cfg, err := s.adminOriginalField(row["FieldId"])
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(cfg)
		name := s.adminFieldName(cfg.Scene)
		value := db.AdminFieldTemplate{ID: "original-" + cfg.Scene, Name: name + " · 原资源方案", Scene: cfg.Scene, Configuration: string(raw), Revision: 1, UpdatedAt: time.Now().UnixMilli()}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&value).Error; err != nil {
			return err
		}
	}
	return nil
}
func (s *GameService) adminFieldName(scene string) string {
	names := map[string]string{"field_001": "森林野外", "field_002": "雪原野外", "field_003": "野外三区", "field_004": "野外四区", "field_005": "野外五区", "field_006": "野外六区"}
	if names[scene] != "" {
		return names[scene]
	}
	return scene
}
func adminFieldWeights(raw string) []model.AdminFieldWeight {
	out := []model.AdminFieldWeight{}
	for _, part := range strings.Split(raw, ";") {
		v := strings.Split(part, ",")
		if len(v) == 2 {
			w, _ := strconv.ParseInt(v[1], 10, 64)
			out = append(out, model.AdminFieldWeight{ID: v[0], Weight: w})
		}
	}
	return out
}
func adminFieldDrops(raw string) []model.AdminFieldDrop {
	out := []model.AdminFieldDrop{}
	for _, part := range strings.Split(raw, "|") {
		pair := strings.SplitN(part, ":", 2)
		if len(pair) != 2 {
			continue
		}
		d := model.AdminFieldDrop{ResourceID: pair[0], Quantities: []model.AdminFieldQuantity{}}
		for _, q := range adminFieldWeights(pair[1]) {
			n, _ := strconv.ParseInt(q.ID, 10, 64)
			d.Quantities = append(d.Quantities, model.AdminFieldQuantity{Amount: n, Weight: q.Weight})
		}
		out = append(out, d)
	}
	return out
}
func adminFieldDropText(drops []model.AdminFieldDrop) string {
	out := []string{}
	for _, d := range drops {
		q := []string{}
		for _, n := range d.Quantities {
			q = append(q, fmt.Sprintf("%d,%d", n.Amount, n.Weight))
		}
		out = append(out, d.ResourceID+":"+strings.Join(q, ";"))
	}
	return strings.Join(out, "|")
}
func adminFieldWeightText(weights []model.AdminFieldWeight) string {
	out := []string{}
	for _, w := range weights {
		out = append(out, fmt.Sprintf("%s,%d", w.ID, w.Weight))
	}
	return strings.Join(out, ";")
}
func (s *GameService) adminOriginalField(scene string) (model.AdminFieldConfiguration, error) {
	row := s.table("Field", "FieldId", scene)
	if row == nil {
		return model.AdminFieldConfiguration{}, constUtil.ErrAdminInput
	}
	cfg := model.AdminFieldConfiguration{Scene: scene, SceneResource: row["FieldMap"], BattleResource: row["BattleMap"], MinLevel: int64(tableNumber(row, "levelLimit")), AllowPK: row["allSafe"] != "1", MineCount: int64(tableNumber(row, "MineMax")), FishCount: int64(tableNumber(row, "FishGroupMax")), MinePoints: []model.AdminFieldPoint{}, FishPoints: []model.AdminFieldPoint{}, MonsterPoints: []model.AdminFieldPoint{}, GatherGroups: []model.AdminFieldGatherGroup{}, MonsterGroups: []model.AdminFieldMonsterGroup{}, BossEnabled: true, BossRewards: []model.AdminFieldBossReward{}}
	groups := map[string]bool{}
	monsters := map[string]bool{}
	for _, spec := range []struct {
		key    string
		target *[]model.AdminFieldPoint
	}{{"MinePoints", &cfg.MinePoints}, {"FishPoints", &cfg.FishPoints}, {"MonsterPoints", &cfg.MonsterPoints}} {
		for _, id := range strings.Split(row[spec.key], ";") {
			if id == "" {
				continue
			}
			p := s.table("FieldResourcePos", "posId", id)
			pointKey, weightKey := "pos", "resourceIdProbs"
			if spec.key == "MonsterPoints" {
				p = s.table("FieldMonsterPos", "monsterPosId", id)
				pointKey, weightKey = "BornPos", "monsterIdProbs"
			}
			if p == nil {
				return cfg, fmt.Errorf("野外原点位缺失: %s", id)
			}
			choices := adminFieldWeights(p[weightKey])
			*spec.target = append(*spec.target, model.AdminFieldPoint{ID: id, Point: p[pointKey], Choices: choices})
			for _, w := range choices {
				if spec.key == "MonsterPoints" {
					monsters[w.ID] = true
				} else {
					groups[w.ID] = true
				}
			}
		}
	}
	for _, r := range s.Tables["FieldResource"] {
		if !groups[r["id"]] {
			continue
		}
		cost := strings.Split(r["cost"], ",")
		fatigue := int64(0)
		if len(cost) == 2 {
			fatigue, _ = strconv.ParseInt(cost[1], 10, 64)
		}
		cfg.GatherGroups = append(cfg.GatherGroups, model.AdminFieldGatherGroup{ID: r["id"], Level: int64(tableNumber(r, "level")), GatherMS: int64(tableNumber(r, "gatherTime")), CooldownMS: int64(tableNumber(r, "cdTime")), Times: int64(tableNumber(r, "times")), Fatigue: fatigue, Drops: adminFieldDrops(r["rewardProbId"])})
	}
	for _, r := range s.Tables["FieldMonster"] {
		if monsters[r["MonId"]] && r["TeamLeader"] == "1" {
			cfg.MonsterGroups = append(cfg.MonsterGroups, model.AdminFieldMonsterGroup{ID: r["MonId"], HeroID: r["HeroId"], Level: int64(tableNumber(r, "Level")), CooldownMS: int64(tableNumber(r, "RebornTime")), Drops: adminFieldDrops(r["rewardGroupId"])})
		}
	}
	for _, r := range s.Tables["WorldBossReward"] {
		reward := model.AdminFieldBossReward{Type: int64(tableNumber(r, "type")), Level: int64(tableNumber(r, "level")), Weighted: strings.Contains(r["reward"], ":"), Drops: []model.AdminFieldDrop{}}
		if reward.Weighted {
			reward.Drops = adminFieldDrops(r["reward"])
		} else {
			for _, item := range strings.Split(r["reward"], ";") {
				pair := strings.Split(item, ",")
				if len(pair) == 2 {
					n, _ := strconv.ParseInt(pair[1], 10, 64)
					reward.Drops = append(reward.Drops, model.AdminFieldDrop{ResourceID: pair[0], Quantities: []model.AdminFieldQuantity{{Amount: n, Weight: 1}}})
				}
			}
		}
		cfg.BossRewards = append(cfg.BossRewards, reward)
	}
	return cfg, nil
}
func adminFieldTemplateDTO(row db.AdminFieldTemplate) (model.AdminFieldTemplateDTO, error) {
	var cfg model.AdminFieldConfiguration
	if json.Unmarshal([]byte(row.Configuration), &cfg) != nil {
		return model.AdminFieldTemplateDTO{}, constUtil.ErrAdminConfiguration
	}
	available := constUtil.P8PackagedFieldScene(row.Scene)
	reason := ""
	if !available {
		reason = constUtil.AdminFieldUnavailable
	}
	return model.AdminFieldTemplateDTO{ID: row.ID, Name: row.Name, Available: available, UnavailableReason: reason, Version: adminVersion(row), Configuration: cfg}, nil
}

// AdminFields lists editable global map resource combinations.
func (s *GameService) AdminFields(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminFieldListResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	var rows []db.AdminFieldTemplate
	if err := tx.Order("scene,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &model.AdminFieldListResponse{Items: []model.AdminFieldTemplateDTO{}, Resources: []model.AdminFieldResource{}}
	if s.AdminCatalogueData != nil {
		for _, item := range s.AdminCatalogueData.Items {
			kinds := []string{}
			if s.AdminFieldDropAllowed(item.ID, "gather") {
				kinds = append(kinds, "gather")
			}
			if s.AdminFieldDropAllowed(item.ID, "monster") {
				kinds = append(kinds, "monster")
			}
			if s.AdminFieldBossDropAllowed(item.ID) {
				kinds = append(kinds, "boss")
			}
			if len(kinds) > 0 {
				maximum := int64(2147483647)
				if !s.AdminFieldBossAmountAllowed(item.ID, 101) {
					maximum = 100
				}
				out.Resources = append(out.Resources, model.AdminFieldResource{AdminItemDTO: item, AllowedKinds: kinds, MaxAmount: maximum})
			}
		}
	}
	for _, r := range rows {
		v, err := adminFieldTemplateDTO(r)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// AdminSaveField uses optimistic versioning and does not alter published snapshots.
func (s *GameService) AdminSaveField(ctx context.Context, actor *model.AdminPrincipal, req model.AdminFieldSaveRequest) (*model.AdminFieldTemplateDTO, error) {
	var out model.AdminFieldTemplateDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		row := db.AdminFieldTemplate{ID: req.ID, Revision: 0}
		if req.ID != "" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", req.ID).Error; err != nil {
				return err
			}
			if adminVersion(row) != req.Version {
				return constUtil.ErrAdminConfigConflict
			}
			if row.Scene != req.Configuration.Scene {
				return constUtil.ErrAdminInput
			}
		} else {
			row.ID = uuid.NewString()
		}
		raw, _ := json.Marshal(req.Configuration)
		row.Name, row.Scene, row.Configuration = req.Name, req.Configuration.Scene, string(raw)
		row.Revision++
		row.UpdatedAt = time.Now().UnixMilli()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := adminAudit(tx, actor, "field.template.save", row.ID, fmt.Sprintf("scene=%s revision=%d", row.Scene, row.Revision)); err != nil {
			return err
		}
		var err error
		out, err = adminFieldTemplateDTO(row)
		return err
	})
	return &out, err
}

// AdminDeleteField removes an unused global template, preserving all deployments.
func (s *GameService) AdminDeleteField(ctx context.Context, actor *model.AdminPrincipal, req model.AdminFieldDeleteRequest) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var row db.AdminFieldTemplate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", req.ID).Error; err != nil {
			return err
		}
		if adminVersion(row) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		var used int64
		if err := tx.Model(&db.AdminFieldDeployment{}).Where("template_id=?", row.ID).Count(&used).Error; err != nil {
			return err
		}
		if used > 0 {
			return errors.New("已有区服部署该方案，请先切换部署")
		}
		if err := tx.Delete(&row).Error; err != nil {
			return err
		}
		return adminAudit(tx, actor, "field.template.delete", row.ID, row.Name)
	})
}

// AdminFieldDeployments shows independent realm deployment state and pending template changes.
func (s *GameService) AdminFieldDeployments(ctx context.Context, actor *model.AdminPrincipal, serverID int64) (*model.AdminFieldDeploymentsResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	var rows []db.AdminFieldDeployment
	if err := tx.Where("server_id=?", serverID).Find(&rows).Error; err != nil {
		return nil, err
	}
	byScene := map[string]db.AdminFieldDeployment{}
	for _, r := range rows {
		byScene[r.Scene] = r
	}
	out := &model.AdminFieldDeploymentsResponse{ServerID: serverID, Items: []model.AdminFieldDeploymentDTO{}}
	for _, field := range s.Tables["Field"] {
		scene := field["FieldId"]
		r, ok := byScene[scene]
		v := model.AdminFieldDeploymentDTO{Scene: scene, Name: s.adminFieldName(scene), TemplateID: "original-" + scene, Enabled: constUtil.P8PackagedFieldScene(scene), Available: constUtil.P8PackagedFieldScene(scene)}
		if ok {
			v.TemplateID, v.Enabled, v.Version = r.TemplateID, r.Enabled, adminVersion(r)
			var t db.AdminFieldTemplate
			if err := tx.First(&t, "id=?", r.TemplateID).Error; err != nil {
				return nil, err
			}
			v.TemplateChanged = t.Configuration != r.Configuration
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// AdminDeployField freezes a template as the realm's next map configuration.
func (s *GameService) AdminDeployField(ctx context.Context, actor *model.AdminPrincipal, req model.AdminFieldDeployRequest) (*model.AdminFieldDeploymentsResponse, error) {
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var t db.AdminFieldTemplate
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&t, "id=?", req.TemplateID).Error; err != nil {
			return err
		}
		if t.Scene != req.Scene || adminVersion(t) != req.TemplateVersion {
			return constUtil.ErrAdminConfigConflict
		}
		if req.Enabled && !constUtil.P8PackagedFieldScene(req.Scene) {
			return errors.New(constUtil.AdminFieldUnavailable)
		}
		var row db.AdminFieldDeployment
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "server_id=? AND scene=?", req.ServerID, req.Scene).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && adminVersion(row) != req.Version || errors.Is(err, gorm.ErrRecordNotFound) && req.Version != "" {
			return constUtil.ErrAdminConfigConflict
		}
		row.ServerID, row.Scene, row.TemplateID, row.Enabled, row.Configuration = req.ServerID, req.Scene, t.ID, req.Enabled, t.Configuration
		row.Revision++
		row.UpdatedAt = time.Now().UnixMilli()
		if err = tx.Save(&row).Error; err != nil {
			return err
		}
		return adminAudit(tx, actor, "field.deploy", fmt.Sprintf("server=%d scene=%s", req.ServerID, req.Scene), fmt.Sprintf("template=%s enabled=%t", t.ID, req.Enabled))
	})
	if err != nil {
		return nil, err
	}
	return s.AdminFieldDeployments(ctx, actor, req.ServerID)
}

// AdminFieldRuntimeTables creates a request-local immutable overlay. Original maps and already
// frozen gathering/fight receipts retain their own snapshots; no shared table is mutated.
func (s *GameService) AdminFieldRuntimeTables(ctx context.Context, serverID int64) (map[string][]map[string]string, error) {
	tx := db.GetIns().WithContext(ctx)
	if !tx.Migrator().HasTable(&db.AdminFieldDeployment{}) {
		return s.Tables, nil
	}
	var deployments []db.AdminFieldDeployment
	if err := tx.Where("server_id=?", serverID).Order("scene").Find(&deployments).Error; err != nil {
		return nil, err
	}
	if len(deployments) == 0 {
		return s.Tables, nil
	}
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

// AdminFieldOriginalReference returns immutable source options to HTTP validation.
func (s *GameService) AdminFieldOriginalReference(scene string) (model.AdminFieldConfiguration, error) {
	return s.adminOriginalField(scene)
}

// AdminFieldDropAllowed follows the concrete gathering and field reward consumers.
func (s *GameService) AdminFieldDropAllowed(id, kind string) bool {
	if kind == "monster" && id == "coin" {
		return true
	}
	if s.table("Item", "itemId", id) != nil || s.table("MineSkill", "MineItemID", id) != nil || s.table("FishSkill", "FishItemID", id) != nil {
		return true
	}
	return kind == "monster" && strings.HasPrefix(id, "hero_exp_") && s.table("Hero", "id", id)["heroClass"] == "99"
}
func adminFieldSortedIDs(v []model.AdminFieldPoint) []string {
	out := []string{}
	for _, p := range v {
		out = append(out, p.ID)
	}
	sort.Strings(out)
	return out
}

// adminFieldFreezeBattle stores accepted settings before a later operator edit.
func (s *GameService) adminFieldFreezeBattle(tx *gorm.DB, id int64, scene, fightID, monsterID string) error {
	if !tx.Migrator().HasTable(&db.AdminFieldBattleSnapshot{}) {
		return nil
	}
	leader := s.fieldLeader(monsterID)
	return tx.Create(&db.AdminFieldBattleSnapshot{FightID: fightID, PlayerID: id, Scene: scene, Reward: leader["rewardGroupId"], CooldownMS: int64(tableNumber(leader, "RebornTime"))}).Error
}
func (s *GameService) adminFieldBattleReward(tx *gorm.DB, id int64, scene, fightID, monsterID string) (string, int64, error) {
	leader := s.fieldLeader(monsterID)
	reward, cooldown := leader["rewardGroupId"], int64(tableNumber(leader, "RebornTime"))
	if !tx.Migrator().HasTable(&db.AdminFieldBattleSnapshot{}) {
		return reward, cooldown, nil
	}
	var row db.AdminFieldBattleSnapshot
	err := tx.First(&row, "fight_id=?", fightID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return reward, cooldown, nil
	}
	if err != nil {
		return "", 0, err
	}
	if row.PlayerID != id || row.Scene != scene {
		return "", 0, constUtil.ErrAdminConfiguration
	}
	return row.Reward, row.CooldownMS, nil
}

// adminFieldBossConfiguration applies only the original boss scene's published definition.
// Existing cycles keep their frozen complete definition and all historical rewards.
func (s *GameService) adminFieldBossConfiguration(cfg *db.P12WorldBossPolicy, def *p12WorldBossDefinition) error {
	var scene string
	for _, p := range def.Tables["WorldBossPos"] {
		if p["id"] == fmt.Sprint(cfg.PositionID) {
			scene = p["scene"]
			break
		}
	}
	row := s.table("Field", "FieldId", scene)
	if row["adminBossEnabled"] == "" {
		return nil
	}
	if row["adminEnabled"] == "false" || row["adminBossEnabled"] == "false" {
		cfg.Enabled = false
		return nil
	}
	var rewards []model.AdminFieldBossReward
	if json.Unmarshal([]byte(row["adminBossRewards"]), &rewards) != nil {
		return constUtil.ErrAdminConfiguration
	}
	copied := make(map[string][]map[string]string, len(def.Tables))
	for key, rows := range def.Tables {
		copied[key] = rows
	}
	rows := []map[string]string{}
	for _, r := range def.Tables["WorldBossReward"] {
		clone := map[string]string{}
		for k, v := range r {
			clone[k] = v
		}
		for _, reward := range rewards {
			if fmt.Sprint(reward.Type) == r["type"] && fmt.Sprint(reward.Level) == r["level"] {
				if reward.Weighted {
					clone["reward"] = adminFieldDropText(reward.Drops)
				} else {
					items := []string{}
					for _, d := range reward.Drops {
						items = append(items, fmt.Sprintf("%s,%d", d.ResourceID, d.Quantities[0].Amount))
					}
					clone["reward"] = strings.Join(items, ";")
				}
				break
			}
		}
		rows = append(rows, clone)
	}
	copied["WorldBossReward"] = rows
	def.Tables = copied
	raw, _ := json.Marshal(def)
	cfg.DefinitionJSON, cfg.DefinitionHash = string(raw), p6Digest(raw)
	return nil
}

// AdminFieldBossDropAllowed checks the original reward delivery consumer without granting.
func (s *GameService) AdminFieldBossDropAllowed(id string) bool {
	ok, err := s.openServerRewardSupported(id + ",1")
	return err == nil && ok
}

// AdminFieldBossAmountAllowed follows the actual attachment grant limit.
func (s *GameService) AdminFieldBossAmountAllowed(id string, amount int64) bool {
	if amount == 0 {
		return true
	}
	ok, err := s.openServerRewardSupported(fmt.Sprintf("%s,%d", id, amount))
	return err == nil && ok
}
