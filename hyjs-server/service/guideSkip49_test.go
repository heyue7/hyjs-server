package service

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGuideSkip49LevelUnorderedPersistenceAndProtection(t *testing.T) {
	f := p1Setup(t)
	var row db.Profile
	db.GetIns().First(&row, f.id)
	if _, err := f.s.SkipGuide(f.ctx, f.id, row.Session, "guide_pet"); err == nil {
		t.Fatal("future level guide accepted")
	}
	if _, err := f.s.SkipGuide(f.ctx, f.id, row.Session, "guide_pvp_shop"); err == nil {
		t.Fatal("chained future guide bypassed parent level")
	}
	if _, err := f.s.SkipGuide(f.ctx, f.id, row.Session, "unknown"); err == nil {
		t.Fatal("unknown guide accepted")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(34); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, guide := range []string{"guide_pvp", "guide_pvp_shop", "guide_recycle", "guide_field_fish", "guide_pet", "guide_sweep"} {
		before, gameBefore := f.profile(t)
		for range 2 {
			out, err := f.s.SkipGuide(f.ctx, f.id, row.Session, guide)
			if err != nil || !out.Skipped || out.GuideID != guide {
				t.Fatalf("%s skip: %v %v", guide, out, err)
			}
		}
		after, gameAfter := f.profile(t)
		before.NoviceGuideSchedule = after.NoviceGuideSchedule
		gameBefore.KV = gameAfter.KV
		beforeGame, _ := json.Marshal(gameBefore)
		afterGame, _ := json.Marshal(gameAfter)
		if !proto.Equal(before, after) || string(beforeGame) != string(afterGame) {
			t.Fatal("guide skip changed business state")
		}
		count := 0
		for _, kv := range gameAfter.KV {
			if kv.GetKey() == constUtil.GuideSkipKeyPrefix+guide && kv.GetValue() == "1" {
				count++
			}
		}
		if count != 1 {
			t.Fatal("skip retry duplicated or lost per-guide choice")
		}
	}
	f.restart(t)
	player, game := f.profile(t)
	if player.GetNoviceGuideSchedule() != 20 || NoviceGuideSkipped(game) {
		t.Fatal("level skip rewrote initial choice or lost cold progress")
	}
	before := proto.Clone(player)
	if _, err := f.s.SkipGuide(f.ctx, f.id, make([]byte, 16), "guide_union"); err == nil {
		t.Fatal("stale guide session accepted")
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("rejected guide changed state")
	}
}
