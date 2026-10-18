package service

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

func TestP16UnavailableContentKeepsPlayableEntrypoints(t *testing.T) {
	for _, target := range []string{"field_003", "field_004", "field_005", "field_006"} {
		request := &pb.CSTransferScene{Target: proto.String(target)}
		before, _ := proto.Marshal(request)
		err := GuardP16UnavailableContent(request)
		if err == nil || !strings.HasPrefix(err.Error(), "暂未开放") {
			t.Fatalf("missing field %s entered: %v", target, err)
		}
		after, _ := proto.Marshal(request)
		if string(before) != string(after) {
			t.Fatal("entry guard mutated the original request")
		}
	}
	for _, request := range []proto.Message{&pb.CSUFStrengthBuildingInfo{}, &pb.CSUFStrengthBuilding{}} {
		if err := GuardP16UnavailableContent(request); err == nil || !strings.HasPrefix(err.Error(), "暂未开放") {
			t.Fatalf("missing war-building consumer entered: %T %v", request, err)
		}
	}
	// 保持已可玩的低阶野外、普通公会培养以及副本冻结回执原处理路径。
	for _, request := range []proto.Message{
		&pb.CSTransferScene{Target: proto.String("field_001")},
		&pb.CSTransferScene{Target: proto.String("field_002")},
		&pb.CSTransferScene{Target: proto.String("town_main")},
		&pb.CSTransferScene{Target: proto.String("town_union")},
		&pb.CSUnionHeroStatueUpgrade{}, &pb.CSUnionGuardTreeUpgrade{},
		&pb.CSRushFightStart{}, &pb.CSRushFightEnd{}, &pb.CSInterExpedition{},
	} {
		if err := GuardP16UnavailableContent(request); err != nil {
			t.Fatalf("unrelated or playable entry blocked: %T %v", request, err)
		}
	}
}
