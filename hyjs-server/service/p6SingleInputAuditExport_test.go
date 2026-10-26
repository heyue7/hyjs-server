package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// A bounded diagnostic replays one SHA-bound automatic player input only.
// Historical optRecord fields stay source evidence; automatic scope never
// silently turns into a recorded driver. The output cannot grant authority.
func TestP6ExportSingleAutomaticInputAudit(t *testing.T) {
	wirePath := os.Getenv("HYJS_P6_SINGLE_AUDIT_WIRE")
	output := os.Getenv("HYJS_P6_SINGLE_AUDIT_OUTPUT")
	if wirePath == "" && output == "" {
		t.Skip("explicit single-source audit disabled")
	}
	p6ExportSingleAutomaticInputAudit(t, wirePath, output)
}

func p6ExportSingleAutomaticInputAudit(t *testing.T, wirePath, output string) {
	t.Helper()
	analysis, err := filepath.EvalSymlinks("../../analysis/p6")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{wirePath, output} {
		absolute, e := filepath.Abs(name)
		if e != nil || !strings.HasPrefix(absolute, analysis+string(os.PathSeparator)) {
			t.Fatal("owned analysis source/output required")
		}
	}
	if _, err = os.Stat(output); err == nil {
		t.Fatal("preserve existing diagnostic evidence")
	}
	raw, err := readTestAnalysis75(wirePath)
	if err != nil {
		t.Fatal(err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil || wire.GetBattleType() != 2 || wire.Attacker == nil || wire.Defender1 == nil || wire.Defender2 != nil {
		t.Fatal("automatic PVP2 diagnostic source", err)
	}
	s := p6OriginalTables(t)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Seed: wire.GetRandSeed(), BattleType: wire.GetBattleType(), MaxRound: wire.GetMaxRound(), DefenderFirst: wire.Attacker.Inst.GetFirstOrder() == 0 && wire.Defender1.Inst.GetFirstOrder() == 1, AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
	if err != nil || result.Outcome.Authoritative {
		t.Fatal("non-authoritative automatic diagnostic", err)
	}
	encoded, err := json.MarshalIndent(map[string]any{"sourceWire": wirePath, "sourceWireSHA256": p6Digest(raw), "scope": "original-pvp-automatic-ai diagnostic only; historical OptRecord ignored explicitly", "terminal": result}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(encoded, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestP6ExportV105FloatingPointDriftCases(t *testing.T) {
	folder := os.Getenv("HYJS_P6_V105_FLOATING_AUDIT_OUTPUT")
	if folder == "" {
		t.Skip("explicit bounded floating audit disabled")
	}
	for _, index := range []int{4, 11} {
		wire := fmt.Sprintf("../../analysis/p6/native-next-real-skills-inputs-v97/%03d.pb", index)
		absolute, err := filepath.EvalSymlinks(wire)
		if err != nil {
			t.Fatal(err)
		}
		p6ExportSingleAutomaticInputAudit(t, absolute, filepath.Join(folder, fmt.Sprintf("%03d.go.json", index)))
	}
}
