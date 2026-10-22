package handler

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

// TestWarriorFeedback47OriginalBatchAdmission covers the shared HTTP/TCP gate
// that precedes service ownership checks on the original multi-select request.
func TestWarriorFeedback47OriginalBatchAdmission(t *testing.T) {
	for _, count := range []int{6, 20} {
		b := &pb.CSWarriorStrengthen{Guid: proto.String("target")}
		for i := 0; i < count; i++ {
			b.Cost = append(b.Cost, &pb.BaseResource{Id: proto.String("hero_exp_001"), Guid: proto.String(fmt.Sprintf("material-%d", i))})
		}
		raw, err := proto.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		decoded := &pb.CSWarriorStrengthen{}
		if err = proto.Unmarshal(raw, decoded); err != nil {
			t.Fatal(err)
		}
		packet := &model.GamePacket{Body: decoded, PlayerId: 1, ZoneId: 1, Timestamp: 47001}
		if err = ValidateGameRequest(packet); err != nil {
			t.Fatalf("original %d-material request: %v", count, err)
		}
		decoded.Cost[0].Num = proto.Int32(2)
		if ValidateGameRequest(packet) == nil {
			t.Fatal("invalid material count admitted")
		}
		decoded.Cost[0].Num = nil
		decoded.Cost[0].Guid = nil
		if ValidateGameRequest(packet) == nil {
			t.Fatal("missing material GUID admitted")
		}
	}
	if ValidateGameRequest(&model.GamePacket{Body: &pb.CSWarriorStrengthen{Guid: proto.String("target")}}) == nil {
		t.Fatal("empty batch admitted")
	}
}
