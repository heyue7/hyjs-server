package service

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13UFPreviewLogoOriginalIDAndLegacyProjection(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, true)
	now := time.Now()
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("旗帜验证")}, 1, now)
	guildID := p13GuildID(t, f, ids[0], now)
	info := p13GuildCall(t, f, ids[0], &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo)
	if info.GetUnionInfo().GetUnionId() != guildID {
		t.Fatal("real guild identity missing")
	}
	var guild db.P13Guild
	if err := db.GetIns().First(&guild, "id=?", guildID).Error; err != nil {
		t.Fatal(err)
	}
	fresh, err := f.s.p13UFGuildPreview(db.GetIns(), guild, 330)
	if err != nil {
		t.Fatal(err)
	}
	var decoded pb.UFInfoPreView
	if proto.Unmarshal(fresh, &decoded) != nil || decoded.GetUnionLogo() != "0" {
		t.Fatal("new frozen preview must carry the original table id")
	}
	// This presentation-only historical PB proves compatibility; it supplies no
	// match, roster, result, eligibility or combat authority.
	for _, row := range f.s.Tables["UnionLogo"] {
		legacy := &pb.UFInfoPreView{UnionId: proto.String(guildID), UnionName: proto.String("旗帜验证"), UnionLogo: proto.String(row["iconName"])}
		raw, err := proto.Marshal(legacy)
		if err != nil {
			t.Fatal(err)
		}
		original := append([]byte(nil), raw...)
		view := &pb.UFInfoPreView{}
		if err = proto.Unmarshal(raw, view); err != nil {
			t.Fatal(err)
		}
		if err = f.s.p13UFClientPreview(view); err != nil || view.GetUnionLogo() != row["id"] {
			t.Fatal("old iconName not projected to original id", row, err)
		}
		if !bytes.Equal(raw, original) || legacy.GetUnionLogo() != row["iconName"] {
			t.Fatal("historical source bytes mutated")
		}
		if err = f.s.p13UFClientPreview(view); err != nil || view.GetUnionLogo() != row["id"] {
			t.Fatal("canonical id not idempotent", err)
		}
	}
	if err := f.s.p13UFClientPreview(&pb.UFInfoPreView{UnionLogo: proto.String("missing-logo")}); err == nil {
		t.Fatal("unknown original resource accepted")
	}
}
