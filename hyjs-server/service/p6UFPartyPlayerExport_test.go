package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"testing"
)

// Independent prospective UF14 Player/Player source. No DB, war ownership,
// calculation, winner, native result or settlement authority is produced.
func TestP6UFPartyPlayerExportProspectiveInput(t *testing.T) {
	if os.Getenv("HYJS_P6_UF_PARTY_PLAYER_EXPORT") != "1" {
		t.Skip("explicit prospective single PK export only")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("HYJS_P6_UF_PARTY_PLAYER_OUT")
	if out != filepath.Join(root, "analysis/p6/guild-party-pvp-input-20261026-r1") {
		t.Fatal("exact fresh PK output directory required")
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("preserve existing output", err)
	}
	read := func(p string) []byte {
		t.Helper()
		r, e := readTestAnalysis75(p)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	parent := filepath.Join(root, "analysis/p6/guild-guard-prospective-inputs-20261026-r1/manifest.json")
	const parentSHA = "a4fd9c297a43aca5793c4be171b45abfa23dd3e0252d6830957f5935d7f26644"
	parentRaw := read(parent)
	if p6Digest(parentRaw) != parentSHA {
		t.Fatal("fixed14 parent drift")
	}
	var metadata struct {
		Cases []map[string]json.RawMessage `json:"cases"`
	}
	if json.Unmarshal(parentRaw, &metadata) != nil || len(metadata.Cases) != 14 {
		t.Fatal("parent14 decode")
	}
	source := metadata.Cases[10]
	text := func(key string) string {
		var v string
		if json.Unmarshal(source[key], &v) != nil {
			t.Fatal("source field", key)
		}
		return v
	}
	pins := map[string]string{}
	var prior model.FrozenBattleInput
	for _, pair := range [][2]string{{"wire", "wireSHA256"}, {"originalCallbackFile", "originalCallbackSHA256"}, {"frozenInput", "p6InputHash"}} {
		name := text(pair[0])
		if filepath.Base(name) != name {
			t.Fatal("unsafe parent filename")
		}
		path := filepath.Join(filepath.Dir(parent), name)
		raw := read(path)
		if p6Digest(raw) != text(pair[1]) {
			t.Fatal("parent byte SHA drift")
		}
		relative, _ := filepath.Rel(root, path)
		pins[filepath.ToSlash(relative)] = p6Digest(raw)
		if pair[0] == "frozenInput" && json.Unmarshal(raw, &prior) != nil {
			t.Fatal("parent frozen decode")
		}
	}
	if prior.Seed != 1256795199 {
		t.Fatal("fixed observed seed drift")
	}
	svc := &GameService{Tables: prior.Tables}
	players := []*pb.BattlePlayer{}
	for camp := 0; camp < 2; camp++ {
		player := proto.Clone(prior.Attacker).(*pb.BattlePlayer)
		player.PlayerId = proto.Int64(int64(camp + 1))
		player.Formation = nil
		player.Inst.FirstOrder = proto.Int32(int32(1 - camp))
		player.Inst.ExtBattleAdds = nil
		player.Inst.Anger = proto.Float64([]float64{88.25, 121.5}[camp])
		for i, id := range []string{"hero_047", "hero_002", "hero_013", "hero_001"} {
			actor, e := svc.actor(svc.table("Hero", "id", id), fmt.Sprintf("pk-party-owner%d-hero%d", camp+1, i+1), []int32{3, 7, 1, 9}[i], false)
			if e != nil {
				t.Fatal(e)
			}
			actor.Level, actor.BloodNumber = proto.Int32(1), proto.Int32(1)
			if camp == 0 && i == 0 {
				actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp() * 0.5)
			}
			if camp == 0 && i == 3 || camp == 1 && i == 2 {
				actor.Inst.CurHp = proto.Float64(0)
			}
			if camp == 1 && i == 1 {
				actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp() * 0.7)
			}
			player.Formation = append(player.Formation, actor)
		}
		players = append(players, player)
	}
	input, err := svc.FreezeBattleInput(model.FrozenBattleInput{Attacker: players[0], Defender: players[1], Seed: 1256795199, MaxRound: 40, BattleType: 14, IsAttToDef: false, DefenderFirst: false, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1), ClientVersion: prior.ClientVersion, AppliedProperties: map[string][]string{"prospective-party-pvp": {"new mathematical4party Player/Player mode14 anchor; no real enrollment/roster/ownership/quote/result/authority"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateP6FirstOrder(input); err != nil {
		t.Fatal("original first-order validator", err)
	}
	qualification, err := svc.ValidateP6GenericMechanisms(t.Context(), input)
	if err != nil || qualification == nil || qualification.NativeCertified || qualification.ActorCount != 8 {
		t.Fatal("original8-source-actor mechanism qualification", qualification, err)
	}
	fightID := "prospective-party-pvp14-seed1256795199"
	typed := &pb.SCUFFightRecordReplay{FightId: proto.String(fightID), RandomSeed: proto.Int64(input.Seed), PlayerList: []*pb.BattlePlayer{input.Attacker, input.Defender}, MaxRound: proto.Int32(40), AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)}
	wire := &pb.SCFightPlayback{Fightid: proto.String(fightID), RandSeed: proto.Int64(input.Seed), BattleType: proto.Int32(14), Attacker: input.Attacker, Defender1: input.Defender, MaxRound: proto.Int32(40), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
	roundtrip, err := svc.FreezeP6UFPlayerInput(typed)
	if err != nil || !proto.Equal(roundtrip.Attacker, input.Attacker) || !proto.Equal(roundtrip.Defender, input.Defender) || roundtrip.Monster != nil || roundtrip.Seed != input.Seed || roundtrip.MaxRound != 40 || roundtrip.BattleType != 14 || roundtrip.DefenderFirst || roundtrip.IsAttToDef || roundtrip.ConfigurationHash != input.ConfigurationHash || typed.WinPlayerId != nil || wire.Defender2 != nil || wire.OptRecord != nil {
		t.Fatal("independent original typed Player/Player reverse projection", err)
	}
	marshal := func(msg proto.Message) []byte {
		t.Helper()
		r, e := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	wireRaw, typedRaw := marshal(wire), marshal(typed)
	var decodedWire pb.SCFightPlayback
	var decodedTyped pb.SCUFFightRecordReplay
	if proto.Unmarshal(wireRaw, &decodedWire) != nil || proto.Unmarshal(typedRaw, &decodedTyped) != nil || !proto.Equal(decodedWire.Attacker, decodedTyped.PlayerList[0]) || !proto.Equal(decodedWire.Defender1, decodedTyped.PlayerList[1]) || decodedTyped.GetRandomSeed() != decodedWire.GetRandSeed() || decodedTyped.WinPlayerId != nil {
		t.Fatal("serialized typed/wire reverse equality")
	}
	frozenRaw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	frozenRaw = append(frozenRaw, '\n')
	actors := func(player *pb.BattlePlayer) map[string]any {
		list := []map[string]any{}
		for _, h := range player.Formation {
			list = append(list, map[string]any{"guid": h.GetGuid(), "heroID": h.GetId(), "pose": h.GetPose(), "bloodNumber": h.GetBloodNumber(), "hp": h.Inst.GetCurHp(), "maxHP": h.Prop.GetHp(), "sha256": p6Digest(marshal(h))})
		}
		return map[string]any{"playerID": player.GetPlayerId(), "firstOrder": player.Inst.GetFirstOrder(), "anger": player.Inst.GetAnger(), "actors": list}
	}
	files := map[string][]byte{"000.pb": wireRaw, "000.SCUFFightRecordReplay.prospective.pb": typedRaw, "000.frozen-input.json": frozenRaw}
	row := map[string]any{"case": "prospective-uf14-party4-v-party4-current-hp-ap-dead-partners", "wire": "000.pb", "wireSHA256": p6Digest(wireRaw), "originalCallback": "SCUFFightRecordReplay", "originalCallbackFile": "000.SCUFFightRecordReplay.prospective.pb", "originalCallbackSHA256": p6Digest(typedRaw), "frozenInput": "000.frozen-input.json", "p6InputHash": p6Digest(frozenRaw), "battleType": 14, "fightId": fightID, "seed": input.Seed, "nativeScope": "original-pvp-automatic-ai", "typedCallbackSourceOnly": true, "typedCallbackExecuted": false, "attacker": actors(input.Attacker), "defender": actors(input.Defender), "parentPins": pins, "parent14CaseMetadata": source, "parent14CaseIndex": 10, "diagnosticOnly": true, "prospectiveDerived": true, "authoritative": false, "nativeCompared": false, "warApplied": false, "mechanismQualification": qualification, "status": "AWAITING_INDEPENDENT_ORIGINAL_PVP14"}
	sourcePins := map[string]string{}
	for _, name := range []string{"hyjs-server/service/p6UFPartyPlayerExport_test.go", "hyjs-server/service/copyService.go", "hyjs-server/service/p6UFPlayerSettlementService.go", "hyjs-server/service/p6GenericMechanismService.go", "analysis/tables/TableHero.tsv"} {
		sourcePins[name] = p6Digest(read(filepath.Join(root, name)))
	}
	result := map[string]any{"schema": "guild-party-pvp-input-v1", "caseCount": 1, "cases": []any{row}, "parentManifest": parent, "parentManifestSHA256": parentSHA, "sourcePins": sourcePins, "diagnosticOnly": true, "prospectiveDerived": true, "naturalWarContinuation": false, "authoritative": false, "nativeCompared": false, "warApplied": false, "parent14Reexecuted": false, "new2Reexecuted": false, "scope": "Independent originalUF14 Player/Player mathematical4slot parties, each3initiallyalive; no natural war/business qualification or borrowed PVE/mode13 authority.", "goComparisonRequirements": []string{"Exact original all events/8 initial actors/terminalHP/AP/all operations/finalPB/13 layers; tolerance0, platform ai_notify.stack separately retained."}}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err = os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(out, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(out, "manifest.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("PK manifest=%s SHA256=%s", filepath.Join(out, "manifest.json"), p6Digest(raw))
}
