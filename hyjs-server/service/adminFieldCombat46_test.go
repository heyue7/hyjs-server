package service

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminFieldLegacyJSONInheritsNewCombatFields 验证旧方案缺新增字段时读取补齐、运行继承原值且不改库。
func TestAdminFieldLegacyJSONInheritsNewCombatFields(t *testing.T) {
	f, actor := adminFieldRuntimeTestSetup(t)
	catalog, err := f.s.AdminFields(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var field model.AdminFieldTemplateDTO
	for _, item := range catalog.Items {
		if item.Configuration.Scene == "field_001" {
			field = item
		}
	}
	if field.ID == "" || len(field.Configuration.MonsterGroups) == 0 || len(field.Configuration.OrdinaryBossGroups) == 0 {
		t.Fatal("original combat reference missing")
	}
	if catalog.Presentation["boss_001"].Detail != "普通野外首领" || catalog.Presentation["world:boss_001"].Detail != "全服共享首领周期" {
		t.Fatal("ordinary and world boss presentation names not separated")
	}
	for _, members := range [][]model.AdminFieldCombatMember{field.Configuration.MonsterGroups[0].Members, field.Configuration.OrdinaryBossGroups[0].Members, field.Configuration.WorldBossMembers} {
		for _, member := range members {
			entry, ok := catalog.Presentation[member.HeroID]
			if !ok || entry.Name == "" {
				t.Fatal("original combat member presentation missing", member.HeroID)
			}
		}
	}
	raw, _ := json.Marshal(field.Configuration)
	var legacy map[string]any
	if json.Unmarshal(raw, &legacy) != nil {
		t.Fatal("legacy fixture")
	}
	delete(legacy, "ordinaryBossEnabled")
	delete(legacy, "ordinaryBossGroups")
	delete(legacy, "worldBossMembers")
	delete(legacy, "worldBossLevel")
	for _, item := range legacy["monsterGroups"].([]any) {
		delete(item.(map[string]any), "members")
	}
	legacyRaw, _ := json.Marshal(legacy)
	var row db.AdminFieldTemplate
	if err = db.GetIns().First(&row, "id=?", field.ID).Error; err != nil {
		t.Fatal(err)
	}
	row.Configuration = string(legacyRaw)
	if err = db.GetIns().Save(&row).Error; err != nil {
		t.Fatal(err)
	}
	catalog, err = f.s.AdminFields(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range catalog.Items {
		if item.ID != field.ID {
			continue
		}
		cfg := item.Configuration
		if cfg.OrdinaryBossEnabled == nil || !*cfg.OrdinaryBossEnabled || len(cfg.OrdinaryBossGroups) != len(field.Configuration.OrdinaryBossGroups) {
			t.Fatal("legacy ordinary boss reference not inherited")
		}
		if len(cfg.MonsterGroups) == 0 || len(cfg.MonsterGroups[0].Members) == 0 || len(cfg.WorldBossMembers) == 0 {
			t.Fatal("legacy members not inherited")
		}
	}
	var stored db.AdminFieldTemplate
	if err = db.GetIns().First(&stored, "id=?", field.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Configuration != string(legacyRaw) {
		t.Fatal("read normalized the stored template")
	}
	// 部署旧内容后，覆盖表继承原数值且启用普通首领。
	deployment := db.AdminFieldDeployment{ServerID: 1, Scene: "field_001", TemplateID: field.ID, Enabled: true, Configuration: stored.Configuration, Revision: 1, UpdatedAt: 1}
	if err = db.GetIns().Create(&deployment).Error; err != nil {
		t.Fatal(err)
	}
	tables, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if row := adminFieldRuntimeTestRow(tables, "Field", "FieldId", "field_001"); row["adminOrdinaryBossEnabled"] != "true" {
		t.Fatal("legacy ordinary boss disabled")
	}
	source := adminFieldRuntimeTestRow(f.s.Tables, "FieldMonster", "MonId", "mon_001")
	current := adminFieldRuntimeTestRow(tables, "FieldMonster", "MonId", "mon_001")
	if source["Hp"] != current["Hp"] || source["Attack"] != current["Attack"] || source["Level"] != current["Level"] {
		t.Fatal("legacy monster inherited zero instead of original")
	}
}

// TestAdminFieldCombatOverridesEnterRuntimeBattle 验证小怪与普通首领数值进入真实战斗输入，且基础表不被污染。
func TestAdminFieldCombatOverridesEnterRuntimeBattle(t *testing.T) {
	f, actor := adminFieldRuntimeTestSetup(t)
	catalog, err := f.s.AdminFields(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var field model.AdminFieldTemplateDTO
	for _, item := range catalog.Items {
		if item.Configuration.Scene == "field_001" {
			field = item
		}
	}
	cfg := field.Configuration
	cfg.MonsterGroups[0].Members[0].Level = 33
	cfg.MonsterGroups[0].Members[0].Hp = 123456
	cfg.MonsterGroups[0].Members[0].Attack = 7777
	cfg.MonsterGroups[0].CooldownMS = 120000
	cfg.OrdinaryBossGroups[0].RebornHours = 6
	cfg.OrdinaryBossGroups[0].Members[0].Level = 40
	cfg.OrdinaryBossGroups[0].Members[0].Hp = 987654
	cfg.OrdinaryBossGroups[0].Members[0].Attack = 4321
	cfg.OrdinaryBossGroups[0].Drops = []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 12, Weight: 1}}}}
	saved, err := f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: "combat-override", Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: cfg.Scene, TemplateID: saved.ID, TemplateVersion: saved.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	tables, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	monsterID := cfg.MonsterGroups[0].Members[0].ID
	monster := adminFieldRuntimeTestRow(tables, "FieldMonster", "MonsterId", monsterID)
	if monster["Level"] != "33" || monster["Hp"] != "123456" || monster["Attack"] != "7777" || monster["RebornTime"] != "120000" {
		t.Fatal("monster runtime values", monster)
	}
	bossRow := adminFieldRuntimeTestRow(tables, "FieldBoss", "Id", cfg.OrdinaryBossGroups[0].Members[0].ID)
	if bossRow["Level"] != "40" || bossRow["Hp"] != "987654" || bossRow["Attack"] != "4321" || bossRow["RebornTime"] != "6" || bossRow["rewardGroupId"] != "coin:12,1" {
		t.Fatal("ordinary boss runtime values", bossRow)
	}
	if source := adminFieldRuntimeTestRow(f.s.Tables, "FieldMonster", "MonsterId", monsterID); source["Hp"] == "123456" {
		t.Fatal("base monster table mutated")
	}
	runtime := &GameService{Tables: tables}
	boss := &pb.SideBoss{BossId: protoString(cfg.OrdinaryBossGroups[0].ID), Uuid: protoString("combat-override")}
	state := &model.GameState{Field: &model.FieldState{BossHP: map[string]map[string]float64{}}}
	battle, err := runtime.fieldBossMonster(boss.GetUuid(), state, boss)
	if err != nil {
		t.Fatal(err)
	}
	if len(battle.Formation) != 4 || battle.Formation[0].GetProp().GetHp() != 987654 || battle.Formation[0].GetProp().GetAtt() != 4321 || battle.Formation[0].GetLevel() != 40 {
		t.Fatal("configured ordinary boss did not enter battle input")
	}
	if tables["Field"][0]["adminOrdinaryBossEnabled"] != "true" {
		t.Fatal("ordinary boss switch not applied")
	}
	monsterActor, err := runtime.actor(monster, "field-monster-override", 1, true)
	if err != nil || monsterActor.GetProp().GetHp() != 123456 || monsterActor.GetProp().GetAtt() != 7777 || monsterActor.GetLevel() != 33 {
		t.Fatal("configured monster did not enter battle actor", err)
	}
}

