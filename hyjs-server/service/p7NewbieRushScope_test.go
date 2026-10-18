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

func p7NewbieRushTestSource(t *testing.T) (*GameService, []*pb.SCRushFightStart, []*pb.BattleOperate) {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p6/p7-newbie047025-local-inputs-r2-20261017/manifest.json")
	if err != nil || p6Digest(raw) != "460d5b87b5310affdf95def39fec86c9408e649e38d040dd3f11f7cced7b21ab" {
		t.Fatal("fixed local input", err)
	}
	var manifest struct {
		Cases []p7PartyRushCase `json:"cases"`
	}
	if json.Unmarshal(raw, &manifest) != nil || len(manifest.Cases) != 4 {
		t.Fatal("four bounded source cases")
	}
	raw, err = os.ReadFile("../../analysis/p6/p7-newbie047025-local-fixtures-r1-20261017/records.json")
	if err != nil || p6Digest(raw) != "678bfb2e94a5c54055208a0b0fb8cc95630de315bc850f5b9b28a16c873b525a" {
		t.Fatal("fixed LOCAL original Engine records", err)
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
		t.Fatal("four local records")
	}
	var starts []*pb.SCRushFightStart
	var operations []*pb.BattleOperate
	for _, row := range manifest.Cases {
		raw, err = os.ReadFile(filepath.Join("../../analysis/p6/p7-newbie047025-local-inputs-r2-20261017", row.Typed))
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
				t.Fatal("local original record", err)
			}
			operations = append(operations, op)
			found = true
		}
		if !found {
			t.Fatal("local record missing source")
		}
	}
	return p6OriginalTables(t), starts, operations
}

func TestP7NewbieRushScopeBoundaries(t *testing.T) {
	s, starts, _ := p7NewbieRushTestSource(t)
	freeze := func(start *pb.SCRushFightStart) *model.FrozenP6RushSeries {
		t.Helper()
		series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		return series
	}
	for _, start := range starts {
		if err := validateP7NewbieRushScope(freeze(start)); err != nil {
			t.Fatal("bounded source", err)
		}
	}
	otherOwner := proto.Clone(starts[0]).(*pb.SCRushFightStart)
	otherOwner.BattlePlayer.PlayerId = proto.Int64(908)
	otherOwner.BattlePlayer.ServerId = proto.Int32(2)
	otherOwner.BattlePlayer.Formation[0].Guid = proto.String("owner908-main")
	otherOwner.BattlePlayer.Formation[1].Guid = proto.String("owner908-guide")
	otherOwner.RandSeed = proto.Int64(42) // RNG parameter, not another mechanic or a phone-proven sample.
	if err := validateP7NewbieRushScope(freeze(otherOwner)); err != nil {
		t.Fatal("legitimate owner/GUID parameter", err)
	}
	for name, change := range map[string]func(*pb.SCRushFightStart){
		"main33":         func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Level = proto.Int32(33) },
		"owner33":        func(x *pb.SCRushFightStart) { x.BattlePlayer.Level = proto.Int32(33) },
		"companion2":     func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[1].Level = proto.Int32(2) },
		"talent":         func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[1].TalentList = []string{"tal_1"} },
		"awake":          func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].AwakeStage = proto.Int32(1) },
		"trained-att":    func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Prop.Att = proto.Float64(152) },
		"partial-hp":     func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[0].Inst.CurHp = proto.Float64(2105) },
		"initial-ap":     func(x *pb.SCRushFightStart) { x.BattlePlayer.Inst.Anger = proto.Float64(1) },
		"duplicate-guid": func(x *pb.SCRushFightStart) { x.BattlePlayer.Formation[1].Guid = x.BattlePlayer.Formation[0].Guid },
		"changed-npc":    func(x *pb.SCRushFightStart) { x.BattleMonsterList[0].Formation[0].Prop.Hp = proto.Float64(1301) },
		"other-scene":    func(x *pb.SCRushFightStart) { x.FightScene = proto.String("arena001") },
		"other-mode":     func(x *pb.SCRushFightStart) { x.RushType = proto.Int32(2) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := proto.Clone(starts[0]).(*pb.SCRushFightStart)
			change(candidate)
			if err := validateP7NewbieRushScope(freeze(candidate)); err == nil {
				t.Fatal("outside bounded family admitted")
			}
		})
	}
	legacyRaw, err := os.ReadFile("../../analysis/p6/p7-party-rush-pending-20261016/000.SCRushFightStart.pb")
	legacy := &pb.SCRushFightStart{}
	if err != nil || proto.Unmarshal(legacyRaw, legacy) != nil {
		t.Fatal(err)
	}
	legacySeries := freeze(legacy)
	if validateP7PartyRushScope(legacySeries) != nil || validateP7NewbieRushScope(legacySeries) == nil {
		t.Fatal("legacy001003 separation lost")
	}
}

func TestP7NewbieRushClosedProofAndExactEndControl(t *testing.T) {
	s, starts, records := p7NewbieRushTestSource(t)
	if constUtil.P7NewbieRushNativeProof != "" {
		t.Fatal("staged family must stay closed until reviewed Android anchor")
	}
	for index, start := range starts {
		series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p7NewbieRushProof(series); err == nil || p7NewbieRushSourceAvailable(series) {
			t.Fatal("local parity granted production authority")
		}
		if out, err := p7NewbieRushPreview(t.Context(), s, series); err == nil || out != nil {
			t.Fatal("unproven preview admitted")
		}
		input, err := p7NewbieRushCalculationInput(series, records[index])
		if err != nil {
			t.Fatal("exact observed original End clock", err)
		}
		if input.DiagnosticManualAutoStartRoundTime == nil || *input.DiagnosticManualAutoStartRoundTime != records[index].SkillOpts[0].GetRoundTime() {
			t.Fatal("exact original End boundary changed")
		}
		out, err := s.computeP7NewbieRushDiagnostic(t.Context(), series, records[index].SkillOpts[0].GetRoundTime())
		if err != nil || out.Outcome.Authoritative || !proto.Equal(out.OriginalOperates, records[index]) {
			t.Fatal("bounded local independent recomputation", err)
		}
		if _, err = p7NewbieRushComputeEnd(t.Context(), s, series, records[index], 1); err == nil {
			t.Fatal("local-only terminal used for business End")
		}
		for name, change := range map[string]func(*pb.BattleOperate){
			"foreign-guid":    func(x *pb.BattleOperate) { x.SkillOpts[0].ActorUid = proto.String("foreign") },
			"defender-first":  func(x *pb.BattleOperate) { x.SkillOpts[0].Camp = proto.Int32(1) },
			"unsampled-clock": func(x *pb.BattleOperate) { x.SkillOpts[0].RoundTime = proto.Float64(1) },
		} {
			t.Run(name, func(t *testing.T) {
				changed := proto.Clone(records[index]).(*pb.BattleOperate)
				change(changed)
				if _, err := p7NewbieRushCalculationInput(series, changed); err == nil {
					t.Fatal("unverified End boundary")
				}
			})
		}
	}
}
