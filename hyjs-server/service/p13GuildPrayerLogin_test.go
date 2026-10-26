package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p13PrayerLoginPublishMDef(t *testing.T, at time.Time) {
	t.Helper()
	raw, err := json.Marshal([]p13GuildPrayerChoice{{Type: "permdef", Value: 1, Weight: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P13GuildPrayerPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "isolated-login-permdef1-v1", "source": constUtil.P13GuildPrayerSource, "pool_json": string(raw), "pool_hash": p6Digest(raw), "published_at": at.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
}
func p13PrayerLoginProfile(t *testing.T, id int64) db.Profile {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	return row
}
func p13PrayerLoginPacket(t *testing.T, f *p1Fixture, id int64, opcode int32, body proto.Message) []proto.Message {
	t.Helper()
	row := p13PrayerLoginProfile(t, id)
	out, err := f.s.dispatch(f.ctx, &model.GamePacket{Opcode: opcode, PlayerId: id, ZoneId: int32(f.s.OriginalServerID()), Session: row.Session, Body: body}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range out {
		if e, ok := m.(*pb.ErrorMessage); ok {
			t.Fatal("original protocol error", e)
		}
	}
	return out
}
func p13PrayerLoginAck(t *testing.T, f *p1Fixture, id int64) *pb.SCGetUnionAddBaseProp {
	t.Helper()
	before := p13PrayerLoginProfile(t, id)
	out, handled, err := f.s.DispatchP13GuildPrayer(f.ctx, id, &pb.CSGetUnionAddBaseProp{}, 0)
	if err != nil || !handled || len(out) != 1 {
		t.Fatal("property callback", handled, err)
	}
	prop, ok := out[0].(*pb.SCGetUnionAddBaseProp)
	if !ok || prop.GetUnionPrayProp().GetPermdef() != 1 {
		t.Fatal("actual active prayer was removed", out)
	}
	after := p13PrayerLoginProfile(t, id)
	if before.Game == nil || after.Game == nil || *before.Game != *after.Game {
		t.Fatal("ack changed Game/history")
	}
	a, b := &pb.Player{}, &pb.Player{}
	if json.Unmarshal([]byte(*before.Player), a) != nil || json.Unmarshal([]byte(*after.Player), b) != nil {
		t.Fatal("player JSON")
	}
	a.NeedRefreshUnionProp = proto.Int32(0)
	if !proto.Equal(a, b) {
		t.Fatal("ack changed a Player field other than refresh bit")
	}
	return prop
}

// This follows the original LoginPlayerLogin wire order: SCPlayerLogin is
// cached, explicit guild properties precede ChangeCallBack/InitPlayerData,
// then CSChangeNameTimes and CSFriendList. Actual Unity acceptance is separate.
func TestP13GuildPrayerLoginThreeRealPrayersCachedPayloadAndSingleRefresh(t *testing.T) {
	f, ids, now := p13PrayerSetup(t)
	id := ids[0]
	// The full original login records Activity/P1/P4 facts. Use the real
	// initializer chain rather than bypassing any login consumer.
	for _, initialize := range []func(context.Context) error{f.s.InitializeOpenServerActivity, f.s.InitializeActivities, f.s.InitializeGameplayPools, f.s.InitializeShop, f.s.InitializeP4} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	p13PrayerLoginPublishMDef(t, now.Add(-time.Second))
	at := now.Add(time.Second)
	f.ctx = context.WithValue(f.ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return at })
	hashes := []string{}
	for i := int64(1); i <= 3; i++ {
		at = now.Add(time.Duration(i) * time.Second)
		out, err := f.s.p13GuildPrayer(f.ctx, id, &pb.CSUnionPray{}, 900+i, at)
		if err != nil || len(out) < 2 {
			t.Fatal("real prayer mutation", err)
		}
		player := out[1].(*pb.SCPlayer).GetPlayer()
		if player.GetNeedRefreshUnionProp() != 1 {
			t.Fatal("normal mutation stopped requesting dynamic refresh")
		}
		var row db.P13GuildPrayerRoll
		if err = db.GetIns().Where("player_id=?", id).Order("ordinal DESC").First(&row).Error; err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, row.SourceHash)
		if i < 3 {
			p13PrayerLoginAck(t, f, id)
		}
	}
	stored, _ := p10LuckyProfile(t, id)
	if stored.GetNeedRefreshUnionProp() != 1 || stored.GetContribution() != 0 {
		t.Fatal("third real paid prayer source", stored)
	}
	at = now.Add(4 * time.Second)
	out := p13PrayerLoginPacket(t, f, id, 20029, &pb.CSPlayerLogin{})
	if len(out) != 1 {
		t.Fatal("original login callback shape", out)
	}
	cached := out[0].(*pb.SCPlayerLogin)
	if cached.GetPlayer().GetNeedRefreshUnionProp() != 0 {
		t.Fatal("cached login would start duplicate HTTP during InitData")
	}
	raw, err := proto.Marshal(cached)
	if err != nil {
		t.Fatal(err)
	}
	wire := &pb.SCPlayerLogin{}
	if err = proto.Unmarshal(raw, wire); err != nil || wire.GetPlayer().GetNeedRefreshUnionProp() != 0 {
		t.Fatal("original login binary flag", err)
	}
	stored, _ = p10LuckyProfile(t, id)
	if stored.GetNeedRefreshUnionProp() != 1 {
		t.Fatal("login projected response prematurely acknowledged durable effect")
	}
	prop := p13PrayerLoginAck(t, f, id)
	if prop.GetUnionPrayProp().GetPermdef() != 1 || cached.GetPlayer().GetNeedRefreshUnionProp() != 0 {
		t.Fatal("properties or cached login changed")
	}
	rename := p13PrayerLoginPacket(t, f, id, 20115, &pb.CSChangeNameTimes{})
	if len(rename) != 1 {
		t.Fatal("original rename count callback", rename)
	}
	if _, ok := rename[0].(*pb.SCChangeNameTimes); !ok {
		t.Fatal("wrong rename protocol")
	}
	friends := p13PrayerLoginPacket(t, f, id, 21097, &pb.CSFriendList{})
	if len(friends) != 1 {
		t.Fatal("original friend-list continuation", friends)
	}
	if _, ok := friends[0].(*pb.SCFriendList); !ok {
		t.Fatal("wrong friend protocol")
	}
	current := p13PrayerLoginPacket(t, f, id, 20101, &pb.CSPlayer{})[0].(*pb.SCPlayer).GetPlayer()
	if current.GetNeedRefreshUnionProp() != 0 || current.GetContribution() != 0 {
		t.Fatal("later SCPlayer resurrected dirty bit or changed debit")
	}
	var rows []db.P13GuildPrayerRoll
	if err = db.GetIns().Where("player_id=?", id).Order("ordinal").Find(&rows).Error; err != nil || len(rows) != 3 {
		t.Fatal("prayer history denominator", err)
	}
	for i, row := range rows {
		if row.SourceHash != hashes[i] {
			t.Fatal("ack rewrote prayer roll", i)
		}
	}
	active, _, _, err := f.s.P13GuildPrayerProperties(f.ctx, db.GetIns(), id, at)
	if err != nil || active.GetPermdef() != 1 {
		t.Fatal("actual active effect disappeared", active, err)
	}
}

func TestP13GuildPrayerLoginFailedProjectionAndUpdateNeverAcknowledge(t *testing.T) {
	f, ids, now := p13PrayerSetup(t)
	id := ids[0]
	p13PrayerLoginPublishMDef(t, now.Add(-time.Second))
	at := now.Add(time.Second)
	f.ctx = context.WithValue(f.ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return at })
	if _, err := f.s.p13GuildPrayer(f.ctx, id, &pb.CSUnionPray{}, 901, at); err != nil {
		t.Fatal(err)
	}
	before := p13PrayerLoginProfile(t, id)
	callback := fmt.Sprintf("isolated-prayer-ack-failure-%d", time.Now().UnixNano())
	if err := db.GetIns().Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" {
			_ = tx.AddError(errors.New("isolated acknowledgement update failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	out, handled, err := f.s.DispatchP13GuildPrayer(f.ctx, id, &pb.CSGetUnionAddBaseProp{}, 0)
	_ = db.GetIns().Callback().Update().Remove(callback)
	if err == nil || !handled || len(out) != 0 {
		t.Fatal("failed update returned property success", out, err)
	}
	after := p13PrayerLoginProfile(t, id)
	if *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("failed acknowledgement committed")
	}
	var roll db.P13GuildPrayerRoll
	if err = db.GetIns().Where("player_id=?", id).First(&roll).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&roll).Update("effect_hash", "corrupt-isolated-fixture").Error; err != nil {
		t.Fatal(err)
	}
	out, handled, err = f.s.DispatchP13GuildPrayer(f.ctx, id, &pb.CSGetUnionAddBaseProp{}, 0)
	if err == nil || !handled || len(out) != 0 {
		t.Fatal("corrupt source produced acknowledgement", err)
	}
	after = p13PrayerLoginProfile(t, id)
	if *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("failed source cleared flag or changed history")
	}
}
