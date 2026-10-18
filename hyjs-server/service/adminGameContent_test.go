package service

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestAdminContentFrozenClaims checks original UI types, explicit payout and immutable old contracts.
func TestAdminContentFrozenClaims(t *testing.T) {
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
	viewer := adminWalletTestActor(t, f, constUtil.AdminRoleViewer)
	req := model.AdminAchievementDTO{Name: "隔离验证等级成就", Kind: "level", Target: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}, Source: "isolated test fixture only", Enabled: true}
	if _, err := f.s.AdminSaveAchievement(f.ctx, viewer, "", req, true, false); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer changed achievement", err)
	}
	created, err := f.s.AdminSaveAchievement(f.ctx, actor, "", req, true, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.s.p5Achievements(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range out[0].(*pb.SCAchievementList).GetList() {
		if a.GetId() == created.ID {
			found = true
			if a.GetType() != 2 || a.GetState() != 1 || a.GetReward() != "coin,7" {
				t.Fatal("wrong original achievement projection", a)
			}
		}
	}
	if !found {
		t.Fatal("created rule missing from actual game")
	}
	req = *created
	req.Target = 999
	req.Rewards[0].Amount = 999
	changed, err := f.s.AdminSaveAchievement(f.ctx, actor, created.ID, req, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminSaveAchievement(f.ctx, actor, created.ID, req, false, false); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale rule overwrote live", err)
	}
	archived, err := f.s.AdminSaveAchievement(f.ctx, actor, created.ID, *changed, false, true)
	if err != nil || archived.Enabled {
		t.Fatal("archive", err)
	}
	before, _ := f.profile(t)
	claim := &pb.CSGetAchievementReward{Id: proto.String(created.ID)}
	if _, err = f.s.p5AchievementReward(f.ctx, f.id, claim, 123001); err != nil {
		t.Fatal("frozen archived claim denied", err)
	}
	if _, err = f.s.p5AchievementReward(f.ctx, f.id, claim, 123001); err != nil {
		t.Fatal("claim retry", err)
	}
	after, _ := f.profile(t)
	if after.GetCoin()-before.GetCoin() != 7 {
		t.Fatal("frozen payout was changed or duplicated", after.GetCoin()-before.GetCoin())
	}
	titles, err := f.s.AdminTitles(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var target model.AdminTitleDTO
	for _, v := range titles.Items {
		if v.Type == 9 {
			target = v
			break
		}
	}
	if target.ID == "" {
		t.Fatal("original level title missing")
	}
	target.Param = "1"
	target.Enabled = true
	target.Show = true
	target.EffectTime = 0
	target.Effects = []model.AdminTitleEffect{{Key: "att", Value: 2}}
	saved, err := f.s.AdminSaveTitle(f.ctx, actor, target.ID, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p5Titles(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	_, g := f.profile(t)
	frozen := p5KV(g, "p5.title:"+target.ID)
	if frozen == "" {
		t.Fatal("future title config not consumed")
	}
	changedTitle := *saved
	changedTitle.Enabled = false
	changedTitle.Effects[0].Value = 999
	if _, err = f.s.AdminSaveTitle(f.ctx, actor, target.ID, changedTitle); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p5Titles(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	_, g = f.profile(t)
	if p5KV(g, "p5.title:"+target.ID) != frozen {
		t.Fatal("owned title snapshot rewritten")
	}
	// A global day change expires actual title bonuses at the same time as the original title page.
	clockGame := &model.GameState{}
	p5SetKV(clockGame, "p5.current-title", target.ID)
	timed, _ := json.Marshal(p5TitleState{Effects: "att,2", DeadTime: time.Now().Add(time.Hour).UnixMilli(), Source: "isolated timed title fixture"})
	p5SetKV(clockGame, "p5.title:"+target.ID, string(timed))
	baseValues := map[string]float64{}
	if err = f.s.p5TitleBonus(clockGame, baseValues); err != nil || baseValues["att"] != 2 {
		t.Fatal("active timed title", err)
	}
	f.s.ServerClockReady = true
	f.s.ServerClockState.Store(&model.ServerGameClockView{OffsetDays: 1})
	expiredValues := map[string]float64{}
	if err = f.s.p5TitleBonus(clockGame, expiredValues); err != nil || expiredValues["att"] != 0 {
		t.Fatal("server date did not expire real title effect", err)
	}
	f.s.ServerClockReady = false
	f.s.ServerClockState.Store(nil)
	f.restart(t)
	if _, err = f.s.p5Achievements(f.ctx, f.id); err != nil {
		t.Fatal("cold achievement", err)
	}
	if count := p1Count(t, &db.P5AchievementClaim{}, "player_id=? AND id=? AND claimed_at>0", f.id, created.ID); count != 1 {
		t.Fatal("claim history lost", count)
	}
}

// TestGameFeedbackOwnerHistory checks original-session ownership at service boundary and cold idempotent replies.
func TestGameFeedbackOwnerHistory(t *testing.T) {
	adminWalletTestEnvironment(t)
	f := p1Setup(t)
	if err := InitializeGameFeedback(f.ctx); err != nil {
		t.Fatal(err)
	}
	operator := adminWalletTestActor(t, f, constUtil.AdminRoleOperator)
	viewer := adminWalletTestActor(t, f, constUtil.AdminRoleViewer)
	req := model.GameFeedbackRequest{RequestID: uuid.NewString(), Category: "bug", Subject: "隔离客服反馈", Content: "原输入内容"}
	created, err := GameFeedbackCreate(f.ctx, f.id, req)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := GameFeedbackCreate(f.ctx, f.id, req)
	if err != nil || retry.ID != created.ID || len(retry.Messages) != 1 {
		t.Fatal("create retry", err)
	}
	req.Content = "different"
	if _, err = GameFeedbackCreate(f.ctx, f.id, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("changed retry accepted", err)
	}
	other, err := LoginAccount(f.ctx, "feedback-"+uuid.NewString(), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = GameFeedbackDetail(f.ctx, other.Id, created.ID); err == nil {
		t.Fatal("cross-account detail leaked")
	}
	r := model.AdminFeedbackReplyRequest{Version: created.Version, Content: "已经收到，正在处理", RequestID: uuid.NewString()}
	if _, err = AdminFeedbackReply(f.ctx, viewer, created.ID, r); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer replied", err)
	}
	replied, err := AdminFeedbackReply(f.ctx, operator, created.ID, r)
	if err != nil || replied.State != "replied" || len(replied.Messages) != 2 {
		t.Fatal("support reply", err)
	}
	same, err := AdminFeedbackReply(f.ctx, operator, created.ID, r)
	if err != nil || len(same.Messages) != 2 {
		t.Fatal("stale-version reply retry duplicated", err)
	}
	if _, err = AdminFeedbackState(f.ctx, operator, created.ID, model.AdminFeedbackStateRequest{Version: created.Version, State: "closed"}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale status accepted", err)
	}
	closed, err := AdminFeedbackState(f.ctx, operator, created.ID, model.AdminFeedbackStateRequest{Version: replied.Version, State: "closed"})
	if err != nil || closed.State != "closed" {
		t.Fatal(err)
	}
	follow, err := GameFeedbackReply(f.ctx, f.id, model.GameFeedbackRequest{ID: created.ID, RequestID: uuid.NewString(), Content: "还有补充"})
	if err != nil || follow.State != "open" || len(follow.Messages) != 3 {
		t.Fatal("player followup", err)
	}
	f.restart(t)
	cold, err := GameFeedbackDetail(f.ctx, f.id, created.ID)
	if err != nil || len(cold.Messages) != 3 || cold.Messages[1].Sender != "support" {
		t.Fatal("cold history", err)
	}
	own, err := GameFeedbackList(f.ctx, f.id, 1, 20)
	if err != nil || own.Total != 1 {
		t.Fatal("owner list", err)
	}
	others, err := GameFeedbackList(f.ctx, other.Id, 1, 20)
	if err != nil || others.Total != 0 {
		t.Fatal("cross-account list", err)
	}
	if count := p1Count(t, &db.AdminAudit{}, "target=? AND action=?", created.ID, "feedback.reply"); count != 1 {
		t.Fatal("support retry duplicated audit", count)
	}
}

// TestAdminOriginalAchievementConditions checks the APK's 36 visible groups and authoritative representative sources.
// Pet histories below are finite accepted-storage fixtures, not proof of executing their original acquisition UI.
func TestAdminOriginalAchievementConditions(t *testing.T) {
	adminWalletTestEnvironment(t)
	f := p1Setup(t)
	if err := InitializeAdminGameContent(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleOperator)
	kinds := AdminAchievementKinds()
	if len(kinds) != 36 {
		t.Fatal("original category count", len(kinds))
	}
	supported := 0
	seen := map[int32]bool{}
	for _, kind := range kinds {
		if seen[kind.Type] || kind.Type < 1 || kind.Type > 36 || kind.Notes == "" {
			t.Fatal("invalid original group", kind)
		}
		seen[kind.Type] = true
		if kind.Supported {
			supported++
		}
	}
	if supported != 22 {
		t.Fatal("invented or missing available progress consumer", supported)
	}
	unsupported := model.AdminAchievementDTO{Name: "未恢复地图隔离草稿", Kind: "awake-5-3", Target: 100, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 1}}, Source: "isolated unsupported condition test", Enabled: true}
	if _, err := f.s.AdminSaveAchievement(f.ctx, actor, "", unsupported, true, false); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("unsupported scenario enabled", err)
	}
	unsupported.Enabled = false
	unsupported.Rewards = nil
	draft, err := f.s.AdminSaveAchievement(f.ctx, actor, "", unsupported, true, false)
	if err != nil || draft.Supported || draft.Enabled {
		t.Fatal("unsupported draft not identified", err)
	}
	if err = db.GetIns().AutoMigrate(&db.P7RushState{}, &db.P9PetInstance{}, &db.P9PetRetired{}, &db.P9PetInvestment{}); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Create(&db.P7RushState{PlayerID: f.id, Day: "2026-10-18", RunID: 1, Floor: 5, HighFloor: 9, TotalStars: 12, RecordStars: 17}).Error; err != nil {
		t.Fatal(err)
	}
	player, g := f.profile(t)
	stars, err := f.s.p5AchievementValue(db.GetIns().WithContext(f.ctx), player, g, &db.P5Achievement{Kind: "trial"})
	if err != nil || stars != 17 {
		t.Fatal("trial used floor instead of original record stars", stars, err)
	}
	g.Copy = &pb.SCCopyPassedInfo{PassedCopyInfo: []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(1)}, {ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(3)}, {ChapterId: proto.String("1"), MissionId: proto.String("2"), Star: proto.Int32(2)}}}
	stars, err = f.s.p5AchievementValue(db.GetIns().WithContext(f.ctx), player, g, &db.P5Achievement{Kind: "copy-stars"})
	if err != nil || stars != 5 {
		t.Fatal("main stars duplicated a node", stars, err)
	}
	if err = f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	pet := &pb.Pet{Guid: proto.String(uuid.NewString()), Id: proto.String("pet_001"), Level: proto.Int32(3), Breached: &pb.PetBreached{Level: proto.Int32(1)}, Eat: &pb.PetEat{Level: proto.Int32(2)}}
	raw, _ := proto.Marshal(pet)
	if err = db.GetIns().Create(&db.P9PetInstance{GUID: pet.GetGuid(), PlayerID: f.id, PetID: pet.GetId(), Data: raw, Source: "finite active fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	pet.Level = proto.Int32(8)
	pet.Breached.Level = proto.Int32(2)
	pet.Eat.Level = proto.Int32(3)
	pet.TalentList = []*pb.PetTalentSkill{{Talent: proto.String("PetTalent_11"), TalentLevel: proto.Int32(0)}, {Talent: proto.String("PetTalent_12"), TalentLevel: proto.Int32(2)}, {Talent: proto.String("PetTalent_13"), TalentLevel: proto.Int32(3)}}
	raw, _ = proto.Marshal(pet)
	if err = db.GetIns().Create(&db.P9PetRetired{GUID: pet.GetGuid(), PlayerID: f.id, PetID: pet.GetId(), Data: raw, Operation: "finite-reset-history", Source: "finite retired fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	family := f.s.table("Pet", "id", "pet_001")
	var otherID string
	for _, row := range f.s.Tables["Pet"] {
		if row["resName"] == family["resName"] && row["nature"] != family["nature"] {
			otherID = row["id"]
			break
		}
	}
	if otherID == "" {
		t.Fatal("original model element variant missing")
	}
	donor := &pb.Pet{Guid: proto.String(uuid.NewString()), Id: proto.String(otherID), Level: proto.Int32(1)}
	donors, _ := json.Marshal([]*pb.Pet{donor})
	if err = db.GetIns().Create(&db.P9PetInvestment{GUID: pet.GetGuid(), PlayerID: f.id, Costs: []byte("{}"), Donors: donors, Source: "finite donor history"}).Error; err != nil {
		t.Fatal(err)
	}
	for kind, want := range map[string]int32{"pet-count": 2, "pet-elements": 2, "pet-level": 8, "pet-breach": 2, "pet-intimacy": 3, "pet-talents": 2} {
		value, err := f.s.p5AchievementValue(db.GetIns().WithContext(f.ctx), player, g, &db.P5Achievement{Kind: kind})
		if err != nil || value != want {
			t.Fatal("native pet source projection", kind, value, want, err)
		}
	}
}
