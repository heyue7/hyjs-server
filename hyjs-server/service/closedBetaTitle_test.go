package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestClosedBetaAccountTitle(t *testing.T) {
	f, _, ctx, actor := terminalFixture(t)
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, f.s)
	if db.GetIns().Dialector.Name() == "mysql" {
		// Exercise the additive upgrade on the disposable project test schema.
		if err := db.GetIns().Exec("ALTER TABLE game_accounts DROP COLUMN beta_user").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.InitializeGameAccountsSchema(ctx); err != nil {
			t.Fatal(err)
		}
		if err := db.InitializeGameAccountsSchema(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(ctx); err != nil {
		t.Fatal(err)
	}
	var betaRole int64
	for _, beta := range []bool{true, false} {
		name := "beta-" + uuid.NewString()
		if _, err := registerGlobalGameAccount(ctx, name, "isolated-test-password", "测试账号"); err != nil {
			t.Fatal(err)
		}
		listed, err := AdminGlobalGameAccounts(ctx, actor, name, 1, 25)
		if err != nil || len(listed.Items) != 1 || listed.Items[0].BetaUser {
			t.Fatalf("new account must start outside the whitelist: %v", err)
		}
		account := listed.Items[0]
		if beta {
			updated, err := AdminUpdateGlobalGameAccount(ctx, actor, account.ID, model.AdminGameAccountUpdateRequest{
				Nickname: account.Nickname, Enabled: account.Enabled, BetaUser: &beta, Version: account.Version, Reason: "隔离测试",
			})
			if err != nil || !updated.BetaUser {
				t.Fatalf("admin whitelist update failed: %v", err)
			}
			account = *updated
		}
		shell, err := LoginAccount(ctx, name, "isolated-test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, shell.Id, "封测验证", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, shell.Id, "封测验证", "hero_043"); err != nil {
			t.Fatal(err)
		}
		messages, err := f.s.p5Titles(ctx, shell.Id)
		if err != nil {
			t.Fatal(err)
		}
		var title *pb.TitleInfo
		for _, value := range messages[0].(*pb.SCTitleList).GetTitleList() {
			if value.GetTitleId() == constUtil.GameAccountBetaTitleID {
				title = value
			}
		}
		if title == nil || (title.GetState() == 1) != beta {
			t.Fatalf("wrong title ownership for beta=%v: %+v", beta, title)
		}
		if beta {
			betaRole = shell.Id
			if _, err = f.s.p5ChangeTitle(ctx, shell.Id, &pb.CSChangeTitle{TitleId: proto.String(constUtil.GameAccountBetaTitleID)}, 1); err != nil {
				t.Fatalf("could not wear the title: %v", err)
			}
			flag := false
			if _, err = AdminUpdateGlobalGameAccount(ctx, actor, account.ID, model.AdminGameAccountUpdateRequest{
				Nickname: account.Nickname, Enabled: account.Enabled, BetaUser: &flag, Version: account.Version, Reason: "隔离测试",
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.restart(t)
	if err := f.s.InitializeGlobalGameAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	var saved db.Profile
	if err := db.GetIns().First(&saved, betaRole).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&saved)
	if err != nil || p5KV(game, "p5.title:"+constUtil.GameAccountBetaTitleID) == "" {
		t.Fatalf("title was not retained after restart: %v", err)
	}
	if err := f.s.p5TitleBonus(game, map[string]float64{}, "hero_043"); err != nil {
		t.Fatalf("the original title without stat effects must be wearable: %v", err)
	}
}
