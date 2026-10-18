package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func p13PrayerSetup(t *testing.T) (*p1Fixture, []int64, time.Time) {
	t.Helper()
	f, ids := p13GuildSetup(t)
	if err := f.s.InitializeP13GuildPrayer(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P13GuildPrayerRoll{}, &db.P13GuildPrayerPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP13GuildPrayer(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Minute)
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("真实祈祷")}, 1, now)
	// Finite account setup provides coin only; contribution must come from fifteen real donations.
	if err := updateProfile(f.ctx, ids[0], func(p *pb.Player, g *model.GameState) error { p.Coin = proto.Int64(2000000); return nil }); err != nil {
		t.Fatal(err)
	}
	for i := int64(0); i < 15; i++ {
		p13GuildCall(t, f, ids[0], &pb.CSUnionDonate{DonateType: proto.String("coin")}, 10+i, now)
	}
	return f, ids, now
}

func p13PrayerPublish(t *testing.T, at time.Time) {
	t.Helper()
	raw, err := json.Marshal([]p13GuildPrayerChoice{{Type: "peratt", Value: 1, Weight: 1}, {Type: "perhp", Value: 2, Weight: 3}, {Type: "perpdef", Value: 3, Weight: 2}, {Type: "permdef", Value: 4, Weight: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P13GuildPrayerPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "local-test-single-percent-v1", "source": constUtil.P13GuildPrayerSource, "pool_json": string(raw), "pool_hash": p6Digest(raw), "published_at": at.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestP13GuildPrayerRealDonationsOriginalFramesOverrideFrozenColdExpiryAndLeave(t *testing.T) {
	f, ids, now := p13PrayerSetup(t)
	id := ids[0]
	if _, err := f.s.p13GuildPrayer(f.ctx, id, &pb.CSUnionPray{}, 100, now); err == nil {
		t.Fatal("default-disabled prayer accepted")
	}
	p13PrayerPublish(t, now.Add(-time.Second))
	var last *pb.BaseProp
	var source string
	var expires int64
	for i := int64(1); i <= 3; i++ {
		at := now.Add(time.Duration(i) * time.Second)
		messages, err := f.s.p13GuildPrayer(f.ctx, id, &pb.CSUnionPray{}, 100+i, at)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := messages[0].(*pb.SCUnionPray); !ok {
			t.Fatal("original empty prayer response", messages)
		}
		infoMessages, err := f.s.p13GuildPrayerInfo(f.ctx, id, at)
		if err != nil {
			t.Fatal(err)
		}
		info := infoMessages[0].(*pb.SCUnionLightChurchInfo)
		if info.GetPrayTimes() != int32(i) || info.GetContribution() != int32(300-i*100) || info.GetInvalidTime() != at.Add(24*time.Hour).UnixMilli() {
			t.Fatal("original used-count/cost/duration", info)
		}
		last, source, expires, err = f.s.P13GuildPrayerProperties(f.ctx, db.GetIns(), id, at)
		if err != nil || source == "" || !proto.Equal(last, info.GetAddProp()) {
			t.Fatal("live single effect", last, source, err)
		}
		raw, err := proto.Marshal(info)
		if err != nil {
			t.Fatal(err)
		}
		decoded := &pb.SCUnionLightChurchInfo{}
		if err = proto.Unmarshal(raw, decoded); err != nil || !proto.Equal(info, decoded) {
			t.Fatal("original info binary", err)
		}
	}
	if _, err := f.s.p13GuildPrayer(f.ctx, id, &pb.CSUnionPray{}, 104, now.Add(4*time.Second)); err == nil {
		t.Fatal("fourth prayer accepted")
	}
	player, _ := p10LuckyProfile(t, id)
	var member db.P13GuildMember
	if err := db.GetIns().First(&member, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if player.GetCoin() != 500000 || player.GetContribution() != 0 || member.Contribution != 300 {
		t.Fatal("spendable/lifetime contribution conservation", player.GetCoin(), player.GetContribution(), member.Contribution)
	}
	var rows []db.P13GuildPrayerRoll
	if err := db.GetIns().Where("player_id=?", id).Order("ordinal").Find(&rows).Error; err != nil || len(rows) != 3 {
		t.Fatal("actual debit audit", len(rows), err)
	}
	for _, row := range rows {
		if _, err := p13GuildPrayerRollRead(&row); err != nil {
			t.Fatal(err)
		}
	}
	if rows[2].SourceHash != source {
		t.Fatal("latest effect did not overwrite")
	}
	// A changed or closed operator pool cannot reinterpret already purchased effects.
	if err := db.GetIns().Model(&db.P13GuildPrayerPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "pool_json": "[]", "pool_hash": "broken", "version": "closed-next-version"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13GuildPrayer(f.ctx, id, &pb.CSUnionPray{}, 103, now.Add(3*time.Second)); err != nil {
		t.Fatal("completed original receipt lost on close", err)
	}
	sqlDB, _ := db.GetIns().DB()
	sqlDB.Close()
	f.open(t)
	if err := f.s.InitializeP13GuildPrayer(f.ctx); err != nil {
		t.Fatal(err)
	}
	after, hash, expiry, err := f.s.P13GuildPrayerProperties(f.ctx, db.GetIns(), id, now.Add(time.Hour))
	if err != nil || !proto.Equal(after, last) || hash != source || expiry != expires {
		t.Fatal("frozen effect changed after cold/close", err)
	}
	after.Peratt = proto.Float64(999)
	again, _, _, err := f.s.P13GuildPrayerProperties(f.ctx, db.GetIns(), id, now.Add(time.Hour))
	if err != nil || !proto.Equal(again, last) {
		t.Fatal("returned property aliases stored source", err)
	}
	// A later real membership term has no entitlement to an earlier prayer.
	var activeMember db.P13GuildMember
	if err := db.GetIns().First(&activeMember, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	joinedAt := activeMember.JoinedAt
	if err := db.GetIns().Model(&activeMember).Update("joined_at", now.Add(30*time.Minute).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	newTerm, newHash, newExpiry, err := f.s.P13GuildPrayerProperties(f.ctx, db.GetIns(), id, now.Add(time.Hour))
	if err != nil || newTerm == nil || !proto.Equal(newTerm, &pb.BaseProp{}) || newHash != "" || newExpiry != 0 {
		t.Fatal("new membership revived prior-term prayer", err)
	}
	if err := db.GetIns().Model(&activeMember).Update("joined_at", joinedAt).Error; err != nil {
		t.Fatal(err)
	}
	inactive, hash, expiry, err := f.s.P13GuildPrayerProperties(f.ctx, db.GetIns(), id, time.UnixMilli(expires))
	if err != nil || inactive == nil || !proto.Equal(inactive, &pb.BaseProp{}) || hash != "" || expiry != 0 {
		t.Fatal("expiry boundary retained prayer", inactive, err)
	}
	guild := p13GuildID(t, f, id, now)
	p13Join(t, f, id, ids[1], guild, 200, now.Add(5*time.Second))
	p13GuildCall(t, f, id, &pb.CSTransferChairman{PlayerId: proto.Int64(ids[1])}, 201, now.Add(6*time.Second))
	p13GuildCall(t, f, id, &pb.CSQuitUnion{}, 202, now.Add(7*time.Second))
	inactive, hash, expiry, err = f.s.P13GuildPrayerProperties(f.ctx, db.GetIns(), id, now.Add(8*time.Second))
	if err != nil || inactive == nil || !proto.Equal(inactive, &pb.BaseProp{}) || hash != "" || expiry != 0 {
		t.Fatal("ex-member retained prayer", err)
	}
	var count int64
	if err = db.GetIns().Model(&db.P13GuildPrayerRoll{}).Where("player_id=?", id).Count(&count).Error; err != nil || count != 3 {
		t.Fatal("membership change erased actual debit history", count, err)
	}
}

func TestP13GuildPrayerSameRequestConcurrentSingleDebit(t *testing.T) {
	f, ids, now := p13PrayerSetup(t)
	p13PrayerPublish(t, now.Add(-time.Second))
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.p13GuildPrayer(f.ctx, ids[0], &pb.CSUnionPray{}, 100, now)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.GetIns().Model(&db.P13GuildPrayerRoll{}).Where("player_id=?", ids[0]).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("concurrent roll count", count, err)
	}
	player, _ := p10LuckyProfile(t, ids[0])
	if player.GetContribution() != 200 {
		t.Fatal("concurrent debit repeated", player.GetContribution())
	}
}

func TestP13GuildPrayerMissingOptionalSchemaIsReadOnlyInactive(t *testing.T) {
	f, ids := p13GuildSetup(t)
	// No prayer initialization is performed. P5 hydration can read old optional deployments.
	if err := db.GetIns().Migrator().DropTable(&db.P13GuildPrayerRoll{}); err != nil {
		t.Fatal(err)
	}
	prop, hash, expires, err := f.s.P13GuildPrayerProperties(context.Background(), db.GetIns(), ids[0], time.Now())
	if err != nil || prop == nil || !proto.Equal(prop, &pb.BaseProp{}) || hash != "" || expires != 0 {
		t.Fatal("missing optional source", prop, err)
	}
	if db.GetIns().Migrator().HasTable(&db.P13GuildPrayerRoll{}) {
		t.Fatal("read API created source table")
	}
}

func TestP13GuildPrayerUnpublishedOrInvalidPoolCannotCharge(t *testing.T) {
	f, ids, now := p13PrayerSetup(t)
	p13PrayerPublish(t, now.Add(time.Minute))
	for index, change := range []map[string]any{
		{},
		{"published_at": now.Add(-time.Second).UnixMilli(), "pool_hash": "broken"},
		{"pool_json": "[{\"type\":\"att\",\"value\":50,\"weight\":1}]", "pool_hash": p6Digest([]byte("[{\"type\":\"att\",\"value\":50,\"weight\":1}]"))},
	} {
		if len(change) > 0 {
			if err := db.GetIns().Model(&db.P13GuildPrayerPolicy{}).Where("id=1").Updates(change).Error; err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.s.p13GuildPrayer(f.ctx, ids[0], &pb.CSUnionPray{}, int64(500+index), now); err == nil {
			t.Fatal("invalid/unpublished local pool charged", index)
		}
	}
	player, _ := p10LuckyProfile(t, ids[0])
	var count int64
	if err := db.GetIns().Model(&db.P13GuildPrayerRoll{}).Where("player_id=?", ids[0]).Count(&count).Error; err != nil || count != 0 || player.GetContribution() != 300 {
		t.Fatal("failed pool changed actual contribution", count, player.GetContribution(), err)
	}
	p13PrayerPublish(t, now.Add(-time.Second))
	if _, err := f.s.p13GuildPrayer(f.ctx, ids[0], &pb.CSUnionPray{}, 500, now); err != nil {
		t.Fatal("failed attempt created completed receipt", err)
	}
}
