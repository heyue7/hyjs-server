package service

import (
	"encoding/base64"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
	"os"
	"testing"
)

func TestP6EliteManualOriginalWholeEndDistinctProof(t *testing.T) {
	s := p6OriginalTables(t)
	raw, err := os.ReadFile("../../analysis/p6/native-elite-manual-original-records-v168.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Record struct {
				Binary string `json:"protobufBase64"`
			} `json:"originalFinalRecord"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &fixture) != nil || len(fixture.Cases) != 2 {
		t.Fatal("original exact native records")
	}
	for i, name := range []string{"000", "001"} {
		raw, err = os.ReadFile("../../analysis/p6/original-elite-manual-inputs-v165/" + name + ".SCRushFightStart.pb")
		if err != nil {
			t.Fatal(err)
		}
		start := &pb.SCRushFightStart{}
		if proto.Unmarshal(raw, start) != nil {
			t.Fatal("typed callback")
		}
		series, err := s.FreezeP6RushSeriesInput(start, false, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		raw, err = base64.StdEncoding.DecodeString(fixture.Cases[i].Record.Binary)
		if err != nil {
			t.Fatal(err)
		}
		record := &pb.BattleOperate{}
		if proto.Unmarshal(raw, record) != nil {
			t.Fatal("original record")
		}
		out, err := s.ComputeP6RushManualAutoSettlement(t.Context(), series, record, 0)
		if err != nil {
			t.Fatal(err)
		}
		if out.Outcome.CapabilityHash != constUtil.P6RushEliteManualNativeProof || out.Outcome.CapabilityHash == constUtil.P6RushManualNativeProof {
			t.Fatal("mode proof mixed")
		}
		out.Outcome.CapabilityHash = constUtil.P6RushManualNativeProof
		if ValidateP6RushManualAutoEnd(series, out, record, 0) == nil {
			t.Fatal("Trial proof authorized Elite")
		}
	}
	if constUtil.P6RushManualNativeProof != "da040619a233c792bc90bbcb4b7eb03e89c3c154de1a6df7f5f4d0fd03b7c2ff" {
		t.Fatal("old Trial identity changed")
	}
}
