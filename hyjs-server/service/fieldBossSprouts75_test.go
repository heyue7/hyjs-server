package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Explicit finite fixture inputs: five existing test accounts, field positions,
// and elapsed creation/return dates. Team membership/invitations are never
// inserted directly; presence/session rows are produced by the real P10 sink.
func TestFieldBossSprouts75OriginalTeamLivePresenceSessionsAndExpiry(t *testing.T) {
	f, ids, recs := p13TeamSetup(t)
	if f.mysql.Database != "" {
		t.Skip("this independent finite integration fixture is SQLite only")
	}
	if err := f.s.InitializeAdminSproutSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	days, offline, returnDays := int64(2), int64(7), int64(3)
	yes := true
	cfg, err := f.s.AdminSproutSettings(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	save := func(newEnabled, returnEnabled, enabled bool) {
		t.Helper()
		cfg, err = f.s.AdminSaveSproutSettings(f.ctx, actor, model.AdminSproutSettingsSaveRequest{Enabled: enabled, Days: &days, NewCharacterEnabled: &newEnabled, ReturnEnabled: &returnEnabled, ReturnOfflineDays: &offline, ReturnDays: &returnDays, Version: cfg.Version, Reason: "隔离小芽队伍来源集成回归"})
		if err != nil {
			t.Fatal(err)
		}
	}
	save(yes, yes, true)
	p13TeamOpen(t)
	if err = db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.AddDate(0, 0, -10).UnixMilli()
	fresh := now.Add(-time.Hour).UnixMilli()
	dates := func(id, created, returned int64) {
		t.Helper()
		if err := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Updates(map[string]any{"created_at": created, "return_sprout_started_at": returned}).Error; err != nil {
			t.Fatal(err)
		}
	}
	position := func(id int64, scene string) {
		t.Helper()
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(scene), Current: &pb.PointNew{}}
			g.PositionLoading = false
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	sync := func(id int64) {
		t.Helper()
		if err := f.s.SyncP10Scene(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	connect := func(id int64) func() {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		close := f.s.RegisterP10Client(f.ctx, id, row.Session, (&p10SceneRecorder{}).send)
		t.Cleanup(close)
		sync(id)
		return close
	}
	count := func(label string, want, maximum int64) {
		t.Helper()
		n, err := f.s.fieldBossSprouts73(db.GetIns().WithContext(f.ctx), ids[0], "field_001", maximum)
		if err != nil || n != want {
			t.Fatalf("%s count=%d want=%d err=%v", label, n, want, err)
		}
	}
	for _, id := range ids {
		dates(id, old, 0)
		position(id, "field_001")
		sync(id)
	}
	dates(ids[0], fresh, 0)
	dates(ids[4], fresh, 0)
	// Real scene synchronization populated same map/line with actual matching sessions.
	var ownPresence db.P10ScenePresence
	if err := db.GetIns().First(&ownPresence, "player_id=?", ids[0]).Error; err != nil || ownPresence.Scene != "field_001" || ownPresence.SessionHash == "" {
		t.Fatal("actual P10 field presence", ownPresence, err)
	}
	if !p10SceneHasPlayer(recs[0].drain(), ids[4]) {
		t.Fatal("same-map nonmember was not actually visible")
	}
	count("self and visible nonmember do not create a team bonus", 0, 2)
	created := p13TeamCall(t, f, ids[0], &pb.CSTeamCreateAndInvite{InvitePlayerIdList: ids[1:4], InviteServerIdList: []int32{1, 1, 1}}, 751001, now).(*pb.SCTeamCreateAndInvite)
	team := created.GetTeam().GetTeamId()
	if team == "" {
		t.Fatal("original team missing")
	}
	dates(ids[1], fresh, 0)
	count("invited eligible player has not joined", 0, 2)
	for i, id := range ids[1:4] {
		dates(id, fresh, 0)
		joined := p13TeamCall(t, f, id, &pb.CSJoinTeam{TeamId: proto.String(team)}, int64(751010+i), now).(*pb.SCJoinTeam)
		if joined.GetToPlayerId() != id {
			t.Fatal("actual join identity")
		}
		want := int64(i + 1)
		if want > 2 {
			want = 2
		}
		count("actual joined 1/2/3 eligible peers capped2", want, 99)
	}
	count("configured cap1", 1, 1)
	count("disabled per-fight cap", 0, 0)
	// Isolate one eligible teammate for each negative boundary; owner remains a sprout.
	dates(ids[2], old, 0)
	dates(ids[3], old, 0)
	count("isolated eligible peer", 1, 2)
	line := func(n int32, stamp int64) {
		t.Helper()
		_, handled, err := f.s.DispatchP10Scene(f.ctx, ids[1], &pb.CSTransferLine{Line: proto.Int32(n)}, stamp)
		if err != nil || !handled {
			t.Fatal("original line transfer", handled, err)
		}
	}
	line(2, 751101)
	count("different real line", 0, 2)
	line(1, 751102)
	count("returned real line", 1, 2)
	position(ids[1], "field_002")
	sync(ids[1])
	count("different real profile map and synced presence", 0, 2)
	position(ids[1], "field_001")
	sync(ids[1])
	count("same map restored", 1, 2)
	position(ids[1], "town_main")
	count("stale field presence cannot override current profile map", 0, 2)
	sync(ids[1])
	position(ids[1], "field_001")
	sync(ids[1])
	count("reconnected map", 1, 2)
	closePeer := connect(ids[1])
	count("online fresh sink", 1, 2)
	closePeer()
	count("offline peer", 0, 2)
	closeOldLive := connect(ids[1])
	defer closeOldLive()
	count("current sink before login rotation", 1, 2)
	var row db.Profile
	if err := db.GetIns().First(&row, ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	var stalePresence db.P10ScenePresence
	if err := db.GetIns().First(&stalePresence, "player_id=?", ids[1]).Error; err != nil || stalePresence.SessionHash != p6Digest(row.Session) {
		t.Fatal("old connection must have genuine nonempty current presence", err)
	}
	freshLogin, err := LoginAccount(f.ctx, row.Uid, "isolated-local-pass")
	if err != nil {
		t.Fatal(err)
	}
	var afterLoginPresence db.P10ScenePresence
	if err := db.GetIns().First(&afterLoginPresence, "player_id=?", ids[1]).Error; err != nil || afterLoginPresence.SessionHash != stalePresence.SessionHash {
		t.Fatal("rotation probe lost the actual stale nonempty presence", err)
	}
	count("actual old nonempty presence and sink rejected after login rotation", 0, 2)
	// The old-session sink really exists but may not inherit a new login's profile.
	staleClose := f.s.RegisterP10Client(f.ctx, ids[1], row.Session, (&p10SceneRecorder{}).send)
	t.Cleanup(staleClose)
	count("old session sink after actual LoginAccount rotation", 0, 2)
	newClose := f.s.RegisterP10Client(f.ctx, ids[1], freshLogin.Session, (&p10SceneRecorder{}).send)
	t.Cleanup(newClose)
	sync(ids[1])
	count("current login restores bonus", 1, 2)
	staleClose()
	count("old disconnect does not remove new sink", 1, 2)
	dates(ids[1], now.AddDate(0, 0, -3).UnixMilli(), 0)
	count("new-character days expired", 0, 2)
	dates(ids[1], old, now.Add(-time.Hour).UnixMilli())
	count("active explicit finite return history", 1, 2)
	dates(ids[1], old, now.AddDate(0, 0, -4).UnixMilli())
	count("return days expired", 0, 2)
	dates(ids[1], old, fresh)
	save(true, false, true)
	count("return policy disabled", 0, 2)
	dates(ids[1], fresh, 0)
	save(false, true, true)
	count("creation policy disabled", 0, 2)
	save(true, true, false)
	count("global sprout policy disabled", 0, 2)
	save(true, true, true)
	count("policy restored", 1, 2)
	p13TeamCall(t, f, ids[1], &pb.CSExitTeam{TeamId: proto.String(team)}, 751201, now)
	count("original exit removes eligible peer", 0, 2)
	p13TeamCall(t, f, ids[0], &pb.CSTeamInvite{TeamId: proto.String(team), InvitePlayerIdList: []int64{ids[1]}, InviteServerIdList: []int32{1}}, 751202, now)
	p13TeamCall(t, f, ids[1], &pb.CSJoinTeam{TeamId: proto.String(team)}, 751203, now)
	count("fresh original reinvite rejoins", 1, 2)
	p13TeamCall(t, f, ids[0], &pb.CSTeamKick{TreamId: proto.String(team), ToPlayerId: proto.Int64(ids[1])}, 751204, now)
	count("original kick removes eligible peer", 0, 2)
}
