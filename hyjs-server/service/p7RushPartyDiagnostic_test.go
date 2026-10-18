package service

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p7PartyRushCase struct {
	Case     string  `json:"case"`
	Wire     string  `json:"wire"`
	WireSHA  string  `json:"wireSHA256"`
	Typed    string  `json:"typedCallback"`
	TypedSHA string  `json:"typedCallbackSHA256"`
	AutoAt   float64 `json:"manualAutoStartRoundTime"`
}

func p7PartyRushSources(t *testing.T) (*GameService, []p7PartyRushCase) {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p6/p7-party-rush-pending-20261016/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		ActorSHA string            `json:"actorSourceSHA256"`
		Cases    []p7PartyRushCase `json:"cases"`
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.ActorSHA != "d0e70186dc665f5e894eaae0a30915cb8032beb2ab86e0950ebd3e0ada41fa94" || len(manifest.Cases) != 4 {
		t.Fatal("bounded historical actual party source changed")
	}
	return p6OriginalTables(t), manifest.Cases
}

func p7PartyRushSeries(t *testing.T, s *GameService, source p7PartyRushCase) *model.FrozenP6RushSeries {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../analysis/p6/p7-party-rush-pending-20261016", source.Typed))
	if err != nil || p6Digest(raw) != source.TypedSHA {
		t.Fatal("typed party source changed", err)
	}
	start := &pb.SCRushFightStart{}
	if proto.Unmarshal(raw, start) != nil {
		t.Fatal("typed party source malformed")
	}
	series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	return series
}

func TestP7PartyRushBoundedDiagnosticNoBusinessAuthority(t *testing.T) {
	s, sources := p7PartyRushSources(t)
	for _, source := range sources {
		t.Run(source.Case, func(t *testing.T) {
			series := p7PartyRushSeries(t, s, source)
			before, _ := json.Marshal(series)
			if err := validateP6RushManualScope(series); err == nil {
				t.Fatal("old single047 proof admitted an unproven two-actor family")
			}
			out, err := s.ComputeP7PartyRushDiagnostic(t.Context(), series, source.AutoAt)
			if err != nil {
				t.Fatal("actual diagnostic must retain any real computation failure", err)
			}
			if out.Outcome.Authoritative || len(out.FinalActors) != 3 {
				t.Fatal("diagnostic source granted authority or lost a real actor")
			}
			after, _ := json.Marshal(series)
			if string(before) != string(after) {
				t.Fatal("diagnostic changed historical actor/input bytes")
			}
			if dir := os.Getenv("HYJS_P7_PARTY_EXPORT"); dir != "" {
				if err = os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(out)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(dir, source.Wire+".go-terminal.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestP7PartyRushOriginalFinalRecordPending(t *testing.T) {
	file := os.Getenv("HYJS_P7_PARTY_NATIVE_RECORDS")
	if file == "" {
		t.Skip("PENDING: four original typed9 party cases have not been captured; diagnostic is never business authority")
	}
	raw, err := os.ReadFile(file)
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P7_PARTY_NATIVE_RECORDS_SHA256") {
		t.Fatal("original record fixture SHA", err)
	}
	var native struct {
		Cases []struct {
			WireSHA string `json:"wireSHA256"`
			Record  struct {
				Base64 string `json:"protobufBase64"`
			} `json:"originalFinalRecord"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &native) != nil || len(native.Cases) != 4 {
		t.Fatal("all four original final records are required")
	}
	s, sources := p7PartyRushSources(t)
	for _, source := range sources {
		found := false
		for _, row := range native.Cases {
			if row.WireSHA == source.WireSHA {
				found = true
				data, err := base64.StdEncoding.DecodeString(row.Record.Base64)
				if err != nil {
					t.Fatal(err)
				}
				record := &pb.BattleOperate{}
				if proto.Unmarshal(data, record) != nil {
					t.Fatal("native original record malformed")
				}
				out, err := s.ComputeP7PartyRushDiagnostic(t.Context(), p7PartyRushSeries(t, s, source), source.AutoAt)
				if err != nil {
					t.Fatal(err)
				}
				if !proto.Equal(record, out.OriginalOperates) {
					t.Fatal("original typed9 complete party record mismatch", source.Case)
				}
			}
		}
		if !found {
			t.Fatal("native original record missing source", source.Case)
		}
	}
	// 此项仅终末原协议逐字段比较，完整核/actor/manager/exports仍需现有原机矩阵。
}
