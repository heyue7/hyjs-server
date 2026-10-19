package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestPeriodMilestonesOriginalClaimsFreezeAndCold(t *testing.T) {
	f, operator := adminModesFixture(t)
	form := adminModeForm("diamond-spent")
	form.Milestones = []model.AdminActivityMilestone{{Goal: 10, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 100}}}, {Goal: 20, Rewards: []model.AdminResourceAmount{{ResourceID: "item_002", Amount: 2}}}, {Goal: 30, Rewards: []model.AdminResourceAmount{{ResourceID: "diamond", Amount: 3}}}}
	created, err := f.s.AdminSaveTypedActivity(f.ctx, operator, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_, key, _ := strings.Cut(created.ID, ":")
	query := func() *pb.SCActivityInfo {
		t.Helper()
		out, e := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)})
		if e != nil {
			t.Fatal(e)
		}
		return out[0].(*pb.SCActivityInfo)
	}
	if len(query().ItemList) != 3 {
		t.Fatal("missing ordered tiers")
	}
	// The finite fixture records an accepted spend fact; queries do not advance it.
	f.event(t, "multi-spend-first", constUtil.P1EventDiamondSpent, 20, time.Now())
	items := query().ItemList
	if items[0].GetStatus() != 1 || items[1].GetStatus() != 1 || items[2].GetStatus() != 0 {
		t.Fatal("threshold eligibility", items)
	}
	request := &pb.CSActivityReward{ActivityId: proto.String(key), Index: proto.Int32(2), GetResources: items[1].RewardList}
	if _, err = f.s.P1Activity(f.ctx, f.id, request); err != nil {
		t.Fatal(err)
	}
	var frozen db.PeriodInstance
	if err = db.GetIns().First(&frozen, "id=?", key).Error; err != nil || len(frozen.Frozen) == 0 {
		t.Fatal("not frozen", err)
	}
	if err = db.GetIns().Model(&frozen).Updates(map[string]any{"milestones": []byte(`[{"goal":1,"rewards":"coin,999"}]`)}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if _, err = f.s.P1Activity(f.ctx, f.id, request); err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_002") != 2 || len(query().ItemList) != 3 {
		t.Fatal("cold duplicated or replaced frozen tiers")
	}
	f.event(t, "multi-spend-first", constUtil.P1EventDiamondSpent, 20, time.Now())
	if query().ItemList[2].GetStatus() != 0 {
		t.Fatal("replayed fact counted twice")
	}
	f.event(t, "multi-spend-next", constUtil.P1EventDiamondSpent, 10, time.Now())
	if query().ItemList[2].GetStatus() != 1 {
		t.Fatal("last tier not earned")
	}
}

func TestReActivityCatalogueFullTiersAndCleanupIdempotence(t *testing.T) {
	f, _ := adminModesFixture(t)
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	var before db.PeriodInstance
	if err := db.GetIns().Where("source NOT LIKE ?", "%Re v1.0.5.1%").First(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&before).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeReActivityCatalogue(f.ctx); err != nil {
		t.Fatal(err)
	}
	forms := f.s.reActivityForms()
	if len(forms) != 10 {
		t.Fatal("missing original type, sign or work", len(forms))
	}
	for _, form := range forms {
		if len(form.Milestones) < 5 {
			t.Fatal("original configuration truncated", form.Mode)
		}
		var row db.AdminActivityDefinition
		if err := db.GetIns().First(&row, "id=?", "re1051_"+strings.ReplaceAll(form.Mode, "-", "_")).Error; err != nil || !row.Enabled || row.Archived {
			t.Fatal("missing enabled original", form.Mode, err)
		}
	}
	var old db.PeriodInstance
	if err := db.GetIns().First(&old, "id=?", before.ID).Error; err != nil {
		t.Fatal("deleted historical consumer", err)
	}
	if old.Enabled {
		t.Fatal("unverified activity still enabled")
	}
	var first []db.AdminActivityOccurrence
	db.GetIns().Order("id").Find(&first)
	firstJSON, _ := json.Marshal(first)
	if err := f.s.InitializeReActivityCatalogue(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	var second []db.AdminActivityOccurrence
	db.GetIns().Order("id").Find(&second)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("restart recreated or changed issued contracts")
	}
}

