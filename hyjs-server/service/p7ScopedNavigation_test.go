package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Keep both original assets and opaque server-provided task identity distinct.
// The original ReadyPage hides missing star2/3 choices; one real star1 is enough.
func TestP7ScopedOriginalFloorAndBattleRoomAreIndependent(t *testing.T) {
	f, cfg, request := p7ScopedFixture(t, constUtil.P7RushTrial)
	cfg.Scene = constUtil.P7OriginalScopedBattleScene
	if err := db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	before, game := f.profile(t)
	replies, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushInfo{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if replies[0].(*pb.SCGetRushInfo).GetCurFloorSceneName() != constUtil.P7OriginalScopedFloorScene {
		t.Fatal("RoomRush got the battle scene")
	}
	after, again := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, game, again) {
		t.Fatal("room eligibility query changed actual profile")
	}
	start, err := f.s.p7Start(f.ctx, f.id, request, 1450, false)
	if err != nil || start.GetFightScene() != constUtil.P7OriginalScopedBattleScene {
		t.Fatal("Start lost original battlefield asset", err)
	}
}
