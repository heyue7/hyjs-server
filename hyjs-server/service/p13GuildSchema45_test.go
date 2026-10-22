package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p13GuildSchemaLegacySettingTable 只在隔离测试中显式创建旧表，模拟已存在的旧部署。
func p13GuildSchemaLegacySettingTable(t *testing.T) {
	t.Helper()
	if err := db.GetIns().AutoMigrate(&db.P13UnionFightSetting{}); err != nil {
		t.Fatal(err)
	}
}

func p13GuildSchemaRow(t *testing.T, id, name string) db.P13Guild {
	t.Helper()
	return db.P13Guild{ID: id, Name: name, Level: 7, Notice: "旧公告", Declaration: "旧宣言", Flag: 2, Coin: 4321, Badge: 6, Exp: 88, DonateScore: 9, ActiveScore: 10, CreatedAt: 123}
}

func p13GuildSchemaReload(t *testing.T, id string) db.P13Guild {
	t.Helper()
	var row db.P13Guild
	if err := db.GetIns().First(&row, "id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

// TestP13GuildSchema45LegacyAutoApplyMigration 核对旧 true/false/缺行/已删公会回填，以及重复迁移不覆盖新值。
func TestP13GuildSchema45LegacyAutoApplyMigration(t *testing.T) {
	f, _ := p13GuildSetup(t)
	p13GuildSchemaLegacySettingTable(t)
	// SQLite 每次使用独立临时库；共享 MySQL 隔离库则先清掉同文件上一次运行的旧行。
	if err := db.GetIns().Where("1=1").Delete(&db.P13UnionFightSetting{}).Error; err != nil {
		t.Fatal(err)
	}
	rows := []db.P13Guild{
		p13GuildSchemaRow(t, "schema-on", "迁移开启"),
		p13GuildSchemaRow(t, "schema-off", "迁移关闭"),
		p13GuildSchemaRow(t, "schema-missing", "迁移缺行"),
	}
	for i := range rows {
		if err := db.GetIns().Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, setting := range []db.P13UnionFightSetting{
		{GuildID: "schema-on", AutoApply: true},
		{GuildID: "schema-off", AutoApply: false},
		{GuildID: "schema-gone", AutoApply: true},
	} {
		row := setting
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.p13GuildMigrateUnionFightAutoApply(f.ctx); err != nil {
		t.Fatal(err)
	}
	on := p13GuildSchemaReload(t, "schema-on")
	off := p13GuildSchemaReload(t, "schema-off")
	missing := p13GuildSchemaReload(t, "schema-missing")
	if on.UnionFightAutoApply == nil || !*on.UnionFightAutoApply {
		t.Fatal("旧 true 未回填", on.UnionFightAutoApply)
	}
	if off.UnionFightAutoApply == nil || *off.UnionFightAutoApply {
		t.Fatal("旧 false 未明确落库", off.UnionFightAutoApply)
	}
	if missing.UnionFightAutoApply == nil || *missing.UnionFightAutoApply {
		t.Fatal("缺旧行未按 false 落库", missing.UnionFightAutoApply)
	}
	var gone int64
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", "schema-gone").Count(&gone).Error; err != nil || gone != 0 {
		t.Fatal("旧表孤儿行创建了公会", gone, err)
	}
	// 迁移不触碰公会资产其他列。
	if on.Level != 7 || on.Notice != "旧公告" || on.Declaration != "旧宣言" || on.Flag != 2 || on.Coin != 4321 || on.Badge != 6 || on.Exp != 88 || on.DonateScore != 9 || on.ActiveScore != 10 || on.CreatedAt != 123 {
		t.Fatal("迁移覆盖了公会资产", on)
	}
	// 已迁移后的关闭值不能被旧 true 覆写；同时把部分行还原为 NULL，验证中断后重入。
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", "schema-on").Update("union_fight_auto_apply", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"schema-off", "schema-missing"} {
		if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", id).Update("union_fight_auto_apply", gorm.Expr("NULL")).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.p13GuildMigrateUnionFightAutoApply(f.ctx); err != nil {
		t.Fatal(err)
	}
	if on = p13GuildSchemaReload(t, "schema-on"); on.UnionFightAutoApply == nil || *on.UnionFightAutoApply {
		t.Fatal("重复迁移用旧 true 覆盖了新的 false", on.UnionFightAutoApply)
	}
	if off = p13GuildSchemaReload(t, "schema-off"); off.UnionFightAutoApply == nil || *off.UnionFightAutoApply {
		t.Fatal("重入未按旧 false 落库", off.UnionFightAutoApply)
	}
	if missing = p13GuildSchemaReload(t, "schema-missing"); missing.UnionFightAutoApply == nil || *missing.UnionFightAutoApply {
		t.Fatal("重入未按缺行 false 落库", missing.UnionFightAutoApply)
	}
}

// TestP13GuildSchema45AutoApplyRuntimeAndFreshStart 核对无旧表启动、新公会明确 false、单列开关和读取口径。
func TestP13GuildSchema45AutoApplyRuntimeAndFreshStart(t *testing.T) {
	f, ids := p13GuildSetup(t)
	// 共享 MySQL 隔离库可能沿用同文件前一用例显式创建的旧表，测试内清理以模拟全新安装。
	if db.GetIns().Migrator().HasTable(&db.P13UnionFightSetting{}) {
		if err := db.GetIns().Migrator().DropTable(&db.P13UnionFightSetting{}); err != nil {
			t.Fatal(err)
		}
	}
	// 无旧表重启不得重建旧 Setting 表。
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if db.GetIns().Migrator().HasTable(&db.P13UnionFightSetting{}) {
		t.Fatal("全新启动重建了旧自动宣战表")
	}
	// 无旧表时，历史 NULL 行也必须由启动迁移明确写成 false。
	fresh := p13GuildSchemaRow(t, "schema-fresh", "全新启动")
	if err := db.GetIns().Create(&fresh).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p13GuildMigrateUnionFightAutoApply(f.ctx); err != nil {
		t.Fatal(err)
	}
	if fresh = p13GuildSchemaReload(t, "schema-fresh"); fresh.UnionFightAutoApply == nil || *fresh.UnionFightAutoApply {
		t.Fatal("无旧表迁移哨兵未清理", fresh.UnionFightAutoApply)
	}
	now := p13UFMondays()
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("开关资产")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	created := p13GuildSchemaReload(t, guild)
	if created.UnionFightAutoApply == nil || *created.UnionFightAutoApply {
		t.Fatal("新公会未明确关闭自动宣战", created.UnionFightAutoApply)
	}
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Updates(map[string]any{"coin": 777, "level": 3, "notice": "保持公告"}).Error; err != nil {
		t.Fatal(err)
	}
	p13UFOpen(t)
	p13UFCall(t, f, ids[0], &pb.CSUFAutoApplySet{IsAuto: proto.Int32(1)}, 2, now)
	// 同一设置重复提交保持幂等，且只改开关列。
	p13UFCall(t, f, ids[0], &pb.CSUFAutoApplySet{IsAuto: proto.Int32(1)}, 2, now)
	on := p13GuildSchemaReload(t, guild)
	if on.UnionFightAutoApply == nil || !*on.UnionFightAutoApply || on.Coin != 777 || on.Level != 3 || on.Notice != "保持公告" {
		t.Fatal("开关更新覆盖资产或未落库", on)
	}
	messages, err := f.s.p13UFQuery(f.ctx, ids[0], &pb.CSUFCurrentStageInfo{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if messages[0].(*pb.SCUFCurrentStageInfo).GetIsAutoApply() != 1 {
		t.Fatal("当前阶段未读取公会开关")
	}
	detail, err := adminGuildDetailFrom(f.s, &adminGuildOwned{guild: on})
	if err != nil || !detail.UnionFightApply {
		t.Fatal("后台详情未读取公会开关", detail, err)
	}
	p13UFCall(t, f, ids[0], &pb.CSUFAutoApplySet{IsAuto: proto.Int32(0)}, 3, now)
	off := p13GuildSchemaReload(t, guild)
	if off.UnionFightAutoApply == nil || *off.UnionFightAutoApply || off.Coin != 777 {
		t.Fatal("关闭开关未单列落库", off)
	}
	messages, err = f.s.p13UFQuery(f.ctx, ids[0], &pb.CSUFCurrentStageInfo{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if messages[0].(*pb.SCUFCurrentStageInfo).GetIsAutoApply() != 0 {
		t.Fatal("当前阶段仍显示已关闭开关")
	}
	detail, err = adminGuildDetailFrom(f.s, &adminGuildOwned{guild: off})
	if err != nil || detail.UnionFightApply {
		t.Fatal("后台详情仍显示已关闭开关", detail, err)
	}
}

// TestP13GuildSchema45UpgradeOriginalDDL exercises an actual pre-column table,
// then adds the column/index and preserves the old choice through restart.
func TestP13GuildSchema45UpgradeOriginalDDL(t *testing.T) {
	f := p1Setup(t)
	if db.GetIns().Migrator().HasTable(&db.P13Guild{}) {
		t.Fatal("fresh isolated schema required")
	}
	if err := db.GetIns().Exec(`CREATE TABLE IF NOT EXISTS p13_guilds (id VARCHAR(64) PRIMARY KEY,name VARCHAR(64) NOT NULL,level INT NOT NULL,notice TEXT NOT NULL,declaration TEXT NOT NULL,flag INT NOT NULL,coin BIGINT NOT NULL,badge INT NOT NULL,exp INT NOT NULL,donate_score INT NOT NULL,active_score INT NOT NULL,hero_statue_open INT NOT NULL DEFAULT 0,guard_tree_open INT NOT NULL DEFAULT 0,created_at BIGINT NOT NULL);`).Error; err != nil {
		t.Fatal(err)
	}
	// Use the original unique-name index separately so SQLite can rebuild legacy column types.
	if err := db.GetIns().Exec("CREATE UNIQUE INDEX idx_p13_guild_name ON p13_guilds(name)").Error; err != nil {
		t.Fatal(err)
	}
	old := p13GuildSchemaRow(t, "upgrade-real", "旧结构迁移")
	if err := db.GetIns().Omit("UnionFightAutoApply").Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	p13GuildSchemaLegacySettingTable(t)
	if err := db.GetIns().Create(&db.P13UnionFightSetting{GuildID: old.ID, AutoApply: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P13Guild{}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.p13GuildMigrateUnionFightAutoApply(f.ctx); err != nil {
		t.Fatal(err)
	}
	got := p13GuildSchemaReload(t, old.ID)
	if got.UnionFightAutoApply == nil || !*got.UnionFightAutoApply || got.Coin != old.Coin || got.Name != old.Name {
		t.Fatal("upgrade lost old choice or assets")
	}
	if !db.GetIns().Migrator().HasIndex(&db.P13Guild{}, "UnionFightAutoApply") {
		t.Fatal("auto enrollment filter index missing")
	}
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", old.ID).Update("union_fight_auto_apply", false).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := f.s.p13GuildMigrateUnionFightAutoApply(f.ctx); err != nil {
		t.Fatal(err)
	}
	got = p13GuildSchemaReload(t, old.ID)
	if got.UnionFightAutoApply == nil || *got.UnionFightAutoApply {
		t.Fatal("restart overwrote new false with legacy true")
	}
}
