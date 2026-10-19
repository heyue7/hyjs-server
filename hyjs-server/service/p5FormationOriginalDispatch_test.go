package service

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP5FormationOriginalWireDispatchAndCold exercises the exact original popup
// requests through authenticated binary frames and the complete game dispatcher.
func TestP5FormationOriginalWireDispatchAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	// This finite isolated prerequisite verifies routing and saving, with the
	// original level gate retained and no claim about natural recruit progress.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		_, err := f.s.GrantFixedReward(p, g, "hero_020,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, state := f.profile(t)
	var partner *pb.Warrior
	for _, warrior := range state.Warriors {
		if warrior.GetId() == "hero_020" {
			partner = warrior
		}
	}
	if partner == nil {
		t.Fatal("isolated owned partner missing")
	}
	if _, err := f.s.formationOn(f.ctx, f.id, &pb.CSFormationOn{Id: partner.Id, Guid: partner.Guid, FormatPos: proto.Int32(2)}); err != nil {
		t.Fatal(err)
	}
	// Read the original popup directory and selected template before dragging.
	listed := copyOverflowWire(t, f, &pb.CSTaticalFormationList{})
	if _, ok := listed[0].(*pb.SCTaticalFormationList); !ok {
		t.Fatal("original 82006 directory", listed)
	}
	selected := copyOverflowWire(t, f, &pb.CSGetTaticalFormation{Id: proto.String("1")})
	view, ok := selected[0].(*pb.SCGetTaticalFormation)
	if !ok || view.GetFormation() == nil {
		t.Fatal("original 82000 selected template", selected)
	}
	positions := map[int32]int32{}
	for _, position := range view.GetFormation().GetPosInfoList() {
		positions[position.GetFormatPos()] = position.GetBattlePos()
	}
	if positions[1] == 0 || positions[2] == 0 || positions[1] == positions[2] {
		t.Fatal("original template distinct occupied positions missing", positions)
	}
	// The popup sends all occupied camp positions, mapping the two actors to
	// each other's original board coordinates. The server keeps their gear.
	change := &pb.CSTaticalFormationChange{Id: proto.String("1"), Formation: proto.String(fmt.Sprintf("1,%d;2,%d", positions[2], positions[1]))}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Field = &model.FieldState{Scene: "field_001", Side: &pb.PlayerSide{PlayerId: proto.Int64(f.id), State: proto.Int32(0), CurHp: proto.Float64(0.4), Anger: proto.Float64(17), BattlePosHps: []*pb.BattlePosHp{
			{BattlePos: proto.Int32(positions[1]), HpPercent: proto.Float64(0)},
			{BattlePos: proto.Int32(positions[2]), HpPercent: proto.Float64(0.8)},
		}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	replies := copyOverflowWire(t, f, change)
	changed, ok := replies[0].(*pb.SCTaticalFormationChange)
	if !ok || changed.GetFormation() == nil {
		t.Fatal("original drag 82002 must return 82003", replies)
	}
	// The original success callback then sends one-way TCP 30094. Its legacy
	// old/new strings must never be trusted to revive the dead camp or replay HP.
	notify := &pb.CSRefreshSideHp{OldFormation: proto.String(fmt.Sprintf("1,%d;2,%d", positions[1], positions[2])), NewFormation: change.Formation}
	for attempt := 0; attempt < 2; attempt++ {
		payload, err := proto.Marshal(notify)
		if err != nil {
			t.Fatal(err)
		}
		raw := make([]byte, 28+len(payload))
		binary.BigEndian.PutUint32(raw[0:4], uint32(len(raw)-4))
		binary.BigEndian.PutUint32(raw[4:8], 1)
		binary.BigEndian.PutUint32(raw[8:12], 30094)
		binary.BigEndian.PutUint32(raw[12:16], uint32(f.s.OriginalServerID()))
		binary.BigEndian.PutUint64(raw[16:24], uint64(f.id))
		binary.BigEndian.PutUint32(raw[24:28], uint32(len(payload)))
		copy(raw[28:], payload)
		packet, err := model.ParseGamePacket(raw, true)
		if err != nil {
			t.Fatal(err)
		}
		var row db.Profile
		if err := db.GetIns().First(&row, f.id).Error; err != nil {
			t.Fatal(err)
		}
		// The existing socket handler binds the previously authorized session.
		packet.Session = row.Session
		notified := f.s.Dispatch(f.ctx, packet, "127.0.0.1:fixture")
		if len(notified) != 1 {
			t.Fatal("30094 should project the existing own field side", notified)
		}
		if _, ok := notified[0].(*pb.SCFieldPlayerState); !ok {
			t.Fatal("30094 must not fall through to unsupported", notified)
		}
	}
	check := func(avatarSwapped bool) {
		t.Helper()
		responses := copyOverflowWire(t, f, &pb.CSFormationList{})
		list, ok := responses[0].(*pb.SCFormationList)
		if !ok {
			t.Fatal("original 20111 projection", responses)
		}
		actual := map[int32]int32{}
		for _, slot := range list.GetFormationList() {
			actual[slot.GetFormatPos()] = slot.GetBattlePos()
		}
		if actual[1] != positions[2] || actual[2] != positions[1] {
			t.Fatal("original board drag not saved", actual)
		}
		_, g := f.profile(t)
		hps := map[int32]float64{}
		for _, hp := range g.Field.Side.GetBattlePosHps() {
			hps[hp.GetBattlePos()] = hp.GetHpPercent()
		}
		wantFirst, wantSecond := 0.8, 0.0
		if avatarSwapped {
			wantFirst, wantSecond = 0.0, 0.8
		}
		if hps[positions[2]] != wantSecond || hps[positions[1]] != wantFirst || g.Field.Side.GetCurHp() != 0.4 || g.Field.Side.GetAnger() != 17 {
			t.Fatal("saved hero HP identity/anger changed on board drag or notification", g.Field.Side)
		}
		selected := copyOverflowWire(t, f, &pb.CSGetTaticalFormation{Id: proto.String("1")})
		current, ok := selected[0].(*pb.SCGetTaticalFormation)
		if !ok || !proto.Equal(current.GetFormation(), changed.GetFormation()) {
			t.Fatal("reopened original popup lost authoritative mapping", selected)
		}
	}
	check(false)
	f.restart(t)
	check(false)
	_, before := f.profile(t)
	beforeRaw, _ := json.Marshal(before)
	// A late notification contains a retired mapping. It returns current state
	// and cannot use client strings to move life back to the previous cell.
	if _, err := f.s.formationSideHPRefresh(f.ctx, f.id, &pb.CSRefreshSideHp{OldFormation: notify.NewFormation, NewFormation: notify.OldFormation}); err != nil {
		t.Fatal(err)
	}
	_, after := f.profile(t)
	afterRaw, _ := json.Marshal(after)
	if string(beforeRaw) != string(afterRaw) {
		t.Fatal("late client notification changed saved life")
	}
	// The separate ArrayPage avatar drag uses 21119. Verify it reaches the
	// existing whole-camp exchange without changing the saved board mapping.
	exchanged := copyOverflowWire(t, f, &pb.CSFormationExChange{FromPos: proto.Int32(1), ToPos: proto.Int32(2)})
	if _, ok := exchanged[0].(*pb.SCFormationExChange); !ok {
		t.Fatal("original avatar drag 21119 must return 21120", exchanged)
	}
	f.restart(t)
	check(true)
}
