package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p6RoundOneNativeActor struct {
	UID         string `json:"uid"`
	RuntimeFPID int32  `json:"runtimeFPID"`
	Attribute   struct {
		HP, MaxHP float64
	} `json:"attribute"`
}

// The independent original caller executes real DoTrans before live Auto0;
// expected SkillOpts are exported afterward and never drive its original AI.
func TestP6RoundOneLiveAutoFormationOriginalNative(t *testing.T) {
	s := p6OriginalTables(t)
	root := "../../analysis/playable-plan-20261017/round1-trans-auto-r3"
	for _, item := range []struct{ name, inputSHA string }{
		{"rush9", "4f59d34b359b5021b92b896940cb0660c84f0eb4c4084d35372fd0f2d9b3cc47"},
		{"awake16", "32ff5ed964ce84cb5a6c06406f3c7b51511ee4753f150c1cbbfd792203d77503"},
	} {
		t.Run(item.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, item.name+"-frozen-input.json"))
			if err != nil || p6Digest(raw) != item.inputSHA {
				t.Fatal("original unchanged input", err)
			}
			var source model.FrozenBattleInput
			if err = json.Unmarshal(raw, &source); err != nil {
				t.Fatal(err)
			}
			// Historical field names are recanonicalized through the same original
			// table loader; actor/seed/Start values are retained byte-for-byte.
			input, err := s.FreezeBattleInput(source)
			if err != nil {
				t.Fatal(err)
			}
			expectedBytes, err := os.ReadFile(filepath.Join(root, item.name+"-original-operates.pb"))
			if err != nil || p6Digest(expectedBytes) != "fc6b8af25388775e2c5c22c417fac4c3013ae2cccf1ccfb1f79fc5a4f7f39bb4" {
				t.Fatal("independent native completePB", err)
			}
			controlBytes, err := os.ReadFile(filepath.Join(root, item.name+"-formation.pb"))
			if err != nil || p6Digest(controlBytes) != "ca7602185a7558cd52810499d35af6234261e43fdd3883b4c35b7da3a65574c4" {
				t.Fatal("separate original formation", err)
			}
			var expected pb.BattleOperate
			var control pb.BattleTransOperate
			if err = proto.Unmarshal(expectedBytes, &expected); err != nil {
				t.Fatal(err)
			}
			if err = proto.Unmarshal(controlBytes, &control); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(input)
			out, err := s.ComputeP6RoundOneLiveAutoFormation(t.Context(), input, &expected, &control)
			if err != nil {
				t.Fatal("round1 real native fullPB comparison", err)
			}
			if !proto.Equal(&expected, out.OriginalOperates) || out.Outcome.Authoritative || out.Outcome.CapabilityHash != "" || out.Outcome.DiagnosticAIDriverPolicy != constUtil.P6RoundOneLiveAutoFormationDriver {
				t.Fatal("whole PB or new driver authority changed")
			}
			recordHash, sourceHash, err := P6RoundOneLiveAutoFormationSourceDigests(input, &expected, &control)
			if err != nil || recordHash != out.OriginalRecordSHA256 || sourceHash != out.CalculationSourceSHA256 {
				t.Fatal("new control source not closed", err)
			}
			var terminal struct {
				State struct {
					Attacker struct {
						Actors []p6RoundOneNativeActor `json:"actors"`
					} `json:"attacker"`
					Defender struct {
						Actors []p6RoundOneNativeActor `json:"actors"`
					} `json:"defender"`
				} `json:"state"`
			}
			native, err := os.ReadFile(filepath.Join(root, item.name+"-terminal.json"))
			if err != nil || json.Unmarshal(native, &terminal) != nil {
				t.Fatal("native terminal", err)
			}
			actors := append(terminal.State.Attacker.Actors, terminal.State.Defender.Actors...)
			if len(actors) != len(out.FinalActors) {
				t.Fatal("native actor count")
			}
			for _, actual := range out.FinalActors {
				found := false
				for _, original := range actors {
					if original.UID != actual.UID {
						continue
					}
					found = true
					runtime := actual.RuntimePose
					if runtime == 0 {
						runtime = actual.Pose
					}
					if original.Attribute.HP != actual.CurrentHP || original.Attribute.MaxHP != actual.MaxHP || original.RuntimeFPID != runtime {
						t.Fatal("native terminal HP/DoTrans MainPosition differs", original, actual)
					}
				}
				if !found {
					t.Fatal("extra non-native actor", actual.UID)
				}
			}
			after, _ := json.Marshal(input)
			recordAfter, _ := proto.MarshalOptions{Deterministic: true}.Marshal(&expected)
			controlAfter, _ := proto.MarshalOptions{Deterministic: true}.Marshal(&control)
			if !bytes.Equal(before, after) || !bytes.Equal(recordAfter, expectedBytes) || !bytes.Equal(controlAfter, controlBytes) {
				t.Fatal("frozen source or original PB/control mutated")
			}
			// A changed same-frame time must fail exact full-record equality;
			// PlaybackAI's one-message clock tolerance is never used by live Auto.
			changed := proto.Clone(&expected).(*pb.BattleOperate)
			changed.SkillOpts[1].RoundTime = proto.Float64(0.1)
			if _, err = s.ComputeP6RoundOneLiveAutoFormation(t.Context(), input, changed, &control); err == nil {
				t.Fatal("modified original command time accepted")
			}
			badControl := proto.Clone(&control).(*pb.BattleTransOperate)
			badControl.RoundNum = proto.Int32(3)
			changed = proto.Clone(&expected).(*pb.BattleOperate)
			changed.AttackerTransOpt = badControl
			if _, _, err = P6RoundOneLiveAutoFormationSourceDigests(input, changed, badControl); err == nil {
				t.Fatal("unrecorded later-round control accepted")
			}
		})
	}
}
