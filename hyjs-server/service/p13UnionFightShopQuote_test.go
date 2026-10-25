package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

// Fixtures retain the exact quote returned to the original client. Purchase
// retries reuse this value; this helper never manufactures or refreshes a quote.
func p13UFTestShopQuote(t *testing.T, quotes map[string]string, item string) string {
	t.Helper()
	quote := quotes[item]
	if quote == "" {
		t.Fatalf("original shop list omitted item %s", item)
	}
	return quote
}

func p13UFTestShopQuoteMap(t *testing.T, list *pb.SCUFShopList) map[string]string {
	t.Helper()
	quotes := map[string]string{}
	for _, item := range list.Items {
		if item.GetItemId() == "" || item.GetShopItemId() == "" || quotes[item.GetItemId()] != "" {
			t.Fatal("original shop list has missing or duplicate quote identity")
		}
		quotes[item.GetItemId()] = item.GetShopItemId()
	}
	return quotes
}

func p13UFTestShopQuotes(t *testing.T, f *p1Fixture, id int64, npc string, now time.Time) map[string]string {
	t.Helper()
	replies, err := f.s.p13UFShopList(f.ctx, id, &pb.CSUFShopList{NpcId: proto.String(npc)}, now)
	if err != nil || len(replies) != 1 {
		t.Fatal("original shop quote list", err)
	}
	list, ok := replies[0].(*pb.SCUFShopList)
	if !ok {
		t.Fatal("original shop quote response type")
	}
	return p13UFTestShopQuoteMap(t, list)
}