// TestAdminFieldOrdinaryWorldSwitchesIndependent 验证普通首领和世界首领开关互不覆盖。
func TestAdminFieldOrdinaryWorldSwitchesIndependent(t *testing.T) {
	f, actor := adminFieldRuntimeTestSetup(t)
	catalog, err := f.s.AdminFields(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var field model.AdminFieldTemplateDTO
	for _, item := range catalog.Items {
		if item.Configuration.Scene == "field_001" {
			field = item
		}
	}
	cfg := field.Configuration
	ordinary := false
	cfg.OrdinaryBossEnabled = &ordinary
	cfg.BossEnabled = true
	saved, err := f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: "switches-a", Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: cfg.Scene, TemplateID: saved.ID, TemplateVersion: saved.Version, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	tables, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	row := adminFieldRuntimeTestRow(tables, "Field", "FieldId", cfg.Scene)
	if row["adminOrdinaryBossEnabled"] != "false" || row["adminBossEnabled"] != "true" {
		t.Fatal("ordinary disable leaked into world boss")
	}
	ordinary = true
	cfg.OrdinaryBossEnabled = &ordinary
	cfg.BossEnabled = false
	saved, err = f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{ID: saved.ID, Name: saved.Name, Version: saved.Version, Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	var deployed model.AdminFieldDeploymentDTO
	for _, item := range deployment.Items {
		if item.Scene == cfg.Scene {
			deployed = item
		}
	}
	if _, err = f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: cfg.Scene, TemplateID: saved.ID, TemplateVersion: saved.Version, Version: deployed.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	tables, err = f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	row = adminFieldRuntimeTestRow(tables, "Field", "FieldId", cfg.Scene)
	if row["adminOrdinaryBossEnabled"] != "true" || row["adminBossEnabled"] != "false" {
		t.Fatal("world disable leaked into ordinary boss")
	}
}

// TestAdminFieldCombatCrossMapIsolation 验证共享原组ID时两张地图各自保留数值与刷新权重。
func TestAdminFieldCombatCrossMapIsolation(t *testing.T) {
	fixture, _ := adminFieldRuntimeTestSetup(t)
	field := func(scene, pos string) map[string]string {
		monsterPos := "monster_pos_" + pos[len(pos)-1:]
		return map[string]string{"FieldId": scene, "FieldMap": "map", "BattleMap": "battle", "bossPosIds": pos, "monsterPosIds": monsterPos, "levelLimit": "1", "MineMax": "0", "FishGroupMax": "0", "allSafe": "1"}
	}
	member := func(id, hp string) map[string]string {
		return map[string]string{"MonsterId": id, "MonId": "mon_shared", "HeroId": "hero_shared", "GroupIndex": "1", "Location": "1", "TeamLeader": "1", "Level": "10", "Hp": hp, "Attack": "10", "PhyDefend": "1", "MagicDefend": "1", "hit": "1", "dodge": "1", "cri": "1", "antiCri": "1", "criFactor": "1", "damageAdd": "0", "damageReduce": "0", "RebornTime": "60000", "rewardGroupId": "coin:1,1"}
	}
	bossRows := []map[string]string{}
	for i, id := range []string{"1", "2", "3", "4"} {
		row := map[string]string{"Id": id, "BossId": "boss_shared", "GroupIndex": "1", "HeroId": "hero_shared", "Level": "10", "Hp": "100", "Attack": "10", "PhyDefend": "1", "MagicDefend": "1", "hit": "1", "dodge": "1", "cri": "1", "antiCri": "1", "criFactor": "1", "damageAdd": "0", "damageReduce": "0", "RebornTime": "1", "rewardGroupId": "coin:1,1", "KillReward": "coin:1,1", "Location": "1"}
		if i == 0 {
			row["TeamLeader"] = "1"
		}
		bossRows = append(bossRows, row)
	}
	firstField, secondField := field("field_001", "boss_pos_1"), field("field_002", "boss_pos_2")
	firstField["monsterPosIds"], secondField["monsterPosIds"] = "monster_pos_1", "monster_pos_2"
	tables := map[string][]map[string]string{
		"Field":           {firstField, secondField},
		"FieldMonster":    {member("1", "100"), member("2", "100")},
		"FieldMonsterPos": {{"monsterPosId": "monster_pos_1", "BornPos": "0,0,0", "monsterIdProbs": "mon_shared,10"}, {"monsterPosId": "monster_pos_2", "BornPos": "1,0,1", "monsterIdProbs": "mon_shared,10"}},
		"FieldBoss":       bossRows,
		"FieldBossPos":    {{"BoxxPosId": "boss_pos_1", "BornPos": "0,0,0", "bossIdProbs": "boss_shared,10"}, {"BoxxPosId": "boss_pos_2", "BornPos": "1,0,1", "bossIdProbs": "boss_shared,10"}},
	}
	config := func(scene, pos, hp string) model.AdminFieldConfiguration {
		enabled := true
		members := []model.AdminFieldCombatMember{
			{ID: "1", HeroID: "hero_shared", Location: 1, GroupIndex: 1, Level: 10, Hp: tableNumberString(hp), Attack: 10, PhyDefend: 1, MagicDefend: 1, Hit: 1, Dodge: 1, Cri: 1, AntiCri: 1, CriFactor: 1},
			{ID: "2", HeroID: "hero_shared", Location: 7, GroupIndex: 1, Level: 10, Hp: tableNumberString(hp), Attack: 10, PhyDefend: 1, MagicDefend: 1, Hit: 1, Dodge: 1, Cri: 1, AntiCri: 1, CriFactor: 1},
		}
		bossMembers := []model.AdminFieldCombatMember{}
		for _, id := range []string{"1", "2", "3", "4"} {
			bossMembers = append(bossMembers, model.AdminFieldCombatMember{ID: id, HeroID: "hero_shared", Location: 1, GroupIndex: 1, Level: 10, Hp: tableNumberString(hp), Attack: 10, PhyDefend: 1, MagicDefend: 1, Hit: 1, Dodge: 1, Cri: 1, AntiCri: 1, CriFactor: 1})
		}
		return model.AdminFieldConfiguration{Scene: scene, MinLevel: 1, AllowPK: true, MonsterPoints: []model.AdminFieldPoint{{ID: "monster_pos_" + pos[len(pos)-1:], Point: "0,0,0", Choices: []model.AdminFieldWeight{{ID: "mon_shared", Weight: 1}}}}, MonsterGroups: []model.AdminFieldMonsterGroup{{ID: "mon_shared", HeroID: "hero_shared", Level: 10, CooldownMS: 60000, Members: members, Drops: []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 1, Weight: 1}}}}}}, OrdinaryBossEnabled: &enabled, OrdinaryBossGroups: []model.AdminFieldBossGroup{{ID: "boss_shared", GroupIndex: 1, HeroID: "hero_shared", RebornHours: 1, Members: bossMembers, Drops: []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 1, Weight: 1}}}}, KillDrops: []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 1, Weight: 1}}}}}}}
	}
	cfg1, cfg2 := config("field_001", "boss_pos_1", "111"), config("field_002", "boss_pos_2", "222")
	cfg1.MonsterGroups[0].Drops[0].Quantities[0].Amount = 11
	cfg2.MonsterGroups[0].Drops[0].Quantities[0].Amount = 22
	cfg1.OrdinaryBossGroups[0].Drops[0].Quantities[0].Amount = 111
	cfg2.OrdinaryBossGroups[0].Drops[0].Quantities[0].Amount = 222
	cfg1.OrdinaryBossGroups[0].KillDrops[0].Quantities[0].Amount = 1111
	cfg2.OrdinaryBossGroups[0].KillDrops[0].Quantities[0].Amount = 2222
	raw1, _ := json.Marshal(cfg1)
	raw2, _ := json.Marshal(cfg2)
	s := &GameService{Tables: tables}
	out, err := s.buildAdminFieldRuntimeTables([]db.AdminFieldDeployment{{ServerID: 1, Scene: "field_001", Configuration: string(raw1)}, {ServerID: 1, Scene: "field_002", Configuration: string(raw2)}})
	if err != nil {
		t.Fatal(err)
	}
	source := adminFieldRuntimeTestRow(tables, "FieldMonster", "MonId", "mon_shared")
	if source["Hp"] != "100" {
		t.Fatal("shared base monster mutated")
	}
	runtime := &GameService{Tables: out}
	for _, tc := range []struct{ scene, pos, hp, monsterReward, bossReward, killReward string }{{"field_001", "1", "111", "coin:11,1", "coin,111", "coin,1111"}, {"field_002", "2", "222", "coin:22,1", "coin,222", "coin,2222"}} {
		rows := runtime.adminFieldCombatRows("FieldMonster", "MonId", "mon_shared", tc.scene)
		if len(rows) != 2 || rows[0]["Hp"] != tc.hp {
			t.Fatal("shared monster values leaked across maps", tc.scene)
		}
		bossRows := runtime.fieldBossRowsInScene("boss_shared", tc.scene)
		if len(bossRows) != 4 || bossRows[0]["Hp"] != tc.hp {
			t.Fatal("shared boss values leaked across maps", tc.scene)
		}
		if runtime.fieldLeaderInScene("mon_shared", tc.scene)["rewardGroupId"] != tc.monsterReward {
			t.Fatal("monster reward leaked across maps")
		}
		leader := runtime.fieldBossLeaderInScene("boss_shared", tc.scene)
		reward, err := fieldBossRewardText(leader["rewardGroupId"])
		if err != nil || reward != tc.bossReward {
			t.Fatal("boss attack reward leaked across maps", err)
		}
		killReward, err := fieldBossRewardText(leader["KillReward"])
		if err != nil || killReward != tc.killReward {
			t.Fatal("boss kill reward leaked across maps", err)
		}
		monster, err := runtime.fieldSpawn("monster_pos_" + tc.pos)
		if err != nil || monster.GetMonId() != "mon_shared" {
			t.Fatal("monster wire ID changed", err)
		}
		boss, err := runtime.fieldBossSpawn("boss_pos_" + tc.pos)
		if err != nil || boss.GetBossId() != "boss_shared" {
			t.Fatal("boss wire ID changed", err)
		}
		battle, err := runtime.fieldBossMonster(boss.GetUuid(), &model.GameState{Field: &model.FieldState{Scene: tc.scene}}, boss)
		if err != nil || len(battle.GetFormation()) != 4 || battle.GetTaskId() != "boss_shared" || battle.Formation[0].GetProp().GetHp() != tableNumberString(tc.hp) {
			t.Fatal("scoped boss values or original battle TaskId", err)
		}
		// 按正式快照入口冻结本地图奖励；发布后再次读取仍返回该战斗原合同。
		fightID := "shared-group-" + tc.scene
		if err := runtime.adminFieldFreezeBattle(db.GetIns(), fixture.id, tc.scene, fightID, "mon_shared"); err != nil {
			t.Fatal(err)
		}
		leader["rewardGroupId"] = "coin:999,1"
		monsterLeader := runtime.fieldLeaderInScene("mon_shared", tc.scene)
		monsterLeader["rewardGroupId"] = "coin:999,1"
		frozenReward, _, err := runtime.adminFieldBattleReward(db.GetIns(), fixture.id, tc.scene, fightID, "mon_shared")
		if err != nil || frozenReward != tc.monsterReward {
			t.Fatal("shared group accepted reward lost scene/frozen contract", err)
		}
		actor, err := runtime.actor(rows[0], "scoped-monster", 1, true)
		if err != nil || actor.GetProp().GetHp() != tableNumberString(tc.hp) {
			t.Fatal("scoped monster values did not enter actor", err)
		}
	}
	if runtime.fieldLeaderInScene("mon_shared", "unpublished")["Hp"] != "100" || runtime.fieldBossLeaderInScene("boss_shared", "unpublished")["Hp"] != "100" {
		t.Fatal("unpublished map did not retain original rows")
	}

}

