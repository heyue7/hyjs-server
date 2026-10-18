package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func p14Setup(t *testing.T) (*p1Fixture, []int64, []byte, time.Time) {
	t.Helper()
	f, ids := p13GuildSetup(t)
	if err := f.s.InitializeP14Cross(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P14PromotionGrant{}, &db.P14PromotionState{}, &db.P14PromotionPolicy{}, &db.P14Receipt{}, &db.P14Member{}, &db.P14Delivery{}, &db.P14ImportReceipt{}, &db.P14TrustedPeer{}, &db.P14TimeSlot{}, &db.P14AuditionRoster{}, &db.P14AuditionBoundary{}, &db.P14AuditionRound{}, &db.P14AuditionMatch{}, &db.P14AuditionMatchPolicy{}, &db.P14AuditionQualification{}, &db.P14SeasonPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Create(&db.P14AuditionMatchPolicy{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP14Promotion(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	key := []byte(strings.Repeat("isolated-peer-key-", 2))
	if err := db.GetIns().Create(&db.P14SeasonPolicy{ID: 1, Enabled: true, SeasonIndex: 7, RegisterStart: now.Add(-time.Hour).UnixMilli(), RegisterEnd: now.Add(time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P14TrustedPeer{ServerID: 2, Enabled: true, Endpoint: "http://127.0.0.1:1", SigningKey: key}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(70); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	return f, ids, key, now
}

// TestP14Cross 注册与上传仅使用可信服号/真实存档，原回执与离线outbox同事务恢复。
func TestP14CrossOriginalSignupUploadReceiptsActualSnapshotAndOfflineRecovery(t *testing.T) {
	f, ids, _, now := p14Setup(t)
	id := ids[0]
	if _, err := f.s.p14Register(f.ctx, id, &pb.CSKFEnterFrightGame{}, 10, now); err == nil {
		t.Fatal("upload before signup")
	}
	updateProfile(f.ctx, ids[1], func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(69); return nil })
	if _, err := f.s.p14Register(f.ctx, ids[1], &pb.CSKFEnterGame{}, 10, now); err == nil {
		t.Fatal("original level70 qualification")
	}
	messages, err := f.s.p14Register(f.ctx, id, &pb.CSKFEnterGame{}, 11, now)
	if err != nil || len(messages) != 2 {
		t.Fatal(err)
	}
	if _, err = f.s.p14Register(f.ctx, id, &pb.CSKFEnterGame{}, 11, now); err != nil {
		t.Fatal(err)
	}
	var member db.P14Member
	db.GetIns().First(&member, "season_index=7 AND server_id=1 AND player_id=?", id)
	if member.Revision != 1 || p14SnapshotDigest(member.Snapshot, member.Battle) != member.SnapshotHash {
		t.Fatal("actual first snapshot", member.Revision)
	}
	party := &pb.BattlePlayer{}
	if proto.Unmarshal(member.Battle, party) != nil || int64(party.GetServerId()) != 1 || party.GetPlayerId() != id {
		t.Fatal("trusted source identity")
	}
	updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Name = proto.String("更新后的原阵容")
		return nil
	})
	if _, err = f.s.p14Register(f.ctx, id, &pb.CSKFEnterGame{}, 12, now); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&member, "season_index=7 AND server_id=1 AND player_id=?", id)
	if member.Revision != 1 {
		t.Fatal("new signup stamp reuploaded")
	}
	uploaded, err := f.s.p14Register(f.ctx, id, &pb.CSKFEnterFrightGame{}, 13, now.Add(time.Millisecond))
	if err != nil || uploaded[0].(*pb.SCKFEnterFrightGame).GetLastFightPower() <= 0 {
		t.Fatal("upload actual fight power", err)
	}
	var deliveries []db.P14Delivery
	db.GetIns().Find(&deliveries)
	if len(deliveries) != 2 {
		t.Fatal("exact outbox revisions", len(deliveries))
	}
	if err = f.s.DeliverP14Snapshots(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, delivery := range deliveries {
		db.GetIns().First(&delivery, "id=?", delivery.ID)
		if delivery.Confirmed || delivery.AttemptCount < 1 {
			t.Fatal("offline lost durable outbox")
		}
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP14Cross(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p14Register(f.ctx, id, &pb.CSKFEnterFrightGame{}, 13, now.Add(2*time.Hour)); err != nil {
		t.Fatal("receipt restoration after cutoff", err)
	}
	db.GetIns().First(&member, "season_index=7 AND server_id=1 AND player_id=?", id)
	if member.Revision != 2 {
		t.Fatal("retry captured new revision")
	}
	if _, err = f.s.p14Register(f.ctx, id, &pb.CSKFEnterFrightGame{}, 14, now.Add(2*time.Hour)); err == nil {
		t.Fatal("cutoff upload")
	}
	query, err := f.s.p14Query(f.ctx, id, &pb.CSKFLogin{}, now)
	if err != nil || query[0].(*pb.SCKFLogin).GetIsEnter() != 1 || query[0].(*pb.SCKFLogin).GetKfOrder() != 7 {
		t.Fatal("original registered state", err)
	}
}

// TestP14Cross 同本地ID不同可信服号不冲突，签名/目标/版本/重复/乱序均核对实际持久阵容。
func TestP14CrossJointIdentitySignedImportsIdempotenceOrderAndCorruption(t *testing.T) {
	f, ids, key, now := p14Setup(t)
	id := ids[0]
	if _, err := f.s.p14Register(f.ctx, id, &pb.CSKFEnterGame{}, 1, now); err != nil {
		t.Fatal(err)
	}
	var local db.P14Member
	db.GetIns().First(&local, "server_id=1 AND player_id=?", id)
	remote := &GameService{Tables: f.s.Tables, ServerID: 2}
	var profile db.Profile
	db.GetIns().First(&profile, id)
	p, g, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	p.Name = proto.String("独立原服同号角色")
	party, err := remote.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	battle, _ := proto.Marshal(party)
	form := &pb.FormationSimpleInfo{}
	proto.Unmarshal(local.Snapshot, form)
	form.Name = p.Name
	formation, _ := proto.Marshal(form)
	makePayload := func(revision int64, name string) (model.P14SnapshotImportRequest, []byte) {
		form.Name = proto.String(name)
		formation, _ := proto.Marshal(form)
		request := model.P14SnapshotImportRequest{ProtocolVersion: 1, DeliveryID: p6Digest([]byte(fmt.Sprintf("p14/%d/%d/%d/%d/%d", 7, 2, id, revision, 1))), SourceServerID: 2, DestinationServerID: 1, SeasonIndex: 7, PlayerID: id, Revision: revision, CapturedAt: now.UnixMilli(), FightPower: form.GetFightPower(), Formation: formation, Battle: battle, SnapshotHash: p14SnapshotDigest(formation, battle), ConfigurationHash: local.ConfigurationHash, ClientVersion: local.ClientVersion, EngineVersion: local.EngineVersion}
		raw, _ := json.Marshal(request)
		return request, raw
	}
	request, raw := makePayload(1, "原服同号1")
	if _, err = f.s.ImportP14Snapshot(f.ctx, request, raw, strings.Repeat("0", 64), now); err == nil {
		t.Fatal("forged signature accepted")
	}
	reply, err := f.s.ImportP14Snapshot(f.ctx, request, raw, P14SignPayload(key, raw), now)
	if err != nil || !reply.Confirmed {
		t.Fatal("trusted source import", err)
	}
	if _, err = f.s.ImportP14Snapshot(f.ctx, request, raw, P14SignPayload(key, raw), now); err != nil {
		t.Fatal("duplicate import", err)
	}
	newer, newRaw := makePayload(3, "原服同号3")
	if _, err = f.s.ImportP14Snapshot(f.ctx, newer, newRaw, P14SignPayload(key, newRaw), now); err != nil {
		t.Fatal(err)
	}
	older, oldRaw := makePayload(2, "原服同号2")
	if _, err = f.s.ImportP14Snapshot(f.ctx, older, oldRaw, P14SignPayload(key, oldRaw), now); err != nil {
		t.Fatal("late older ack", err)
	}
	var rows []db.P14Member
	db.GetIns().Where("player_id=?", id).Order("server_id").Find(&rows)
	if len(rows) != 2 || rows[0].Revision != 1 || rows[1].Revision != 3 {
		t.Fatal("combined identity or high-water lost", len(rows))
	}
	conflicting, conflictRaw := makePayload(3, "另一同版本载荷")
	if _, err = f.s.ImportP14Snapshot(f.ctx, conflicting, conflictRaw, P14SignPayload(key, conflictRaw), now); err == nil {
		t.Fatal("same delivery changed content")
	}
	incompatible, rawBad := makePayload(4, "版本不兼容")
	incompatible.EngineVersion = "untrusted-version"
	rawBad, _ = json.Marshal(incompatible)
	if _, err = f.s.ImportP14Snapshot(f.ctx, incompatible, rawBad, P14SignPayload(key, rawBad), now); err == nil {
		t.Fatal("version mismatch accepted")
	}
	wrong, rawWrong := makePayload(5, "错目的")
	wrong.DestinationServerID = 2
	rawWrong, _ = json.Marshal(wrong)
	if _, err = f.s.ImportP14Snapshot(f.ctx, wrong, rawWrong, P14SignPayload(key, rawWrong), now); err == nil {
		t.Fatal("wrong destination accepted")
	}
	tampered, _ := makePayload(6, "字段与签名原体不一致")
	if _, err = f.s.ImportP14Snapshot(f.ctx, tampered, raw, P14SignPayload(key, raw), now); err == nil {
		t.Fatal("unsigned interpreted field accepted")
	}
	query, err := f.s.p14Query(f.ctx, id, &pb.CSKFPlayerInfo{PlayerId: proto.Int64(id), ServerId: proto.Int32(2)}, now)
	if err != nil || query[0].(*pb.SCKFPlayerInfo).GetFormationInfo().GetName() != "原服同号3" {
		t.Fatal("original cross-server profile", err)
	}
	if _, _, err = f.s.DispatchP14Cross(context.Background(), id, &pb.CSKFGuess{}, 99); err == nil {
		t.Fatal("unknown guess charged")
	}
	_ = formation
}
