package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p8ReservationFixture struct {
	*p1Fixture
	base  context.Context
	ids   []int64
	pos   []string
	lines []int32
}

func p8ReservationSetup(t *testing.T, scene, kind string) *p8ReservationFixture {
	t.Helper()
	f := &p8ReservationFixture{p1Fixture: p1Setup(t)}
	if err := f.s.LoadP8Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP8Gather, f.s.InitGems, f.s.InitCooking, f.s.InitializeP10Scene} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", constUtil.P10ScenePolicyID).
		Updates(map[string]any{"enabled": true, "line_count": 2, "max_players": 100}).Error; err != nil {
		t.Fatal(err)
	}
	field := f.s.p8GatherMap(scene)
	if field == nil {
		t.Fatal("missing field configuration", scene)
	}
	pointsKey := "MinePoints"
	if kind == "fish" {
		pointsKey = "FishPoints"
	}
	points := strings.Split(field[pointsKey], ";")
	if len(points) < 2 {
		t.Fatal("fixture needs two distinct configured points")
	}
	f.pos = []string{points[0], points[0], points[1], points[0], points[0]}
	f.lines = []int32{1, 1, 1, 2, 0}
	f.ids = append(f.ids, f.id)
	for i := 1; i < len(f.pos); i++ {
		account, err := LoginAccount(f.ctx, fmt.Sprintf("p8-reservation-%s-%d-%d", scene, i, time.Now().UnixNano()), "isolated-test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, account.Id, "采集点位隔离", "hero_043"); err != nil {
			t.Fatal(err)
		}
		f.ids = append(f.ids, account.Id)
	}
	f.base = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	for i, id := range f.ids {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
			p.Level = proto.Int32(30)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Save(&db.GemCraftState{PlayerID: id, Level: 6}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Save(&db.CookingCraftState{PlayerID: id, Level: 6}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String(scene)}); err != nil {
			t.Fatal(err)
		}
		node, err := f.s.p8SpawnNode(id, scene, f.pos[i], 1)
		if err != nil {
			t.Fatal(err)
		}
		node.ScheduledAt = fieldGatherBoundary(time.Now().UnixMilli(), []int64{0, 12})
		if err = db.GetIns().Save(node).Error; err != nil {
			t.Fatal(err)
		}
		cfg, err := p8DecodeConfig(node.Snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(f.ctx, id, func(_ *pb.Player, g *model.GameState) error {
			g.PositionLoading = false
			g.Position.Current = proto.Clone(cfg.Point).(*pb.PointNew)
			g.Field.Side.Tiredness = proto.Int32(200)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		var profile db.Profile
		if err = db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		if f.lines[i] > 0 {
			presence := db.P10ScenePresence{PlayerID: id, Scene: scene, Line: f.lines[i], SessionHash: p6Digest(profile.Session), Frame: []byte{}}
			if err = db.GetIns().Save(&presence).Error; err != nil {
				t.Fatal(err)
			}
			key := p10ClientKey{f.s.p10ClientOwner(), id}
			sink := &p10ClientSink{session: append([]byte(nil), profile.Session...), send: func(proto.Message) error { return nil }}
			p10ClientSinks.Store(key, sink)
			t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
		}
	}
	f.s.P8GatherReady = true
	return f
}

func (f *p8ReservationFixture) start(index int, stamp int64) ([]proto.Message, error) {
	request := &pb.CSFieldResourceStartGather{PosId: proto.String(f.pos[index])}
	ctx := context.WithValue(f.base, constUtil.GameRequestContextKey, request)
	return f.s.p8StartGather(ctx, f.ids[index], request, stamp)
}

func (f *p8ReservationFixture) claim(index int, stamp int64) ([]proto.Message, error) {
	request := &pb.CSFieldGetResource{}
	ctx := context.WithValue(f.base, constUtil.GameRequestContextKey, request)
	return f.s.p8ClaimGather(ctx, f.ids[index], request, stamp)
}

func (f *p8ReservationFixture) attempt(t *testing.T, index int) db.P8GatherAttempt {
	t.Helper()
	var attempt db.P8GatherAttempt
	if err := db.GetIns().First(&attempt, "player_id=?", f.ids[index]).Error; err != nil {
		t.Fatal(err)
	}
	return attempt
}

func (f *p8ReservationFixture) cancel(t *testing.T, index int) {
	t.Helper()
	if err := updateProfileWithTx(f.ctx, f.ids[index], func(tx *gorm.DB, _ *pb.Player, game *model.GameState) error {
		return f.s.P8CancelGather(tx, f.ids[index], game)
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *p8ReservationFixture) due(t *testing.T, index int) {
	t.Helper()
	attempt := f.attempt(t, index)
	deadline := time.Now().UnixMilli() - 1
	if err := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=? AND token=?", f.ids[index], attempt.Token).Update("ready_at", deadline).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P8ResourceReservation{}).Where("player_id=? AND token=?", f.ids[index], attempt.Token).Update("occupied_until", deadline).Error; err != nil {
		t.Fatal(err)
	}
}

func p8ExpectStart(t *testing.T, f *p8ReservationFixture, index int, stamp int64) proto.Message {
	t.Helper()
	result, err := f.start(index, stamp)
	if err != nil || len(result) == 0 || result[0].(*pb.SCFieldResourceStartGather).GetResult() != constUtil.P8ResourceAvailable {
		t.Fatalf("start %d: %v", index, err)
	}
	return result[0]
}

func p8ExpectOccupied(t *testing.T, f *p8ReservationFixture, index int, stamp int64) {
	t.Helper()
	result, err := f.start(index, stamp)
	if err == nil || !strings.Contains(err.Error(), constUtil.P8ResourceOccupied) || len(result) != 0 {
		t.Fatalf("point should be busy without a 2003 frame: result=%v error=%v", result, err)
	}
}

func TestP8ReservationsKnownLinesUnknownLineAndLateAcceptedClaim(t *testing.T) {
	for _, scene := range []string{"field_001", "field_002"} {
		for _, kind := range []string{"mine", "fish"} {
			t.Run(scene+"-"+kind, func(t *testing.T) {
				f := p8ReservationSetup(t, scene, kind)
				first := p8ExpectStart(t, f, 0, 1001)
				original := f.attempt(t, 0)
				if original.ReservationVersion != constUtil.P8ReservationVersion {
					t.Fatal("new attempt lacks a reservation version")
				}
				p8ExpectOccupied(t, f, 1, 0)
				p8ExpectStart(t, f, 2, 0)    // same line, another point
				p8ExpectStart(t, f, 3, 0)    // same point, another line
				p8ExpectOccupied(t, f, 4, 0) // unknown line conservatively covers both
				if replay := p8ExpectStart(t, f, 0, 1001); !proto.Equal(first, replay) {
					t.Fatal("same active receipt did not replay")
				}
				if current := f.attempt(t, 0); current.Token != original.Token || current.ReadyAt != original.ReadyAt || current.Rewards != original.Rewards {
					t.Fatal("replay changed the accepted attempt")
				}
				f.cancel(t, 0)
				if _, err := f.start(0, 1001); err == nil {
					t.Fatal("cancelled start receipt restarted the animation")
				}
				second := p8ExpectStart(t, f, 1, 1002)
				accepted := f.attempt(t, 1)
				f.due(t, 1)
				p8ExpectStart(t, f, 0, 0)       // first actor uses the now-completed point
				p8ExpectOccupied(t, f, 1, 1002) // old start cannot replay over its new owner
				f.cancel(t, 0)
				if replay := p8ExpectStart(t, f, 1, 1002); !proto.Equal(second, replay) {
					t.Fatal("accepted attempt could not regain its point after reconnect")
				}
				recovered := f.attempt(t, 1)
				if recovered.Token != accepted.Token || recovered.Rewards != accepted.Rewards {
					t.Fatal("late start replay changed the accepted reward")
				}
				claimed, err := f.claim(1, 2002)
				if err != nil || claimed[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable {
					t.Fatalf("accepted delayed claim failed: %v", err)
				}
				again, err := f.claim(1, 2002)
				if err != nil || !proto.Equal(claimed[0], again[0]) {
					t.Fatalf("claim receipt changed: %v", err)
				}
				if _, err = f.start(1, 1002); err == nil {
					t.Fatal("completed start receipt restarted the animation")
				}
				// The replayed animation retains its own short lease after a fast
				// claim; another actor cannot overlap that animation.
				p8ExpectOccupied(t, f, 0, 0)
				if err = db.GetIns().Model(&db.P8ResourceReservation{}).Where("player_id=? AND token=?", f.ids[1], accepted.Token).Update("occupied_until", time.Now().UnixMilli()-1).Error; err != nil {
					t.Fatal(err)
				}
				f.cancel(t, 3)
				p8ExpectStart(t, f, 4, 0)
				p8ExpectOccupied(t, f, 0, 0)
				p8ExpectOccupied(t, f, 3, 0)
				f.cancel(t, 4)
				p8ExpectStart(t, f, 0, 0)
			})
		}
	}
}

func TestP8LegacyAcceptedAttemptsKeepRewardsAndBlockUnknownLinesThroughGrace(t *testing.T) {
	f := p8ReservationSetup(t, "field_001", "mine")
	p8ExpectStart(t, f, 0, 0)
	legacy := f.attempt(t, 0)
	// Model an already accepted pre-upgrade attempt with no reservation record.
	if err := db.GetIns().Where("player_id=? AND token=?", f.ids[0], legacy.Token).Delete(&db.P8ResourceReservation{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=?", f.ids[0]).Update("reservation_version", 0).Error; err != nil {
		t.Fatal(err)
	}
	p8ExpectOccupied(t, f, 1, 0)
	p8ExpectOccupied(t, f, 3, 0)
	p8ExpectStart(t, f, 0, 0) // idempotent legacy start remains accepted
	if current := f.attempt(t, 0); current.Token != legacy.Token || current.Rewards != legacy.Rewards || current.ReadyAt != legacy.ReadyAt {
		t.Fatal("legacy retry changed the frozen attempt")
	}
	if err := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=?", f.ids[0]).Update("ready_at", time.Now().UnixMilli()-1).Error; err != nil {
		t.Fatal(err)
	}
	p8ExpectOccupied(t, f, 1, 0)
	if err := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=?", f.ids[0]).Update("ready_at", time.Now().UnixMilli()-constUtil.P8ResourceLeaseGraceMS-1).Error; err != nil {
		t.Fatal(err)
	}
	p8ExpectStart(t, f, 1, 0)
	claimed, err := f.claim(0, 3001)
	if err != nil || claimed[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable {
		t.Fatalf("accepted legacy claim did not survive new owner: %v", err)
	}
	if count := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[0], "p8:gather:"+legacy.Token); count != 1 {
		t.Fatal("legacy claim did not preserve exactly one fact", count)
	}
}

func TestP8ReservationGraceAndLateClaimPreserveNewOwner(t *testing.T) {
	f := p8ReservationSetup(t, "field_001", "mine")
	p8ExpectStart(t, f, 0, 0)
	original := f.attempt(t, 0)
	var lease db.P8ResourceReservation
	if err := db.GetIns().Where("server_id=? AND scene=? AND pos_id=? AND line=?", f.s.OriginalServerID(), original.Scene, original.PosID, original.ReservationLine).First(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if lease.OccupiedUntil != original.ReadyAt+constUtil.P8ResourceLeaseGraceMS {
		t.Fatal("accepted attempt did not retain its configured grace", lease.OccupiedUntil, original.ReadyAt)
	}
	if err := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=?", f.ids[0]).Update("ready_at", time.Now().UnixMilli()-1).Error; err != nil {
		t.Fatal(err)
	}
	p8ExpectOccupied(t, f, 1, 0) // ready but no claim still keeps the point
	claimed, err := f.claim(0, 5001)
	if err != nil || len(claimed) == 0 || claimed[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable {
		t.Fatalf("normal claim did not complete: %v", err)
	}
	p8ExpectStart(t, f, 1, 0) // normal claim releases the point immediately
	f.cancel(t, 1)
	p8ExpectStart(t, f, 0, 0)
	old := f.attempt(t, 0)
	f.due(t, 0) // simulate the full ready time plus grace elapsing
	p8ExpectStart(t, f, 1, 0)
	newOwner := f.attempt(t, 1)
	late, err := f.claim(0, 5002)
	if err != nil || len(late) == 0 || late[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable {
		t.Fatalf("late accepted reward failed: %v", err)
	}
	if err = db.GetIns().Where("server_id=? AND scene=? AND pos_id=? AND line=?", f.s.OriginalServerID(), old.Scene, old.PosID, old.ReservationLine).First(&lease).Error; err != nil || lease.PlayerID != f.ids[1] || lease.Token != newOwner.Token {
		t.Fatalf("late claim changed the new owner's reservation: %+v %v", lease, err)
	}
	p8ExpectOccupied(t, f, 0, 0)
}

func TestP8ReservationRejectsChangedLineUntilOwnLeaseEnds(t *testing.T) {
	f := p8ReservationSetup(t, "field_001", "mine")
	p8ExpectStart(t, f, 0, 6001)
	original := f.attempt(t, 0)
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.ids[0]).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	for _, stamp := range []int64{0, 6001} {
		if messages, err := f.start(0, stamp); err == nil || len(messages) != 0 {
			t.Fatalf("a changed line restarted an existing animation: %v %v", messages, err)
		}
	}
	if current := f.attempt(t, 0); current.Token != original.Token || current.ReservationLine != 1 || current.Rewards != original.Rewards {
		t.Fatal("rejected replay changed the accepted attempt")
	}
	p8ExpectOccupied(t, f, 1, 0)
	p8ExpectStart(t, f, 3, 0) // same point in the other line remains independent
	f.cancel(t, 3)
	f.cancel(t, 0)
	p8ExpectStart(t, f, 1, 0)
	p8ExpectStart(t, f, 0, 0) // after cancellation it now reserves line 2
	if current := f.attempt(t, 0); current.ReservationLine != 2 {
		t.Fatal("new attempt did not use the authenticated new line")
	}
}

func TestP8StartReceiptColdReplayAndAttemptTokenABA(t *testing.T) {
	f := p8ReservationSetup(t, "field_001", "mine")
	first := p8ExpectStart(t, f, 0, 4101)
	accepted := f.attempt(t, 0)
	var receipt db.P8GatherReceipt
	if err := db.GetIns().First(&receipt, "player_id=? AND operation=? AND timestamp=?", f.ids[0], constUtil.P8GatherStartOperation, 4101).Error; err != nil || receipt.AttemptToken != accepted.Token {
		t.Fatalf("start receipt did not bind its accepted token: %+v %v", receipt, err)
	}
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = pool.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if replay := p8ExpectStart(t, f, 0, 4101); !proto.Equal(first, replay) {
		t.Fatal("cold replay changed the original start response")
	}
	if current := f.attempt(t, 0); current.Token != accepted.Token || current.Rewards != accepted.Rewards || current.ReadyAt != accepted.ReadyAt {
		t.Fatal("cold replay changed the accepted attempt")
	}
	f.cancel(t, 0)
	p8ExpectStart(t, f, 0, 0)
	newAttempt := f.attempt(t, 0)
	if newAttempt.Token == accepted.Token {
		t.Fatal("a later attempt reused the previous token")
	}
	if _, err = f.start(0, 4101); err == nil {
		t.Fatal("old receipt incorrectly matched a later attempt at the same point")
	}
	if current := f.attempt(t, 0); current.Token != newAttempt.Token || !current.Active {
		t.Fatal("stale start receipt changed the new attempt")
	}
	f.due(t, 0)
	claimed, err := f.claim(0, 4201)
	if err != nil || claimed[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable {
		t.Fatalf("new attempt claim failed: %v", err)
	}
	again, err := f.claim(0, 4201)
	if err != nil || !proto.Equal(claimed[0], again[0]) {
		t.Fatalf("claim receipt changed after the stale start replay: %v", err)
	}
	if n := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[0], "p8:gather:"+newAttempt.Token); n != 1 {
		t.Fatal("claim receipt replay duplicated or lost its fact", n)
	}
}

func TestP8LegacyStartReceiptDoesNotGuessAttemptIdentity(t *testing.T) {
	f := p8ReservationSetup(t, "field_001", "fish")
	p8ExpectStart(t, f, 0, 5101)
	accepted := f.attempt(t, 0)
	if err := db.GetIns().Model(&db.P8GatherReceipt{}).
		Where("player_id=? AND operation=? AND timestamp=?", f.ids[0], constUtil.P8GatherStartOperation, 5101).
		Update("attempt_token", "").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.start(0, 5101); err == nil {
		t.Fatal("legacy start receipt guessed a token identity")
	}
	if current := f.attempt(t, 0); current.Token != accepted.Token || current.Rewards != accepted.Rewards || !current.Active {
		t.Fatal("unidentified legacy receipt changed the accepted attempt")
	}
	f.due(t, 0)
	claimed, err := f.claim(0, 5201)
	if err != nil || claimed[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable {
		t.Fatalf("legacy receipt blocked the accepted claim: %v", err)
	}
}