func TestPeriodMilestonesLoginDirectoryStableIndices(t *testing.T) {
	f, operator := adminModesFixture(t)
	form := adminModeForm("login-total")
	form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 10}}}, {Goal: 2, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 20}}}}
	if _, err := f.s.AdminSaveTypedActivity(f.ctx, operator, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.event(t, "directory-two-days", constUtil.P1EventLoginDay, 2, time.Now())
	out, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.P1LoginDirectoryID)})
	if err != nil {
		t.Fatal(err)
	}
	items := out[0].(*pb.SCActivityInfo).ItemList
	var second *pb.ActivityItem
	for _, item := range items {
		if item.GetTotal() == 2 && len(item.RewardList) == 1 && item.RewardList[0].GetNum() == 20 {
			second = item
		}
	}
	if second == nil || second.GetStatus() != 1 {
		t.Fatal("second tier missing from login directory")
	}
	claim := &pb.CSActivityReward{ActivityId: proto.String(constUtil.P1LoginDirectoryID), Index: second.Index, GetResources: second.RewardList}
	if _, err = f.s.P1Activity(f.ctx, f.id, claim); err != nil {
		t.Fatal("directory child index", err)
	}
	f.restart(t)
	if _, err = f.s.P1Activity(f.ctx, f.id, claim); err != nil {
		t.Fatal(err)
	}
	p, _ := f.profile(t)
	if p.GetCoin() != 20 {
		t.Fatal("directory replay duplicated grant", p.GetCoin())
	}
}

func TestReCatalogueCleanupLegacyAndFutureSeedsRemainClosed(t *testing.T) {
	f, _ := adminModesFixture(t)
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.ActivityRules{}); err != nil {
		t.Fatal(err)
	}
	legacy := db.ActivityRules{ID: constUtil.P1LegacyLoginID, Enabled: true, Source: "finite legacy cleanup fixture"}
	if err := db.GetIns().Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P15SeasonalBundle{}).Where("1=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeReActivityCatalogue(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&legacy, "id=?", legacy.ID).Error; err != nil || legacy.Enabled {
		t.Fatal("legacy rule still open", err)
	}
	var enabledBundles int64
	if err := db.GetIns().Model(&db.P15SeasonalBundle{}).Where("enabled=?", true).Count(&enabledBundles).Error; err != nil || enabledBundles != 0 {
		t.Fatal("legacy wrapper still open", err, enabledBundles)
	}
	// Cross a real calendar-month boundary through the normal seed functions.
	future := time.Now().In(time.FixedZone("UTC+8", 8*3600)).AddDate(0, 1, 0)
	if err := f.s.seedP1Periods(f.ctx, future); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []func(*gorm.DB, time.Time) error{f.s.seedP4Periods, f.s.seedP4Commerce, f.s.seedP4Welfare} {
		if err := seed(db.GetIns().WithContext(f.ctx), future); err != nil {
			t.Fatal(err)
		}
	}
	boundary := time.Date(future.Year(), future.Month(), 1, 0, 0, 0, 0, future.Location()).UnixMilli()
	for _, table := range []any{&db.PeriodInstance{}, &db.P4CommerceRule{}, &db.P4WelfareRule{}, &db.P15SeasonalBundle{}} {
		var count int64
		if err := db.GetIns().Model(table).Where("start_at>=? AND enabled=?", boundary, true).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("future placeholder reopened", table, count, err)
		}
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	var imported int64
	if err := db.GetIns().Model(&db.AdminActivityDefinition{}).Where("archived=? AND source_activity_id LIKE ?", false, "%"+future.Format("200601")+"%").Count(&imported).Error; err != nil || imported != 0 {
		t.Fatal("future placeholder reimported", imported, err)
	}
}

func TestReCatalogueMultipleRealmsDoNotReimportIssuedConsumers(t *testing.T) {
	f, _ := adminModesFixture(t)
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeReActivityCatalogue(f.ctx); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := db.GetIns().Model(&db.AdminActivityDefinition{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	second := &GameService{ServerID: 2, Tables: f.s.Tables, P1Ready: true, P4Ready: true}
	if err := second.InitializeReActivityCatalogue(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	var after int64
	if err := db.GetIns().Model(&db.AdminActivityDefinition{}).Count(&after).Error; err != nil || before != after {
		t.Fatal("issued per-realm consumers were reimported", before, after, err)
	}
	for _, serverID := range []int64{1, 2} {
		var count int64
		if err := db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("server_id=? AND id LIKE ? AND enabled=?", serverID, "restore_re1051_%", true).Count(&count).Error; err != nil || count != 12 {
			t.Fatal("missing realm configuration", serverID, count, err)
		}
	}
}
