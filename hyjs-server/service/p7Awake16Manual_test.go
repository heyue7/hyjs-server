package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"testing"
)

// 代表来自真实原机HTTP81011。实际命令只驱动我方，伤害/敌方AI/终末由现有原引擎移植重算。
func TestP7Awake16ActualPhoneManualRecordStrictReplay(t *testing.T) {
	root := "../../analysis/playable-plan-20261017"
	frozen, err := os.ReadFile(filepath.Join(root, "awake-phone-r2/frozen-combat-input.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := decodeP6StoredInput(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var input model.FrozenBattleInput
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "awake-original-end-capture-r1/original-End.pb"))
	if err != nil || p6Digest(body) != "4696d6ac79fcaf3fd906e38ef77b1482a7a81dd1b143068ae17093ed62580b21" {
		t.Fatal("original phone End pin", err)
	}
	end := &pb.CSAwakeCopyFightEnd{}
	if err = proto.Unmarshal(body, end); err != nil {
		t.Fatal(err)
	}
	if err = validateP7Awake16ManualRecord(&input, end.GetOptRecord()); err != nil {
		t.Fatal("manual record initial scope", err)
	}
	check, _ := json.Marshal(input.Tables)
	if p6Digest(check) != input.ConfigurationHash {
		t.Fatalf("frozen tables digest differs %s != %s", p6Digest(check), input.ConfigurationHash)
	}
	terminal, err := (&GameService{}).computeP7Awake16ManualTerminal(t.Context(), &input, end.GetOptRecord())
	if err != nil {
		t.Fatal("original phone command replay", err)
	}
	if err = validateP7Awake16ManualTerminal(&input, terminal, end.GetOptRecord(), end.GetIsWinClient()); err != nil {
		t.Fatalf("whole original phone record/terminal mismatch: %v; supplied=%v computed=%v", err, end.GetOptRecord(), terminal.OriginalOperates)
	}
	// 客户端终末与赢家可审计，真实回放输出不能沿用自动family的能力标识。
	if terminal.Outcome.Authoritative || terminal.Outcome.CapabilityHash != "" {
		t.Fatal("diagnostic replay promoted authority")
	}
	changed := proto.Clone(end.GetOptRecord()).(*pb.BattleOperate)
	changed.SkillOpts[0].ActorUid = proto.String("foreign-owner")
	if err = validateP7Awake16ManualRecord(&input, changed); err == nil {
		t.Fatal("foreign actor command admitted")
	}
	if err = validateP7Awake16ManualTerminal(&input, terminal, end.GetOptRecord(), 1-end.GetIsWinClient()); err == nil {
		t.Fatal("fake win admitted")
	}
	changed = proto.Clone(end.GetOptRecord()).(*pb.BattleOperate)
	changed.AttackerTotalLeftHp = proto.Float64(changed.GetAttackerTotalLeftHp() + 1)
	if err = validateP7Awake16ManualTerminal(&input, terminal, changed, end.GetIsWinClient()); err == nil {
		t.Fatal("fake final HP admitted")
	}
	if constUtil.P7Awake16ManualProof == "" {
		if _, err = (&GameService{}).ComputeP7Awake16ManualSettlement(t.Context(), &input, end.GetOptRecord(), end.GetIsWinClient()); err == nil {
			t.Fatal("empty independent proof admitted live settlement")
		}
	} else {
		accepted, computeErr := (&GameService{}).ComputeP7Awake16ManualSettlement(t.Context(), &input, end.GetOptRecord(), end.GetIsWinClient())
		if computeErr != nil || !accepted.Outcome.Authoritative || accepted.Outcome.CapabilityHash != constUtil.P7Awake16ManualProof {
			t.Fatal("independent manual authorization", computeErr)
		}
		if err = validateP7Awake16EndReceipt(&input, accepted, end.GetOptRecord(), end.GetIsWinClient()); err != nil {
			t.Fatal("cold manual receipt", err)
		}
		if err = ValidateP7Awake16CertifiedReceipt(&input, accepted); err == nil {
			t.Fatal("manual receipt reused automatic certificate")
		}
	}
	if output := os.Getenv("HYJS_P7_AWAKE_MANUAL_OUTPUT"); output != "" {
		report := struct {
			OriginalEndSHA256     string                      `json:"originalEndSHA256"`
			OriginalPhoneCaptured bool                        `json:"originalPhoneCaptured"`
			StrictRecordMatched   bool                        `json:"strictRecordMatched"`
			FrozenSHA256          string                      `json:"frozenSHA256"`
			Terminal              *model.CombatFinalHPOutcome `json:"terminal"`
		}{p6Digest(body), true, true, p6Digest(frozen), terminal}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
