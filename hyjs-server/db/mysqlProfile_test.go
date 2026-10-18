package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMySQLMigrationExact 使用专属临时库验证二进制、大小写、空值、重试和历史序号。
func TestMySQLMigrationExact(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_") {
		t.Skip("isolated MySQL test database required")
	}
	cfg := MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}
	ctx := context.Background()
	d, err := OpenGameMySQL(ctx, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	sqlD, _ := d.DB()
	defer sqlD.Close()
	path := filepath.Join(t.TempDir(), "source.sqlite3")
	s, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Exec(profileSchema); err != nil {
		t.Fatal(err)
	}
	for i, uid := range []string{"CasePlayer", "caseplayer", "caseplayer "} {
		var salt any = []byte{}
		if i == 0 {
			salt = nil
		}
		if _, err = s.Exec("INSERT INTO profiles(id,uid,session,player,game,created_at,password_salt,password_hash) VALUES(?,?,?,?,?,?,?,?)", i+1, uid, []byte{0, 255, 1}, " {\"playerId\":1} ", nil, int64(1800000000123), salt, []byte{254, 0, 253}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.Exec("UPDATE sqlite_sequence SET seq=999 WHERE name='profiles'"); err != nil {
		t.Fatal(err)
	}
	if err = InitGameplayPoolSchema(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Exec("CREATE TABLE gameplay_pools (id TEXT PRIMARY KEY,name TEXT,source TEXT,enabled INTEGER,min_level INTEGER,single_cost TEXT,ten_cost TEXT,single_ticket TEXT,ten_ticket TEXT,free_seconds INTEGER,free_per_day INTEGER,guarantee_every INTEGER,coin_per_draw INTEGER,start_at INTEGER,end_at INTEGER,reward_end_at INTEGER,initial_diamond_pool INTEGER,jackpot_per_million INTEGER,pool_contribution_bps INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Exec("INSERT INTO gameplay_pools VALUES('custom','pool','fixture',1,20,'coin100','','','',10,1,10,2,0,0,0,100,2,3)"); err != nil {
		t.Fatal(err)
	}
	report, err := MigrateSQLiteToMySQL(ctx, path, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Inserted != 3 || report.Sequence != 999 || report.Pools != 1 {
		t.Fatalf("unexpected report %+v", report)
	}
	again, err := MigrateSQLiteToMySQL(ctx, path, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if again.Inserted != 0 || again.SHA256 != report.SHA256 {
		t.Fatal("non-idempotent import")
	}
	var nullSalt, emptySalt Profile
	d.First(&nullSalt, 1)
	d.First(&emptySalt, 2)
	if nullSalt.PasswordSalt != nil || emptySalt.PasswordSalt == nil || len(emptySalt.PasswordSalt) != 0 {
		t.Fatal("NULL and empty blob merged")
	}
	var pool GameplayPool
	if err = d.First(&pool, "id=?", "custom").Error; err != nil || pool.InitialDiamondPool != 100 {
		t.Fatal("pool not preserved", err)
	}
	p := Profile{Uid: "new-account", Session: []byte{1}}
	if err = d.Create(&p).Error; err != nil || p.Id <= 999 {
		t.Fatal("historical auto-increment lost", err)
	}
	// 先遇到可插入记录，再遇到冲突，证明整批事务回滚。
	if _, err = s.Exec("INSERT INTO profiles(id,uid,session) VALUES(0,'before-conflict',X'00')"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Exec("UPDATE profiles SET player='changed' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	if _, err = MigrateSQLiteToMySQL(ctx, path, cfg, true); err == nil {
		t.Fatal("conflict accepted")
	}
	var count int64
	d.Model(&Profile{}).Where("uid=?", "before-conflict").Count(&count)
	if count != 0 {
		t.Fatal("partial import after conflict")
	}
	if _, err = s.Exec("UPDATE profiles SET player=' {\"playerId\":1} ' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	if _, err = MigrateSQLiteToMySQL(ctx, path, cfg, false); err != nil {
		t.Fatal(err)
	}
	d.Model(&Profile{}).Where("uid=?", "before-conflict").Count(&count)
	if count != 0 {
		t.Fatal("dry run changed destination")
	}
	if _, err = MigrateSQLiteToMySQL(ctx, path, cfg, true); err != nil {
		t.Fatal(err)
	}
	var zero Profile
	if err = d.First(&zero, "id=0").Error; err != nil || zero.Uid != "before-conflict" {
		t.Fatal("zero id was remapped", err)
	}
}

// TestMySQLLegacyColumns 确认早期五列表无需写回源库即可迁移，并保留负数原始主键。
func TestMySQLLegacyColumns(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_") {
		t.Skip("isolated MySQL test database required")
	}
	cfg := MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}
	path := filepath.Join(t.TempDir(), "legacy.sqlite3")
	s, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Exec("CREATE TABLE profiles(id INTEGER PRIMARY KEY,uid TEXT UNIQUE,session BLOB NOT NULL,player TEXT,created_at INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Exec("INSERT INTO profiles VALUES(-7,'legacy-negative',X'',NULL,NULL),(20001,'legacy-columns',X'00FF','{}',1234567890123)"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = MigrateSQLiteToMySQL(ctx, path, cfg, true); err != nil {
		t.Fatal(err)
	}
	d, err := OpenGameMySQL(ctx, cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	sqlD, _ := d.DB()
	defer sqlD.Close()
	var p Profile
	if err = d.First(&p, 20001).Error; err != nil {
		t.Fatal(err)
	}
	if p.Game != nil || p.PasswordHash != nil || p.PasswordSalt != nil || p.CreatedAt == nil || *p.CreatedAt != 1234567890123 {
		t.Fatal("legacy missing columns changed")
	}
	p = Profile{}
	if err = d.First(&p, "id=-7").Error; err != nil {
		t.Fatal(err)
	}
	var columns int
	if err = s.QueryRow("SELECT count(*) FROM pragma_table_info('profiles')").Scan(&columns); err != nil || columns != 5 {
		t.Fatal("source schema changed", err)
	}
	if _, err = s.Exec("CREATE TABLE unknown_runtime(id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err = MigrateSQLiteToMySQL(ctx, path, cfg, true); err == nil {
		t.Fatal("unknown table silently omitted")
	}
}
