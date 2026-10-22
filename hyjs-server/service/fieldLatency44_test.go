package service

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	gormlogger "gorm.io/gorm/logger"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestFieldLatency44MoveDispatch measures the authenticated route, then checks
// persisted movement and resource recovery instead of timing a detached helper.
func TestFieldLatency44MoveDispatch(t *testing.T) {
	f, _ := adminModesFixture(t)
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Energy = proto.Int32(10)
		g.PositionLoading = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	// Set only this isolated fixture's existing recovery cursor after the real
	// full-to-low transition has established its normal new cycle.
	player, _, err := decodeProfile(&owner)
	if err != nil {
		t.Fatal(err)
	}
	player.EnergyLastRefreshTime = proto.Int64(time.Now().Add(-35 * time.Minute).UnixMilli())
	raw, err := json.Marshal(player)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&owner).Update("player", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	var (
		previous = db.GetIns().Config.Logger
		recorder = &adminActivityQueryRecorder{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	)
	db.GetIns().Config.Logger = recorder
	defer func() { db.GetIns().Config.Logger = previous }()
	point := &pb.PointNew{X: proto.Float32(3), Y: proto.Float32(0), Z: proto.Float32(4)}
	started := time.Now()
	result := f.s.Dispatch(f.ctx, &model.GamePacket{Opcode: 30001, PlayerId: f.id, ZoneId: 1, Session: owner.Session, Body: &pb.CSMove{PointNew: point}}, "127.0.0.1")
	elapsed := time.Since(started)
	for _, response := range result {
		if failure, ok := response.(*pb.ErrorMessage); ok {
			t.Fatal(failure.GetMsg())
		}
	}
	t.Logf("authenticated move sql=%d profileReads=%d activityQueries=%d elapsed=%s", len(recorder.sqls), recorder.countContaining("SELECT * FROM `profiles`"), recorder.countContaining("admin_activity"), elapsed)
	counts := map[string]int{}
	pattern := regexp.MustCompile("(?i)(?:FROM|JOIN|INTO|UPDATE|TABLE) [`\"]?([a-z_0-9]+)")
	for _, query := range recorder.sqls {
		if match := pattern.FindStringSubmatch(query); len(match) == 2 {
			counts[match[1]]++
		}
	}
	t.Logf("query tables: %v", counts)
	p, g := f.profile(t)
	if !proto.Equal(g.Position.Current, point) || p.GetEnergy() != 11 {
		t.Fatal("movement or resource recovery lost", g.Position.Current, p.GetEnergy())
	}
	// A repeated position still validates the session and runs guards, while the
	// already-persisted resource and unchanged statistics cause no extra writes.
	recorder.reset()
	f.s.Dispatch(f.ctx, &model.GamePacket{Opcode: 30001, PlayerId: f.id, ZoneId: 1, Session: owner.Session, Body: &pb.CSMove{PointNew: point}}, "127.0.0.1")
	if recorder.countContaining("UPDATE `profiles`") != 0 || recorder.countContaining("UPDATE `p4_player_stats`") != 0 {
		t.Fatal("unchanged position or activity statistics were written again")
	}
	// A rotated session is rejected before any position can be persisted.
	bad := proto.Clone(point).(*pb.PointNew)
	bad.X = proto.Float32(99)
	rejected := f.s.Dispatch(f.ctx, &model.GamePacket{Opcode: 30001, PlayerId: f.id, ZoneId: 1, Session: make([]byte, 16), Body: &pb.CSMove{PointNew: bad}}, "127.0.0.1")
	if len(rejected) != 1 {
		t.Fatal("invalid session was accepted")
	}
	if _, ok := rejected[0].(*pb.ErrorMessage); !ok {
		t.Fatal("invalid session was accepted")
	}
	_, saved := f.profile(t)
	if !proto.Equal(saved.Position.Current, point) {
		t.Fatal("rejected movement changed position")
	}
}
