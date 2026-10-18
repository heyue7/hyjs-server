package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p14PromotionFormationSetup(t *testing.T) (*p1Fixture, []int64, time.Time, time.Time) {
	t.Helper()
	f, ids, start, end := p14PromotionSetup(t)
	if err := f.s.InitializeP14Qualification(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("1=1").Delete(&db.P14PromotionFormation{}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Where("1=1").Delete(&db.P14PromotionFormation{}) })
	return f, ids, start, end
}

func TestP14PromotionFormationActualQualifiedUploadOriginalQueryColdAndImmutableAudition(t *testing.T) {
	f, ids, start, end := p14PromotionFormationSetup(t)
	id := ids[0]
	at := start.Add(time.Minute)
	if _, err := f.s.p14Register(f.ctx, id, &pb.CSKFEnterFrightGame{}, 5001, at); err == nil {
		t.Fatal("default-off promotion upload")
	}
	if err := db.GetIns().Model(&db.P14PromotionPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	var beforeRoster db.P14AuditionRoster
	var beforeMember db.P14Member
	db.GetIns().First(&beforeRoster, "season_index=7 AND server_id=1 AND player_id=?", id)
	db.GetIns().First(&beforeMember, "season_index=7 AND server_id=1 AND player_id=?", id)
	fallback, err := f.s.p14Query(f.ctx, id, &pb.CSKFPlayerInfo{PlayerId: proto.Int64(id), ServerId: proto.Int32(1), Type: proto.Int32(2)}, at)
	original := &pb.FormationSimpleInfo{}
	if proto.Unmarshal(beforeRoster.Snapshot, original) != nil || err != nil || !proto.Equal(fallback[0].(*pb.SCKFPlayerInfo).FormationInfo, original) {
		t.Fatal("real qualified fallback", err)
	}
	if _, err = f.s.p14Register(f.ctx, ids[4], &pb.CSKFEnterFrightGame{}, 5002, at); err == nil {
		t.Fatal("outsider uploaded through roster membership")
	}
	if err = updateProfile(f.ctx, id, func(p *pb.Player, game *model.GameState) error {
		p.Name = proto.String("真实晋级新阵容")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		out, err := f.s.p14Register(f.ctx, id, &pb.CSKFEnterFrightGame{}, 5003, at)
		if err != nil || out[0].(*pb.SCKFEnterFrightGame).GetLastFightPower() <= 0 {
			t.Fatal("original stage6 upload", err)
		}
	}
	var row db.P14PromotionFormation
	if err = db.GetIns().First(&row, "season_index=7 AND server_id=1 AND player_id=?", id).Error; err != nil || row.Revision != 1 {
		t.Fatal("exact actual version", err, row.Revision)
	}
	var outboxes []db.P14Delivery
	db.GetIns().Find(&outboxes)
	promotion := 0
	for _, delivery := range outboxes {
		var request model.P14SnapshotImportRequest
		if json.Unmarshal(delivery.Payload, &request) == nil && request.ProtocolVersion == constUtil.P14PromotionFormationProtocolVersion {
			promotion++
			if request.DeliveryID != p14PromotionFormationID(request) || request.SnapshotHash != row.SnapshotHash {
				t.Fatal("signed purpose not bound to original current capture")
			}
		}
	}
	if promotion != 1 {
		t.Fatal("retry generated promotion outboxes", promotion)
	}
	var afterRoster db.P14AuditionRoster
	var afterMember db.P14Member
	db.GetIns().First(&afterRoster, "season_index=7 AND server_id=1 AND player_id=?", id)
	db.GetIns().First(&afterMember, "season_index=7 AND server_id=1 AND player_id=?", id)
	if beforeRoster.SourceHash != afterRoster.SourceHash || beforeRoster.SnapshotHash != afterRoster.SnapshotHash || beforeMember.SnapshotHash != afterMember.SnapshotHash || beforeMember.Revision != afterMember.Revision {
		t.Fatal("promotion rewrote signup/audition")
	}
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	out, err := f.s.p14Query(f.ctx, id, &pb.CSKFPlayerInfo{PlayerId: proto.Int64(id), ServerId: proto.Int32(1), Type: proto.Int32(2)}, at.Add(time.Second))
	if err != nil || out[0].(*pb.SCKFPlayerInfo).FormationInfo.GetName() != "真实晋级新阵容" {
		t.Fatal("cold current promotion formation", err)
	}
	if _, err = f.s.p14Register(f.ctx, id, &pb.CSKFEnterFrightGame{}, 5004, end); err == nil {
		t.Fatal("closed stage6 accepted new upload")
	}
	if _, err = f.s.p14Register(f.ctx, id, &pb.CSKFEnterFrightGame{}, 5003, end); err != nil {
		t.Fatal("completed receipt lost at stage transition", err)
	}
	var scores int64
	db.GetIns().Model(&db.P14PromotionState{}).Count(&scores)
	if scores != 0 {
		t.Fatal("formation capture changed points or score")
	}
}

func TestP14PromotionFormationSignedImportRequiresRealForeignQualification(t *testing.T) {
	f, ids, start, _ := p14PromotionFormationSetup(t)
	if err := db.GetIns().Model(&db.P14PromotionPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterFrightGame{}, 5005, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var deliveries []db.P14Delivery
	db.GetIns().Find(&deliveries)
	var request model.P14SnapshotImportRequest
	for _, delivery := range deliveries {
		var item model.P14SnapshotImportRequest
		if json.Unmarshal(delivery.Payload, &item) == nil && item.ProtocolVersion == constUtil.P14PromotionFormationProtocolVersion {
			request = item
		}
	}
	request.SourceServerID, request.DestinationServerID = 2, 1
	battle := &pb.BattlePlayer{}
	proto.Unmarshal(request.Battle, battle)
	battle.ServerId = proto.Int32(2)
	request.Battle, _ = proto.Marshal(battle)
	request.SnapshotHash = p14SnapshotDigest(request.Formation, request.Battle)
	request.DeliveryID = p14PromotionFormationID(request)
	raw, _ := json.Marshal(request)
	var peer db.P14TrustedPeer
	db.GetIns().First(&peer, "server_id=2")
	_, err := f.s.ImportP14Snapshot(f.ctx, request, raw, P14SignPayload(peer.SigningKey, raw), start.Add(2*time.Minute))
	if err == nil {
		t.Fatal("trusted signature alone fabricated foreign promotion资格")
	}
	var count int64
	db.GetIns().Model(&db.P14PromotionFormation{}).Where("server_id=2").Count(&count)
	if count != 0 {
		t.Fatal("failed import persisted partial formation")
	}
	// A purpose-swapped payload cannot use the original registration receipt ID.
	request.ProtocolVersion = constUtil.P14ProtocolVersion
	raw, _ = json.Marshal(request)
	if _, err = f.s.ImportP14Snapshot(f.ctx, request, raw, P14SignPayload(peer.SigningKey, raw), start.Add(2*time.Minute)); err == nil {
		t.Fatal("purpose2 ID accepted as signup")
	}
}
