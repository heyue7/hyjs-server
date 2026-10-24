package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func p7RoundLiveAutoPhone66(t *testing.T) (*GameService, *model.FrozenP6RushSeries, []*pb.CSRushFightEnd) {
	t.Helper()
	return p7RoundLiveAutoPhone66Fixture(t, "testdata/p7_round_live_auto_phone66.json")
}

func p7RoundLiveAutoPhone66Fixture(t *testing.T, filename string) (*GameService, *model.FrozenP6RushSeries, []*pb.CSRushFightEnd) {
	t.Helper()
	var fixture struct {
		StartHex string `json:"startHex"`
		Ends     []struct {
			SHA    string `json:"sha256"`
			EndHex string `json:"endHex"`
		} `json:"ends"`
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	start := &pb.SCRushFightStart{}
	body, err := hex.DecodeString(fixture.StartHex)
	if err != nil {
		t.Fatal(err)
	}
	if err = proto.Unmarshal(body, start); err != nil {
		t.Fatal(err)
	}
	s, _, _ := p6ActualManualSeries(t)
	series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	var ends []*pb.CSRushFightEnd
	for _, source := range fixture.Ends {
		body, err = hex.DecodeString(source.EndHex)
		if err != nil || p6Digest(body) != source.SHA {
			t.Fatal("original End digest", err)
		}
		end := &pb.CSRushFightEnd{}
		if err = proto.Unmarshal(body, end); err != nil || end.GetFightId() != start.GetFightId() {
			t.Fatal("original End owner", err)
		}
		ends = append(ends, end)
	}
	return s, series, ends
}

func TestP7RoundLiveAuto66PhoneAndColdReceipt(t *testing.T) {
	s, series, ends := p7RoundLiveAutoPhone66(t)
	original, _ := json.Marshal(series)
	for index, end := range ends {
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		controls, err := p7RoundLiveAutoControls(series, end.GetOptRecord())
		if err != nil {
			t.Fatal(err)
		}
		out, err := p7RoundLiveAutoRushComputeEnd(ctx, s, series, end.GetOptRecord(), end.GetResult())
		cancel()
		if err != nil {
			t.Fatalf("actual phone %d: %v", index, err)
		}
		if !proto.Equal(end.GetOptRecord(), out.OriginalOperates) {
			t.Fatal("full actual PB mismatch")
		}
		body, _ := json.Marshal(out)
		if folder := os.Getenv("HYJS_ROUND66_OUTPUT"); folder != "" {
			if len(body) > 10*1024*1024 {
				t.Fatal("bounded diagnostic output exceeded")
			}
			if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("%03d.go-final.json", index)), body, 0600); err != nil {
				t.Fatal(err)
			}
		}
		var cold model.CombatFinalHPOutcome
		if err = json.Unmarshal(body, &cold); err != nil {
			t.Fatal(err)
		}
		if err = p7GenericMechanismRushValidateEnd(series, &cold, end.GetOptRecord(), end.GetResult()); err != nil {
			t.Fatal("cold receipt", err)
		}
		t.Logf("phone=%d controls=%v skills=%d round=%d hp=%g/%g fullPB=true", index, controls, len(out.OriginalOperates.SkillOpts), out.Outcome.FinalRound, out.OriginalOperates.GetAttackerTotalLeftHp(), out.OriginalOperates.GetDefenderTotalLeftHp())
		if index == len(ends)-1 {
			wrongDriver := cold
			wrongOutcome := *cold.Outcome
			wrongDriver.Outcome = &wrongOutcome
			wrongOutcome.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
			if p7GenericMechanismRushValidateEnd(series, &wrongDriver, end.GetOptRecord(), end.GetResult()) == nil {
				t.Fatal("new receipt downgraded to old driver")
			}
			wrongOutcome.DiagnosticAIDriverPolicy = constUtil.P6RoundLiveAutoDriver
			wrongOutcome.CapabilityHash = constUtil.P7GenericMechanismCompatibilityProof
			if p7GenericMechanismRushValidateEnd(series, &wrongDriver, end.GetOptRecord(), end.GetResult()) == nil {
				t.Fatal("new receipt borrowed old proof")
			}
			eliteStart := proto.Clone(series.Start).(*pb.SCRushFightStart)
			eliteStart.RushType = proto.Int32(constUtil.P7RushElite)
			elite, err := s.FreezeP6RushSeriesInput(eliteStart, false, false, nil, constUtil.P6OriginalClientHash)
			if err != nil {
				t.Fatal(err)
			}
			eliteOut, err := p7RoundLiveAutoRushComputeEnd(t.Context(), s, elite, end.GetOptRecord(), end.GetResult())
			if err != nil || eliteOut.CalculationSourceSHA256 == out.CalculationSourceSHA256 {
				t.Fatal("typed10 diagnostic projection/source separation", err)
			}
			accepted, err := p7GenericMechanismRushComputeEnd(t.Context(), s, series, end.GetOptRecord(), end.GetResult())
			if err != nil || accepted.Outcome.DiagnosticAIDriverPolicy != constUtil.P6RoundLiveAutoDriver {
				t.Fatal("new independent fallback", err)
			}
			for _, variant := range []string{"foreignActor", "NaN", "clock", "skill", "hp", "winner"} {
				bad := proto.Clone(end.GetOptRecord()).(*pb.BattleOperate)
				reported := end.GetResult()
				switch variant {
				case "foreignActor":
					bad.SkillOpts[0].ActorUid = proto.String("foreign")
				case "NaN":
					bad.SkillOpts[0].RoundTime = proto.Float64(math.NaN())
				case "clock":
					bad.SkillOpts[0].RoundTime = proto.Float64(bad.SkillOpts[0].GetRoundTime() + 0.05)
				case "skill":
					bad.SkillOpts[0].SkillIndex = proto.Int32(2)
				case "hp":
					bad.AttackerTotalLeftHp = proto.Float64(bad.GetAttackerTotalLeftHp() + 1)
				case "winner":
					reported = 1 - reported
				}
				if _, err = p7RoundLiveAutoRushComputeEnd(t.Context(), s, series, bad, reported); err == nil {
					t.Fatal("tamper accepted", variant)
				}
			}
			cold.FinalActors[0].CurrentHP++
			if p7GenericMechanismRushValidateEnd(series, &cold, end.GetOptRecord(), end.GetResult()) == nil {
				t.Fatal("cold final HP tamper")
			}
			swapped := *series
			input := *series.Template
			swapped.Template = &input
			input.Seed++
			if p7GenericMechanismRushValidateEnd(&swapped, out, end.GetOptRecord(), end.GetResult()) == nil {
				t.Fatal("cross frozen owner/source accepted")
			}
		}
	}
	after, _ := json.Marshal(series)
	if string(original) != string(after) {
		t.Fatal("frozen JSON/hash changed")
	}
}

