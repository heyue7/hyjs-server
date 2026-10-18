package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"testing"
)

func p6ManualWaitInput(t *testing.T, s *GameService) (*model.FrozenBattleInput, *pb.BattleOperate) {
	t.Helper()
	input := p6Input(t, s, p6Actor("hero_047", "manual-player", "047_1,047_2", 1, 40000), p6Actor("hero_047", "manual-target", "047_1", 1, 40000), 2)
	baseline, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	operate := proto.Clone(baseline.Operates).(*pb.BattleOperate)
	for _, op := range operate.SkillOpts {
		if op.GetRoundNum() == 1 {
			op.RoundTime = proto.Float64(30.100000000000158)
		}
	}
	return input, operate
}

func TestP6OriginalWireGuidManualWaitAndSameFrameQueue(t *testing.T) {
	s := p6OriginalTables(t)
	input, operate := p6ManualWaitInput(t, s)
	if len(operate.SkillOpts) < 2 || operate.SkillOpts[0].GetActorUid() != "manual-player" || operate.SkillOpts[1].GetActorUid() != "manual-player" {
		t.Fatal("internal camp prefix leaked to original wire")
	}
	out, err := s.ComputeFrozenBattle(t.Context(), input, operate)
	if err != nil {
		t.Fatal(err)
	}
	if !p6OriginalOperateEqual(operate, out.Operates) || out.Authoritative {
		t.Fatal("manual original operation replay changed")
	}
	presses := 0
	for _, event := range out.Events {
		if event.Round == 1 && event.Kind == "skill_press" {
			presses++
			if event.Actor != "0_manual-player" || math.Abs(event.Time-30.100000000000158) > 1e-8 {
				t.Fatal("manual wait or internal identity changed", event)
			}
		}
	}
	if presses != 2 {
		t.Fatal("same-frame basic queue changed", presses)
	}
	// protobuf-net omits default camp, index, time and zero HP fields in original frames.
	native := proto.Clone(operate).(*pb.BattleOperate)
	for _, op := range native.SkillOpts {
		if op.GetCamp() == 0 {
			op.Camp = nil
		}
		if op.GetSkillIndex() == 0 {
			op.SkillIndex = nil
		}
		if op.GetRoundTime() == 0 {
			op.RoundTime = nil
		}
	}
	if native.GetAttackerTotalLeftHp() == 0 {
		native.AttackerTotalLeftHp = nil
	}
	if native.GetDefenderTotalLeftHp() == 0 {
		native.DefenderTotalLeftHp = nil
	}
	if _, err = s.ComputeFrozenBattle(t.Context(), input, native); err != nil {
		t.Fatal("native omitted default values rejected", err)
	}
	bad := proto.Clone(native).(*pb.BattleOperate)
	bad.DefenderTotalLeftHp = proto.Float64(native.GetDefenderTotalLeftHp() + 1)
	if _, err = s.ComputeFrozenBattle(t.Context(), input, bad); err == nil {
		t.Fatal("different native end HP accepted")
	}
	bad = proto.Clone(native).(*pb.BattleOperate)
	bad.SkillOpts[0].ActorUid = proto.String("0_manual-player")
	if _, err = s.ComputeFrozenBattle(t.Context(), input, bad); err == nil {
		t.Fatal("internal UID accepted as original wire GUID")
	}
}
