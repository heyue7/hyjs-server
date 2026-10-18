package service

import (
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestNoviceGuideSkipPreservesProgressResourcesAndLaterGuides(t *testing.T) {
	for _, schedule := range []int32{0, 1, 9, 12, 14, 19, 20} {
		player := &pb.Player{PlayerId: proto.Int64(7), NoviceGuideSchedule: proto.Int32(schedule), Level: proto.Int32(8), Diamond: proto.Int64(120), Coin: proto.Int64(500)}
		game := &model.GameState{Quests: []string{"quest_1"}, Items: []*pb.SimpleItem{{Id: proto.String("item_005"), Num: proto.Int32(8)}}, KV: []*pb.KeyValueEntry{{Key: proto.String("existing"), Value: proto.String("keep")}}}
		before := proto.Clone(player).(*pb.Player)
		beforeGame, _ := json.Marshal(game)
		applyNoviceGuideSkip(player, game)
		applyNoviceGuideSkip(player, game)
		want := schedule
		if want < constUtil.NoviceGuideInitialCompleted {
			want = constUtil.NoviceGuideInitialCompleted
		}
		if !NoviceGuideSkipped(game) || len(game.KV) != 2 || player.GetNoviceGuideSchedule() != want {
			t.Fatalf("schedule %d skip did not persist idempotently: %+v", schedule, player)
		}
		before.NoviceGuideSchedule = proto.Int32(want)
		if !proto.Equal(before, player) {
			t.Fatalf("skip changed resources, level or identity at schedule %d", schedule)
		}
		game.KV = game.KV[:1]
		afterGame, _ := json.Marshal(game)
		if string(beforeGame) != string(afterGame) {
			t.Fatalf("skip changed quest or inventory at schedule %d", schedule)
		}
		game.KV = append(game.KV, &pb.KeyValueEntry{Key: proto.String(constUtil.NoviceGuideSkipKey), Value: proto.String("1")})
		persisted, _ := json.Marshal(game)
		var cold model.GameState
		if json.Unmarshal(persisted, &cold) != nil || !NoviceGuideSkipped(&cold) {
			t.Fatal("skip choice did not survive profile JSON reload")
		}
		service := &GameService{}
		if service.recruitGuidePlayer(player, &cold) != player {
			t.Fatal("skipped beginner guide was projected back to recruit after cold reload")
		}
	}
}

func TestNoviceGuideSkipSessionPersistenceAndColdProjection(t *testing.T) {
	f := p1Setup(t)
	p, before := f.profile(t)
	var row db.Profile
	if err := db.GetIns().WithContext(f.ctx).First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SkipNoviceGuide(f.ctx, f.id, make([]byte, 16)); err == nil {
		t.Fatal("invalid session was allowed to skip")
	}
	unchangedP, unchangedG := f.profile(t)
	if !proto.Equal(p, unchangedP) || NoviceGuideSkipped(unchangedG) {
		t.Fatal("failed session changed tutorial state")
	}
	for i := 0; i < 2; i++ {
		result, err := SkipNoviceGuide(f.ctx, f.id, row.Session)
		if err != nil || !result.Skipped || result.NoviceGuideSchedule != 12 {
			t.Fatal("skip or retry failed", err, result)
		}
	}
	coldP, coldG := f.profile(t)
	if !NoviceGuideSkipped(coldG) || coldP.GetNoviceGuideSchedule() != 12 || len(coldG.Warriors) != len(before.Warriors) || len(coldG.Items) != len(before.Items) || coldP.GetDiamond() != p.GetDiamond() || coldP.GetCoin() != p.GetCoin() {
		t.Fatal("cold skip lost state or awarded resources")
	}
	query := &pb.SCPlayer{Player: coldP}
	login := &pb.SCPlayerLogin{Player: proto.Clone(coldP).(*pb.Player)}
	f.s.resumeRecruitGuide(f.ctx, []proto.Message{query, login})
	if query.Player.GetNoviceGuideSchedule() != 12 || login.Player.GetNoviceGuideSchedule() != 12 {
		t.Fatal("cold login restarted initial teaching")
	}
	// A response prepared before the skip must not reintroduce the old UI step.
	stale := &pb.SCPlayer{Player: proto.Clone(p).(*pb.Player)}
	stale.Player.NoviceGuideSchedule = proto.Int32(0)
	f.s.resumeRecruitGuide(f.ctx, []proto.Message{stale})
	if stale.Player.GetNoviceGuideSchedule() != 12 {
		t.Fatal("late pre-skip snapshot restarted initial teaching")
	}
}

func TestChapterRewardColdLoginResumesOnlyClaimedGuide(t *testing.T) {
	f := p1Setup(t)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.NoviceGuideSchedule = proto.Int32(9)
		g.Quests = []string{"quest_1", "quest_5"}
		g.GuideRecruit = &pb.SCWarriorRecruitGuide{}
		g.Copy.PassedChapterInfo = []*pb.CopyChapterInfo{{ChapterId: proto.String("1"), Reward0Claimed: proto.Bool(true)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p, _ := f.profile(t)
	query := &pb.SCPlayer{Player: p}
	login := &pb.SCPlayerLogin{Player: proto.Clone(p).(*pb.Player)}
	f.s.resumeRecruitGuide(f.ctx, []proto.Message{query, login})
	if p.GetNoviceGuideSchedule() != 9 || query.Player.GetNoviceGuideSchedule() != 10 || login.Player.GetNoviceGuideSchedule() != 10 {
		t.Fatal("cold guide snapshot resume or clone contract failed")
	}
	cold, _ := f.profile(t)
	if cold.GetNoviceGuideSchedule() != 10 || cold.GetCoin() != p.GetCoin() || cold.GetDiamond() != p.GetDiamond() {
		t.Fatal("cold resume did not persist or changed reward balance")
	}
}
