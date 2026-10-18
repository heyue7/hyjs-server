package service

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

func TestP7PartyRushBusinessCertificateRequiresExactIndependentSource(t *testing.T) {
	raw, err := os.ReadFile("../../analysis/p6/party-rush-manual-capability-certificate-20261016.json")
	if err != nil || p6Digest(raw) != constUtil.P7RushPartyManualNativeProof {
		t.Fatal("independent typed9 party certificate SHA", err)
	}
	s, cases := p7PartyRushSources(t)
	series := p7PartyRushSeries(t, s, cases[0])
	if !p7PartyRushSourceAvailable(series) {
		t.Fatal("certified original source is not available")
	}
	family, err := p7ManualRushFamilyForSeries(series)
	if err != nil || !family.Party || family.ScopeProof != constUtil.P7RushPartyManualNativeProof {
		t.Fatal("certified source did not select its independent family", err)
	}
	for _, borrowed := range []string{"", constUtil.P6RushManualNativeProof, constUtil.P6RushStarterNativeProof, constUtil.P6ActualPartyNativeProof} {
		if _, err := p7ManualRushFamilyForReceipt(constUtil.P7RushTrial, constUtil.P7RushPartyManualDriverSource, borrowed); err == nil {
			t.Fatal("old/other-mode proof upgraded a party pending/receipt", borrowed)
		}
	}
	if _, err := (p7ManualRushFamily{DriverSource: constUtil.P7RushPartyManualDriverSource, Party: true}).Preview(t.Context(), s, series); err == nil {
		t.Fatal("preview created business authority without an independent certificate")
	}
	if validateP7PartyRushCertifiedTerminal(nil, nil, "") == nil {
		t.Fatal("empty terminal/proof accepted")
	}
}

func TestP7PartyRushBusinessActualOriginalEndStrictWholeRecord(t *testing.T) {
	s, cases := p7PartyRushSources(t)
	series := p7PartyRushSeries(t, s, cases[0])
	family, err := p7ManualRushFamilyForSeries(series)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../analysis/p6/p7-party-rush-fixtures-r2-20261016/000.original.BattleOperate.pb")
	if err != nil || p6Digest(raw) != "876a8702571500c345fee44104993887c244ab979a776bc69e7c6245eb4cfa12" {
		t.Fatal("actual original record bytes", err)
	}
	record := &pb.BattleOperate{}
	if proto.Unmarshal(raw, record) != nil {
		t.Fatal("actual original record malformed")
	}
	out, err := family.ComputeEnd(t.Context(), s, series, record, 1)
	if err != nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P7RushPartyManualNativeProof || len(out.FinalActors) != 3 {
		t.Fatal("independent whole original End did not settle", err)
	}
	preview, err := family.Preview(t.Context(), s, series)
	if err != nil || family.ValidatePreview(series, preview) != nil {
		t.Fatal("certified Auto0 preview", err)
	}
	variants := map[string]func(*pb.BattleOperate){
		"missing-tail": func(r *pb.BattleOperate) { r.SkillOpts = r.SkillOpts[:len(r.SkillOpts)-1] },
		"duplicate": func(r *pb.BattleOperate) {
			r.SkillOpts = append(r.SkillOpts, proto.Clone(r.SkillOpts[0]).(*pb.BattleSkillOperate))
		},
		"clock": func(r *pb.BattleOperate) {
			r.SkillOpts[0].RoundTime = proto.Float64(math.Nextafter(r.SkillOpts[0].GetRoundTime(), math.Inf(1)))
		},
		"hp":        func(r *pb.BattleOperate) { r.AttackerTotalLeftHp = proto.Float64(r.GetAttackerTotalLeftHp() + 1) },
		"transform": func(r *pb.BattleOperate) { r.AttackerTransOpt = &pb.BattleTransOperate{} },
		"reorder": func(r *pb.BattleOperate) {
			if len(r.SkillOpts) < 2 {
				t.Fatal("native complete party record lacks two actual actions")
			}
			r.SkillOpts[0], r.SkillOpts[1] = r.SkillOpts[1], r.SkillOpts[0]
		},
	}
	for name, mutate := range variants {
		t.Run(name, func(t *testing.T) {
			changed := proto.Clone(record).(*pb.BattleOperate)
			mutate(changed)
			if family.ValidateEnd(series, out, changed, 1) == nil {
				t.Fatal("nonidentical whole original record accepted")
			}
		})
	}
	if family.ValidateEnd(series, out, record, 0) == nil {
		t.Fatal("false winner accepted")
	}
}

func TestP7PartyRushLegacyManualIdentitiesStayIndependent(t *testing.T) {
	for _, kind := range []int32{constUtil.P7RushTrial, constUtil.P7RushElite} {
		_, source, proof, _ := p7RushManualIdentity(kind)
		family, err := p7ManualRushFamilyForReceipt(kind, source, proof)
		if err != nil || family.Party || family.ScopeProof != proof || family.DriverSource != source {
			t.Fatal("legacy immutable manual identity changed", kind, err)
		}
		if _, err = p7ManualRushFamilyForReceipt(kind, source, constUtil.P7RushPartyManualNativeProof); err == nil {
			t.Fatal("party identity replaced an old single047 proof")
		}
	}
	if _, err := p7ManualRushFamilyForReceipt(constUtil.P7RushElite, constUtil.P7RushPartyManualDriverSource, constUtil.P7RushPartyManualNativeProof); err == nil {
		t.Fatal("Trial party source expanded Elite")
	}
}

