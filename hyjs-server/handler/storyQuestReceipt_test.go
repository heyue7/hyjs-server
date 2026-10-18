package handler

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestStoryQuestReceiptOriginalSocketAndIngress 保留原无戳TCP，同时在公共入口拒绝畸形任务标识。
func TestStoryQuestReceiptOriginalSocketAndIngress(t *testing.T) {
	for _, stamp := range []int64{0, 100} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 4, Timestamp: stamp, Body: &pb.CSQuestReward{QuestId: proto.String("quest_16")}}); err != nil {
			t.Fatal("original socket or HTTP quest was rejected", stamp, err)
		}
	}
	for _, quest := range []string{"", " quest_16", "quest_16 ", "quest_16\n", "quest\x00_16", strings.Repeat("q", 65)} {
		if ValidateGameRequest(&model.GamePacket{PlayerId: 4, Timestamp: 100, Body: &pb.CSQuestReward{QuestId: proto.String(quest)}}) == nil {
			t.Fatalf("malformed quest identifier reached dispatch: %q", quest)
		}
	}
	if ValidateGameRequest(&model.GamePacket{PlayerId: 4, Timestamp: -1, Body: &pb.CSQuestReward{QuestId: proto.String("quest_16")}}) == nil {
		t.Fatal("negative quest timestamp reached dispatch")
	}
}
