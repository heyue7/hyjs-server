package service

import (
	"encoding/json"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Pure frozen calculations only. Native type8 recorded evidence remains a
// separate release prerequisite; this test cannot certify that capability.
func TestP12WantedRecordedSourceBindingAndTerminalTamper(t *testing.T) {
	s, input := p12WantedOriginalFixture(t)
	automatic, err := s.ComputeP12WantedProtocolFinalHP(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	source := proto.Clone(automatic.OriginalOperates).(*pb.BattleOperate)
	out, err := s.computeP12WantedLiveAutoDiagnostic(t.Context(), input, source)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateP12WantedRecorded(input, source, out); err != nil {
		t.Fatal(err)
	}
	if out.Outcome.Authoritative {
		t.Fatal("uncertified core promoted authority")
	}
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"source", "source-hash", "hp", "configuration"} {
		t.Run(mode, func(t *testing.T) {
			var changed model.CombatFinalHPOutcome
			if json.Unmarshal(body, &changed) != nil {
				t.Fatal("decode")
			}
			supplied := proto.Clone(source).(*pb.BattleOperate)
			switch mode {
			case "source":
				supplied.FinalRoundNum = proto.Int32(supplied.GetFinalRoundNum() + 1)
			case "source-hash":
				changed.OriginalRecordSHA256 = "foreign"
			case "hp":
				changed.FinalActors[0].CurrentHP++
			case "configuration":
				changed.ConfigurationHash = "foreign"
			}
			if validateP12WantedRecorded(input, supplied, &changed) == nil {
				t.Fatal("accepted changed recorded evidence")
			}
		})
	}
	if !constUtil.P12WantedRecordedCertified {
		if _, err = s.computeP12WantedRecorded(t.Context(), input, source); err == nil {
			t.Fatal("missing native proof admitted settlement")
		}
	}
}

// Explicit private inputs only: this diagnoses the current independent native
// live manual-to-auto source, without certifying or writing business state.
func TestP12Wanted65PrivateNativeLiveAutoDiagnostic(t *testing.T) {
	sourcePath, inputPath := os.Getenv("HYJS_WANTED65_NATIVE_SOURCE"), os.Getenv("HYJS_WANTED65_FROZEN_EXPORT")
	if sourcePath == "" || inputPath == "" {
		t.Skip("private native evidence paths not supplied")
	}
	sourceBody, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	var record pb.BattleOperate
	if err = proto.Unmarshal(sourceBody, &record); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	var exported struct {
		Input string `json:"input"`
	}
	var input model.FrozenBattleInput
	if json.Unmarshal(body, &exported) != nil || json.Unmarshal([]byte(exported.Input), &input) != nil {
		t.Fatal("private frozen input decode")
	}
	out, err := (&GameService{}).computeP12WantedLiveAutoDiagnostic(t.Context(), &input, &record)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&record, out.OriginalOperates) || out.Outcome.Authoritative {
		t.Fatal("native source mismatch or diagnostic authority")
	}
	t.Logf("strict native source PASS: round=%d operations=%d hp=%g/%g recordSHA=%s calculationSHA=%s", out.Outcome.FinalRound, len(out.OriginalOperates.SkillOpts), out.OriginalOperates.GetAttackerTotalLeftHp(), out.OriginalOperates.GetDefenderTotalLeftHp(), out.OriginalRecordSHA256, out.CalculationSourceSHA256)
}