func TestP7RoundLiveAuto66Floor8RetrySource(t *testing.T) {
	s, series, ends := p7RoundLiveAutoPhone66Fixture(t, "testdata/p7_round_live_auto_phone66_floor8.json")
	end := ends[0]
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	out, err := p7GenericMechanismRushComputeEnd(ctx, s, series, end.GetOptRecord(), end.GetResult())
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome.DiagnosticAIDriverPolicy != constUtil.P6RoundLiveAutoDriver || !proto.Equal(out.OriginalOperates, end.GetOptRecord()) || out.Outcome.FinalRound != 17 || len(out.OriginalOperates.SkillOpts) != 45 {
		t.Fatal("floor8 strict original End fallback")
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var cold model.CombatFinalHPOutcome
	if err = json.Unmarshal(raw, &cold); err != nil {
		t.Fatal(err)
	}
	if err = p7GenericMechanismRushValidateEnd(series, &cold, end.GetOptRecord(), end.GetResult()); err != nil {
		t.Fatal("floor8 same-End cold receipt", err)
	}
	t.Logf("floor8 original End: skills=45 round=17 HP=%g/%g fullPB=true coldReceipt=true", out.OriginalOperates.GetAttackerTotalLeftHp(), out.OriginalOperates.GetDefenderTotalLeftHp())
}
