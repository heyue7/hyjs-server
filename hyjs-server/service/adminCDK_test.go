package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAdminCDKOriginalRedemptionConcurrencyAndCold uses the original client
// request and claim ledger, including a shared last-code race between roles.
func TestAdminCDKOriginalRedemptionConcurrencyAndCold(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if e := f.s.LoadPaymentTables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	for _, init := range []func(context.Context) error{f.s.InitializePayments, f.s.InitializeAdminGameServers, f.s.InitializeAdminCDK} {
		if e := init(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	f.ctx = context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, f.s.OriginalServerID())
	other, e := LoginAccount(f.ctx, "cdk-other-"+t.Name(), "fixture-password")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = CreatePlayer(f.ctx, other.Id, "兑换测试乙", "hero_043"); e != nil {
		t.Fatal(e)
	}
	original, _ := f.profile(t)
	before := original.GetCoin()
	req := model.AdminCDKCreateRequest{Name: "原协议测试码", Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}, Count: 2, TotalLimit: 1, Unlimited: true, Enabled: true, Reason: "finite isolated original protocol verification"}
	generated, e := f.s.AdminCreateCDK(f.ctx, actor, req)
	if e != nil || len(generated.Codes) != 2 {
		t.Fatal(e, generated)
	}
	code := generated.Codes[0]
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []int64{f.id, other.Id} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, err := f.s.paymentOffer(f.ctx, id, &pb.CSConvertGiftCode{GiftCode: proto.String(strings.ToLower(code))}, time.Now())
			results <- err
		}(id)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("shared quota overspent", wins)
	}
	var stored db.PaymentGiftCode
	if e = db.GetIns().First(&stored, "hash=?", generated.Items[0].ID).Error; e != nil || stored.Used != 1 {
		t.Fatal(e, stored.Used)
	}
	if count := p1Count(t, &db.MembershipClaim{}, "kind=?", "gift:"+stored.Hash); count != 1 {
		t.Fatal("claim ledger duplicated", count)
	}
	for _, id := range []int64{f.id, other.Id} {
		if _, e = f.s.paymentOffer(f.ctx, id, &pb.CSConvertGiftCode{GiftCode: proto.String(code)}, time.Now()); e == nil {
			t.Fatal("exhausted code granted again")
		}
	}
	page, e := f.s.AdminCDKList(f.ctx, actor, "", 1, 20)
	if e != nil || page.Total != 2 {
		t.Fatal(e, page)
	}
	used := page.Items[0]
	for _, v := range page.Items {
		if v.ID == stored.Hash {
			used = v
		}
	}
	update := model.AdminCDKUpdateRequest{Name: used.Name, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 8}}, TotalLimit: 1, Unlimited: true, Enabled: true, Version: used.Version, Reason: "reject used reward edit"}
	if _, e = f.s.AdminUpdateCDK(f.ctx, actor, used.ID, update); e == nil {
		t.Fatal("used reward was rewritten")
	}
	// The unused second code can be disabled and scheduled without touching the redeemed code.
	var fresh model.AdminCDKDTO
	for _, v := range page.Items {
		if v.ID != stored.Hash {
			fresh = v
		}
	}
	update.Rewards = fresh.Rewards
	update.Name = fresh.Name
	update.Version = fresh.Version
	update.Enabled = false
	closed, e := f.s.AdminUpdateCDK(f.ctx, actor, fresh.ID, update)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.paymentOffer(f.ctx, f.id, &pb.CSConvertGiftCode{GiftCode: proto.String(generated.Codes[1])}, time.Now()); e == nil {
		t.Fatal("disabled code redeemed")
	}
	update.Version = closed.Version
	update.Enabled = true
	update.Unlimited = false
	update.StartAt = time.Now().Add(time.Hour).UnixMilli()
	update.EndAt = time.Now().Add(2 * time.Hour).UnixMilli()
	timed, e := f.s.AdminUpdateCDK(f.ctx, actor, fresh.ID, update)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.paymentOffer(f.ctx, f.id, &pb.CSConvertGiftCode{GiftCode: proto.String(generated.Codes[1])}, time.Now()); e == nil {
		t.Fatal("future code redeemed early")
	}
	update.Version = timed.Version
	update.Unlimited = true
	update.StartAt = 0
	update.EndAt = 0
	if _, e = f.s.AdminUpdateCDK(f.ctx, actor, fresh.ID, update); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.paymentOffer(f.ctx, f.id, &pb.CSConvertGiftCode{GiftCode: proto.String(generated.Codes[1])}, time.Now()); e != nil {
		t.Fatal("original frontend redemption", e)
	}
	after, _ := f.profile(t)
	if after.GetCoin() < before+7 || after.GetCoin() > before+14 {
		t.Fatal("unexpected original reward", before, after.GetCoin())
	}
	claims, e := f.s.AdminCDKClaims(f.ctx, actor, fresh.ID, 1, 20)
	if e != nil || claims.Total != 1 || claims.Items[0].PlayerID != f.id {
		t.Fatal(e, claims)
	}
	f.restart(t)
	if _, e = f.s.paymentOffer(f.ctx, f.id, &pb.CSConvertGiftCode{GiftCode: proto.String(generated.Codes[1])}, time.Now()); e == nil {
		t.Fatal("cold duplicate redeemed")
	}
	recovered, _ := f.profile(t)
	if recovered.GetCoin() != after.GetCoin() {
		t.Fatal("cold duplicate changed money")
	}
	if _, e = f.s.AdminCDKList(context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(99)), actor, "", 1, 20); e == nil {
		t.Fatal("foreign realm read")
	}
	req.Count = 1
	req.Code = "ABCD"
	short, e := f.s.AdminCreateCDK(f.ctx, actor, req)
	if e != nil || strings.Contains(short.Items[0].CodeHint, "ABCD") {
		t.Fatal("short code leaked through its persistent hint", e)
	}
}