func TestP7PartyRushFirstCommandOnlySelectsCandidateClock(t *testing.T) {
	s, cases := p7PartyRushSources(t)
	series := p7PartyRushSeries(t, s, cases[0])
	before, _ := json.Marshal(series)
	for _, hero := range series.Template.Attacker.Formation {
		supplied := &pb.BattleOperate{SkillOpts: []*pb.BattleSkillOperate{{RoundNum: proto.Int32(1), RoundTime: proto.Float64(30.1), Camp: proto.Int32(0), ActorUid: hero.Guid, SkillIndex: proto.Int32(2)}}, FinalRoundNum: proto.Int32(40), AttackerTotalLeftHp: proto.Float64(999999)}
		input, err := p7PartyRushCalculationInput(series, supplied)
		if err != nil || input.DiagnosticAIDriverPolicy != constUtil.P6DiagnosticManualAutoStart || input.DiagnosticManualAutoStartRoundTime == nil || *input.DiagnosticManualAutoStartRoundTime != 30.1 || !proto.Equal(input.Attacker, series.Template.Attacker) || !proto.Equal(input.Monster, series.Template.Monster) {
			t.Fatal("candidate clock changed actual actors or selected playback", err)
		}
		// A candidate input never signs the forged result/tail; public End must
		// independently compute and compare every operation and terminal HP.
		family := p7ManualRushFamily{DriverSource: constUtil.P7RushPartyManualDriverSource, ScopeProof: constUtil.P7RushPartyManualNativeProof, Party: true}
		if family.ValidateEnd(series, nil, supplied, 1) == nil {
			t.Fatal("candidate clock authorized supplied winner/HP without independently calculated terminal")
		}
	}
	after, _ := json.Marshal(series)
	if string(before) != string(after) {
		t.Fatal("candidate control mutated frozen typed source")
	}
	base := &pb.BattleOperate{SkillOpts: []*pb.BattleSkillOperate{{RoundNum: proto.Int32(1), RoundTime: proto.Float64(0), Camp: proto.Int32(0), ActorUid: series.Template.Attacker.Formation[0].Guid, SkillIndex: proto.Int32(0)}}}
	mutations := map[string]func(*pb.BattleOperate){
		"other-actor": func(record *pb.BattleOperate) { record.SkillOpts[0].ActorUid = proto.String("unseen") },
		"enemy-first": func(record *pb.BattleOperate) { record.SkillOpts[0].Camp = proto.Int32(1) },
		"later-round": func(record *pb.BattleOperate) { record.SkillOpts[0].RoundNum = proto.Int32(2) },
		"bad-skill":   func(record *pb.BattleOperate) { record.SkillOpts[0].SkillIndex = proto.Int32(3) },
		"nan-clock":   func(record *pb.BattleOperate) { record.SkillOpts[0].RoundTime = proto.Float64(math.NaN()) },
		"late-clock":  func(record *pb.BattleOperate) { record.SkillOpts[0].RoundTime = proto.Float64(110.1) },
		"transform":   func(record *pb.BattleOperate) { record.AttackerTransOpt = &pb.BattleTransOperate{} },
		"unknown":     func(record *pb.BattleOperate) { record.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01}) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			record := proto.Clone(base).(*pb.BattleOperate)
			mutate(record)
			if _, err := p7PartyRushCalculationInput(series, record); err == nil {
				t.Fatal("unproven first control admitted")
			}
		})
	}
}

func TestP7PartyRushScopeRetainsActorAndTypeBoundaries(t *testing.T) {
	s, cases := p7PartyRushSources(t)
	base := p7PartyRushSeries(t, s, cases[0])
	mutations := map[string]func(*pb.SCRushFightStart){
		"elite":        func(start *pb.SCRushFightStart) { start.RushType = proto.Int32(constUtil.P7RushElite) },
		"other-hero":   func(start *pb.SCRushFightStart) { start.BattlePlayer.Formation[1].Id = proto.String("hero_025") },
		"talent":       func(start *pb.SCRushFightStart) { start.BattlePlayer.Formation[0].TalentList = []string{"tal_1"} },
		"missing-hero": func(start *pb.SCRushFightStart) { start.BattlePlayer.Formation = start.BattlePlayer.Formation[:1] },
		"fraction-hp":  func(start *pb.SCRushFightStart) { start.BattlePlayer.Formation[0].Prop.Hp = proto.Float64(100.5) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			start := proto.Clone(base.Start).(*pb.SCRushFightStart)
			mutate(start)
			series, err := s.FreezeP6RushSeriesInput(start, start.GetRushType() == constUtil.P7RushTrial, false, nil, constUtil.P6OriginalClientHash)
			if err == nil && validateP7PartyRushDiagnosticScope(series) == nil {
				t.Fatal("party boundary expanded beyond current typed9 complete001003 source")
			}
		})
	}
}
