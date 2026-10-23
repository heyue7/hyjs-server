package service

import (
	pb "hyjs-server/model/protocol"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestBulk58ActivityExactDiscountTotals(t *testing.T) {
	f := p4Setup(t)
	product := p4Product{Cost: "diamond,3", DiscountBps: 5000}
	var item pb.ActivityShopItem
	if err := f.s.p4BatchPurchasePrices(&item, product, &p4CommerceProgress{}, 1, 10); err != nil {
		t.Fatal(err)
	}
	var totals []int32
	for raw := item.ProtoReflect().GetUnknown(); len(raw) > 0; {
		number, kind, n := protowire.ConsumeTag(raw)
		if number != 100 || kind != protowire.BytesType || n < 0 {
			t.Fatal("invalid compatible price extension")
		}
		payload, n2 := protowire.ConsumeBytes(raw[n:])
		if n2 < 0 {
			t.Fatal("invalid total quote")
		}
		var resource pb.BaseResource
		if err := proto.Unmarshal(payload, &resource); err != nil {
			t.Fatal(err)
		}
		if resource.GetId() != "diamond" {
			t.Fatal("wrong cost resource")
		}
		totals = append(totals, resource.GetNum())
		raw = raw[n+n2:]
	}
	// 3 at 50% rounds to 2 for one, while ten rounds to 15, not 20.
	if len(totals) != 10 || totals[0] != 2 || totals[9] != 15 {
		t.Fatal("batch totals were inferred from rounded unit", totals)
	}
}