// TestAdminFieldOrdinaryBossDisableKeepsAcceptedBattle 验证关闭普通首领撤下空闲实例但保留战斗实例。
func TestAdminFieldOrdinaryBossDisableKeepsAcceptedBattle(t *testing.T) {
	f := &model.FieldState{
		Bosses:    []*pb.SideBoss{{Uuid: protoString("idle"), State: protoInt32(1)}, {Uuid: protoString("fighting"), State: protoInt32(2)}},
		RespawnAt: map[string]int64{"idle": time.Now().UnixMilli() + 1000},
		BossHP:    map[string]map[string]float64{"idle": {"x": 1}},
	}
	s := &GameService{Tables: map[string][]map[string]string{}}
	if err := s.refreshFieldBosses(f, map[string]string{"adminOrdinaryBossEnabled": "false", "bossPosIds": "boss_pos_001"}); err != nil {
		t.Fatal(err)
	}
	if len(f.Bosses) != 1 || f.Bosses[0].GetUuid() != "fighting" || f.RespawnAt["idle"] != 0 || f.BossHP["idle"] != nil {
		t.Fatal("disabled ordinary boss did not keep accepted battle only")
	}
}

// TestAdminFieldWorldBossCycleFreezeAndNextCycle 验证世界首领已开始周期冻结，下一周期应用新等级与成员。
func TestAdminFieldWorldBossCycleFreezeAndNextCycle(t *testing.T) {
	f, actor := adminFieldRuntimeTestSetup(t)
	if err := f.s.InitializeP12WorldBoss(f.ctx); err != nil {
		t.Fatal(err)
	}
	catalog, err := f.s.AdminFields(f.ctx, actor)
	if err != nil || catalog.WorldBoss == nil || catalog.WorldBoss.Scene == "" {
		t.Fatal("world boss current scene", err)
	}
	var field model.AdminFieldTemplateDTO
	for _, item := range catalog.Items {
		if item.Configuration.Scene == catalog.WorldBoss.Scene {
			field = item
		}
	}
	cfg := field.Configuration
	level := int64(3)
	cfg.WorldBossLevel = &level
	cfg.WorldBossMembers[0].Hp = 333333
	cfg.WorldBossMembers[0].Attack = 3333
	saved, err := f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: "world-cycle", Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: cfg.Scene, TemplateID: saved.ID, TemplateVersion: saved.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	tables, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &GameService{Tables: tables, P12WorldBossBattleReady: false, AdminCatalogueData: f.s.AdminCatalogueData}
	location := time.FixedZone("UTC+8", 8*3600)
	first := time.Date(2026, 10, 22, 12, 30, 0, 0, location)
	tx := db.GetIns().WithContext(context.WithValue(f.ctx, constUtil.GameServiceContextKey, runtime))
	policy, def, err := p12WorldBossConfig(tx)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Level != 3 || !defContains(def, "333333") {
		t.Fatal("new policy level/member not applied", policy.Level)
	}
	// 调用正式世界首领战斗构造器，验证基础覆盖仍乘原等级/部位系数。
	monster, totalHP, err := runtime.p12WorldBossBattleMonster(def, policy.Level)
	if err != nil {
		t.Fatal(err)
	}
	member := cfg.WorldBossMembers[0]
	var factor map[string]string
	for _, row := range def.Tables["WorldBossFactor"] {
		if row["Level"] == "3" && row["Location"] == strconv.FormatInt(member.Location, 10) {
			factor = row
			break
		}
	}
	if factor == nil || len(monster.Formation) != 4 || totalHP <= 0 {
		t.Fatal("world boss combat coefficient/input missing")
	}
	var found bool
	for _, actor := range monster.Formation {
		if actor.GetGuid() == "worldboss-"+member.ID {
			found = true
			if actor.GetLevel() != 3 || math.Abs(actor.GetProp().GetHp()-member.Hp*tableNumber(factor, "Hp")) > 1e-6 || math.Abs(actor.GetProp().GetAtt()-member.Attack*tableNumber(factor, "Attack")) > 1e-6 {
				t.Fatal("world boss configured base values did not enter combat with original factors")
			}
		}
	}
	if !found {
		t.Fatal("configured world boss member missing in battle")
	}
	cycle, frozen, err := p12WorldBossCycle(tx, policy, def, first)
	if err != nil {
		t.Fatal(err)
	}
	originalDefinition := cycle.DefinitionJSON
	if cycle.Level != 3 || !defContains(frozen, "333333") {
		t.Fatal("new cycle did not use configured values")
	}
	// 修改方案后同一已开始周期保持原定义、等级和成员。
	cfg.WorldBossMembers[0].Hp = 444444
	level = 4
	cfg.WorldBossLevel = &level
	saved, err = f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{ID: saved.ID, Name: saved.Name, Version: saved.Version, Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.s.AdminFieldDeployments(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	var deployed model.AdminFieldDeploymentDTO
	for _, item := range current.Items {
		if item.Scene == cfg.Scene {
			deployed = item
		}
	}
	if _, err = f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: cfg.Scene, TemplateID: saved.ID, TemplateVersion: saved.Version, Version: deployed.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	tables, err = f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	runtime = &GameService{Tables: tables, P12WorldBossBattleReady: false, AdminCatalogueData: f.s.AdminCatalogueData}
	tx = db.GetIns().WithContext(context.WithValue(f.ctx, constUtil.GameServiceContextKey, runtime))
	policy, def, err = p12WorldBossConfig(tx)
	if err != nil {
		t.Fatal(err)
	}
	cycle, frozen, err = p12WorldBossCycle(tx, policy, def, first.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if cycle.DefinitionJSON != originalDefinition || cycle.Level != 3 || defContains(frozen, "444444") {
		t.Fatal("started world boss cycle was rewritten")
	}
	// 下一原窗口创建新周期并使用新方案。
	next := time.Date(2026, 10, 22, 21, 45, 0, 0, location)
	cycle, frozen, err = p12WorldBossCycle(tx, policy, def, next)
	if err != nil {
		t.Fatal(err)
	}
	if cycle.Level != 4 || !defContains(frozen, "444444") {
		t.Fatal("next world boss cycle did not apply configuration")
	}
	var started db.P12WorldBossCycle
	if err = db.GetIns().First(&started, "id=?", cycle.ID).Error; err != nil || started.Level != 4 {
		t.Fatal("next cycle persistence", err)
	}
}

func protoString(value string) *string { return &value }
func protoInt32(value int32) *int32    { return &value }
func tableNumberString(value string) float64 {
	number, _ := strconv.ParseFloat(value, 64)
	return number
}
func defContains(def *p12WorldBossDefinition, value string) bool {
	raw, _ := json.Marshal(def)
	return json.Valid(raw) && len(raw) != 0 && stringContains(string(raw), value)
}
func stringContains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

// TestAdminFieldMemberPresentationUsesOriginalCatalogue 验证所有成员按原英雄名key及原图源投影。
func TestAdminFieldMemberPresentationUsesOriginalCatalogue(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{
		"Hero":         {{"id": "member-a", "name": "name-a", "iconName": "sprite-a"}, {"id": "member-b", "name": "name-b", "iconName": "sprite-b"}},
		"FieldMonster": {{"HeroId": "member-a", "TeamLeader": "0"}},
		"WorldBoss":    {{"HeroId": "member-b", "show": "0"}},
	}, AdminCatalogueData: &model.AdminCatalogueData{DisplayNames: map[string]string{"name-a": "成员甲", "name-b": "成员乙"}, SpriteURLs: map[string]string{"sprite-a": "/original/a.png", "sprite-b": "/original/b.png"}}}
	out := s.adminFieldPresentation()
	if out["member-a"].Name != "成员甲" || out["member-a"].IconURL != "/original/a.png" || out["member-b"].Name != "成员乙" || out["member-b"].IconURL != "/original/b.png" {
		t.Fatal("member name/icon did not follow original catalogue mappings")
	}
}
