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

// TestP13GuildWelfare 返回原未创建/未祈祷事实，已捐献的贡献与公会资产来自实际持久记录。
func TestP13GuildWelfareUncreatedOriginalQueriesContributionAndNoUnknownCharge(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("只读福利")}, 1, now)
	p13GuildCall(t, f, ids[0], &pb.CSUnionDonate{DonateType: proto.String("coin")}, 2, now)
	var before db.Profile
	if err := db.GetIns().First(&before, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	for _, req := range []proto.Message{&pb.CSUnionHeroStatueInfo{}, &pb.CSUnionGuardTreeInfo{}, &pb.CSUnionLightChurchInfo{}} {
		messages, handled, err := f.s.DispatchP13Guild(f.ctx, ids[0], req, 3)
		if !handled || err != nil {
			t.Fatal(req, err)
		}
		switch b := messages[0].(type) {
		case *pb.SCUnionHeroStatueInfo:
			if b.GetCoinNum() != 100000 || b.GetBadgeNum() != 0 || len(b.UnionPropAddAtt) != 0 {
				t.Fatal("uncreated statue invented tech", b)
			}
		case *pb.SCUnionGuardTreeInfo:
			if b.GetCoinNum() != 100000 || b.GetBadgeNum() != 0 || len(b.UnionPropAddDef) != 0 {
				t.Fatal("uncreated tree invented tech", b)
			}
		case *pb.SCUnionLightChurchInfo:
			if b.GetPrayTimes() != 0 || b.GetContribution() != 20 || b.AddProp == nil || b.GetInvalidTime() != 0 || !proto.Equal(b.AddProp, &pb.BaseProp{}) {
				t.Fatal("actual unprayed state", b)
			}
		}
		if _, _, err = f.s.DispatchP13Guild(f.ctx, ids[1], req, 3); err == nil {
			t.Fatal("outsider read guild benefits")
		}
	}
	for _, req := range []proto.Message{&pb.CSUnionHeroStatueUpgrade{Type: proto.String("att")}, &pb.CSUnionGuardTreeUpgrade{Type: proto.String("hp")}, &pb.CSUnionPray{}} {
		_, handled, err := f.s.DispatchP13Guild(f.ctx, ids[0], req, 4)
		if !handled || err == nil || !strings.Contains(err.Error(), "原规则核实") {
			t.Fatal("unknown charged/opened", req, err)
		}
	}
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Update("management_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[0], &pb.CSUnionLightChurchInfo{}, 5); err != nil {
		t.Fatal("closed policy lost actual readonly facts", err)
	}
	var after db.Profile
	if err := db.GetIns().First(&after, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	if *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("readonly/blocked benefits changed profile")
	}
	var guild db.P13Guild
	if err := db.GetIns().First(&guild, "id=?", p13GuildID(t, f, ids[0], now)).Error; err != nil {
		t.Fatal(err)
	}
	if guild.Coin != 100000 || guild.Badge != 0 {
		t.Fatal("unknown shared charge")
	}
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[0], &pb.CSUnionOpenBuilder{Type: proto.String("HeroStatue")}, 4); err == nil {
		t.Fatal("closed/underfunded build allowed")
	}
	if constUtil.P13GuildPrayerUnverified == "" {
		t.Fatal("unnamed unverified prayer boundary")
	}
}
