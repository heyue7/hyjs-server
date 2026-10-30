package service

import (
	"encoding/hex"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Real native type-5 input and engine determine the winner. The test uses
// finite isolated chips/coins and proves the original response is recoverable.
func TestGGOPVPPositiveStampNativeRecovery(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	f, attacker, defender, sessions := p12FieldPVPSetup(t)
	p12FieldPVPEnable(t)
	if err := db.GetIns().AutoMigrate(&db.AdminFieldDeployment{}, &db.GGOFieldReceipt{}, &db.ClientUpdateSession{}, &db.P8GatherAttempt{}); err != nil {
		t.Fatal(err)
	}
	cfg := ggoLineTestConfiguration(t)
	values, err := ggoChipValues(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.s.GGOIntegrationReady, f.s.P12FieldPVPReady = true, true
	f.s.GGOFieldConfig, f.s.GGOFieldEconomy, f.s.GGOFieldRevision = cfg, cfg, "pvp-isolated-release"
	database := db.GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	f.s.adminFieldRuntimeCache.Store(&adminFieldRuntimeSnapshot{serverID: f.s.OriginalServerID(), database: database, sqlDatabase: pool, signature: adminFieldDeploymentSignature(nil), tables: f.s.Tables, ggo: cfg, ggoEconomy: cfg, ggoRevision: "pvp-isolated-release"})
	ids := []int64{attacker, defender}
	for i, id := range ids {
		err = updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Coin = proto.Int64(100000)
			g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(constUtil.GGOFieldScene), Current: &pb.PointNew{X: proto.Float32(20 + float32(i)), Y: proto.Float32(0), Z: proto.Float32(20)}}
			hp := float64(1)
			if i == 1 {
				hp = .01
			}
			g.PositionLoading = false
			g.Field = &model.FieldState{Scene: constUtil.GGOFieldScene, Side: &pb.PlayerSide{PlayerId: proto.Int64(id), State: proto.Int32(0), CurHp: proto.Float64(hp), Anger: proto.Float64(0)}}
			g.GGOField = &model.GGOFieldState{Chips: [6]int64{10, 2, 0, 0, 0, 3}, EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: values}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		binding := db.ClientUpdateSession{PlayerID: id, SessionHash: clientUpdateSessionHash(sessions[id]), BuildCode: cfg.Runtime.MinClientBuild}
		if err = db.GetIns().Save(&binding).Error; err != nil {
			t.Fatal(err)
		}
		if err = f.s.SyncP10Scene(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	request := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(defender), FightType: proto.Int32(0)}
	now := time.Now()
	stamp := int64(2026102803001)
	if _, err = f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], request, 0, now); err == nil {
		t.Fatal("unstamped desert TCP request accepted")
	}
	// A safe-zone rejection must remain a rejection even after position and
	// six-second presentation timing change.
	if err = updateProfile(f.ctx, attacker, func(_ *pb.Player, g *model.GameState) error {
		g.Position.Current = &pb.PointNew{X: proto.Float32(-7), Y: proto.Float32(0), Z: proto.Float32(-6)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.SyncP10Scene(f.ctx, attacker, false); err != nil {
		t.Fatal(err)
	}
	rejectStamp := stamp - 1
	rejected, err := f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], request, rejectStamp, now)
	if err != nil || rejected[0].(*pb.SCSidePvpFightResult).GetCode() != 2007 {
		t.Fatal("safe-zone refusal", err)
	}
	wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	rejectQuery := model.GGOFieldPVPStatusRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: attacker, SessionHex: hex.EncodeToString(sessions[attacker])}, Stamp: rejectStamp, NativeRequest: wire}
	refusal, err := f.s.GGOFieldPVPStatus(f.ctx, rejectQuery, sessions[attacker])
	if err != nil || refusal.Status != "rejected" || refusal.PresentationCurrent || len(refusal.NativeState) != 0 {
		t.Fatal("refusal receipt", refusal, err)
	}
	if err = updateProfile(f.ctx, attacker, func(_ *pb.Player, g *model.GameState) error {
		g.Position.Current = &pb.PointNew{X: proto.Float32(20), Y: proto.Float32(0), Z: proto.Float32(20)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.SyncP10Scene(f.ctx, attacker, false); err != nil {
		t.Fatal(err)
	}
	again, err := f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], request, rejectStamp, now.Add(10*time.Second))
	if err != nil || !proto.Equal(again[0], rejected[0]) {
		t.Fatal("lost refusal became a different result", err)
	}
	first, err := f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], request, stamp, now)
	if err != nil || len(first) < 1 {
		t.Fatal("native PK failed", err)
	}
	native := first[0].(*pb.SCSidePvpFightResult)
	if native.GetCode() != 1 || native.GetWinPlayerId() != attacker {
		t.Fatal("expected legal attacking winner", native)
	}
	replayed, err := f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], request, stamp, now.Add(10*time.Second))
	if err != nil || !proto.Equal(native, replayed[0]) {
		t.Fatal("positive stamp lost after presentation lease", err)
	}
	var before [2][6]int64
	for i, id := range ids {
		var row db.Profile
		if err = db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, e := decodeProfile(&row)
		if e != nil {
			t.Fatal(e)
		}
		if p.GetCoin() != 100000 {
			t.Fatal("desert PK changed ordinary coins")
		}
		before[i] = g.GGOField.Chips
	}
	for i := range before[0] {
		if before[0][i]+before[1][i] != 2*([6]int64{10, 2, 0, 0, 0, 3})[i] {
			t.Fatal("chip transfer failed conservation", before)
		}
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	q := model.GGOFieldPVPStatusRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: attacker, SessionHex: hex.EncodeToString(sessions[attacker])}, Stamp: stamp, NativeRequest: raw}
	status, err := f.s.GGOFieldPVPStatus(f.ctx, q, sessions[attacker])
	if err != nil || status.Status != "settled" || status.FightID != native.GetFightId() || !status.PresentationCurrent || len(status.NativeState) == 0 {
		t.Fatal("status recovery", status, err)
	}
	// Alternate valid protobuf field order models another client serializer.
	// The server canonicalizes the original request before matching its hash.
	reordered := protowire.AppendTag(nil, 3, protowire.VarintType)
	reordered = protowire.AppendVarint(reordered, 0)
	reordered = protowire.AppendTag(reordered, 1, protowire.VarintType)
	reordered = protowire.AppendVarint(reordered, uint64(defender))
	canonicalQuery := q
	canonicalQuery.NativeRequest = reordered
	canonicalStatus, e := f.s.GGOFieldPVPStatus(f.ctx, canonicalQuery, sessions[attacker])
	if e != nil || canonicalStatus.Status != "settled" || canonicalStatus.RequestHash != status.RequestHash {
		t.Fatal("protobuf ordering changed request identity", e)
	}
	var projected pb.SCFieldPlayerState
	if err = proto.Unmarshal(status.NativeState, &projected); err != nil || len(projected.GetPlayer()) != 2 {
		t.Fatal("missing current native combat state", err)
	}
	if err = db.GetIns().Model(&db.P12FieldPVPState{}).Where("player_id=?", attacker).Update("busy_until", 0).Error; err != nil {
		t.Fatal(err)
	}
	expired, err := f.s.GGOFieldPVPStatus(f.ctx, q, sessions[attacker])
	if err != nil || expired.PresentationCurrent || len(expired.NativeState) != 0 {
		t.Fatal("expired presentation remained current", expired, err)
	}
	var recovered pb.SCSidePvpFightResult
	if err = proto.Unmarshal(status.NativeResponse, &recovered); err != nil || !proto.Equal(native, &recovered) {
		t.Fatal("native response changed", err)
	}
	changed := proto.Clone(request).(*pb.CSSidePvpFightResult)
	changed.FightType = proto.Int32(1)
	if _, err = f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], changed, stamp, now.Add(11*time.Second)); err == nil {
		t.Fatal("same stamp with different body accepted")
	}
	other := q
	other.PlayerID = defender
	other.SessionHex = hex.EncodeToString(sessions[defender])
	otherRequest := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(attacker), FightType: proto.Int32(0)}
	other.NativeRequest, err = proto.MarshalOptions{Deterministic: true}.Marshal(otherRequest)
	if err != nil {
		t.Fatal(err)
	}
	unseen, err := f.s.GGOFieldPVPStatus(f.ctx, other, sessions[defender])
	if err != nil || unseen.Status != "unknown" {
		t.Fatal("another character read initiator receipt", unseen, err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, attacker).Error; err != nil {
		t.Fatal(err)
	}
	rotated, err := LoginAccount(f.ctx, profile.Uid, "isolated-pk-password")
	if err != nil {
		t.Fatal(err)
	}
	q.SessionHex = hex.EncodeToString(rotated.Session)
	status, err = f.s.GGOFieldPVPStatus(f.ctx, q, rotated.Session)
	if err != nil || status.Status != "settled" {
		t.Fatal("new session cannot recover", status, err)
	}
	if _, err = f.s.GGOFieldPVPStatus(f.ctx, q, sessions[attacker]); err == nil {
		t.Fatal("rotated session retained receipt access")
	}
	var rows int64
	if err = db.GetIns().Model(&db.P12FieldPVPReceipt{}).Where("player_id=? AND stamp=?", attacker, stamp).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatal("duplicate native settlement", rows, err)
	}
}
