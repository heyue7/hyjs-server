package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestPetEquip48LoginUsesInitializedFeature(t *testing.T) {
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.SkillSpecialtyRule{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.SkillSpecialtyRule{ID: constUtil.SkillSpecialtyRuleID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.KV = []*pb.KeyValueEntry{{Key: proto.String("petEquipSwitch"), Value: proto.String("on")}, {Key: proto.String("petEquipSwitch"), Value: proto.String("off")}, {Key: proto.String("keep"), Value: proto.String("saved")}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	for _, ready := range []bool{false, true, false} {
		f.s.P9PetsReady = ready
		out, err := f.s.dispatch(f.ctx, &model.GamePacket{PlayerId: f.id, ZoneId: 1, Session: row.Session, Body: &pb.CSKVInfo{}}, "127.0.0.1")
		if err != nil || len(out) != 1 {
			t.Fatalf("login KV: %v %v", out, err)
		}
		reply, ok := out[0].(*pb.SCKVInfo)
		if !ok {
			t.Fatalf("login KV type %T", out[0])
		}
		count, kept := 0, false
		want := "off"
		if ready {
			want = "on"
		}
		for _, entry := range reply.GetEntry() {
			if entry.GetKey() == "petEquipSwitch" {
				count++
				if entry.GetValue() != want {
					t.Fatalf("ready=%v switch=%s", ready, entry.GetValue())
				}
			}
			kept = kept || entry.GetKey() == "keep" && entry.GetValue() == "saved"
		}
		if count != 1 || !kept {
			t.Fatalf("duplicate switch or lost saved KV: %v", reply)
		}
	}
}
