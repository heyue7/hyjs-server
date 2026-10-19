package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func TestPlayerDisplaySystemOnly(t *testing.T) {
	tabs := &pb.SCActivityList{ActivityList: []*pb.ActivityTab{{ActivityId: proto.String("ReID"), Name: proto.String("Re开工挑战")}}}
	info := &pb.SCActivityInfo{Information: proto.String("主线胜利累计1次，可领取Re开工挑战。")}
	player := &pb.Player{Name: proto.String("Re小白")}
	SanitizePlayerDisplay([]proto.Message{tabs, info, player})
	if tabs.GetActivityList()[0].GetName() != "开工挑战" || tabs.GetActivityList()[0].GetActivityId() != "ReID" || strings.Contains(info.GetInformation(), "Re") || player.GetName() != "Re小白" {
		t.Fatal("display scope", tabs, info, player)
	}
	if PlayerDisplayName("Reina") != "Reina" || PlayerDisplayName("Re · 招财猫") != "招财猫" || PlayerDisplayName("月光餐厅 · Re规则") != "月光餐厅" || playerDisplayText("月光餐厅 · Re规则") != "月光餐厅" {
		t.Fatal("name cleanup boundary")
	}
}

func TestAchievementPriorityRedPointFrozenClaim(t *testing.T) {
	f, _ := reAchievementFixture(t)
	if err := db.GetIns().Where("1=1").Delete(&db.P5Achievement{}).Error; err != nil {
		t.Fatal(err)
	}
	rules := []db.P5Achievement{{ID: "a_unreached", Name: "未达", Kind: "level", Type: 2, Target: 999, Reward: "coin,1", Source: "fixture"}, {ID: "z_ready", Name: "可领", Kind: "level", Type: 2, Target: 1, Reward: "coin,1", Source: "fixture"}}
	if err := db.GetIns().Create(&rules).Error; err != nil {
		t.Fatal(err)
	}
	points, err := f.s.WelfareAchievementRedPoints(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if welfarePoint(points, "achievement") != 1 {
		t.Fatal("earned achievement absent", points)
	}
	if p1Count(t, &db.P5AchievementClaim{}, "player_id=?", f.id) != 0 {
		t.Fatal("menu query froze achievement")
	}
	list, err := f.s.p5Achievements(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.SplitN(list[0].(*pb.SCAchievementList).List[0].GetId(), "~", 2)[0] != "z_ready" {
		t.Fatal("claimable achievement not first", list)
	}
	if _, err = f.s.p5AchievementReward(f.ctx, f.id, &pb.CSGetAchievementReward{Id: proto.String("z_ready")}, 93222); err != nil {
		t.Fatal(err)
	}
	points, err = f.s.WelfareAchievementRedPoints(f.ctx, f.id)
	if err != nil || welfarePoint(points, "achievement") != 0 {
		t.Fatal("claimed redpoint retained", points, err)
	}
	list, err = f.s.p5Achievements(f.ctx, f.id)
	if err != nil || list[0].(*pb.SCAchievementList).List[1].GetState() != 2 {
		t.Fatal("claimed bucket", list, err)
	}
}
func welfarePoint(points *pb.SCRedPoint, key string) int32 {
	for _, p := range points.GetRedPointList() {
		if p.GetType() == key {
			return p.GetValue()
		}
	}
	return -1
}

func TestWelfareRedPointContinuedAndNoPaidCat(t *testing.T) {
	f, actor := adminModesFixture(t)
	at := time.Now()
	_, key := reWelfarePublish(t, f, actor, "continued-spend", at)
	reWelfarePublish(t, f, actor, "fortune-cat", at)
	points, err := f.s.WelfareAchievementRedPoints(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if welfarePoint(points, "continuedRecharge") != 0 || welfarePoint(points, "fortuneCat") == 1 {
		t.Fatal("unearned or paid action red", points)
	}
	// A committed server cost event is the authoritative same-day fact for this representative.
	if err = f.s.RecordP1Cost(db.GetIns().WithContext(f.ctx), f.id, "native-test-redpoint", []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(3000)}}, at); err != nil {
		t.Fatal(err)
	}
	points, err = f.s.WelfareAchievementRedPoints(f.ctx, f.id)
	if err != nil || welfarePoint(points, "continuedRecharge") != 1 {
		t.Fatal("real consumer progress absent", points, err)
	}
	if p1Count(t, &db.P4WelfareState{}, "player_id=? AND rule_id=?", f.id, key) != 0 {
		t.Fatal("menu froze reward")
	}
}

func TestWelfareRedPointGuideEarnedThenCleared(t *testing.T) {
	f, _ := adminModesFixture(t)
	cfg := p1Instance(t, f, "kaiGongActivity_p4_feedback", constUtil.P1EventCopyWin, "cumulative", 1, time.Now().Add(time.Hour), 0)
	if err := f.s.RecordP1Event(db.GetIns().WithContext(f.ctx), f.id, "main-copy-committed:1", constUtil.P1EventCopyWin, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	beforeDeliveries := p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", f.id, "activity:"+cfg.ID)
	points, err := f.s.WelfareAchievementRedPoints(f.ctx, f.id)
	if err != nil || welfarePoint(points, "kaiGongActivity") != 1 {
		t.Fatal("guide completed red absent", points, err)
	}
	// Read-only menu polling must not create a reward receipt or freeze another contract.
	if p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", f.id, "activity:"+cfg.ID) != beforeDeliveries {
		t.Fatal("poll generated delivery")
	}
	out, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
	if err != nil {
		t.Fatal(err)
	}
	item := out[0].(*pb.SCActivityInfo).GetItemList()[0]
	if _, err = f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(cfg.ID), Index: item.Index, GetResources: item.RewardList}); err != nil {
		t.Fatal(err)
	}
	points, err = f.s.WelfareAchievementRedPoints(f.ctx, f.id)
	if err != nil || welfarePoint(points, "kaiGongActivity") != 0 {
		t.Fatal("claimed guide red retained", points, err)
	}
}
