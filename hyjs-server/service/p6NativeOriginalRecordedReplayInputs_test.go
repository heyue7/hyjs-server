package service

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// These records are original GetRecordOperates output, never Go predictions.
// Both-camp entries exercise the original PVE camp0 driver and automatic enemy.
func p6ActualOriginalRecordedZeroCases(t *testing.T) []*pb.SCFightPlayback {
	t.Helper()
	raw, err := readTestAnalysis75("../../analysis/p6/native-actual-paid-copy-auto-original-records-v86.json")
	if err != nil || p6Digest(raw) != "012ed908ee4175d49644ce153d182af0806476637c4362287d8ff9b5f3d0137b" {
		t.Fatal("original recorded source fixture SHA", err)
	}
	var source struct {
		Cases []struct {
			Wire   string `json:"wire"`
			SHA    string `json:"wireSHA256"`
			Record struct {
				Binary string `json:"protobufBase64"`
				SHA    string `json:"protobufSHA256"`
			} `json:"originalFinalRecord"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &source); err != nil || len(source.Cases) != 2 {
		t.Fatal("original recorded source two cases", err)
	}
	var cases []*pb.SCFightPlayback
	for _, c := range source.Cases {
		raw, err = readTestAnalysis75(filepath.Join("../..", c.Wire))
		if err != nil || p6Digest(raw) != c.SHA {
			t.Fatal("original zero-factor source PB SHA", err)
		}
		wire := &pb.SCFightPlayback{}
		if err = proto.Unmarshal(raw, wire); err != nil {
			t.Fatal(err)
		}
		raw, err = base64.StdEncoding.Strict().DecodeString(c.Record.Binary)
		if err != nil || p6Digest(raw) != c.Record.SHA {
			t.Fatal("actual original GetRecordOperates protobuf SHA", err)
		}
		wire.OptRecord = &pb.BattleOperate{}
		if err = proto.Unmarshal(raw, wire.OptRecord); err != nil {
			t.Fatal(err)
		}
		// The original diagnostic SC input had scalar-default0/0. Preserve
		// exactly those effective values; these probes are separate from paid
		// natural Clear1/1 and cannot certify natural win/settlement scopes.
		wire.AttackerFactor = proto.Float64(wire.GetAttackerFactor())
		wire.DefenderFactor = proto.Float64(wire.GetDefenderFactor())
		cases = append(cases, wire)
	}
	return cases
}

func TestP6NativeOriginalRecordedBothCampsSuppliedReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for _, wire := range p6ActualOriginalRecordedZeroCases(t) {
		input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
		if err != nil {
			t.Fatal(err)
		}
		original := proto.Clone(wire.OptRecord).(*pb.BattleOperate)
		out, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, wire.OptRecord)
		if err != nil || !proto.Equal(original, wire.OptRecord) || !proto.Equal(original, out.SuppliedOriginalOperates) || !p6OriginalProtocolRecordEqual(original, out.OriginalOperates) || out.Outcome.Authoritative {
			t.Fatal("actual original both-camp recorded replay or source isolation", err)
		}
	}
}

func TestP6ExportActualOriginalBothCampsRecordedReplayInputs(t *testing.T) {
	directory := os.Getenv("HYJS_P6_ORIGINAL_RECORDED_REPLAY_OUTPUT")
	if directory == "" {
		t.Skip("explicit original recorded replay export disabled")
	}
	root, err := filepath.EvalSymlinks("../../analysis/p6")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	directory, err = filepath.Abs(directory)
	if err != nil || !strings.HasPrefix(directory, root+string(filepath.Separator)) {
		t.Fatal("owned original replay evidence directory required", err)
	}
	if err = os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	var cases []map[string]any
	for index, wire := range p6ActualOriginalRecordedZeroCases(t) {
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		name := []string{"000.pb", "001.pb"}[index]
		if err = os.WriteFile(filepath.Join(directory, name), raw, 0644); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"case": []string{"original-diagnostic-zero-factor/task10/both-camp-recorded", "original-diagnostic-zero-factor/task12/both-camp-recorded"}[index], "wire": name, "wireSHA256": p6Digest(raw), "fightId": wire.GetFightid(), "seed": wire.GetRandSeed(), "battleType": 1, "firstCamp": 0, "damageFactors": map[string]float64{"attacker": 0, "defender": 0}, "sourceOriginalRecordedFixture": "analysis/p6/native-actual-paid-copy-auto-original-records-v86.json", "sourceOriginalRecordedFixtureSHA256": "012ed908ee4175d49644ce153d182af0806476637c4362287d8ff9b5f3d0137b", "sourceBoundary": "Actual original diagnostic SC initialization0/0 and actual original GetRecordOperates full both-camp protobuf; natural actors came from genuine paid source but these effective diagnostic factors differ from natural Clear1/1. No Go predictedrecord or serverwinner."})
	}
	raw, err := json.MarshalIndent(map[string]any{"scope": "Original diagnostic zero-factor PVE playback with actual original full both-camp records; separate from natural paid factors1/1; native numerical/record comparison pending", "nativeCompared": false, "authoritative": false, "cases": cases}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "manifest.json"), append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Log("actual original full record replay input manifest", p6Digest(append(raw, '\n')))
}
