package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

// The actual realm is a trusted service setting. A numeric local player ID
// cannot turn an explicitly foreign profile request into a local lookup.
func TestP10ProfileConfiguredLocalRealmRejectsForeignIdentity(t *testing.T) {
	f, target, _ := p10Setup(t)
	f.s.ServerID = 2
	query := func(server int32) (*pb.SCFindPlayerInfo, error) {
		messages, handled, err := f.s.DispatchP10Profile(f.ctx, f.id, &pb.CSFindPlayerInfo{PlayerId: proto.Int64(target), ServerId: proto.Int32(server)})
		if !handled {
			t.Fatal("original profile dispatch lost")
		}
		if err != nil {
			return nil, err
		}
		return messages[0].(*pb.SCFindPlayerInfo), nil
	}
	local, err := query(2)
	if err != nil || local.GetFormationInfo().GetName() != "社交乙" {
		t.Fatal("configured realm2 local player unavailable", local, err)
	}
	defaultLocal, err := query(0)
	if err != nil || !proto.Equal(local, defaultLocal) {
		t.Fatal("original unspecified local realm changed", err)
	}
	for _, foreign := range []int32{-1, 1, 3} {
		if _, err = query(foreign); err == nil {
			t.Fatal("explicit foreign identity leaked local numeric ID", foreign)
		}
	}
}
