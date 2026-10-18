package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestRecruitGuideResumeAfterStory 验证后续主线已完成时，领取入口仍可续接且不回退存档。
func TestRecruitGuideResumeAfterStory(t *testing.T) {
	s, ctx, _ := newDailyCopyTest(t)
	id := dailyCopyTestPlayer(t, ctx, "recruit-guide-resume", 2)
	if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.NoviceGuideSchedule = proto.Int32(4)
		p.Coin = proto.Int64(108760)
		g.Quests = []string{"quest_1", "quest_2"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	p, _, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	query := &pb.SCPlayer{Player: p}
	login := &pb.SCPlayerLogin{Player: p}
	s.resumeRecruitGuide(ctx, []proto.Message{query, login})
	if query.Player.GetNoviceGuideSchedule() != 1 || login.Player.GetNoviceGuideSchedule() != 1 || !proto.Equal(query.Player, login.Player) {
		t.Fatal("pending guide was skipped", query, login)
	}
	if p.GetNoviceGuideSchedule() != 4 || query.Player.GetCoin() != 108760 {
		t.Fatal("response projection changed the stored input or wallet")
	}
	claimed, err := s.recruitGuide(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	s.resumeRecruitGuide(ctx, claimed)
	if claimed[1].(*pb.SCPlayer).Player.GetNoviceGuideSchedule() != 4 {
		t.Fatal("claim lost completed story progress")
	}
	retried, err := s.recruitGuide(ctx, id)
	if err != nil || !proto.Equal(claimed[0], retried[0]) {
		t.Fatal("guide retry changed its receipt", err)
	}
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	p, game, err := decodeProfile(&row)
	if err != nil || p.GetNoviceGuideSchedule() != 4 || p.GetCoin() != 108760 || len(game.Quests) != 2 || len(game.Warriors) != 2 {
		t.Fatal("stored progress or single-instance claim changed", err, p, game)
	}
}

// TestRecruitGuideLegacyHero 保留已有固定伙伴但缺少旧回执的存档展示。
func TestRecruitGuideLegacyHero(t *testing.T) {
	s, _, _ := newDailyCopyTest(t)
	p := &pb.Player{NoviceGuideSchedule: proto.Int32(4)}
	game := &model.GameState{Quests: []string{"quest_1"}, Warriors: []*pb.Warrior{{Id: proto.String("hero_025")}}}
	if s.recruitGuidePlayer(p, game) != p {
		t.Fatal("existing guide hero was presented as an unclaimed reward")
	}
}
