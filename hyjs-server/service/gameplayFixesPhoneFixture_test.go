package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Extend the existing explicitly isolated phone fixture; never run on a normal
// development or production schema. All claims and fights remain phone actions.
func TestGameplayFixesPhoneFixture(t *testing.T) {
	if os.Getenv("HYJS_GAMEPLAY_FIXES_PHONE") != "1" {
		t.Skip("explicit isolated phone fixture required")
	}
	TestTrial82PhoneFixture(t)
	var login struct {
		PlayerID int64 `json:"playerId"`
	}
	raw, err := os.ReadFile(os.Getenv("HYJS_TRIAL82_PHONE_FIXTURE_FILE"))
	if err != nil || json.Unmarshal(raw, &login) != nil || login.PlayerID != 2 {
		t.Fatal("isolated phone identity unavailable", err)
	}
	s := &GameService{ServerID: 1}
	ctx := context.WithValue(context.Background(), constUtil.GameServiceContextKey, s)
	for _, load := range []func(string) error{s.LoadTables, s.LoadP7Tables, s.LoadP13GuildTables} {
		if err = load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	s.InstallGrowth74Resources()
	var operator db.AdminUser
	if err = db.GetIns().Where("username=?", "qa77-fixture-admin").First(&operator).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	page, err := s.AdminShop(ctx, "point")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyTrialDailyGoods(ctx, actor, page.Version); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P7RushState{}).Where("player_id=? AND run_id=?", login.PlayerID, 1).Updates(map[string]any{"floor": 4, "high_floor": 3, "total_stars": 9}).Error; err != nil {
		t.Fatal(err)
	}
	// This finite fixture uses the ordinary catalogue directly; the inherited
	// calendar has no trial shop occurrence, so its hours must be unlimited.
	if err = db.GetIns().Model(&db.AdminShopAvailability{}).Where("shop_key=?", "point").Update("unlimited", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeP10Social, s.InitializeP13Guild} {
		if err = init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	p13GuildOpen(t, true)
	now := LocalPlayNow(ctx)
	p13GuildDonateBoxEnable(t, now)
	if err = updateProfile(ctx, login.PlayerID, func(p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(30000000)
		_, gainErr := s.GrantFixedReward(p, g, "item_089,50")
		return gainErr
	}); err != nil {
		t.Fatal(err)
	}
	f := &p1Fixture{s: s, ctx: ctx, id: login.PlayerID}
	p13GuildCall(t, f, f.id, &pb.CSUnionCreate{Name: proto.String("本轮隔离验收")}, 7001, now)
	p13GuildDonateBoxEarnFirst(t, f, f.id, now)
	// A finite entry-only QA rule bypasses the first boss prerequisite; the
	// original boss, math, costs and End verification remain enabled and unchanged.
	if err = db.GetIns().Model(&db.P7RushStageRule{}).Where("kind=? AND task_id=?", constUtil.P7RushElite, "01").Updates(map[string]any{"enabled": true, "condition_task_id": "", "source": "gameplay120 isolated entry fixture: original elite 01 actors; prerequisite bypass"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{&db.P7RushPolicy{}, &db.P7RushScopedPolicy{}, &db.P7RushManualPolicy{}} {
		if err = db.GetIns().Model(row).Where("id IN ?", []int64{1, 2}).Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Log("isolated daily goods configured; real donations earned box one; first original boss entry enabled; no claims or fights created by this extension")
}
