package service

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13RecoveryTestFrame(dead bool) *pb.InstUFPlayer {
	frame := &pb.InstUFPlayer{Guid: proto.Int64(71), State: proto.Int32(5), DeadNum: proto.Int32(3), KillNum: proto.Int32(2), OwnScore: proto.Float64(17), OwnToken: proto.Float64(9), Items: []*pb.InstUFItem{{Id: proto.String("ufitem_011"), LeftUseNum: proto.Int32(3)}}, FightInfo: &pb.InstFightable{Guid: proto.Int64(71), Index: []int32{4, 1, 3, 2}, HpPos: []float64{10, 0, 180, 39.5}, MaxHpPos: []float64{100, 200, 300, 400}, Anger: proto.Float64(88.25)}}
	if dead {
		frame.State = proto.Int32(3)
		frame.LeftTime = proto.Float32(7)
		frame.TotalTime = proto.Float32(20)
		frame.FightInfo.HpPos = []float64{0, 0, 0, 0}
	}
	return frame
}

func TestP13UFPlayerRecoveryRecordVerifyLifeAndRollback(t *testing.T) {
	f, _ := p13GuildSetup(t)
	if err := f.s.InitializeP13UnionFightItem(f.ctx); err != nil {
		t.Fatal(err)
	}
	for n, item := range []string{"ufitem_011", "ufitem_016", "ufitem_012"} {
		t.Run(item, func(t *testing.T) {
			before := p13RecoveryTestFrame(item == "ufitem_012")
			after := proto.Clone(before).(*pb.InstUFPlayer)
			percent := int64(25)
			if item == "ufitem_012" {
				percent = 100
			}
			if err := p13UFItemLife(after, &db.P13UnionFightItemOffer{ItemID: item, HPPercent: percent}); err != nil {
				t.Fatal(err)
			}
			want := []float64{35, 0, 255, 139.5}
			if item == "ufitem_012" {
				want = []float64{100, 200, 300, 400}
				if after.GetState() != 0 || after.GetLeftTime() != 0 || after.GetTotalTime() != 0 {
					t.Fatal("revive timers not cleared")
				}
			}
			for i, hp := range want {
				if after.FightInfo.HpPos[i] != hp {
					t.Fatal("pose HP", i, after.FightInfo.HpPos, want)
				}
			}
			if after.FightInfo.GetAnger() != before.FightInfo.GetAnger() || after.GetDeadNum() != before.GetDeadNum() || after.GetKillNum() != before.GetKillNum() || after.GetOwnScore() != before.GetOwnScore() || !proto.Equal(after.Items[0], before.Items[0]) {
				t.Fatal("life mutation changed AP/ordinal/resources/inventory")
			}
			evidence := db.P13UnionFightPlayerRecovery{MatchID: "recovery-test", Kind: constUtil.P13UFRecoveryItem, ItemID: item, Stamp: int64(n + 1), CasterID: 71, OfferHash: p6Digest([]byte(item)), HPPercent: percent, InventoryBefore: 3, InventoryAfter: 2, LogicalAt: 10000, AppliedAt: 10001}
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return p13UFPlayerRecoveryRecord(tx, before, after, evidence) }); err != nil {
				t.Fatal(err)
			}
			var stored db.P13UnionFightPlayerRecovery
			if err := db.GetIns().First(&stored, "match_id=? AND item_id=?", "recovery-test", item).Error; err != nil {
				t.Fatal(err)
			}
			b, a, err := p13UFPlayerRecoveryVerify(&stored)
			if err != nil || !proto.Equal(b, before) || !proto.Equal(a, after) {
				t.Fatal("record/verify roundtrip", err)
			}
			changes := map[string]func(*db.P13UnionFightPlayerRecovery){
				"cross-player":     func(r *db.P13UnionFightPlayerRecovery) { r.PlayerID++ },
				"cross-caster":     func(r *db.P13UnionFightPlayerRecovery) { r.CasterID++ },
				"unknown-item":     func(r *db.P13UnionFightPlayerRecovery) { r.ItemID = "ufitem_999" },
				"inventory-scalar": func(r *db.P13UnionFightPlayerRecovery) { r.InventoryAfter = r.InventoryBefore },
				"HP": func(r *db.P13UnionFightPlayerRecovery) {
					v := proto.Clone(after).(*pb.InstUFPlayer)
					v.FightInfo.HpPos[0]++
					r.After, _ = proto.MarshalOptions{Deterministic: true}.Marshal(v)
					r.AfterHash = p6Digest(r.After)
				},
				"inventory-frame": func(r *db.P13UnionFightPlayerRecovery) {
					v := proto.Clone(after).(*pb.InstUFPlayer)
					v.Items[0].LeftUseNum = proto.Int32(2)
					r.After, _ = proto.MarshalOptions{Deterministic: true}.Marshal(v)
					r.AfterHash = p6Digest(r.After)
				},
				"AP": func(r *db.P13UnionFightPlayerRecovery) {
					v := proto.Clone(after).(*pb.InstUFPlayer)
					v.FightInfo.Anger = proto.Float64(0)
					r.After, _ = proto.MarshalOptions{Deterministic: true}.Marshal(v)
					r.AfterHash = p6Digest(r.After)
				},
			}
			for name, change := range changes {
				t.Run(name, func(t *testing.T) {
					bad := stored
					change(&bad)
					bad.ID = p13UFPlayerRecoveryID(&bad)
					bad.SourceHash = p13UFPlayerRecoveryHash(&bad)
					if _, _, err := p13UFPlayerRecoveryVerify(&bad); err == nil {
						t.Fatal("semantic tamper accepted")
					}
				})
			}
			bad := stored
			bad.SourceHash = "bad"
			if _, _, err := p13UFPlayerRecoveryVerify(&bad); err == nil {
				t.Fatal("sourcehash tamper accepted")
			}
		})
	}
	// Stamp zero is a real non-idempotent use: repeated original client packets
	// at one logical millisecond receive independent random receipt identities.
	before := p13RecoveryTestFrame(false)
	after := proto.Clone(before).(*pb.InstUFPlayer)
	if err := p13UFItemLife(after, &db.P13UnionFightItemOffer{ItemID: "ufitem_011", HPPercent: 25}); err != nil {
		t.Fatal(err)
	}
	evidence := db.P13UnionFightPlayerRecovery{MatchID: "recovery-zero", Kind: constUtil.P13UFRecoveryItem, ItemID: "ufitem_011", CasterID: 71, OfferHash: p6Digest([]byte("zero")), HPPercent: 25, InventoryBefore: 3, InventoryAfter: 2, LogicalAt: 10000, AppliedAt: 10000}
	for n := 0; n < 2; n++ {
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return p13UFPlayerRecoveryRecord(tx, before, after, evidence) }); err != nil {
			t.Fatal(err)
		}
	}
	var rows []db.P13UnionFightPlayerRecovery
	if err := db.GetIns().Where("match_id=?", "recovery-zero").Find(&rows).Error; err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	if rows[0].ID == rows[1].ID || rows[0].Nonce == rows[1].Nonce || len(rows[0].Nonce) != 32 || len(rows[1].Nonce) != 32 {
		t.Fatal("stamp0 nonce collision")
	}
	sentinel := errors.New("after life mutation failure")
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		evidence.MatchID = "recovery-rollback"
		if err := p13UFPlayerRecoveryRecord(tx, before, after, evidence); err != nil {
			return err
		}
		if err := tx.Create(&db.P13UnionFightItemState{MatchID: evidence.MatchID, PlayerID: 71, NormalUntil: 10000, SourceHash: p6Digest([]byte("same transaction"))}).Error; err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P13UnionFightPlayerRecovery{}, &db.P13UnionFightItemState{}} {
		var count int64
		if err := db.GetIns().Model(table).Where("match_id=?", "recovery-rollback").Count(&count).Error; err != nil || count != 0 {
			t.Fatal("transaction leaked", count, err)
		}
	}
}

func TestP13UFPlayerRecoveryRejectPartialDeathRevive(t *testing.T) {
	frame := p13RecoveryTestFrame(false)
	frame.State = proto.Int32(3)
	if err := p13UFItemLife(frame, &db.P13UnionFightItemOffer{ItemID: "ufitem_012", HPPercent: 100}); err == nil {
		t.Fatal("partial party revive accepted")
	}
	frame = p13RecoveryTestFrame(true)
	frame.State = proto.Int32(0)
	if err := p13UFItemLife(frame, &db.P13UnionFightItemOffer{ItemID: "ufitem_011", HPPercent: 25}); err == nil {
		t.Fatal("ordinary potion revived completely dead party")
	}
}
