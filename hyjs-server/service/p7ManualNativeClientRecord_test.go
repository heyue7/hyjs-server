package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"os"
	"testing"
)

// Export a finite client-control fixture for a separately started isolated HTTP
// run. This does not call any DB or certify an actual physical phone operation.
func TestP7ManualNativeClientRecord(t *testing.T) {
	source := os.Getenv("HYJS_P7_MANUAL_CLIENT_INPUT")
	destination := os.Getenv("HYJS_P7_MANUAL_CLIENT_OUTPUT")
	if source == "" || destination == "" {
		t.Skip("explicit isolated native test control fixture")
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	var input model.FrozenBattleInput
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	input.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(30.100000000000158)
	out, err := (&GameService{}).ComputeOriginalP6ProtocolFinalHP(t.Context(), &input, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(out.OriginalOperates)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(destination, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
