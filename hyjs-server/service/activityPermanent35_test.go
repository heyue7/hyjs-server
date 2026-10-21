package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestActivityPermanent35SourceAndHistory 验证旧日期不限制常驻入口、单一橙池读写和真正领取幂等。
func TestActivityPermanent35SourceAndHistory(t *testing.T) {
	f, actor := adminNativeFixture(t, true)
	if err := f.s.InitializeFirstDouble(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP2(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeKingRecruitWorlds(f.ctx); err != nil {
		t.Fatal(err)
	}
	// 保留一个已经结束的旧橙卡轮换，它不能再占列表或覆盖常驻池。
	old := db.P4RecruitRotation{ID: "p4:recruit:old35", Kind: 2, Enabled: true, StartAt: 1, EndAt: 2, Rewards: []byte("[]"), Frozen: []byte("[]")}
	if err := db.GetIns().Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if err := f.s.InitializeRecruitBalance35(f.ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("configuration migration changed player")
	}
	rows, err := f.s.AdminNativeActivities(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	var orange model.AdminNativeActivityDTO
	for _, row := range rows {
		if row.Kind == "recruit-rotation" {
			count++
			orange = row
		}
	}
	if count != 1 || orange.ID != "recruit:permanent" || !orange.Permanent || !orange.Enabled || orange.Frozen {
		t.Fatal("not a single active permanent source", count, orange.ID)
	}
	quality := map[string]int{}
	for _, prize := range orange.Form.Prizes {
		q := f.s.table("Hero", "id", prize.ResourceID)["quality"]
		quality[q]++
		if prize.Guaranteed != (q == "20") {
			t.Fatal("orange guarantee classification", q)
		}
	}
	for _, q := range []string{"12", "13", "18", "20"} {
		if quality[q] == 0 {
			t.Fatal("missing quality", q)
		}
	}
	// 保存真正更新基础池并能被原costType2信息和抽取读取；旧轮换不再影响。
	orange.Form.Prizes[0].Weight++
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, actor, orange.ID, model.AdminNativeActivitySaveRequest{Version: orange.Version, Enabled: true, Form: orange.Form})
	if err != nil {
		t.Fatal(err)
	}
	actual, end, err := f.s.loadP4Rotation(db.GetIns().WithContext(f.ctx), 2, time.Now().AddDate(10, 0, 0))
	if err != nil || end != 0 || actual[0].Weight != saved.Form.Prizes[0].Weight {
		t.Fatal("permanent source disconnected", end, err)
	}
	if _, err = f.s.recruitInfo(f.ctx, f.id, &pb.CSWarriorRecruitGet{CostType: []int32{2}}); err != nil {
		t.Fatal(err)
	}
	// 基金和首充旧日期只保留为历史，过期后仍能进入；限时支付类型继续按原窗口。
	if err = db.GetIns().Model(&db.ActivityRules{}).Where("id=?", "open_server_fund").Updates(map[string]any{"start_at": int64(1), "end_at": int64(2), "reward_end_at": int64(3)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.openServerFund(f.ctx, f.id, &pb.CSOpenServerFundInfo{}); err != nil {
		t.Fatal("permanent fund expired", err)
	}
	first := adminNativeFind(t, f, actor, "payment-first")
	first.Form.StartAt = 1
	first.Form.EndAt = 2
	first.Form.ClaimEndAt = 3
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, actor, first.ID, model.AdminNativeActivitySaveRequest{Version: first.Version, Enabled: true, Form: first.Form}); err != nil {
		t.Fatal(err)
	}
	key := first.ID[len("payment:"):]
	id := adminNativeNewPlayer(t, f, "permanent35", 1, 100)
	var product db.PaymentProduct
	if err = db.GetIns().Where("kind=?", "once").First(&product).Error; err != nil {
		t.Fatal(err)
	}
	adminNativeConfirmedOrder(t, f, id, product.ID, product.Cents)
	if _, err = f.s.paymentActivity(f.ctx, id, &pb.CSActivityInfo{ActivityId: proto.String(key)}, time.Now()); err != nil {
		t.Fatal("permanent first gift expired", err)
	}
	if _, err = f.s.paymentActivity(f.ctx, id, &pb.CSActivityReward{ActivityId: proto.String(key), Index: proto.Int32(1)}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.paymentActivity(f.ctx, id, &pb.CSActivityReward{ActivityId: proto.String(key), Index: proto.Int32(1)}, time.Now()); err == nil {
		t.Fatal("first gift claimed twice")
	}
	// 不修改原SKU或终身订单，开启双倍后只影响尚未使用的SKU。
	double := adminNativeFind(t, f, actor, "first-double")
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, actor, double.ID, model.AdminNativeActivitySaveRequest{Version: double.Version, Enabled: true, Form: double.Form}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(product)
	if !f.s.firstDoubleOriginalSKU(&product) {
		t.Fatal("fixture missing original SKU")
	}
	if err = f.s.firstDoubleProduct(db.GetIns().WithContext(f.ctx), id, &product); err != nil || product.FirstDouble {
		t.Fatal("confirmed lifetime SKU regained double", err)
	}
	var check db.PaymentProduct
	_ = json.Unmarshal(raw, &check)
	if err = f.s.firstDoubleProduct(db.GetIns().WithContext(f.ctx), f.id, &check); err != nil || !check.FirstDouble {
		t.Fatal("new SKU double not active", err)
	}
	var oldAfter db.P4RecruitRotation
	if err = db.GetIns().First(&oldAfter, "id=?", old.ID).Error; err != nil || string(oldAfter.Frozen) != string(old.Frozen) {
		t.Fatal("legacy rotation history changed", err)
	}
	var rules []db.RecruitRuntimeRule
	db.GetIns().Order("kind").Find(&rules)
	if len(rules) != 2 || rules[0].GuaranteeEvery != constUtil.RecruitBalance35GuaranteeEvery {
		t.Fatal("runtime initialization")
	}
}
