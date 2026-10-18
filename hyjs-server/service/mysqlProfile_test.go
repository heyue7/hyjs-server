package service

import (
	"context"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"strings"
	"sync"
	"testing"
)

// TestMySQLProfileTransactions 用真实 MySQL 验证多连接更新与账号大小写隔离。
func TestMySQLProfileTransactions(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_") {
		t.Skip("isolated MySQL test database required")
	}
	ctx := context.Background()
	if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.GetIns().DB()
	defer sqlDB.Close()
	var ids []int64
	for _, uid := range []string{"TxCase", "txcase", "txcase "} {
		row, err := LoginAccount(ctx, uid, "test-password")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.Id)
	}
	if ids[0] == ids[1] || ids[1] == ids[2] {
		t.Fatal("case-sensitive uid merged")
	}
	if _, err := CreatePlayer(ctx, ids[0], "test", "hero_047"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- updateProfile(ctx, ids[0], func(p *pb.Player, g *model.GameState) error { value := p.GetCoin() + 1; p.Coin = &value; return nil })
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var row db.Profile
	if err := db.GetIns().First(&row, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	p, _, err := decodeProfile(&row)
	if err != nil || p.GetCoin() != 40 {
		t.Fatalf("lost update coin=%d err=%v", p.GetCoin(), err)
	}
	old := append([]byte(nil), row.Session...)
	next, err := LoginAccount(ctx, "TxCase", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AuthorizeAccount(ctx, row.Id, old); err == nil {
		t.Fatal("old session survived rotation")
	}
	if _, err = AuthorizeAccount(ctx, row.Id, next.Session); err != nil {
		t.Fatal(err)
	}
	// 同时创建同一个新账号，唯一键竞争重试后均认证到相同账号。
	same := make(chan *db.Profile, 4)
	fail := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := LoginAccount(ctx, "parallel-create", "test-password")
			same <- p
			fail <- e
		}()
	}
	wg.Wait()
	close(same)
	close(fail)
	for e := range fail {
		if e != nil {
			t.Fatal(e)
		}
	}
	var id int64
	for p := range same {
		if id == 0 {
			id = p.Id
		}
		if id != p.Id {
			t.Fatal("duplicate new account")
		}
	}
}
