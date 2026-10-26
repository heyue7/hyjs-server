package service

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 原宿主先独立执行兼容后的六绿十二个真实演员，Go再核完整导出和逐人HP。
func TestP15GreenCompatibleOriginalEngineAndResourceTableConsistency(t *testing.T) {
	s := p6OriginalTables(t)
	// P6专用夹具未载入库存表；用真实TSV加载器补全原Frag，保留全部逐行一致性校验。
	if err := s.loadP7Table("../../analysis/tables", "Frag"); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal("idempotent compatibility", err)
	}
	if err := s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	root := "../../analysis/green-hero-compat-20261017"
	for _, name := range []string{"Hero", "Frag"} {
		raw, err := readTestAnalysis75(root + "/assets-r3/Table" + name + ".tsv")
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
		keys := strings.Split(lines[1], "\t")
		for _, line := range lines[5:] {
			cols := strings.Split(line, "\t")
			if len(cols) == 0 || cols[0] == "" {
				continue
			}
			row := s.table(name, "id", cols[0])
			if row == nil {
				t.Fatal("client row absent server", name, cols[0])
			}
			for i, value := range cols {
				if i < len(keys) && row[keys[i]] != value {
					t.Fatal("client/server row mismatch", name, cols[0], keys[i])
				}
			}
		}
	}
	wireBytes, err := readTestAnalysis75(root + "/native-r1/original-input.pb")
	if err != nil || p6Digest(wireBytes) != "fdee30a30dfd659449430a7c52538d9fee66406e78748fc6d027f9806ee15f96" {
		t.Fatal("independent native source", err)
	}
	var wire pb.SCFightPlayback
	if err = proto.Unmarshal(wireBytes, &wire); err != nil {
		t.Fatal(err)
	}
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	out, qualification, err := s.ComputeP6GenericMechanismBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal("actual compatible six-character engine", err)
	}
	if qualification.ActorCount != 12 || qualification.NativeCertified || out.Outcome.Authoritative {
		t.Fatal("compatibility count/authority", qualification)
	}
	trace, err := readTestAnalysis75(root + "/native-r1/original.native.jsonl")
	if err != nil || p6Digest(trace) != "f8b7bfca0537f7888046843b40d6cfc802088795811e2fee4c705adecd3c63d6" {
		t.Fatal("original host independent output", err)
	}
	var finalRecord pb.BattleOperate
	var terminal struct {
		Attacker, Defender struct {
			Actors []struct {
				UID       string                      `json:"uid"`
				HeroID    string                      `json:"heroId"`
				Attribute struct{ HP, MaxHP float64 } `json:"attribute"`
			} `json:"actors"`
		}
	}
	var winner bool
	skills := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(trace)))
	scanner.Buffer(make([]byte, 4096), 8<<20)
	for scanner.Scan() {
		var event struct {
			Kind        string          `json:"kind"`
			State       json.RawMessage `json:"state"`
			Data        json.RawMessage `json:"data"`
			AttackerWon bool            `json:"attackerWon"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			t.Fatal("native event decode")
		}
		switch event.Kind {
		case "state":
			if json.Unmarshal(event.State, &terminal) != nil {
				t.Fatal("native terminal decode")
			}
		case "original_final_operates":
			var data struct {
				ProtobufBase64 string `json:"protobufBase64"`
			}
			if json.Unmarshal(event.Data, &data) != nil {
				t.Fatal("native record decode")
			}
			raw, e := base64.StdEncoding.DecodeString(data.ProtobufBase64)
			if e != nil || proto.Unmarshal(raw, &finalRecord) != nil {
				t.Fatal("native complete protobuf", e)
			}
		case "result":
			winner = event.AttackerWon
		case "skill_begin":
			var data struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(event.Data, &data)
			skills[data.ID] = true
		}
	}
	if err = scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if (out.Outcome.Winner == 0) != winner || !proto.Equal(&finalRecord, out.OriginalOperates) {
		t.Fatal("native winner or complete final export differs", out.Outcome.Winner, winner)
	}
	actors := append(terminal.Attacker.Actors, terminal.Defender.Actors...)
	if len(actors) != 12 || len(out.FinalActors) != 12 {
		t.Fatal("actual twelve actors missing")
	}
	for _, actual := range out.FinalActors {
		matched := false
		for _, original := range actors {
			if actual.UID == original.UID {
				matched = true
				if actual.CurrentHP != original.Attribute.HP || actual.MaxHP != original.Attribute.MaxHP {
					t.Fatal("native exact HP differs", actual, original)
				}
			}
		}
		if !matched {
			t.Fatal("extra final actor", actual.UID)
		}
	}
	for _, skill := range []string{"1067_1", "1068_1", "1070_1"} {
		if !skills[skill] {
			t.Fatal("real reused normal attack never executed", skill)
		}
	}
	for _, hero := range []string{"hero_056", "hero_057", "hero_058", "hero_059", "hero_060", "hero_061"} {
		row := s.table("Hero", "id", hero)
		if row["SkillId2"] != "" || row["UltSkillId"] != "" || row["attBase"] != "10.0" || row["hpBase"] != "50.0" {
			t.Fatal("green power changed", row)
		}
		frag := s.table("Frag", "id", "frag_"+hero)
		if frag["num"] != "10" || frag["price"] != "0" {
			t.Fatal("green fragment local fallback", frag)
		}
	}
}

// 每个绿色角色先从实际owned Warrior/Formation生产战斗投影，再核原三槽和绿色基础面板。
func TestP15GreenOwnedFormationBattleProjection(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	// 与main相同后续加载成长/采集/精灵表，确认不会覆盖兼容行。
	for _, load := range []func(string) error{s.LoadP5Tables, s.LoadP8Tables, s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"hero_056", "hero_057", "hero_058", "hero_059", "hero_060", "hero_061"} {
		t.Run(id, func(t *testing.T) {
			w := &pb.Warrior{Id: proto.String(id), Guid: proto.String("owned-green-" + id), Level: proto.Int32(1), Talent: proto.Int32(0), Awake: &pb.Awake{}, Breached: &pb.Breached{Level: proto.Int32(1)}}
			p := &pb.Player{PlayerId: proto.Int64(913057), Level: proto.Int32(1), HeroWarId: proto.String("unrelated-main"), Name: proto.String("green-owned-projection")}
			g := &model.GameState{Warriors: []*pb.Warrior{w}, Formation: []*pb.FormationItem{{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Id: w.Id, Guid: w.Guid, Pos: proto.Int32(0)}}}}}
			before := proto.Clone(w)
			party, err := s.battlePlayer(p, g)
			if err != nil || len(party.GetFormation()) != 1 {
				t.Fatal("real owned formation cannot enter battle", err)
			}
			actor := party.Formation[0]
			want := s.table("Hero", "id", id)["SkillId1"] + ",,"
			if actor.GetId() != id || actor.GetGuid() != w.GetGuid() || actor.GetSkillOrder() != want || actor.GetAwakenSkill() != "" || actor.GetAwakeStage() != 0 || len(actor.GetTalentList()) != 0 {
				t.Fatal("green actor protocol changed", actor)
			}
			if actor.GetProp().GetAtt() != 10 || actor.GetProp().GetHp() != 50 || actor.GetProp().GetPdef() != 5 || actor.GetProp().GetMdef() != 5 {
				t.Fatal("green base stats were inflated", actor.GetProp())
			}
			if !proto.Equal(before, w) {
				t.Fatal("battle projection changed owned warrior")
			}
			for _, invalid := range []*pb.Warrior{
				{Id: w.Id, SkillOrder: proto.String(want)},
				{Id: w.Id, AwakenSkill: proto.String("1070_1,2")},
				{Id: w.Id, Awake: &pb.Awake{Stage: proto.Int32(1)}},
			} {
				if _, _, err = s.P5BattleSkillTransport(invalid); err == nil {
					t.Fatal("forged green order/awakening accepted", invalid)
				}
			}
			forged := proto.Clone(w).(*pb.Warrior)
			forged.Talent = proto.Int32(1)
			if _, err = s.battleWarrior(p, forged, g, g.Formation[0]); err == nil {
				t.Fatal("green acquired missing talent growth")
			}
		})
	}
}
