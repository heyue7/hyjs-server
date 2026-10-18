package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// This suite performs no database initialization or writes. It covers the
// finite publication and original three-card projection before native testing.
func TestP11ArenaPrizePurePublicationAndThreeCards(t *testing.T) {

	nilGrant := db.P11ArenaPrizeAward{FightID: "empty-grant"}
	emptyGrant := nilGrant
	emptyGrant.GrantPB = []byte{}
	if p11ArenaPrizeAwardHash(nilGrant) != p11ArenaPrizeAwardHash(emptyGrant) {
		t.Fatal("SQL empty-BLOB normalization changed source")
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	start := time.Date(2026, 10, 15, 12, 0, 0, 0, zone)
	end := time.Date(2026, 11, 1, 0, 0, 0, 0, zone)
	policy := db.P11ArenaPrizePolicy{ID: 1, Enabled: true, Version: "local-three-distinct-box-quantities", Source: constUtil.P11ArenaPrizeSource, PoolJSON: `[{"id":"item_027","num":1,"weight":1},{"id":"item_027","num":2,"weight":2},{"id":"item_027","num":3,"weight":3}]`, PublishedAt: start.Add(-time.Second).UnixMilli(), StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), MinLevel: 10, DailyWins: 1, WindowWins: 3}
	pool, err := p11ArenaPrizePool(&policy)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	offer := db.P11ArenaPrizeOffer{Month: "202610", Snapshot: string(raw), SourceHash: p6Digest(raw), CapturedAt: start.UnixMilli()}
	if _, _, err = p11ArenaPrizeOfferRules(&offer); err != nil {
		t.Fatal(err)
	}
	// Explicit source draws pick quantity3 first. Only that actual delta enters
	// ResultObject; the quantity1/2 grey cards stay outside the original consumer.
	chosen := []p11ArenaPrizeDraw{{Index: 2, Value: 3, Total: 6}, {Index: 0, Value: 0, Total: 3}, {Index: 1, Value: 0, Total: 2}}
	cards, draws, err := p11ArenaPrizeCards(pool, chosen)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 3 || cards[0].GetNum() != 3 || cards[1].GetNum() != 1 || cards[2].GetNum() != 2 || len(draws) != 3 {
		t.Fatal("draw source mismatch", cards, draws)
	}
	base := &pb.SCPvpFightResult{FightId: proto.String("source-fight"), Result: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("energy"), Num: proto.Int32(-2)}}}}
	response := proto.Clone(base).(*pb.SCPvpFightResult)
	response.BoxList = cards
	p1AppendGain(response.Result, &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String(cards[0].GetId()), Num: proto.Int32(cards[0].GetNum())}}})
	if len(response.Result.GetSimps()) != 2 || response.Result.Simps[1].GetNum() != 3 || !proto.Equal(p11ArenaPrizeBase(response), base) {
		t.Fatal("preview added assets or changed original base", response)
	}
	if !p11ArenaPrizeLegacy(base) || p11ArenaPrizeLegacy(response) {
		t.Fatal("legacy/new source boundary")
	}
	// Stored draws must replay the same icons without drawing a second reward.
	for card := 0; card < 3; card++ {
		altered := append([]p11ArenaPrizeDraw(nil), chosen...)
		altered[card].Total++
		if _, _, err = p11ArenaPrizeCards(pool, altered); err == nil {
			t.Fatal("altered frozen draw accepted", card)
		}
	}
	for _, modify := range []func(*db.P11ArenaPrizePolicy){
		func(p *db.P11ArenaPrizePolicy) { p.PublishedAt = p.StartAt + 1 },
		func(p *db.P11ArenaPrizePolicy) { p.DailyWins = 0 },
		func(p *db.P11ArenaPrizePolicy) { p.WindowWins = 0 },
		func(p *db.P11ArenaPrizePolicy) { p.MinLevel = 9 },
		func(p *db.P11ArenaPrizePolicy) { p.Source = "unknown-private-probability" },
		func(p *db.P11ArenaPrizePolicy) { p.PoolJSON = `[{"id":"exp","num":1,"weight":1}]` },
		func(p *db.P11ArenaPrizePolicy) { p.PoolJSON = `[{"id":"hero_047","num":1,"weight":1}]` },
		func(p *db.P11ArenaPrizePolicy) { p.PoolJSON = `[{"id":"item_027","num":0,"weight":1}]` },
		func(p *db.P11ArenaPrizePolicy) { p.PoolJSON = `[{"id":"item_027","num":1,"weight":0}]` },
	} {
		changed := policy
		modify(&changed)
		if _, err = p11ArenaPrizePool(&changed); err == nil {
			t.Fatal("invalid publication accepted", changed)
		}
	}
	changed := policy
	changed.EndAt = end.Add(time.Millisecond).UnixMilli()
	badRaw, _ := json.Marshal(changed)
	badOffer := offer
	badOffer.Snapshot = string(badRaw)
	badOffer.SourceHash = p6Digest(badRaw)
	if _, _, err = p11ArenaPrizeOfferRules(&badOffer); err == nil {
		t.Fatal("cross-month publication accepted")
	}
	// The actual source is reproducible and has no dependence on current policy.
	for i := 0; i < 8; i++ {
		sampled, replay, e := p11ArenaPrizeCards(pool, nil)
		if e != nil {
			t.Fatal(e)
		}
		copyCards, _, e := p11ArenaPrizeCards(pool, replay)
		if e != nil {
			t.Fatal(e)
		}
		sum := int32(0)
		for j := range sampled {
			sum += sampled[j].GetNum()
			if !proto.Equal(sampled[j], copyCards[j]) {
				t.Fatal("non-reproducible source")
			}
		}
		if sum != 6 {
			t.Fatal("local without-replacement preview changed", sampled)
		}
	}
}
