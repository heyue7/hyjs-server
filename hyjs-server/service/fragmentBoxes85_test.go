package service

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

func TestFragmentBoxes85BatchChoicesAndReceipt(t *testing.T) {
	f := p1Setup(t)
	s, ctx, id := f.s, f.ctx, f.id
	if err := s.InitializeDailyCopy(ctx); err != nil {
		t.Fatal(err)
	}
	initializeP2Test(t, s, ctx)
	stamp := int64(85000)
	for _, box := range []string{"item_120", "item_121", "item_122", "item_156", "item_348"} {
		var option map[string]string
		for _, row := range s.Tables["ItemUse"] {
			if row["itemId"] == box {
				option = row
				break
			}
		}
		if option == nil {
			t.Fatal("missing original choices", box)
		}
		pair := strings.Split(option["result"], ",")
		if len(pair) != 2 || !strings.HasPrefix(pair[0], "frag_") {
			t.Fatal("unexpected reward", option)
		}
		perBox, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || perBox < 1 {
			t.Fatal(option)
		}
		index := int32(tableNumber(option, "odds"))
		for _, quantity := range []int32{1, 10, 32, 100} {
			p2Funding(t, s, ctx, id, fmt.Sprintf("%s,%d", box, quantity))
			before, gameBefore := p2Profile(t, id)
			request := &pb.CSItemUse{CostItem: p2Item(box, quantity), GetItemIndex: proto.Int32(index)}
			stamp++
			response, err := s.itemUse(ctx, id, request, stamp)
			if err != nil {
				t.Fatalf("%s x%d: %v", box, quantity, err)
			}
			result := response[0].(*pb.SCItemUse)
			var gained int64
			for _, item := range result.GetGet().GetSimps() {
				if item.GetId() != pair[0] {
					t.Fatal("wrong selected fragment", item)
				}
				gained += int64(item.GetNum())
			}
			if result.GetCost().GetId() != box || result.GetCost().GetNum() != quantity || gained != int64(quantity)*perBox {
				t.Fatal("batch response mismatch", box, quantity, result)
			}
			after, gameAfter := p2Profile(t, id)
			if simpleOwned(before, gameBefore, box)-simpleOwned(after, gameAfter, box) != int64(quantity) || simpleOwned(after, gameAfter, pair[0])-simpleOwned(before, gameBefore, pair[0]) != gained {
				t.Fatal("batch assets mismatch", box, quantity)
			}
			retry, err := s.itemUse(ctx, id, request, stamp)
			if err != nil || !proto.Equal(response[0], retry[0]) {
				t.Fatal("receipt retry", box, err)
			}
			replayed, replayGame := p2Profile(t, id)
			if !proto.Equal(after, replayed) || simpleOwned(replayed, replayGame, pair[0]) != simpleOwned(after, gameAfter, pair[0]) {
				t.Fatal("replayed batch changed assets", box)
			}
		}
		p2Funding(t, s, ctx, id, box+",2")
		before, gameBefore := p2Profile(t, id)
		stamp++
		if _, err := s.itemUse(ctx, id, &pb.CSItemUse{CostItem: p2Item(box, 3), GetItemIndex: proto.Int32(index)}, stamp); err == nil {
			t.Fatal("insufficient box accepted", box)
		}
		stamp++
		if _, err := s.itemUse(ctx, id, &pb.CSItemUse{CostItem: p2Item(box, 1), GetItemIndex: proto.Int32(-1)}, stamp); err == nil {
			t.Fatal("invalid choice accepted", box)
		}
		after, gameAfter := p2Profile(t, id)
		if !proto.Equal(before, after) || simpleOwned(before, gameBefore, box) != simpleOwned(after, gameAfter, box) || simpleOwned(before, gameBefore, pair[0]) != simpleOwned(after, gameAfter, pair[0]) {
			t.Fatal("failed choice consumed assets", box)
		}
	}
}
