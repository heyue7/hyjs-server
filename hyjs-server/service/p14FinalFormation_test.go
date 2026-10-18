package service

import (
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p14FinalFormationPureOffer() *db.P14FinalFormationOffer {
	offer := &db.P14FinalFormationOffer{SeasonIndex: 61, RoundStage: 8, Version: "prepublished-local-first-final-v1", Source: constUtil.P14FinalFormationSource, PublishedAt: 1000, QualificationHash: strings.Repeat("a", 64), StageStart: 10000, FightStart: 1000000, FightEnd: 2000000, LockAt: 700000}
	offer.SourceHash = p14FinalFormationOfferHash(offer)
	return offer
}

// The original rule locks uploads five minutes before battle, including the
// exact boundary. No round result or winner is an input to this test.
func TestP14FinalFormationOriginalFiveMinuteLockAndPrepublishedCalendar(t *testing.T) {
	offer := p14FinalFormationPureOffer()
	for _, at := range []int64{offer.StageStart - 1, offer.StageStart, offer.LockAt - 1, offer.LockAt, offer.FightStart, offer.FightEnd} {
		if err := p14FinalFormationCaptureAllowed(offer, at); (err == nil) != (at >= offer.StageStart && at < offer.LockAt) {
			t.Fatalf("capture at %d: %v", at, err)
		}
	}
	for _, mutate := range []func(*db.P14FinalFormationOffer){
		func(o *db.P14FinalFormationOffer) { o.LockAt++ },
		func(o *db.P14FinalFormationOffer) { o.PublishedAt = o.StageStart + 1 },
		func(o *db.P14FinalFormationOffer) { o.RoundStage = 10 },
		func(o *db.P14FinalFormationOffer) { o.QualificationHash = "" },
	} {
		changed := *offer
		mutate(&changed)
		changed.SourceHash = p14FinalFormationOfferHash(&changed)
		if p14FinalFormationValidateOffer(&changed) == nil {
			t.Fatal("unproven round, qualification or altered original lock")
		}
	}
}

func TestP14FinalFormationPurposeBindsRoundCommonQualificationAndOfferPreservesOldJSON(t *testing.T) {
	old := model.P14SnapshotImportRequest{ProtocolVersion: 2, SeasonIndex: 61, SourceServerID: 1, DestinationServerID: 2, PlayerID: 7, Revision: 1}
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"roundStage", "finalQualificationHash", "finalOfferHash"} {
		if strings.Contains(string(raw), field) {
			t.Fatal("legacy v1/v2 payload bytes gained final metadata", field)
		}
	}
	request := old
	request.ProtocolVersion = 3
	request.RoundStage = 8
	request.FinalQualificationHash = strings.Repeat("a", 64)
	request.FinalOfferHash = strings.Repeat("b", 64)
	id := p14FinalFormationID(request)
	if id == p14PromotionFormationID(old) {
		t.Fatal("purpose2 and purpose3 collided")
	}
	for _, mutate := range []func(*model.P14SnapshotImportRequest){
		func(r *model.P14SnapshotImportRequest) { r.RoundStage = 10 },
		func(r *model.P14SnapshotImportRequest) { r.FinalQualificationHash = strings.Repeat("c", 64) },
		func(r *model.P14SnapshotImportRequest) { r.FinalOfferHash = strings.Repeat("d", 64) },
		func(r *model.P14SnapshotImportRequest) { r.SourceServerID = 2; r.DestinationServerID = 1 },
	} {
		changed := request
		mutate(&changed)
		if p14FinalFormationID(changed) == id {
			t.Fatal("round/source binding omitted from delivery identity")
		}
	}
}

func TestP14FinalFormationFrozenProtoCannotBorrowJointIdentityOrLateCapture(t *testing.T) {
	offer := p14FinalFormationPureOffer()
	frame, _ := proto.Marshal(&pb.FormationSimpleInfo{FightPower: proto.Int32(330)})
	battle, _ := proto.Marshal(&pb.BattlePlayer{PlayerId: proto.Int64(1), ServerId: proto.Int32(2), Formation: []*pb.BattleHero{{}}})
	row := &db.P14FinalFormation{SeasonIndex: 61, RoundStage: 8, ServerID: 2, PlayerID: 1, Revision: 1, CapturedAt: offer.LockAt - 1, FightPower: 330, Snapshot: frame, Battle: battle, SnapshotHash: p14SnapshotDigest(frame, battle), QualificationHash: offer.QualificationHash, OfferHash: offer.SourceHash, Source: constUtil.P14FinalFormationSource}
	row.SourceHash = p14FinalFormationHash(row)
	if _, err := p14FinalFormationRead(row, offer); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*db.P14FinalFormation){
		func(r *db.P14FinalFormation) { r.ServerID = 1 },
		func(r *db.P14FinalFormation) { r.CapturedAt = offer.LockAt },
		func(r *db.P14FinalFormation) { r.QualificationHash = strings.Repeat("e", 64) },
		func(r *db.P14FinalFormation) { r.OfferHash = strings.Repeat("f", 64) },
	} {
		changed := *row
		mutate(&changed)
		changed.SourceHash = p14FinalFormationHash(&changed)
		if _, err := p14FinalFormationRead(&changed, offer); err == nil {
			t.Fatal("invalid capture admitted")
		}
	}
}
