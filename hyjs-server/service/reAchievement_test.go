package service

import (
	"bytes"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func reAchievementFixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	adminWalletTestEnvironment(t)
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := InitializeAdminGameContent(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleOperator)
	return f, actor
}
func reNativeAchievements(t *testing.T, f *p1Fixture) map[string]*pb.AchievementInfo {
	t.Helper()
	out, handled, err := f.s.DispatchP5(f.ctx, f.id, &pb.CSAchievementList{}, 0)
	if err != nil || !handled {
		t.Fatal("original achievement list", err)
	}
	rows := map[string]*pb.AchievementInfo{}
	for _, row := range out[0].(*pb.SCAchievementList).GetList() {
		rows[strings.SplitN(row.GetId(), "~", 2)[0]] = row
	}
	return rows
}
func TestReAchievementsNative(t *testing.T) {
	f, actor := reAchievementFixture(t)
	manual := db.P5Achievement{ID: "a306_level_5", Name: "运营原配置", Type: 2, Kind: "level", Target: 9, Reward: "coin,7", Source: "operator existing configuration"}
	if err := db.GetIns().Create(&manual).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeReAchievements(f.ctx); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P5Achievement{}).Count(&count)
	if count != 35 {
		t.Fatal("Re configuration catalogue incomplete", count)
	}
	var saved db.P5Achievement
	db.GetIns().First(&saved, "id=?", manual.ID)
	if string(adminJSON(manual)) != string(adminJSON(saved)) {
		t.Fatal("manual configuration overwritten")
	}
	if err := f.s.RecordP1Login(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	initial := reNativeAchievements(t, f)
	if initial["ach_login"].GetState() != 0 || initial["ach_login"].GetType() != 37 || initial["ach_level"].GetState() != 1 {
		t.Fatal("native button semantics or true login source", initial["ach_login"], initial["ach_level"])
	}
	if _, err := f.s.p5AchievementReward(f.ctx, f.id, &pb.CSGetAchievementReward{Id: proto.String("ach_level")}, 901); err == nil {
		t.Fatal("unearned level reward granted")
	}
	request := &pb.CSGetAchievementReward{Id: proto.String("ach_login")}
	first, err := f.s.p5AchievementReward(f.ctx, f.id, request, 902)
	if err != nil {
		t.Fatal(err)
	}
	gain := first[0].(*pb.SCGetAchievementReward).GetGet()
	if len(gain.GetSimps()) != 1 || gain.GetSimps()[0].GetId() != "item_002" || gain.GetSimps()[0].GetNum() != 10 {
		t.Fatal("Re native reward changed", gain)
	}
	before, _ := f.profile(t)
	if _, err = f.s.p5AchievementReward(f.ctx, f.id, request, 902); err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if before.GetCoin() != after.GetCoin() || before.GetDiamond() != after.GetDiamond() {
		t.Fatal("receipt replay changed balance")
	}
	listing, err := AdminAchievements(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var edit model.AdminAchievementDTO
	for _, row := range listing.Items {
		if row.ID == "ach_level" {
			edit = row
		}
	}
	edit.Target = 999
	edit.Rewards = []model.AdminResourceAmount{{ResourceID: "diamond", Amount: 999}}
	if _, err = f.s.AdminSaveAchievement(f.ctx, actor, edit.ID, edit, false, false); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(20); return nil }); err != nil {
		t.Fatal(err)
	}
	visible := reNativeAchievements(t, f)
	if visible["ach_level"].GetState() != 0 || visible["ach_level"].GetTotalNum() != 20 || visible["ach_level"].GetReward() != "item_006,50" {
		t.Fatal("old frozen rule overwritten", visible["ach_level"])
	}
	if visible["ach_login"].GetState() != 2 {
		t.Fatal("claimed original reward not completed")
	}
	if _, err = f.s.p5AchievementReward(f.ctx, f.id, &pb.CSGetAchievementReward{Id: proto.String("ach_level")}, 903); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err = f.s.InitializeReAchievements(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p5AchievementReward(f.ctx, f.id, request, 904); err == nil {
		t.Fatal("Re claim regranted after cold")
	}
	saved = db.P5Achievement{}
	if err = db.GetIns().First(&saved, "id=?", edit.ID).Error; err != nil || saved.Target != 999 || saved.Reward != "diamond,999" {
		t.Fatal("restart replaced operator changes", saved, err)
	}
}
func TestReOSAchievementTriples(t *testing.T) {
	f, actor := reAchievementFixture(t)
	if err := f.s.InitializeReAchievements(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Finite fixture uses the actual trusted login-event producer for seven distinct calendar days.
	for i := 0; i < 7; i++ {
		at := now.AddDate(0, 0, -i)
		if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
			return f.s.RecordP1Event(tx, f.id, "login:"+p1Day(at), constUtil.P1EventLoginDay, 1, at)
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(40)
		g.Copy = &pb.SCCopyPassedInfo{}
		for _, row := range f.s.Tables["CopyMission"] {
			if len(g.Copy.PassedCopyInfo) == 30 {
				break
			}
			g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, &pb.CopySimpleInfo{ChapterId: proto.String(row["ChapterId"]), MissionId: proto.String(row["MissionId"]), Star: proto.Int32(5)})
		}
		first := g.Copy.PassedCopyInfo[0]
		g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, &pb.CopySimpleInfo{ChapterId: first.ChapterId, MissionId: first.MissionId, Star: proto.Int32(1)}, &pb.CopySimpleInfo{ChapterId: proto.String("wrong"), MissionId: first.MissionId, Star: proto.Int32(5)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, g := f.profile(t)
	stages, stars := f.s.p5AchievementCopyProgress(g)
	if stages != 30 || stars != 150 {
		t.Fatal("same stage double count or real five stars truncated", stages, stars)
	}
	visible := reNativeAchievements(t, f)
	if visible["os310_hero_077"].GetState() != 0 || visible["os310_hero_077"].GetCurrentNum() != 3 || visible["os310_hero_078"].GetState() != 1 || visible["os310_hero_078"].GetCurrentNum() != 0 {
		t.Fatal("compound conditions or cache reused another tier", visible["os310_hero_077"], visible["os310_hero_078"])
	}
	listing, err := AdminAchievements(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range listing.Items {
		if row.ID == "os310_hero_077" {
			if row.Requirements == nil || row.Requirements.LoginDays != 7 || row.Requirements.Level != 40 || row.Requirements.Stages != 30 {
				t.Fatal("backend omitted actual OS requirements", row)
			}
			row.Requirements.Level = 100
			if _, err = f.s.AdminSaveAchievement(f.ctx, actor, row.ID, row, false, false); err != nil {
				t.Fatal("OS config CRUD", err)
			}
		}
	}
	wireID := visible["os310_hero_077"].GetId()
	if wireID != "os310_hero_077~7~40~30" {
		t.Fatal("missing immutable display requirements", wireID)
	}
	for _, bad := range []string{"os310_hero_077~7~100~30", "os310_hero_077~7~40~030", "ach_login~1~1~1"} {
		if _, e := f.s.p5AchievementReward(f.ctx, f.id, &pb.CSGetAchievementReward{Id: proto.String(bad)}, 910); e == nil {
			t.Fatal("forged display requirements accepted", bad)
		}
	}
	request := &pb.CSGetAchievementReward{Id: proto.String(wireID)}
	out, err := f.s.p5AchievementReward(f.ctx, f.id, request, 911)
	if err != nil {
		t.Fatal("accepted original OS contract changed by editor", err)
	}
	replayed, e := f.s.p5AchievementReward(f.ctx, f.id, &pb.CSGetAchievementReward{Id: proto.String("os310_hero_077")}, 911)
	if e != nil || !proto.Equal(out[0], replayed[0]) {
		t.Fatal("canonical receipt changed across display ID", e)
	}
	var receipt db.P5MutationReceipt
	if e = db.GetIns().First(&receipt, "player_id=? AND operation=? AND timestamp=?", f.id, "achievement", 911).Error; e != nil {
		t.Fatal(e)
	}
	canonicalBytes, _ := proto.Marshal(&pb.CSGetAchievementReward{Id: proto.String("os310_hero_077")})
	if !bytes.Equal(receipt.Request, canonicalBytes) {
		t.Fatal("receipt persisted derived display ID")
	}
	wars := out[0].(*pb.SCGetAchievementReward).GetGet().GetWars()
	if len(wars) != 1 || wars[0].GetId() != "hero_077" {
		t.Fatal("actual original OS hero not granted", wars)
	}
	_, after := f.profile(t)
	if len(after.Warriors) != len(g.Warriors)+1 {
		t.Fatal("wrong OS instance grant")
	}
	if _, err = f.s.p5AchievementReward(f.ctx, f.id, request, 912); err == nil {
		t.Fatal("OS first acquisition granted twice")
	}
	// Existing acquisition is read as completed without fabricating a paid receipt or erasing old state.
	other, err := LoginAccount(f.ctx, "re-existing-os-owner", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "已有OS角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, other.Id, func(p *pb.Player, g *model.GameState) error {
		_, err := f.s.GrantFixedReward(p, g, "hero_078,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	owner := f.id
	f.id = other.Id
	if got := reNativeAchievements(t, f)["os310_hero_078"]; got.GetState() != 2 {
		t.Fatal("Re first acquisition did not recognize owned original hero", got)
	}
	if _, err = f.s.p5AchievementReward(f.ctx, f.id, &pb.CSGetAchievementReward{Id: proto.String("os310_hero_078")}, 913); err == nil {
		t.Fatal("owned OS was regranted")
	}
	f.id = owner
	f.restart(t)
	if err = f.s.InitializeReAchievements(f.ctx); err != nil {
		t.Fatal(err)
	}
	var frozen db.P5AchievementClaim
	if err = db.GetIns().First(&frozen, "player_id=? AND id=?", f.id, "os310_hero_077").Error; err != nil {
		t.Fatal(err)
	}
	var snapshot db.P5Achievement
	if err = json.Unmarshal([]byte(frozen.Snapshot), &snapshot); err != nil || snapshot.RequiredLevel != 40 || frozen.ClaimedAt <= 0 {
		t.Fatal("old compound snapshot lost", snapshot, err)
	}
}
