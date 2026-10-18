package service

import (
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP8OriginalGuideResources uses finite level18 eligibility and actual original transfer/guide grants.
func TestP8OriginalGuideResources(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP8Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP8Gather(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(18)
		p.NoviceGuideSchedule = proto.Int32(9)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	call := func(fish bool, stamp int64) ([]proto.Message, error) {
		if fish {
			return f.s.p8GuideResource(f.ctx, f.id, &pb.CSGuideFish{}, stamp, true)
		}
		return f.s.p8GuideResource(f.ctx, f.id, &pb.CSGuideMine{}, stamp, false)
	}
	if _, err := call(true, 1); err == nil {
		t.Fatal("town tutorial award accepted")
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(false, 2); err == nil {
		t.Fatal("mine before original fish accepted")
	}
	if _, err := call(true, 3); err == nil {
		t.Fatal("guide award away from original pos0 accepted")
	}
	move := func(x, y, z float32) {
		t.Helper()
		if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
			g.Position.Current = &pb.PointNew{X: proto.Float32(x), Y: proto.Float32(y), Z: proto.Float32(z)}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	move(7, -.5, 17)
	var (
		wg     sync.WaitGroup
		errors = make(chan error, 8)
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := call(true, 4); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	p, g := f.profile(t)
	count := func(items []*pb.SimpleItem, key string) int32 {
		var total int32
		for _, item := range items {
			if item.GetId() == key {
				total += item.GetNum()
			}
		}
		return total
	}
	if p.GetNoviceGuideSchedule() != constUtil.P8GuideFishSchedule || p1Count(t, &db.P8GatherReceipt{}, "player_id=?", f.id) != 1 {
		t.Fatal("original fish completion/receipt not unique")
	}
	for _, key := range []string{"FishItem_007", "FishItem_013", "FishItem_019"} {
		if count(g.Cooking, key) != 1 {
			t.Fatal("original fish grant differs", key, g.Cooking)
		}
	}
	if _, err := call(true, 5); err == nil {
		t.Fatal("fresh timestamp repeated lifetime fish grant")
	}
	f.restart(t)
	move(20.84, -.45, 7.61)
	response, err := call(false, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(response[0].(*pb.SCGuideMine).GetGet().GetSimps()) != 1 || response[0].(*pb.SCGuideMine).GetGet().GetSimps()[0].GetNum() != 2 {
		t.Fatal("original two ore grant differs")
	}
	f.restart(t)
	p, g = f.profile(t)
	if p.GetNoviceGuideSchedule() != constUtil.P8GuideMineSchedule || count(g.Gems, "MineItem_007") != 2 || g.Field.Side.GetTiredness() != 200 {
		t.Fatal("cold guide completion/inventory/fatigue differs")
	}
	if _, err = call(false, 7); err == nil {
		t.Fatal("fresh timestamp repeated lifetime mine grant")
	}
	if _, err = call(false, 6); err != nil {
		t.Fatal("committed original mine packet failed cold", err)
	}
}
