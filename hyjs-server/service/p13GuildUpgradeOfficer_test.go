package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Original union_des and UnionInfoTab expose this transaction to both officers.
// Resources are finite local prerequisites; only the original upgrade debits them.
func TestP13GuildUpgradeVicePermissionAtomicRetryAndDemotion(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, true)
	now := time.Now()
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("副会长升级")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guild, 2, now)
	p13Join(t, f, ids[0], ids[2], guild, 3, now)
	p13GuildCall(t, f, ids[0], &pb.CSPromotionViceChairman{PlayerId: proto.Int64(ids[1])}, 4, now)
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Updates(map[string]any{"coin": int64(10000000), "badge": int32(10), "exp": int32(2000)}).Error; err != nil {
		t.Fatal(err)
	}
	read := func() db.P13Guild {
		var row db.P13Guild
		if err := db.GetIns().First(&row, "id=?", guild).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	before := read()
	if _, err := f.s.p13GuildEconomy(f.ctx, ids[2], &pb.CSUnionUpgrade{}, 5, now); err == nil {
		t.Fatal("ordinary member upgraded")
	}
	if after := read(); after.Level != before.Level || after.Coin != before.Coin || after.Badge != before.Badge || after.Exp != before.Exp {
		t.Fatal("permission rejection debited resources")
	}
	for range 2 {
		p13GuildCall(t, f, ids[1], &pb.CSUnionUpgrade{}, 6, now)
	}
	if after := read(); after.Level != 2 || after.Coin != 0 || after.Badge != 0 || after.Exp != 0 {
		t.Fatal("vice original debit or retry changed", after.Level, after.Coin, after.Badge, after.Exp)
	}
	p13GuildCall(t, f, ids[0], &pb.CSDismissViceChairman{PlayerId: proto.Int64(ids[1])}, 7, now)
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Updates(map[string]any{"coin": int64(20000000), "badge": int32(20), "exp": int32(5000)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13GuildEconomy(f.ctx, ids[1], &pb.CSUnionUpgrade{}, 8, now); err == nil {
		t.Fatal("demoted officer retained upgrade permission")
	}
	if after := read(); after.Level != 2 || after.Coin != 20000000 || after.Badge != 20 || after.Exp != 5000 {
		t.Fatal("demoted request debited resources")
	}
}
