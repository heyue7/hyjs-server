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

// Both consumers use unchanged original typed sources and independently exported
// native PBs. Formation controls do not supply skill decisions to the server.
func TestP7RoundOneAutoFormationOriginalNativeConsumersAndCold(t *testing.T) {
	s := p6OriginalTables(t)
	root := "../../analysis/playable-plan-20261017/round1-trans-auto-r3/"
	read := func(name, digest string) []byte {
		t.Helper()
		raw, err := os.ReadFile(name)
		if err != nil || p6Digest(raw) != digest {
			t.Fatal("fixed original source", name, err)
		}
		return raw
	}
	for _, name := range []string{"rush9", "awake16"} {
		t.Run(name, func(t *testing.T) {
			var expected pb.BattleOperate
			if err := proto.Unmarshal(read(root+name+"-original-operates.pb", "fc6b8af25388775e2c5c22c417fac4c3013ae2cccf1ccfb1f79fc5a4f7f39bb4"), &expected); err != nil {
				t.Fatal(err)
			}
			var out *model.CombatFinalHPOutcome
			var err error
			var validate func(*model.CombatFinalHPOutcome) error
			if name == "rush9" {
				var start pb.SCRushFightStart
				if err = proto.Unmarshal(read("../../analysis/p6/p7-party-rush-pending-20261016/000.SCRushFightStart.pb", "eab3a262ae1fdae9ed8217e66dc6d848faa9918041d21f0ac31f481e37e7889a"), &start); err != nil {
					t.Fatal(err)
				}
				series, e := s.FreezeP6RushSeriesInput(&start, true, false, nil, constUtil.P6OriginalClientHash)
				if e != nil {
					t.Fatal(e)
				}
				out, err = p7GenericMechanismRushComputeEnd(t.Context(), s, series, &expected, 1)
				validate = func(v *model.CombatFinalHPOutcome) error {
					return p7GenericMechanismRushValidateEnd(series, v, &expected, 1)
				}
			} else {
				var source model.FrozenBattleInput
				if err = json.Unmarshal(read(root+"awake16-frozen-input.json", "32ff5ed964ce84cb5a6c06406f3c7b51511ee4753f150c1cbbfd792203d77503"), &source); err != nil {
					t.Fatal(err)
				}
				input, e := s.FreezeBattleInput(source)
				if e != nil {
					t.Fatal(e)
				}
				out, err = s.ComputeP7Awake16GenericSettlement(t.Context(), input, &expected, 1)
				validate = func(v *model.CombatFinalHPOutcome) error {
					return validateP7Awake16GenericEnd(t.Context(), input, v, &expected, 1)
				}
			}
			if err != nil {
				t.Fatal("original round1 formation/Auto consumer", err)
			}
			if !out.Outcome.Authoritative || out.Outcome.DiagnosticAIDriverPolicy != constUtil.P6RoundOneLiveAutoFormationDriver || !proto.Equal(out.OriginalOperates, &expected) {
				t.Fatal("actual control driver/full PB lost")
			}
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			var cold model.CombatFinalHPOutcome
			if json.Unmarshal(raw, &cold) != nil || validate(&cold) != nil {
				t.Fatal("serialized control source/driver not recoverable")
			}
			cold.Outcome.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
			if validate(&cold) == nil {
				t.Fatal("new control receipt borrowed old driver")
			}
		})
	}
}
