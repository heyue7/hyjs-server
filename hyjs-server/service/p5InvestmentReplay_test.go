package service

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP5InvestmentNoCostReplayPreservesHistoricalBytes protects old unsorted
// investment records while keeping real additional costs canonical.
func TestP5InvestmentNoCostReplayPreservesHistoricalBytes(t *testing.T) {
	game := &model.GameState{}
	before := &pb.Warrior{Id: proto.String("hero_001"), Guid: proto.String("actual-investment"), Level: proto.Int32(1)}
	after := proto.Clone(before).(*pb.Warrior)
	after.Level = proto.Int32(2)
	costs := []*pb.BaseResource{{Id: proto.String("item_449"), Num: proto.Int32(3)}, {Id: proto.String("coin"), Num: proto.Int32(100)}, {Id: proto.String("item_445"), Num: proto.Int32(2)}}
	if err := RecordP5Investment(game, before.GetGuid(), before, after, costs); err != nil {
		t.Fatal(err)
	}
	var state p5Investment
	json.Unmarshal([]byte(p5KV(game, "p5.invest:"+before.GetGuid())), &state)
	// Simulate a valid old record's established array order without modifying costs.
	state.Costs = []*pb.BaseResource{state.Costs[2], state.Costs[0], state.Costs[1]}
	old, _ := json.Marshal(state)
	p5SetKV(game, "p5.invest:"+before.GetGuid(), string(old))
	for i := 0; i < 100; i++ {
		if err := RecordP5Investment(game, before.GetGuid(), after, after, nil); err != nil {
			t.Fatal(err)
		}
		if p5KV(game, "p5.invest:"+before.GetGuid()) != string(old) {
			t.Fatal("zero-cost replay changed established investment bytes")
		}
	}
	changed := proto.Clone(after).(*pb.Warrior)
	changed.Level = proto.Int32(3)
	if err := RecordP5Investment(game, before.GetGuid(), changed, changed, nil); err == nil {
		t.Fatal("untracked instance passed no-cost replay guard")
	}
	if err := RecordP5Investment(game, before.GetGuid(), after, changed, []*pb.BaseResource{{Id: proto.String("item_445"), Num: proto.Int32(1)}}); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal([]byte(p5KV(game, "p5.invest:"+before.GetGuid())), &state)
	if len(state.Costs) != 3 || state.Costs[0].GetId() != "coin" || state.Costs[1].GetId() != "item_445" || state.Costs[1].GetNum() != 3 || state.Costs[2].GetId() != "item_449" {
		t.Fatal("new paid costs were not accumulated and sorted", state.Costs)
	}
}
