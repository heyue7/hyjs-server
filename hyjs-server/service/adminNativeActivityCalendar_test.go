package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminNativePaymentManagedCalendar verifies that a saved native payment rule remains
// reachable after calendar adoption, while untouched placeholders stay closed.
func TestAdminNativePaymentManagedCalendar(t *testing.T) {
	f, actor := adminNativeFixture(t, true)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version}); err != nil {
		t.Fatal(err)
	}
	row := adminNativeFind(t, f, actor, "payment-total")
	key := strings.TrimPrefix(row.ID, "payment:")
	if allowed, err := localPlayActivityAccess(db.GetIns().WithContext(f.ctx), key, false); err != nil || allowed {
		t.Fatal("untouched legacy payment escaped managed calendar", allowed, err)
	}
	now := time.Now()
	row.Form.StartAt = now.Add(-time.Hour).UnixMilli()
	row.Form.EndAt = now.Add(time.Hour).UnixMilli()
	row.Form.ClaimEndAt = row.Form.EndAt
	row.Form.Threshold = 1
	row.Form.Rewards = []model.AdminResourceAmount{{ResourceID: "coin", Amount: 19}}
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, actor, row.ID, model.AdminNativeActivitySaveRequest{Version: row.Version, Enabled: true, Alias: "仅后台累计充值别名", Form: row.Form})
	if err != nil {
		t.Fatal(err)
	}
	tabs, err := f.s.PaymentActivityTabs(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tab := range tabs {
		found = found || tab.GetActivityId() == key
	}
	if !found {
		t.Fatal("saved native payment missing from real activity tabs")
	}
	playerID := adminNativeNewPlayer(t, f, "calendar-payment", 0, 0)
	var product db.PaymentProduct
	if err = db.GetIns().Where("kind=?", "once").First(&product).Error; err != nil {
		t.Fatal(err)
	}
	adminNativeConfirmedOrder(t, f, playerID, product.ID, 100)
	if _, err = f.s.paymentActivity(f.ctx, playerID, &pb.CSActivityInfo{ActivityId: proto.String(key)}, time.Now()); err != nil {
		t.Fatal("native payment consumer blocked by calendar", err)
	}
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, actor, row.ID, model.AdminNativeActivitySaveRequest{Version: saved.Version, Enabled: false, Form: saved.Form}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.paymentActivity(f.ctx, playerID, &pb.CSActivityInfo{ActivityId: proto.String(key)}, time.Now()); err == nil {
		t.Fatal("stopped native payment reopened from old enabled snapshot")
	}
}
