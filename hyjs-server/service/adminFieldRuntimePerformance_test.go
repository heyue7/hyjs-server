package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// adminFieldRuntimeTestRow 按原表业务键查找覆盖表中的一行，避免依赖表文件顺序。
func adminFieldRuntimeTestRow(tables map[string][]map[string]string, table, key, id string) map[string]string {
	for _, row := range tables[table] {
		if row[key] == id {
			return row
		}
	}
	return nil
}

// adminFieldRuntimeTestClone 深拷贝字符串行，用于比较原始表是否被覆盖构建修改。
func adminFieldRuntimeTestClone(row map[string]string) map[string]string {
	if row == nil {
		return nil
	}
	clone := make(map[string]string, len(row))
	for k, v := range row {
		clone[k] = v
	}
	return clone
}

func adminFieldRuntimeTestSetup(t *testing.T) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f := p1Setup(t)
	for _, load := range []func(string) error{f.s.LoadP8Tables, f.s.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeAdminFields(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "runtime-cache-" + t.Name(), Role: constUtil.AdminRoleOperator, Enabled: true, PasswordHash: "isolated-no-login"}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return f, &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
}

// TestAdminFieldRuntimeSnapshotVisibility 使用真实查询与发布路径验证快照命中、发布/停用可见性、
// 区服/服务/数据库隔离、原表只读和失败不回落旧配置。
func TestAdminFieldRuntimeSnapshotVisibility(t *testing.T) {
	f, actor := adminFieldRuntimeTestSetup(t)
	catalog, err := f.s.AdminFields(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var original model.AdminFieldTemplateDTO
	for _, item := range catalog.Items {
		if item.Configuration.Scene == "field_001" {
			original = item
		}
	}
	if original.ID == "" {
		t.Fatal("missing original field template")
	}
	originalRow := adminFieldRuntimeTestClone(adminFieldRuntimeTestRow(f.s.Tables, "Field", "FieldId", "field_001"))
	if originalRow == nil {
		t.Fatal("missing original field row")
	}
	deployment, err := f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: "field_001", TemplateID: original.ID, TemplateVersion: original.Version, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	firstCache := f.s.adminFieldRuntimeCache.Load()
	if firstCache == nil {
		t.Fatal("runtime snapshot not stored")
	}
	firstField := adminFieldRuntimeTestRow(first, "Field", "FieldId", "field_001")
	if firstField == nil || firstField["adminEnabled"] != "true" {
		t.Fatal("published field not applied")
	}
	// 相同已提交内容再次读取命中同一快照，不重建六张覆盖表。
	second, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() || f.s.adminFieldRuntimeCache.Load() != firstCache {
		t.Fatal("identical content did not reuse snapshot")
	}
	// 原基础 Tables 保持原值，覆盖只存在于快照。
	if current := adminFieldRuntimeTestRow(f.s.Tables, "Field", "FieldId", "field_001"); !reflect.DeepEqual(current, originalRow) {
		t.Fatal("original tables mutated by runtime snapshot")
	}
	// 新配置发布后下一次读取立即使用新值。
	changed := original.Configuration
	changed.MineCount = 2
	saved, err := f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: "runtime-cache-updated", Configuration: changed})
	if err != nil {
		t.Fatal(err)
	}
	var deployed model.AdminFieldDeploymentDTO
	for _, item := range deployment.Items {
		if item.Scene == "field_001" {
			deployed = item
		}
	}
	deployment, err = f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: "field_001", TemplateID: saved.ID, TemplateVersion: saved.Version, Version: deployed.Version, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if row := adminFieldRuntimeTestRow(updated, "Field", "FieldId", "field_001"); row["MineMax"] != "2" || f.s.adminFieldRuntimeCache.Load() == firstCache {
		t.Fatal("published content not visible on next read")
	}
	// 停用后下一次读取立即生效。
	for _, item := range deployment.Items {
		if item.Scene == "field_001" {
			deployed = item
		}
	}
	if _, err = f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: "field_001", TemplateID: saved.ID, TemplateVersion: saved.Version, Version: deployed.Version, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	disabled, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if row := adminFieldRuntimeTestRow(disabled, "Field", "FieldId", "field_001"); row["adminEnabled"] != "false" {
		t.Fatal("deactivation not visible on next read")
	}
	// 直接改库不改 Revision 时，完整内容签名仍能发现并重建。
	var stored db.AdminFieldDeployment
	if err = db.GetIns().First(&stored, "server_id=? AND scene=?", 1, "field_001").Error; err != nil {
		t.Fatal(err)
	}
	outside := changed
	outside.MineCount = 3
	raw, _ := json.Marshal(outside)
	stored.Configuration = string(raw)
	if err = db.GetIns().Save(&stored).Error; err != nil {
		t.Fatal(err)
	}
	external, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if row := adminFieldRuntimeTestRow(external, "Field", "FieldId", "field_001"); row["MineMax"] != "3" || row["adminRevision"] != adminVersion(stored) {
		t.Fatal("external content change without revision not visible")
	}
	// 其他区服没有部署时返回自己的原表，不读取本服快照。
	other, err := f.s.AdminFieldRuntimeTables(f.ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if row := adminFieldRuntimeTestRow(other, "Field", "FieldId", "field_001"); row["MineMax"] != originalRow["MineMax"] || row["adminEnabled"] != "" {
		t.Fatal("realm snapshot crossed into undeployed realm")
	}
	// 独立基础服务各自持有快照，不共享缓存。
	independent := &GameService{Tables: f.s.Tables, ServerID: f.s.ServerID}
	if _, err = independent.AdminFieldRuntimeTables(f.ctx, 1); err != nil {
		t.Fatal(err)
	}
	if independent.adminFieldRuntimeCache.Load() == nil || independent.adminFieldRuntimeCache.Load() == f.s.adminFieldRuntimeCache.Load() {
		t.Fatal("independent service shared runtime snapshot")
	}
	// 不同 Tables 的基础服务不复用其他服务的快照。
	otherTables := make(map[string][]map[string]string, len(f.s.Tables))
	for k, v := range f.s.Tables {
		otherTables[k] = v
	}
	otherFieldRows := make([]map[string]string, 0, len(f.s.Tables["Field"]))
	for _, row := range f.s.Tables["Field"] {
		clone := adminFieldRuntimeTestClone(row)
		if clone["FieldId"] == "field_001" {
			clone["MineMax"] = "777"
		}
		otherFieldRows = append(otherFieldRows, clone)
	}
	otherTables["Field"] = otherFieldRows
	foreign := &GameService{Tables: otherTables}
	foreignTables, err := foreign.AdminFieldRuntimeTables(f.ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if row := adminFieldRuntimeTestRow(foreignTables, "Field", "FieldId", "field_001"); row["MineMax"] != "777" {
		t.Fatal("snapshot reused across different base tables")
	}
	// 构建错误不返回旧快照冒充成功。
	oldCache := f.s.adminFieldRuntimeCache.Load()
	stored.Configuration = "{"
	if err = db.GetIns().Save(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if tables, err := f.s.AdminFieldRuntimeTables(f.ctx, 1); err == nil || tables != nil {
		t.Fatal("invalid deployment returned cached configuration")
	}
	if f.s.adminFieldRuntimeCache.Load() != oldCache {
		t.Fatal("failed build replaced previous snapshot")
	}
	// 修复内容后下一次读取恢复，冷恢复仍读取已提交部署。
	stored.Configuration = string(raw)
	if err = db.GetIns().Save(&stored).Error; err != nil {
		t.Fatal(err)
	}
	beforeRestart := f.s.adminFieldRuntimeCache.Load()
	f.restart(t)
	cold, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if row := adminFieldRuntimeTestRow(cold, "Field", "FieldId", "field_001"); row["MineMax"] != "3" || f.s.adminFieldRuntimeCache.Load() == beforeRestart {
		t.Fatal("cold recovery reused old database snapshot")
	}
	// 数据库错误直接返回，不读取内存旧配置。
	sqlDatabase, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	_ = sqlDatabase.Close()
	if tables, err := f.s.AdminFieldRuntimeTables(f.ctx, 1); err == nil || tables != nil {
		t.Fatal("database error returned cached configuration")
	}
	f.restart(t)
	if tables, err := f.s.AdminFieldRuntimeTables(f.ctx, 1); err != nil || adminFieldRuntimeTestRow(tables, "Field", "FieldId", "field_001")["MineMax"] != "3" {
		t.Fatal("database reopen recovery", err)
	}
}

// TestAdminFieldRuntimeSnapshotCost 记录同输入冷构建与命中的分配和耗时，比例由运行结果决定。
func TestAdminFieldRuntimeSnapshotCost(t *testing.T) {
	f, _ := adminFieldRuntimeTestSetup(t)
	var template db.AdminFieldTemplate
	if err := db.GetIns().First(&template, "id=?", "original-field_001").Error; err != nil {
		t.Fatal(err)
	}
	deployment := db.AdminFieldDeployment{ServerID: 1, Scene: "field_001", TemplateID: template.ID, Enabled: true, Configuration: template.Configuration, Revision: 1, UpdatedAt: 1}
	if err := db.GetIns().Create(&deployment).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdminFieldRuntimeTables(f.ctx, 1); err != nil {
		t.Fatal(err)
	}
	var callErr error
	hitAllocs := testing.AllocsPerRun(20, func() {
		callErr = nil
		_, callErr = f.s.AdminFieldRuntimeTables(f.ctx, 1)
	})
	if callErr != nil {
		t.Fatal(callErr)
	}
	coldAllocs := testing.AllocsPerRun(20, func() {
		callErr = nil
		f.s.adminFieldRuntimeCache.Store(nil)
		_, callErr = f.s.AdminFieldRuntimeTables(f.ctx, 1)
	})
	if callErr != nil {
		t.Fatal(callErr)
	}
	const runs = 100
	start := time.Now()
	for i := 0; i < runs; i++ {
		if _, err := f.s.AdminFieldRuntimeTables(f.ctx, 1); err != nil {
			t.Fatal(err)
		}
	}
	hitDuration := time.Since(start) / runs
	start = time.Now()
	for i := 0; i < runs; i++ {
		f.s.adminFieldRuntimeCache.Store(nil)
		if _, err := f.s.AdminFieldRuntimeTables(f.ctx, 1); err != nil {
			t.Fatal(err)
		}
	}
	coldDuration := time.Since(start) / runs
	t.Logf("runtime tables cost: hit allocs/op=%.1f duration/op=%s, cold allocs/op=%.1f duration/op=%s", hitAllocs, hitDuration, coldAllocs, coldDuration)
	// 命中跳过六张覆盖表复制，分配数应少于冷构建；具体比例以本次实测记录为准。
	if hitAllocs >= coldAllocs {
		t.Fatalf("hit allocations not lower than cold build: hit=%.1f cold=%.1f", hitAllocs, coldAllocs)
	}
}

func adminFieldRuntimeBenchmarkService(b *testing.B) (*GameService, context.Context) {
	b.Helper()
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(b.TempDir(), "runtime-tables.db")); err != nil {
		b.Fatal(err)
	}
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP8Tables, s.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			b.Fatal(err)
		}
	}
	if err := s.InitializeAdminFields(ctx); err != nil {
		b.Fatal(err)
	}
	var template db.AdminFieldTemplate
	if err := db.GetIns().First(&template, "id=?", "original-field_001").Error; err != nil {
		b.Fatal(err)
	}
	deployment := db.AdminFieldDeployment{ServerID: 1, Scene: "field_001", TemplateID: template.ID, Enabled: true, Configuration: template.Configuration, Revision: 1, UpdatedAt: 1}
	if err := db.GetIns().Create(&deployment).Error; err != nil {
		b.Fatal(err)
	}
	sqlDatabase, err := db.GetIns().DB()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = sqlDatabase.Close() })
	return s, ctx
}

func BenchmarkAdminFieldRuntimeTablesCold(b *testing.B) {
	s, ctx := adminFieldRuntimeBenchmarkService(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.adminFieldRuntimeCache.Store(nil)
		if _, err := s.AdminFieldRuntimeTables(ctx, 1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAdminFieldRuntimeTablesHit(b *testing.B) {
	s, ctx := adminFieldRuntimeBenchmarkService(b)
	if _, err := s.AdminFieldRuntimeTables(ctx, 1); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.AdminFieldRuntimeTables(ctx, 1); err != nil {
			b.Fatal(err)
		}
	}
}
