package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6RushStarterOriginalTypedStructureAndPendingProof(t *testing.T) {
	service := &GameService{}
	for _, name := range []string{"000", "001", "002", "003"} {
		raw, err := readTestAnalysis75("../../analysis/p6/actual-local-p7-rush-terminal-inputs-v127/" + name + ".rush-series.json")
		if err != nil {
			t.Fatal(err)
		}
		var series model.FrozenP6RushSeries
		if err = json.Unmarshal(raw, &series); err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(series)
		if err = validateP6RushStarterScope(&series); err != nil {
			t.Fatal("actual P7 producer rejected", err)
		}
		out, err := service.ComputeOriginalP6ProtocolFinalHP(context.Background(), series.Template, nil)
		if err != nil || out.Outcome.Authoritative || validateP6RushStarterTerminal(series.Template, out) != nil {
			t.Fatal("actual automatic terminal", err)
		}
		final, err := service.ComputeP6RushStarterSettlement(context.Background(), &series)
		if constUtil.P6RushStarterNativeProof == "" {
			if err == nil || final != nil {
				t.Fatal("unproven P7 family opened")
			}
		} else if err != nil || !final.Outcome.Authoritative {
			t.Fatal("independent certified family rejected", err)
		}
		reported := int32(0)
		if out.Outcome.Winner == 0 {
			reported = 1
		}
		if ValidateP6RushStarterEnd(&series, out, out.OriginalOperates, reported) == nil {
			t.Fatal("diagnostic terminal promoted by end validation")
		}
		if constUtil.P6RushStarterNativeProof != "" && ValidateP6RushStarterEnd(&series, final, final.OriginalOperates, reported) != nil {
			t.Fatal("same certified full automatic record rejected")
		}
		after, _ := json.Marshal(series)
		if string(before) != string(after) {
			t.Fatal("source actor/pose/breach/wrapper mutated")
		}
		for _, change := range []func(*model.FrozenP6RushSeries){func(v *model.FrozenP6RushSeries) { v.PlayerAutomatic = false }, func(v *model.FrozenP6RushSeries) { v.Template.BattleType = 11 }, func(v *model.FrozenP6RushSeries) { v.Start.WinKey = proto.Int32(1) }, func(v *model.FrozenP6RushSeries) { v.Template.PVECompatibilityRule = "local-pve-no-target-finish-v1" }, func(v *model.FrozenP6RushSeries) { v.Template.Attacker.Inst.ExtBattleAdds = []int32{1} }, func(v *model.FrozenP6RushSeries) { v.Template.Monster.Formation[0].BreachedLevel = proto.Int32(1) }, func(v *model.FrozenP6RushSeries) {
			v.Start.BattleMonsterList = append(v.Start.BattleMonsterList, v.Start.BattleMonsterList[0])
		}} {
			var copy model.FrozenP6RushSeries
			if err = json.Unmarshal(before, &copy); err != nil {
				t.Fatal(err)
			}
			change(&copy)
			if validateP6RushStarterScope(&copy) == nil {
				t.Fatal("manual/unknown mode/criteria/extra wave source admitted")
			}
		}
	}
}

func TestP6RushStarterStrictWholeRecordBindingRejectsAnyNumericalOrActionDifference(t *testing.T) {
	source := &pb.BattleOperate{FinalRoundNum: proto.Int32(3), AttackerTotalLeftHp: proto.Float64(12.3), DefenderTotalLeftHp: proto.Float64(0), SkillOpts: []*pb.BattleSkillOperate{{RoundNum: proto.Int32(1), RoundTime: proto.Float64(0), Camp: proto.Int32(0), ActorUid: proto.String("actual-main"), SkillIndex: proto.Int32(0)}, {RoundNum: proto.Int32(2), RoundTime: proto.Float64(1.2), Camp: proto.Int32(1), ActorUid: proto.String("actual-enemy"), SkillIndex: proto.Int32(1)}}}
	original := proto.Clone(source).(*pb.BattleOperate)
	defaults := proto.Clone(source).(*pb.BattleOperate)
	defaults.DefenderTotalLeftHp = nil
	defaults.SkillOpts[0].RoundTime = nil
	defaults.SkillOpts[0].Camp = nil
	defaults.SkillOpts[0].SkillIndex = nil
	if !p6RushStarterStrictRecordEqual(source, defaults) || !proto.Equal(source, original) {
		t.Fatal("explicit scalar defaults or source mutation")
	}
	changes := map[string]func(*pb.BattleOperate){
		"round": func(v *pb.BattleOperate) { v.FinalRoundNum = proto.Int32(4) },
		"currentHPoneULP": func(v *pb.BattleOperate) {
			v.AttackerTotalLeftHp = proto.Float64(math.Nextafter(v.GetAttackerTotalLeftHp(), math.Inf(1)))
		},
		"opClockoneULP": func(v *pb.BattleOperate) {
			v.SkillOpts[1].RoundTime = proto.Float64(math.Nextafter(v.SkillOpts[1].GetRoundTime(), math.Inf(1)))
		},
		"playerClock":  func(v *pb.BattleOperate) { v.SkillOpts[0].RoundTime = proto.Float64(.1) },
		"camp":         func(v *pb.BattleOperate) { v.SkillOpts[1].Camp = proto.Int32(0) },
		"actor":        func(v *pb.BattleOperate) { v.SkillOpts[0].ActorUid = proto.String("other") },
		"skill":        func(v *pb.BattleOperate) { v.SkillOpts[0].SkillIndex = proto.Int32(1) },
		"order":        func(v *pb.BattleOperate) { v.SkillOpts[0], v.SkillOpts[1] = v.SkillOpts[1], v.SkillOpts[0] },
		"missingEnemy": func(v *pb.BattleOperate) { v.SkillOpts = v.SkillOpts[:1] },
		"extraAction": func(v *pb.BattleOperate) {
			v.SkillOpts = append(v.SkillOpts, proto.Clone(v.SkillOpts[0]).(*pb.BattleSkillOperate))
		},
		"nilAction":     func(v *pb.BattleOperate) { v.SkillOpts[0] = nil },
		"transform":     func(v *pb.BattleOperate) { v.AttackerTransOpt = &pb.BattleTransOperate{} },
		"unknownRecord": func(v *pb.BattleOperate) { v.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 1}) },
		"unknownAction": func(v *pb.BattleOperate) { v.SkillOpts[0].ProtoReflect().SetUnknown([]byte{0x98, 0x06, 1}) },
		"nonfinite":     func(v *pb.BattleOperate) { v.SkillOpts[0].RoundTime = proto.Float64(math.NaN()) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			copy := proto.Clone(source).(*pb.BattleOperate)
			change(copy)
			if p6RushStarterStrictRecordEqual(source, copy) {
				t.Fatal("non-equivalent original client record accepted")
			}
		})
	}
}
