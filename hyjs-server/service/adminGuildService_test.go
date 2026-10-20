package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func adminGuildTestActor(t *testing.T, role string) *model.AdminPrincipal {
	t.Helper()
	if err := AdminInitialize(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "guild-" + role + "-" + t.Name(), PasswordHash: "fixture", Role: role, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return &model.AdminPrincipal{UserID: user.ID, Role: role, Username: user.Username}
}

func adminGuildTestCreate(t *testing.T, f *p1Fixture, id int64, name string) string {
	t.Helper()
	p13GuildOpen(t, true)
	now := time.Now()
	if _, err := f.s.p13GuildCreate(f.ctx, id, &pb.CSUnionCreate{Name: proto.String(name)}, 1, now); err != nil {
		t.Fatal(err)
	}
	info := p13GuildCall(t, f, id, &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo)
	if info.GetUnionInfo().GetUnionId() == "" {
		t.Fatal("missing created guild")
	}
	return info.GetUnionInfo().GetUnionId()
}

func TestAdminGuildSettingsVersionRuntimeAndPolicy(t *testing.T) {
	f, _ := p13GuildSetup(t)
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	before, err := f.s.AdminGuildSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Groups) != 5 || len(before.UnionLevels.Rows) == 0 || len(before.UnionTech.Rows) == 0 {
		t.Fatal("incomplete settings", len(before.Groups), len(before.UnionLevels.Rows), len(before.UnionTech.Rows))
	}
	for _, group := range before.Groups {
		for _, field := range group.Fields {
			if field.Label == "" || field.Label == field.Key {
				t.Fatalf("missing original label %s", field.Key)
			}
		}
	}
	originalScore := f.s.table("GameConfigure", "StaticName", "UnionLoginScore")["NameValue"]
	if _, err = f.s.AdminSaveGuildSettings(ctx, actor, model.AdminGuildSettingSaveRequest{
		Version: before.Version,
		Reason:  "调整登录积分",
		Fields:  []model.AdminGuildConfigValue{{Key: "UnionLoginScore", Value: "7"}},
		Policies: &model.AdminGuildPolicySave{Guild: &model.AdminGuildGatePolicy{
			ManagementEnabled: true, EconomyEnabled: true, ChatEnabled: true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	after, err := f.s.AdminGuildSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if after.UpdatedAt < time.Now().Add(-time.Minute).UnixMilli() || after.UpdatedAt > time.Now().Add(time.Minute).UnixMilli() {
		t.Fatal("guild setting update timestamp lost millisecond unit", after.UpdatedAt)
	}
	var score string
	for _, group := range after.Groups {
		for _, field := range group.Fields {
			if field.Key == "UnionLoginScore" {
				score = field.Value
			}
		}
	}
	if score != "7" || !after.Policies.Guild.ManagementEnabled || !after.Policies.Guild.EconomyEnabled || !after.Policies.Guild.ChatEnabled {
		t.Fatal("settings not persisted", score, after.Policies.Guild)
	}
	if f.s.table("GameConfigure", "StaticName", "UnionLoginScore")["NameValue"] != originalScore {
		t.Fatal("shared static table was mutated")
	}
	runtimeTables, err := AdminGuildRuntimeTables(ctx, f.s.Tables)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range runtimeTables["GameConfigure"] {
		if row["StaticName"] == "UnionLoginScore" {
			found = row["NameValue"] == "7"
			if runtimeTables["GameConfigure"][0]["StaticName"] == "UnionLoginScore" {
				t.Fatal("runtime clone reused row map")
			}
		}
	}
	if !found {
		t.Fatal("runtime table override missing")
	}
	if _, err = f.s.AdminSaveGuildSettings(ctx, actor, model.AdminGuildSettingSaveRequest{Version: before.Version, Reason: "stale"}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale settings overwritten", err)
	}
	if _, err = f.s.AdminSaveGuildSettings(ctx, actor, model.AdminGuildSettingSaveRequest{Version: after.Version, Reason: "unknown", Fields: []model.AdminGuildConfigValue{{Key: "UnionUnknown", Value: "1"}}}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("unknown field accepted", err)
	}
	if _, err = f.s.AdminSaveGuildSettings(ctx, actor, model.AdminGuildSettingSaveRequest{Version: after.Version, Reason: "cross limit", Fields: []model.AdminGuildConfigValue{{Key: "UnionMinDailyDonateCoin", Value: "99999999"}}}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("cross limit accepted", err)
	}
	viewer := adminGuildTestActor(t, constUtil.AdminRoleViewer)
	if _, err = f.s.AdminSaveGuildSettings(ctx, viewer, model.AdminGuildSettingSaveRequest{Version: after.Version, Reason: "viewer"}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer wrote settings", err)
	}
}

func TestAdminGuildRealmManagementAndDissolve(t *testing.T) {
	f, ids := p13GuildSetup(t)
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	guildID := adminGuildTestCreate(t, f, ids[0], "后台管理公会")
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	list, err := f.s.AdminGuilds(ctx, "", 1, 20)
	if err != nil || list.Total != 1 || len(list.Items) != 1 {
		t.Fatal("actual guild not listed", list, err)
	}
	detail, err := f.s.AdminGuildDetail(ctx, guildID)
	if err != nil || len(detail.Members) != 1 || detail.Members[0].Role != constUtil.P13GuildChairman {
		t.Fatal(detail, err)
	}
	detail, err = f.s.AdminUpdateGuild(ctx, actor, guildID, model.AdminGuildUpdateRequest{
		Version: detail.Version, Reason: "修正共享资产", Name: detail.Name, Level: detail.Level,
		Notice: "新公告", Declaration: "新宣言", Flag: 1, Coin: 123, Badge: 4, Exp: 5,
		DonateScore: 6, ActiveScore: 7, HeroStatueOpen: 1, GuardTreeOpen: 1,
	})
	if err != nil || detail.Coin != 123 || detail.Notice != "新公告" || detail.HeroStatueOpen != 1 {
		t.Fatal("guild update", detail, err)
	}
	detail, err = f.s.AdminAddGuildMember(ctx, actor, guildID, model.AdminGuildMemberAddRequest{Version: detail.Version, Reason: "补入成员", PlayerID: ids[1]})
	if err != nil || len(detail.Members) != 2 {
		t.Fatal("member add", detail, err)
	}
	var target model.AdminGuildMemberDTO
	for _, member := range detail.Members {
		if member.PlayerID == ids[1] {
			target = member
		}
	}
	detail, err = f.s.AdminUpdateGuildMember(ctx, actor, guildID, ids[1], model.AdminGuildMemberUpdateRequest{
		Version: target.Version, Reason: "转让会长并修正贡献", Contribution: 42, TodayCoin: 100, TodayBadge: 2, Role: constUtil.P13GuildChairman,
	})
	if err != nil {
		t.Fatal(err)
	}
	chairmen, current := 0, model.AdminGuildMemberDTO{}
	for _, member := range detail.Members {
		if member.Role == constUtil.P13GuildChairman {
			chairmen++
			current = member
		}
	}
	if current.DonateDay != p13GuildDay(LocalPlayNow(ctx)) || current.TodayCoin != 100 || current.TodayBadge != 2 {
		t.Fatal("daily donation edit did not bind the current operating day", current)
	}
	if chairmen != 1 || current.PlayerID != ids[1] || current.Contribution != 42 {
		t.Fatal("chairman transfer", detail.Members)
	}
	if _, err = f.s.AdminUpdateGuildMember(ctx, actor, guildID, ids[1], model.AdminGuildMemberUpdateRequest{Version: current.Version, Reason: "非法降职", Role: constUtil.P13GuildMember}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("chairman demoted directly", err)
	}
	detail, err = f.s.AdminRemoveGuildMember(ctx, actor, guildID, ids[0], model.AdminGuildMemberRemoveRequest{Version: detail.Version, Reason: "清理成员"})
	if err != nil || len(detail.Members) != 1 {
		t.Fatal("member remove", detail, err)
	}
	var removed db.P13GuildMember
	if err = db.GetIns().First(&removed, "player_id=?", ids[0]).Error; err != nil || removed.QuitUntil <= time.Now().UnixMilli() {
		t.Fatal("original cooldown missing", removed, err)
	}
	detail, err = f.s.AdminAddGuildMember(ctx, actor, guildID, model.AdminGuildMemberAddRequest{Version: detail.Version, Reason: "加入第三人", PlayerID: ids[2]})
	if err != nil || len(detail.Members) != 2 {
		t.Fatal("second add", detail, err)
	}
	other := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(2))
	if list, err = f.s.AdminGuilds(other, "", 1, 20); err != nil || list.Total != 0 {
		t.Fatal("cross realm listed", list, err)
	}
	if _, err = f.s.AdminGuildDetail(other, guildID); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("cross realm detail", err)
	}
	if _, err = f.s.AdminDissolveGuild(ctx, actor, guildID, model.AdminGuildDissolveRequest{Version: detail.Version, ConfirmName: "错误名字", Reason: "测试"}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("wrong confirm name accepted", err)
	}
	if _, err = f.s.AdminDissolveGuild(ctx, actor, guildID, model.AdminGuildDissolveRequest{Version: detail.Version, ConfirmName: detail.Name, Reason: "正式解散"}); err != nil {
		t.Fatal(err)
	}
	if list, err = f.s.AdminGuilds(ctx, "", 1, 20); err != nil || list.Total != 0 {
		t.Fatal("guild still listed", list, err)
	}
}

func TestP13GuildMemberAvatarUsesFirstFormationHeroAndFallback(t *testing.T) {
	f, ids := p13GuildSetup(t)
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	guildID := adminGuildTestCreate(t, f, ids[0], "头像投影公会")
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	detail, err := f.s.AdminGuildDetail(ctx, guildID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminAddGuildMember(ctx, actor, guildID, model.AdminGuildMemberAddRequest{Version: detail.Version, Reason: "头像成员", PlayerID: ids[1]}); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, ids[1], func(_ *pb.Player, g *model.GameState) error {
		second := &pb.Warrior{Id: proto.String("hero_020"), Guid: proto.String("avatar-second"), Level: proto.Int32(1)}
		g.Warriors = append(g.Warriors, second)
		g.Formation = []*pb.FormationItem{
			{FormatPos: proto.Int32(2), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: proto.String("hero_020"), Guid: proto.String("avatar-second")}}},
			{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: proto.String("hero_047"), Guid: proto.String("missing-main")}}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	tx := db.GetIns().WithContext(f.ctx)
	member, err := p13GuildMember(tx, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.s.p13GuildPlayer(tx, ids[0], ids[1], member, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if info.GetHeroWarId() != "hero_020" {
		t.Fatal("first formation hero not used", info.GetHeroWarId())
	}
	if err = updateProfile(f.ctx, ids[1], func(_ *pb.Player, g *model.GameState) error {
		g.Formation = []*pb.FormationItem{
			{FormatPos: proto.Int32(2), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: proto.String("hero_020"), Guid: proto.String("ghost")}}},
			{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: proto.String("hero_047"), Guid: proto.String("main")}}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err = f.s.p13GuildPlayer(tx, ids[0], ids[1], member, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if info.GetHeroWarId() != "hero_047" {
		t.Fatal("invalid formation did not fall back to main hero", info.GetHeroWarId())
	}
}

// TestAdminGuildSpendableContribution31 patches only the available balance and rejects a stale wallet observation.
func TestAdminGuildSpendableContribution31(t *testing.T) {
	t.Setenv("HYJS_P13_MYSQL_DATABASE", "")
	f, ids := p13GuildSetup(t)
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	guildID := adminGuildTestCreate(t, f, ids[0], "可用贡献校验")
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	var profile db.Profile
	if err := db.GetIns().First(&profile, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(*profile.Player), &raw); err != nil {
		t.Fatal(err)
	}
	raw["contribution"] = json.RawMessage("20")
	raw["futureGuildWallet"] = json.RawMessage(`{"marker":"keep","amount":777}`)
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&profile).Update("player", string(encoded)).Error; err != nil {
		t.Fatal(err)
	}
	beforeGame := *profile.Game
	beforePlayer := raw
	detail, err := f.s.AdminGuildDetail(ctx, guildID)
	if err != nil {
		t.Fatal(err)
	}
	target := detail.Members[0]
	if target.SpendableContribution != 20 {
		t.Fatal("balance not displayed", target)
	}
	expected, value := int32(20), int32(7)
	req := model.AdminGuildMemberUpdateRequest{Version: target.Version, Reason: "分别设置累计与可用贡献", Contribution: 42, Role: target.Role, SpendableContribution: &value, ExpectedSpendableContribution: &expected}
	detail, err = f.s.AdminUpdateGuildMember(ctx, actor, guildID, ids[0], req)
	if err != nil {
		t.Fatal(err)
	}
	target = detail.Members[0]
	if target.Contribution != 42 || target.SpendableContribution != 7 {
		t.Fatal("contribution fields mixed", target)
	}
	if err = db.GetIns().First(&profile, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	var after map[string]json.RawMessage
	if err = json.Unmarshal([]byte(*profile.Player), &after); err != nil {
		t.Fatal(err)
	}
	if *profile.Game != beforeGame || len(after) != len(beforePlayer) {
		t.Fatal("unrelated profile shape changed")
	}
	for key, before := range beforePlayer {
		if key != "contribution" && string(after[key]) != string(before) {
			t.Fatalf("unrelated player field %s changed", key)
		}
	}
	var audit db.AdminAudit
	if err = db.GetIns().Where("action=?", "guild.member.update").Order("id DESC").First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	var detailAudit struct {
		OldSpendableContribution int32 `json:"oldSpendableContribution"`
		NewSpendableContribution int32 `json:"newSpendableContribution"`
	}
	if err = json.Unmarshal([]byte(audit.Detail), &detailAudit); err != nil || detailAudit.OldSpendableContribution != 20 || detailAudit.NewSpendableContribution != 7 {
		t.Fatal("available balance audit missing", audit.Detail, err)
	}
	// A live purchase changes the wallet without modifying the guild-member row version.
	after["contribution"] = json.RawMessage("6")
	encoded, err = json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&profile).Update("player", string(encoded)).Error; err != nil {
		t.Fatal(err)
	}
	expected = 7
	value = 3
	req.Version = target.Version
	req.Contribution = 99
	if _, err = f.s.AdminUpdateGuildMember(ctx, actor, guildID, ids[0], req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale available balance accepted", err)
	}
	detail, err = f.s.AdminGuildDetail(ctx, guildID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Members[0].Contribution != 42 || detail.Members[0].SpendableContribution != 6 {
		t.Fatal("conflicting mutation was committed", detail.Members[0])
	}
	// Legacy requests continue to update lifetime contribution without touching the wallet.
	req.SpendableContribution = nil
	req.ExpectedSpendableContribution = nil
	req.Contribution = 43
	detail, err = f.s.AdminUpdateGuildMember(ctx, actor, guildID, ids[0], req)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Members[0].Contribution != 43 || detail.Members[0].SpendableContribution != 6 {
		t.Fatal("legacy request changed wallet", detail.Members[0])
	}
}