// Unlimited shared quota keeps original per-role claims, date gates and saved rewards.
func TestAdminCDKUnlimitedQuotaKeepsRoleClaims(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.LoadPaymentTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializePayments, f.s.InitializeAdminGameServers, f.s.InitializeAdminCDK} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.ctx = context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, f.s.OriginalServerID())
	ids := []int64{f.id}
	for _, suffix := range []string{"a", "b", "c"} {
		account, err := LoginAccount(f.ctx, "cdku-"+p6Digest([]byte(t.Name()))[:20]+suffix, "fixture-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, account.Id, "兑换测试"+suffix, "hero_043"); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, account.Id)
	}
	generated, err := f.s.AdminCreateCDK(f.ctx, actor, model.AdminCDKCreateRequest{Name: "不限总次数", Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}, Count: 1, TotalLimit: 0, Unlimited: true, Enabled: true, Reason: "isolated quota conversion verification"})
	if err != nil || len(generated.Items) != 1 || generated.Items[0].TotalLimit != 0 {
		t.Fatal(err, generated)
	}
	redeem := func(id int64) error {
		_, e := f.s.paymentOffer(f.ctx, id, &pb.CSConvertGiftCode{GiftCode: proto.String(generated.Codes[0])}, time.Now())
		return e
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range ids[:2] {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); results <- redeem(id) }(id)
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal("unlimited code rejected another role", e)
		}
	}
	after, _ := f.profile(t)
	if err = redeem(f.id); err == nil {
		t.Fatal("unlimited quota granted same role twice")
	}
	current, _ := f.profile(t)
	if current.GetCoin() != after.GetCoin() {
		t.Fatal("duplicate changed reward")
	}
	page, err := f.s.AdminCDKList(f.ctx, actor, "不限总次数", 1, 20)
	if err != nil || page.Total != 1 || page.Items[0].Used != 2 {
		t.Fatal(err, page)
	}
	row := page.Items[0]
	update := model.AdminCDKUpdateRequest{Name: row.Name, Rewards: row.Rewards, TotalLimit: 1, Unlimited: true, Enabled: true, Version: row.Version, Reason: "quota conversion"}
	if _, err = f.s.AdminUpdateCDK(f.ctx, actor, row.ID, update); err == nil {
		t.Fatal("limited quota below committed usage accepted")
	}
	update.TotalLimit = 2
	changed, err := f.s.AdminUpdateCDK(f.ctx, actor, row.ID, update)
	if err != nil {
		t.Fatal(err)
	}
	if err = redeem(ids[2]); err == nil {
		t.Fatal("finite quota failed to stop new role")
	}
	update.Version = changed.Version
	update.TotalLimit = 0
	changed, err = f.s.AdminUpdateCDK(f.ctx, actor, row.ID, update)
	if err != nil || changed.Used != 2 || changed.TotalLimit != 0 {
		t.Fatal("unlimited conversion reset history", err, changed)
	}
	if err = redeem(ids[2]); err != nil {
		t.Fatal("unlimited conversion did not reopen", err)
	}
	page, err = f.s.AdminCDKList(f.ctx, actor, "不限总次数", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	update.Version = page.Items[0].Version
	update.Unlimited = false
	update.StartAt = time.Now().Add(time.Hour).UnixMilli()
	update.EndAt = time.Now().Add(2 * time.Hour).UnixMilli()
	changed, err = f.s.AdminUpdateCDK(f.ctx, actor, row.ID, update)
	if err != nil {
		t.Fatal(err)
	}
	if err = redeem(ids[3]); err == nil {
		t.Fatal("unlimited quota bypassed date window")
	}
	update.Version = changed.Version
	update.Unlimited = true
	update.StartAt = 0
	update.EndAt = 0
	update.Enabled = false
	changed, err = f.s.AdminUpdateCDK(f.ctx, actor, row.ID, update)
	if err != nil {
		t.Fatal(err)
	}
	if err = redeem(ids[3]); err == nil {
		t.Fatal("unlimited quota bypassed stop")
	}
	update.Version = changed.Version
	update.Enabled = true
	if _, err = f.s.AdminUpdateCDK(f.ctx, actor, row.ID, update); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err = redeem(f.id); err == nil {
		t.Fatal("cold recovery lost per-role claim")
	}
	if err = redeem(ids[3]); err != nil {
		t.Fatal("cold unlimited code rejected fresh role", err)
	}
	page, err = f.s.AdminCDKList(f.ctx, actor, "不限总次数", 1, 20)
	if err != nil || page.Items[0].TotalLimit != 0 || page.Items[0].Used != 4 {
		t.Fatal(err, page)
	}
	claims, err := f.s.AdminCDKClaims(f.ctx, actor, row.ID, 1, 20)
	if err != nil || claims.Total != 4 {
		t.Fatal("original claim ledger mismatch", err, claims)
	}
	legacy := "legacy-zero-" + p6Digest([]byte(t.Name()))[:12]
	if err = db.GetIns().Create(&db.PaymentGiftCode{Hash: p3Hash([]byte(legacy)), Reward: "coin:7", Limit: 0, Source: "legacy-fixture", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.paymentOffer(f.ctx, f.id, &pb.CSConvertGiftCode{GiftCode: proto.String(legacy)}, time.Now()); err == nil {
		t.Fatal("legacy zero quota unexpectedly opened")
	}
}
