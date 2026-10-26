package service

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13UFGateImpactExactSourceAndTransition(t *testing.T) {
	before := &pb.InstUFGate{Guid: proto.Int64(98), Id: proto.Int32(2), Camp: proto.Int32(0), Level: proto.Int32(1), Hp: proto.Float64(10000), MaxHp: proto.Float64(10000)}
	after := proto.Clone(before).(*pb.InstUFGate)
	after.Hp = proto.Float64(9600)
	rawBefore, _ := proto.Marshal(before)
	rawAfter, _ := proto.Marshal(after)
	row := db.P13UnionFightGateImpact{MatchID: "impact-test", GateGUID: 98, Ordinal: 1, Kind: constUtil.P13UFGateImpactCar, OfferHash: strings.Repeat("a", 64), CarGUID: 91, HitOrdinal: 1, Percent: -4, LogicalAt: 1000, AppliedAt: 1001, Before: rawBefore, After: rawAfter, BeforeHash: p6Digest(rawBefore), AfterHash: p6Digest(rawAfter)}
	row.ID = p6Digest([]byte(fmt.Sprintf("%s/%d/%d/gate-impact", row.MatchID, row.GateGUID, row.Ordinal)))
	row.SourceHash = p13UFGateImpactHash(&row)
	if _, _, err := p13UFGateImpactVerify(&row); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*db.P13UnionFightGateImpact){
		func(r *db.P13UnionFightGateImpact) { r.GateGUID++ },
		func(r *db.P13UnionFightGateImpact) { r.Ordinal++ },
		func(r *db.P13UnionFightGateImpact) { r.After = append([]byte(nil), r.After...); r.After[0] ^= 1 },
		func(r *db.P13UnionFightGateImpact) { r.Percent = -5; r.SourceHash = p13UFGateImpactHash(r) },
		func(r *db.P13UnionFightGateImpact) { r.AppliedAt = 999; r.SourceHash = p13UFGateImpactHash(r) },
		func(r *db.P13UnionFightGateImpact) { r.PlayerID = 7; r.SourceHash = p13UFGateImpactHash(r) },
	} {
		changed := row
		mutate(&changed)
		if _, _, err := p13UFGateImpactVerify(&changed); err == nil {
			t.Fatal("damaged gate source or transition accepted")
		}
	}
}
