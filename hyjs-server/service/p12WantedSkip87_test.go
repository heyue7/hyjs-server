package service

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Public original-engine inputs are derived only by setting each enemy's
// initial current HP to 1. This tests real calculation and strict records,
// without claiming a natural player progression or a device capture.
func TestP12Wanted87OriginalSkipKillAndLateFinishRejected(t *testing.T) {
	raw, err := os.ReadFile("testdata/p12_wanted_skip87.json")
	if err != nil || p6Digest(raw) != "48e9e669ef6cc80ef01166d8e6b4878b63e27d79fbac091658beb5d0ac328e15" {
		t.Fatal("fixed original Skip fixture", err)
	}
	var fixture struct {
		Cases []struct {
			Name             string `json:"name"`
			Wire             string `json:"wire"`
			WireSHA256       string `json:"wireSHA256"`
			Record           string `json:"record"`
			RecordSHA256     string `json:"recordSHA256"`
			LateFinish       string `json:"lateFinish"`
			LateFinishSHA256 string `json:"lateFinishSHA256"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &fixture) != nil || len(fixture.Cases) != 2 {
		t.Fatal("original Skip case count")
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			decode := func(encoded, digest string, target proto.Message) {
				t.Helper()
				body, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil || p6Digest(body) != digest || proto.Unmarshal(body, target) != nil {
					t.Fatal("fixed original protobuf source", err)
				}
			}
			wire := &pb.SCFightPlayback{}
			record, late := &pb.BattleOperate{}, &pb.BattleOperate{}
			decode(row.Wire, row.WireSHA256, wire)
			decode(row.Record, row.RecordSHA256, record)
			decode(row.LateFinish, row.LateFinishSHA256, late)
			if wire.GetBattleType() != 8 || len(wire.GetDefender2().GetFormation()) != 4 || record.GetFinalRoundNum() != 1 || late.GetFinalRoundNum() != 2 || record.GetDefenderTotalLeftHp() != 0 {
				t.Fatal("original skipped kill and post-end FinishRound boundary")
			}
			for _, actor := range wire.Defender2.Formation {
				if actor.GetInst().GetCurHp() != 1 {
					t.Fatal("finite initial HP derivation differs")
				}
			}
			changed := proto.Clone(record).(*pb.BattleOperate)
			changed.FinalRoundNum = proto.Int32(2)
			if !proto.Equal(changed, late) {
				t.Fatal("late tail changed more than the observed final round")
			}
			s := p6OriginalTables(t)
			input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: 8, AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
			if err != nil {
				t.Fatal(err)
			}
			out, err := s.computeP12WantedRecorded(t.Context(), input, record)
			if err != nil || out == nil || out.Outcome == nil || out.Outcome.Winner != 0 || !out.Outcome.Authoritative || !proto.Equal(record, out.OriginalOperates) {
				t.Fatal("original Skip kill rejected by independent calculation", err)
			}
			if _, err := s.computeP12WantedRecorded(t.Context(), input, late); err == nil {
				t.Fatal("post-end altered record accepted")
			}
		})
	}
}
