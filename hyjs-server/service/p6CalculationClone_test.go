package service

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6CalculationCloneRetainsOriginalWireAndIsolatesAllLayers(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "owner", "047_1,047_2,047_3", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 3)
	f.WinConditions = []*pb.BattleWinCondition{{WinKey: proto.Int32(0), WinValue: proto.Int32(7)}, nil}
	f.KFReplay = &pb.KFReplayData{PlayerList: []*pb.BattlePlayer{f.Attacker, f.Defender}}
	f.AppliedProperties = map[string][]string{"owner": {"hp", "attack"}, "nil": nil, "empty": {}}
	f.Tables["nilRows"] = nil
	f.Tables["emptyRows"] = []map[string]string{}
	f.Tables["nilRow"] = []map[string]string{nil, {"Id": "retained"}}
	// Unknown protobuf fields and explicit zero presence must survive the copy.
	f.Attacker.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
	f.Attacker.Inst.FirstOrder = proto.Int32(0)
	cloned, err := cloneP6CalculationInput(f)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(cloned.Attacker, f.Attacker) || !proto.Equal(cloned.KFReplay, f.KFReplay) || !proto.Equal(cloned.WinConditions[0], f.WinConditions[0]) || !reflect.DeepEqual(cloned.Tables, f.Tables) || !reflect.DeepEqual(cloned.AppliedProperties, f.AppliedProperties) {
		t.Fatal("original wire, presence or ordered table shapes changed")
	}
	cloned.Attacker.Formation[0].Inst.CurHp = proto.Float64(1)
	cloned.KFReplay.PlayerList[0].Formation[0].Inst.CurHp = proto.Float64(2)
	cloned.WinConditions[0].WinValue = proto.Int32(8)
	cloned.AppliedProperties["owner"][0] = "changed"
	cloned.Tables["Hero"][0]["Id"] = "changed"
	cloned.Tables["nilRow"][1]["Id"] = "changed"
	if f.Attacker.Formation[0].Inst.GetCurHp() != 1000 || f.KFReplay.PlayerList[0].Formation[0].Inst.GetCurHp() != 1000 || f.WinConditions[0].GetWinValue() != 7 || f.AppliedProperties["owner"][0] != "hp" || f.Tables["Hero"][0]["Id"] == "changed" || f.Tables["nilRow"][1]["Id"] != "retained" {
		t.Fatal("calculation copy aliases original mutable input")
	}
}

func TestP6CalculationCloneRejectsNonfiniteNestedKFPanel(t *testing.T) {
	for _, number := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		input := &model.FrozenBattleInput{KFReplay: &pb.KFReplayData{PlayerList: []*pb.BattlePlayer{{Formation: []*pb.BattleHero{{Inst: &pb.BattleHeroInst{CurHp: proto.Float64(number)}}}}}}}
		if _, err := cloneP6CalculationInput(input); err == nil {
			t.Fatal("nested KF nonfinite panel accepted", number)
		}
	}
}

func TestP6CalculationCopyOriginalTablePerformanceComparison(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "owner", "047_1", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 3)
	// Compare the two isolation paths over the actual frozen original table set.
	// Timing is diagnostic only; correctness and input isolation are the gates.
	start := time.Now()
	for i := 0; i < 10; i++ {
		raw, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		var old model.FrozenBattleInput
		if err = json.Unmarshal(raw, &old); err != nil || !reflect.DeepEqual(old.Tables, f.Tables) {
			t.Fatal("previous JSON baseline", err)
		}
	}
	oldDuration := time.Since(start)
	start = time.Now()
	for i := 0; i < 10; i++ {
		cloned, err := cloneP6CalculationInput(f)
		if err != nil || !reflect.DeepEqual(cloned.Tables, f.Tables) {
			t.Fatal("typed clone equivalent original tables", err)
		}
	}
	t.Logf("original table isolation 10 copies: JSON=%s typed=%s", oldDuration, time.Since(start))
}
