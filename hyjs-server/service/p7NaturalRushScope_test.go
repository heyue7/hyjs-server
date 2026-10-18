package service

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7NaturalRushTestSource(t *testing.T) (*GameService, []*pb.SCRushFightStart, []*pb.BattleOperate) {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p6/p7-numeric047025-local-inputs-r1-20261017/manifest.json")
	if err != nil || p6Digest(raw) != "7e74d64e647ee3bc0de9f48afd10cea8bae14ea14461652342d06b88ab8eceac" {
		t.Fatal("fixed local input", err)
	}
	var manifest struct {
		Cases []p7PartyRushCase `json:"cases"`
	}
	if json.Unmarshal(raw, &manifest) != nil || len(manifest.Cases) != 4 {
		t.Fatal("four bounded source cases")
	}
	raw, err = os.ReadFile("../../analysis/p6/p7-numeric047025-android-fixtures-r1-20261017/records.json")
	if err != nil || p6Digest(raw) != "be6883a4d93de76ad167402ea02251d66bf1000bc714280deaa7404d05bf5bf6" {
		t.Fatal("fixed actual Android original Engine records", err)
	}
	var records struct {
		Cases []struct {
			WireSHA string `json:"wireSHA256"`
			Record  struct {
				Base64 string `json:"protobufBase64"`
			} `json:"originalFinalRecord"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &records) != nil || len(records.Cases) != 4 {
		t.Fatal("four actual Android records")
	}
	var starts []*pb.SCRushFightStart
	var operations []*pb.BattleOperate
	for _, row := range manifest.Cases {
		raw, err = os.ReadFile(filepath.Join("../../analysis/p6/p7-numeric047025-local-inputs-r1-20261017", row.Typed))
		if err != nil || p6Digest(raw) != row.TypedSHA {
			t.Fatal("typed source SHA", err)
		}
		start := &pb.SCRushFightStart{}
		if proto.Unmarshal(raw, start) != nil {
			t.Fatal("typed source decode")
		}
		starts = append(starts, start)
		found := false
		for _, record := range records.Cases {
			if record.WireSHA != row.WireSHA {
				continue
			}
			raw, err = base64.StdEncoding.DecodeString(record.Record.Base64)
			op := &pb.BattleOperate{}
			if err != nil || proto.Unmarshal(raw, op) != nil {
				t.Fatal("actual Android original record", err)
			}
			operations = append(operations, op)
			found = true
		}
		if !found {
			t.Fatal("actual Android record missing source")
		}
	}
	return p6OriginalTables(t), starts, operations
}

// Numeric parameters vary independently from fixed skills, talent and initial conditions.
func TestP7NaturalRushMechanismAndNumericBoundary(t *testing.T) {
	s, starts, _ := p7NaturalRushTestSource(t)
	freeze := func(start *pb.SCRushFightStart) *model.FrozenP6RushSeries {
		t.Helper()
		series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		return series
	}
	for _, start := range starts {
		if err := validateP7NaturalRushScope(freeze(start)); err != nil {
			t.Fatal("actual original numeric source", err)
		}
	}
	changed := proto.Clone(starts[0]).(*pb.SCRushFightStart)
	changed.BattlePlayer.PlayerId = proto.Int64(908)
	changed.BattlePlayer.ServerId = proto.Int32(2)
	changed.BattlePlayer.Level = proto.Int32(33)
	changed.BattlePlayer.Formation[0].Level = proto.Int32(33)
	changed.BattlePlayer.Formation[0].Guid = proto.String("owner908-main")
	changed.BattlePlayer.Formation[1].Guid = proto.String("owner908-guide")
	changed.BattlePlayer.Formation[1].Level = proto.Int32(4)
	changed.BattlePlayer.Formation[0].Prop.Att = proto.Float64(300.123456789)
	changed.BattlePlayer.Formation[1].Prop.Pdef = proto.Float64(99.99999999999)
	changed.RandSeed = proto.Int64(42)
	if err := validateP7NaturalRushScope(freeze(changed)); err != nil {
		t.Fatal("legal P5 numeric parameters were treated as exact PB whitelist", err)
	}
	for name, change := range map[string]func(*pb.SCRushFightStart){
		"below-original22": func(x *pb.SCRushFightStart) {
			x.BattlePlayer.Level = proto.Int32(21)
			x.BattlePlayer.Formation[0].Level = proto.Int32(21)
		},
		"main-level-mismatch":  func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Level = proto.Int32(33) },
		"companion-over-owner": func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[1].Level = proto.Int32(101) },
		"unseen-talent":        func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[1].TalentList = []string{"tal_1"} },
		"awake":                func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].AwakeStage = proto.Int32(1) },
		"skill-order": func(x *pb.SCRushFightStart) {
			x.BattlePlayer.Formation[0].SkillOrder = proto.String("047_3,047_2,047_1")
		},
		"partial-hp": func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Inst.CurHp = proto.Float64(2) },
		"fractional-maxHP": func(x *pb.SCRushFightStart) {
			x.BattlePlayer.Formation[0].Prop.Hp = proto.Float64(2105.5)
			x.BattlePlayer.Formation[0].Inst.CurHp = proto.Float64(2105.5)
		},
		"initial-ap":     func(x *pb.SCRushFightStart) { x.BattlePlayer.Inst.Anger = proto.Float64(1) },
		"extra-damage":   func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Prop.DamageAdd = proto.Float64(1) },
		"percentage":     func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Prop.Peratt = proto.Float64(1) },
		"critical-class": func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Prop.Cri = proto.Float64(11) },
		"nonfinite":      func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Prop.Att = proto.Float64(math.Inf(1)) },
		"negative":       func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Prop.Pdef = proto.Float64(-1) },
		"duplicate-guid": func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[1].Guid = x.BattlePlayer.Formation[0].Guid },
		"unseen-npc":     func(x *pb.SCRushFightStart) { x.BattleMonsterList[0].Formation[0].Prop.Hp = proto.Float64(1301) },
		"other-scene":    func(x *pb.SCRushFightStart) { x.FightScene = proto.String("arena001") },
		"other-mode":     func(x *pb.SCRushFightStart) { x.RushType = proto.Int32(2) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := proto.Clone(starts[0]).(*pb.SCRushFightStart)
			change(candidate)
			series, err := s.FreezeP6RushSeriesInput(candidate, true, false, nil, constUtil.P6OriginalClientHash)
			if err == nil && validateP7NaturalRushScope(series) == nil {
				t.Fatal("outside certified mechanism admitted")
			}
		})
	}
}

func TestP7NaturalRushExactAndroidEndAndIndependentProof(t *testing.T) {
	raw, err := os.ReadFile("../../analysis/p6/natural047025-numeric-android-capability-certificate-20261017.json")
	if err != nil || p6Digest(raw) != constUtil.P7NaturalRushNativeProof || constUtil.P7NaturalRushNativeProof == constUtil.P7RushPartyManualNativeProof || constUtil.P7NaturalRushNativeProof == constUtil.P6LinearPhysicalNativeProof {
		t.Fatal("independent actual Android proof missing or borrowed", err)
	}
	var cert struct {
		FullSchema  string `json:"originalFullSchemaStatus"`
		Difference  string `json:"originalFullSchemaDifference"`
		Global      bool   `json:"globalAuthoritative"`
		NativeCases int    `json:"nativeCases"`
	}
	if json.Unmarshal(raw, &cert) != nil || cert.FullSchema != "FAIL" || cert.Difference != "ai_notify.data.stack" || cert.Global || cert.NativeCases != 4 {
		t.Fatal("certificate rewrote the retained schema failure or global authority")
	}
	s, starts, records := p7NaturalRushTestSource(t)
	for index, start := range starts {
		series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		proof, err := p7NaturalRushProof(series)
		if err != nil || proof != constUtil.P7NaturalRushNativeProof || !p7NaturalRushSourceAvailable(series) {
			t.Fatal("actual source unavailable after independent activation", err)
		}
		family, err := p7ManualRushFamilyForSeries(series)
		if err != nil || !family.Natural || family.ScopeProof != proof || family.DriverSource != constUtil.P7NaturalRushDriverSource {
			t.Fatal("family fell through to another certificate", err)
		}
		if _, err = p7ManualRushFamilyForReceipt(constUtil.P7RushTrial, constUtil.P7NaturalRushDriverSource, constUtil.P7RushPartyManualNativeProof); err == nil {
			t.Fatal("001003 proof admitted Natural receipt")
		}
		input, err := p7NaturalRushCalculationInput(series, records[index])
		if err != nil || input.DiagnosticManualAutoStartRoundTime == nil || *input.DiagnosticManualAutoStartRoundTime != records[index].SkillOpts[0].GetRoundTime() {
			t.Fatal("actual Android first clock changed", err)
		}
		reported := int32(0)
		if records[index].GetDefenderTotalLeftHp() == 0 && records[index].GetAttackerTotalLeftHp() > 0 {
			reported = 1
		}
		out, err := p7NaturalRushComputeEnd(t.Context(), s, series, records[index], reported)
		if err != nil || out == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof || !proto.Equal(out.OriginalOperates, records[index]) {
			t.Fatal("actual Android full End did not independently settle", index, err)
		}
		if p7NaturalRushValidateEnd(series, out, records[index], 1-reported) == nil {
			t.Fatal("client winner replaced independent result")
		}
		changed := proto.Clone(records[index]).(*pb.BattleOperate)
		changed.AttackerTotalLeftHp = proto.Float64(changed.GetAttackerTotalLeftHp() + 1)
		if p7NaturalRushValidateEnd(series, out, changed, reported) == nil {
			t.Fatal("client terminal HP altered")
		}
		changed = proto.Clone(records[index]).(*pb.BattleOperate)
		changed.SkillOpts = changed.SkillOpts[:len(changed.SkillOpts)-1]
		if p7NaturalRushValidateEnd(series, out, changed, reported) == nil {
			t.Fatal("missing operation accepted")
		}
		out.Outcome.CapabilityHash = constUtil.P7RushPartyManualNativeProof
		if p7NaturalRushValidateEnd(series, out, records[index], reported) == nil {
			t.Fatal("foreign certificate signed Natural result")
		}
		if index == 0 {
			diagnostic, err := s.computeP7NaturalRushDiagnostic(t.Context(), series, 0)
			if err != nil || diagnostic.Outcome.Authoritative || diagnostic.Outcome.CapabilityHash != "" {
				t.Fatal("diagnostic promoted global/uncertified authority", err)
			}
		}
	}
}

func TestP7NaturalRushFiniteFirstAutoAndForgedOldRecordRejected(t *testing.T) {
	s, starts, records := p7NaturalRushTestSource(t)
	series, err := s.FreezeP6RushSeriesInput(starts[0], true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(series)
	for _, at := range []float64{0, 1, 30.1, constUtil.P7NaturalRushDelayedAutoRoundTime, 110} {
		input, err := p7NaturalRushInputAt(series, at)
		if err != nil || input.DiagnosticManualAutoStartRoundTime == nil || *input.DiagnosticManualAutoStartRoundTime != at || !proto.Equal(input.Attacker, series.Template.Attacker) || !proto.Equal(input.Monster, series.Template.Monster) {
			t.Fatal("legal finite first Auto clock rejected or input altered", at, err)
		}
	}
	for name, at := range map[string]float64{"negative": -0.1, "over-limit": 110.1, "nan": math.NaN(), "infinite": math.Inf(1)} {
		t.Run(name, func(t *testing.T) {
			changed := proto.Clone(records[0]).(*pb.BattleOperate)
			changed.SkillOpts[0].RoundTime = proto.Float64(at)
			if _, err := p7NaturalRushCalculationInput(series, changed); err == nil {
				t.Fatal("invalid first Auto clock accepted")
			}
		})
	}
	for name, change := range map[string]func(*pb.BattleOperate){
		"foreign-guid":   func(x *pb.BattleOperate) { x.SkillOpts[0].ActorUid = proto.String("foreign") },
		"defender-first": func(x *pb.BattleOperate) { x.SkillOpts[0].Camp = proto.Int32(1) },
		"later-round":    func(x *pb.BattleOperate) { x.SkillOpts[0].RoundNum = proto.Int32(2) },
		"bad-skill":      func(x *pb.BattleOperate) { x.SkillOpts[0].SkillIndex = proto.Int32(3) },
		"transform":      func(x *pb.BattleOperate) { x.AttackerTransOpt = &pb.BattleTransOperate{} },
		"unknown":        func(x *pb.BattleOperate) { x.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01}) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := proto.Clone(records[0]).(*pb.BattleOperate)
			change(changed)
			if _, err := p7NaturalRushCalculationInput(series, changed); err == nil {
				t.Fatal("foreign first command admitted")
			}
		})
	}
	// Clock1 is now legal as a candidate, while the old clock0 operation list,
	// target/clock order and terminal cannot be reused to claim that result.
	changed := proto.Clone(records[0]).(*pb.BattleOperate)
	changed.SkillOpts[0].RoundTime = proto.Float64(1)
	if _, err = p7NaturalRushCalculationInput(series, changed); err != nil {
		t.Fatal("clock1 still treated as an unseen sample whitelist", err)
	}
	if _, err = p7NaturalRushComputeEnd(t.Context(), s, series, changed, 1); err == nil {
		t.Fatal("old clock0 End impersonated clock1")
	}
	after, _ := json.Marshal(series)
	if string(before) != string(after) {
		t.Fatal("candidate clock changed frozen original typed source")
	}
}
