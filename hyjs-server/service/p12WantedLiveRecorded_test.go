package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"testing"
)

func TestP12Wanted86NativeRoundControlsStrictRecord(t *testing.T) {
	raw, err := os.ReadFile("testdata/p12_wanted_live_controls86.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			NativeCaseIndex int    `json:"nativeCaseIndex"`
			RecordBase64    string `json:"recordBase64"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &fixture) != nil || len(fixture.Cases) != 2 {
		t.Fatal("native fixtures")
	}
	for _, c := range fixture.Cases {
		wire := p12WantedNativeTypedInput(t, p12WantedNativeCases(t)[c.NativeCaseIndex])
		s := p6OriginalTables(t)
		input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: 20, BattleType: 8, AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
		if err != nil {
			t.Fatal(err)
		}
		body, err := base64.StdEncoding.DecodeString(c.RecordBase64)
		if err != nil {
			t.Fatal(err)
		}
		record := &pb.BattleOperate{}
		if proto.Unmarshal(body, record) != nil {
			t.Fatal("record")
		}
		if _, err = s.computeP12WantedLiveAutoDiagnostic(t.Context(), input, record); err == nil {
			t.Fatal("new source did not cover a later-round manual timing")
		}
		out, err := s.computeP12WantedRecorded(t.Context(), input, record)
		if err != nil {
			t.Fatal("exact original Wanted recorded source", c.NativeCaseIndex, err)
		}
		if out.Outcome.CapabilityHash != p12WantedLiveRecordedProof || !proto.Equal(record, out.OriginalOperates) {
			t.Fatal("new typed proof missing")
		}
		automatic, err := s.ComputeP12WantedProtocolFinalHP(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		inputJSON, _ := json.Marshal(input)
		originalJSON, _ := json.Marshal(automatic)
		startProto, _ := proto.Marshal(wire)
		fight := db.P12WantedFight{InputJSON: string(inputJSON), InputHash: p6Digest(inputJSON), TerminalJSON: string(originalJSON), TerminalHash: p6Digest(originalJSON), StartProto: startProto, StartHash: p6Digest(startProto)}
		request := &pb.CSRebelFightEnd{FightId: proto.String("public-native-controls"), OptRecord: record}
		if err = p12WantedStoreRecorded(&fight, request, out); err != nil {
			t.Fatal(err)
		}
		fight.EndRequestHash = p6Digest(fight.RecordedEndRequest)
		coldJSON, _ := json.Marshal(fight)
		var cold db.P12WantedFight
		if json.Unmarshal(coldJSON, &cold) != nil {
			t.Fatal("cold fight")
		}
		if err = p12WantedRecordedReceipt(&cold, request); err != nil {
			t.Fatal("cold live recorded receipt", err)
		}
		for _, field := range []string{"proof", "source", "request", "terminal"} {
			bad := cold
			switch field {
			case "proof":
				bad.RecordedProof = "foreign-proof"
			case "source":
				bad.InputHash = "foreign-source"
			case "request":
				bad.EndRequestHash = "foreign-end"
			case "terminal":
				bad.RecordedTerminalHash = "foreign-terminal"
			}
			if p12WantedRecordedReceipt(&bad, request) == nil {
				t.Fatal("forged cold receipt accepted", field)
			}
		}
		for _, field := range []string{"hp", "round", "skill", "owner"} {
			bad := proto.Clone(record).(*pb.BattleOperate)
			switch field {
			case "hp":
				bad.DefenderTotalLeftHp = proto.Float64(bad.GetDefenderTotalLeftHp() + 1)
			case "round":
				bad.FinalRoundNum = proto.Int32(bad.GetFinalRoundNum() + 1)
			case "skill":
				bad.SkillOpts[0].SkillIndex = proto.Int32(99)
			case "owner":
				bad.SkillOpts[0].ActorUid = proto.String("foreign")
			}
			if _, err = s.computeP12WantedRecorded(t.Context(), input, bad); err == nil {
				t.Fatal("forged record accepted", field)
			}
		}
		altered := *input
		altered.BattleType = 9
		if _, err = s.computeP12WantedLiveRecorded(t.Context(), &altered, record); err == nil {
			t.Fatal("foreign mode accepted")
		}
	}
}
func TestP12Wanted86PrivateSingleAuthorizedFight(t *testing.T) {
	path := os.Getenv("HYJS_WANTED86_PRIVATE")
	if path == "" {
		t.Skip("only explicit private authorized fixture")
	}
	raw, err := os.ReadFile(path + "/fight.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Input model.FrozenBattleInput `json:"input"`
	}
	if json.Unmarshal(raw, &source) != nil {
		t.Fatal("source")
	}
	var record *pb.BattleOperate
	body, err := os.ReadFile(path + "/private-current/native.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range splitWanted86Lines(body) {
		var r struct {
			Kind string `json:"kind"`
			Data struct {
				PB string `json:"protobufBase64"`
			} `json:"data"`
		}
		if json.Unmarshal(line, &r) == nil && r.Kind == "original_final_operates" {
			raw, e := base64.StdEncoding.DecodeString(r.Data.PB)
			if e != nil {
				t.Fatal(e)
			}
			record = &pb.BattleOperate{}
			if proto.Unmarshal(raw, record) != nil {
				t.Fatal("record")
			}
		}
	}
	if record == nil {
		t.Fatal("native record missing")
	}
	out, err := (&GameService{Tables: source.Input.Tables}).computeP12WantedRecorded(context.Background(), &source.Input, record)
	if err != nil {
		t.Fatal("authorized native record", err)
	}
	if out.Outcome.CapabilityHash != p12WantedLiveRecordedProof {
		t.Fatal("expected live proof")
	}
	t.Logf("native exact operations=%d round=%d", len(record.SkillOpts), record.GetFinalRoundNum())
}
func splitWanted86Lines(raw []byte) [][]byte {
	var rows [][]byte
	start := 0
	for i, x := range raw {
		if x == '\n' {
			rows = append(rows, raw[start:i])
			start = i + 1
		}
	}
	return append(rows, raw[start:])
}
