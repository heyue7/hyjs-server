package model

import (
	pb "hyjs-server/model/protocol"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestPurchaseBatchWireCompatibility(t *testing.T) {
	field := func(value uint64) []byte {
		return protowire.AppendVarint(protowire.AppendTag(nil, 100, protowire.VarintType), value)
	}
	for _, test := range []struct {
		name string
		raw  []byte
		want int32
	}{
		{"legacy", nil, 1}, {"one", field(1), 1}, {"ten", field(10), 10}, {"maximum", field(100), 100},
		{"zero", field(0), 0}, {"too-many", field(101), 0}, {"negative", field(^uint64(0)), 0},
		{"duplicate", append(field(2), field(3)...), 0},
		{"wrong-wire", protowire.AppendBytes(protowire.AppendTag(nil, 100, protowire.BytesType), []byte{1}), 0},
		{"truncated", []byte{0xa0, 0x06, 0x80}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := &pb.CSGetOpenServerReward{ItemId: proto.Int32(183), HeroId: proto.String("frag_hero_019")}
			wire, err := proto.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			wire = append(wire, test.raw...)
			var request pb.CSGetOpenServerReward
			if err = proto.Unmarshal(wire, &request); err != nil {
				if test.want == 0 {
					return
				}
				t.Fatal(err)
			}
			got, err := GamePurchaseCount(&request)
			if test.want == 0 {
				if err == nil {
					t.Fatal("accepted invalid wire quantity")
				}
				return
			}
			if err != nil || got != test.want || request.GetItemId() != 183 || request.GetHeroId() != "frag_hero_019" {
				t.Fatal("original fields or quantity changed", got, err)
			}
		})
	}
}
