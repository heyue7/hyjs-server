package service

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"io"
	"math"
	"strings"
	"testing"
)

func p7LiveRecordedOriginal34(t *testing.T) (*GameService, *model.FrozenP6RushSeries, []*pb.CSRushFightEnd) {
	t.Helper()
	var fixture struct {
		StartHex         string `json:"startHex"`
		StartSHA256      string `json:"startSHA256"`
		TablesSHA256     string `json:"tablesSHA256"`
		TablesGzipBase64 string `json:"tablesGzipBase64"`
		Ends             []struct {
			SHA    string `json:"sha256"`
			EndHex string `json:"endHex"`
		} `json:"ends"`
	}
	raw, err := readTestAnalysis75("testdata/p7_live_recorded_original34.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	compressed, err := base64.StdEncoding.DecodeString(fixture.TablesGzipBase64)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	tables, err := io.ReadAll(io.LimitReader(reader, 8*1024*1024))
	reader.Close()
	if err != nil || p6Digest(tables) != fixture.TablesSHA256 {
		t.Fatal("frozen original table digest", err)
	}
	service := &GameService{}
	if err = json.Unmarshal(tables, &service.Tables); err != nil {
		t.Fatal(err)
	}
	raw, err = hex.DecodeString(fixture.StartHex)
	if err != nil || p6Digest(raw) != fixture.StartSHA256 {
		t.Fatal("redacted Start digest", err)
	}
	start := &pb.SCRushFightStart{}
	if err = proto.Unmarshal(raw, start); err != nil {
		t.Fatal(err)
	}
	if start.GetBattlePlayer().GetPlayerId() != 1 || start.GetBattlePlayer().GetServerId() != 1 || start.GetBattlePlayer().GetNickName() != "" {
		t.Fatal("fixture identity must be removed")
	}
	series, err := service.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	var ends []*pb.CSRushFightEnd
	for _, item := range fixture.Ends {
		raw, err = hex.DecodeString(item.EndHex)
		if err != nil || p6Digest(raw) != item.SHA {
			t.Fatal("original547 End digest", err)
		}
		end := &pb.CSRushFightEnd{}
		if err = proto.Unmarshal(raw, end); err != nil {
			t.Fatal(err)
		}
		if end.GetFightId() != start.GetFightId() {
			t.Fatal("fight binding")
		}
		ends = append(ends, end)
	}
	return service, series, ends
}
func TestP7LiveRecordedOriginal34GoldenAndColdReceipt(t *testing.T) {
	s, series, ends := p7LiveRecordedOriginal34(t)
	input, err := p7GenericMechanismRushCalculationInput(series, ends[0].GetOptRecord())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.ComputeP6GenericMechanismBattle(t.Context(), input, ends[0].GetOptRecord()); err == nil || !strings.Contains(err.Error(), "anger=96.8") {
		t.Fatalf("original Playback baseline boundary changed: %v", err)
	}
	for i, end := range ends {
		out, err := p7LiveRecordedRushComputeEnd(t.Context(), s, series, end.GetOptRecord(), end.GetResult())
		if err != nil {
			t.Fatalf("golden%d: %v", i, err)
		}
		if !proto.Equal(end.GetOptRecord(), out.OriginalOperates) {
			t.Fatal("complete PB mismatch")
		}
		if out.Outcome.DiagnosticAIDriverPolicy != constUtil.P6LiveRecordedDriver || out.Outcome.CapabilityHash != constUtil.P7LiveRecordedRushProof {
			t.Fatal("driver identity")
		}
		body, _ := json.Marshal(out)
		var cold model.CombatFinalHPOutcome
		if err = json.Unmarshal(body, &cold); err != nil {
			t.Fatal(err)
		}
		if err = p7GenericMechanismRushValidateEnd(series, &cold, end.GetOptRecord(), end.GetResult()); err != nil {
			t.Fatal("cold receipt", err)
		}
	}
	end := ends[0]
	out, err := p7GenericMechanismRushComputeEnd(t.Context(), s, series, end.GetOptRecord(), end.GetResult())
	if err != nil {
		t.Fatal("integrated manual", err)
	}
	if out.Outcome.DiagnosticAIDriverPolicy != constUtil.P6LiveRecordedDriver || out.OriginalOperates.GetFinalRoundNum() != 11 || out.OriginalOperates.GetAttackerTotalLeftHp() != 359570.19351156184 || out.OriginalOperates.GetDefenderTotalLeftHp() != 0 || out.Outcome.Winner != 0 {
		t.Fatal("manual golden terminal/driver")
	}
	for _, end := range ends[1:] {
		out, err = p7GenericMechanismRushComputeEnd(t.Context(), s, series, end.GetOptRecord(), end.GetResult())
		if err != nil {
			t.Fatal("old Auto precedence", err)
		}
		if out.Outcome.DiagnosticAIDriverPolicy == constUtil.P6LiveRecordedDriver {
			t.Fatal("existing Auto path receipt changed")
		}
		body, _ := json.Marshal(out)
		var cold model.CombatFinalHPOutcome
		if err = json.Unmarshal(body, &cold); err != nil {
			t.Fatal(err)
		}
		if err = p7GenericMechanismRushValidateEnd(series, &cold, end.GetOptRecord(), end.GetResult()); err != nil {
			t.Fatal("old Auto cold receipt", err)
		}
	}
}
func TestP7LiveRecordedOriginal34TamperedCommandsRejected(t *testing.T) {
	s, series, ends := p7LiveRecordedOriginal34(t)
	source := ends[0]
	cases := []struct {
		name   string
		change func(*pb.CSRushFightEnd)
	}{
		{"negative_time", func(e *pb.CSRushFightEnd) { v := -0.1; e.OptRecord.SkillOpts[0].RoundTime = &v }},
		{"nonfinite_time", func(e *pb.CSRushFightEnd) { v := math.NaN(); e.OptRecord.SkillOpts[0].RoundTime = &v }},
		{"ultimate_before_anger", func(e *pb.CSRushFightEnd) { v := 10.399999999999979; e.OptRecord.SkillOpts[10].RoundTime = &v }},
		{"actor", func(e *pb.CSRushFightEnd) { v := "missing-actor"; e.OptRecord.SkillOpts[0].ActorUid = &v }},
		{"skill", func(e *pb.CSRushFightEnd) { v := int32(2); e.OptRecord.SkillOpts[0].SkillIndex = &v }},
		{"npc_time", func(e *pb.CSRushFightEnd) {
			for _, op := range e.OptRecord.SkillOpts {
				if op.GetCamp() == 1 {
					v := op.GetRoundTime() + 0.1
					op.RoundTime = &v
					break
				}
			}
		}},
		{"npc_skill", func(e *pb.CSRushFightEnd) {
			for _, op := range e.OptRecord.SkillOpts {
				if op.GetCamp() == 1 {
					v := int32(2)
					op.SkillIndex = &v
					break
				}
			}
		}},
		{"terminal_hp", func(e *pb.CSRushFightEnd) {
			v := e.OptRecord.GetAttackerTotalLeftHp() + 1
			e.OptRecord.AttackerTotalLeftHp = &v
		}},
		{"result", func(e *pb.CSRushFightEnd) { v := int32(0); e.Result = &v }}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := proto.Clone(source).(*pb.CSRushFightEnd)
			tc.change(e)
			_, err := p7LiveRecordedRushComputeEnd(t.Context(), s, series, e.GetOptRecord(), e.GetResult())
			if err == nil {
				t.Fatal("tampered source accepted")
			}
			if tc.name == "ultimate_before_anger" && !strings.Contains(err.Error(), "anger=") {
				t.Fatalf("expected original anger legality rejection: %v", err)
			}
		})
	}
}
func TestP7LiveRecordedReceiptIdentityRejected(t *testing.T) {
	s, series, ends := p7LiveRecordedOriginal34(t)
	end := ends[0]
	out, err := p7LiveRecordedRushComputeEnd(t.Context(), s, series, end.GetOptRecord(), end.GetResult())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(out)
	cases := []struct {
		name   string
		change func(*model.CombatFinalHPOutcome)
	}{
		{"driver", func(o *model.CombatFinalHPOutcome) {
			o.Outcome.DiagnosticAIDriverPolicy = constUtil.P6RoundLiveAutoDriver
		}},
		{"proof", func(o *model.CombatFinalHPOutcome) {
			o.Outcome.CapabilityHash = constUtil.P7GenericMechanismCompatibilityProof
		}},
		{"source_hash", func(o *model.CombatFinalHPOutcome) { o.CalculationSourceSHA256 = "changed" }},
		{"record_hash", func(o *model.CombatFinalHPOutcome) { o.OriginalRecordSHA256 = "changed" }},
		{"authority", func(o *model.CombatFinalHPOutcome) { o.Outcome.Authoritative = false }},
		{"actor_hp", func(o *model.CombatFinalHPOutcome) { o.FinalActors[0].CurrentHP++ }}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cold model.CombatFinalHPOutcome
			if err := json.Unmarshal(body, &cold); err != nil {
				t.Fatal(err)
			}
			tc.change(&cold)
			if err := p7GenericMechanismRushValidateEnd(series, &cold, end.GetOptRecord(), end.GetResult()); err == nil {
				t.Fatal("tampered receipt accepted")
			}
		})
	}
	legacyRecord, legacySource, err := p7GenericMechanismRushManualSourceDigest(series.Template, end.GetOptRecord())
	if err != nil {
		t.Fatal(err)
	}
	liveRecord, liveSource, err := p6LiveRecordedSourceDigests(series.Template, end.GetOptRecord())
	if err != nil {
		t.Fatal(err)
	}
	if liveRecord != legacyRecord || liveSource == legacySource {
		t.Fatal("driver/proof digest domain")
	}
}

func TestP7LiveRecordedScope(t *testing.T) {
	_, series, ends := p7LiveRecordedOriginal34(t)
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		t.Fatal(err)
	}
	record := ends[0].GetOptRecord()
	for _, battleType := range []int32{8, 14, 16, 17} {
		view := *input
		view.BattleType = battleType
		if err := validateP6LiveRecordedInput(&view, record); err == nil {
			t.Fatalf("mode%d admitted", battleType)
		}
	}
	view := *input
	view.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	if err := validateP6LiveRecordedInput(&view, record); err == nil {
		t.Fatal("conflicting driver admitted")
	}
	transformed := proto.Clone(record).(*pb.BattleOperate)
	transformed.AttackerTransOpt = &pb.BattleTransOperate{}
	if err := validateP6LiveRecordedInput(input, transformed); err == nil {
		t.Fatal("unanchored formation admitted")
	}
}
