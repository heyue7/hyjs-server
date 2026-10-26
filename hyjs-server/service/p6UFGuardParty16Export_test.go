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

func TestP6UFGuardParty16ExportProspectiveInputs(t *testing.T) {
	if os.Getenv("HYJS_P6_UF_GUARD_PARTY16_EXPORT") != "1" {
		t.Skip("explicit prospective export only; no DB/authority")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("HYJS_P6_UF_GUARD_PARTY16_OUT")
	if out != filepath.Join(root, "analysis/p6/guild-guard-party16-inputs-20261026-r1") {
		t.Fatal("exact new output directory required")
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("fresh output directory required", err)
	}
	parent := filepath.Join(root, "analysis/p6/guild-guard-prospective-inputs-20261026-r1/manifest.json")
	read := func(p string) []byte {
		t.Helper()
		r, e := readTestAnalysis75(p)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	parentRaw := read(parent)
	const parentSHA = "a4fd9c297a43aca5793c4be171b45abfa23dd3e0252d6830957f5935d7f26644"
	if p6Digest(parentRaw) != parentSHA {
		t.Fatal("exact14 parent changed")
	}
	var parentManifest map[string]json.RawMessage
	if json.Unmarshal(parentRaw, &parentManifest) != nil {
		t.Fatal("parent manifest decode")
	}
	var cases []map[string]json.RawMessage
	if json.Unmarshal(parentManifest["cases"], &cases) != nil || len(cases) != 14 {
		t.Fatal("exact14 parent cases")
	}
	get := func(row map[string]json.RawMessage, key string) string {
		var v string
		if json.Unmarshal(row[key], &v) != nil {
			t.Fatal("missing string", key)
		}
		return v
	}
	artifacts := map[string][]byte{}
	for _, row := range cases {
		for _, pair := range [][2]string{{"wire", "wireSHA256"}, {"originalCallbackFile", "originalCallbackSHA256"}, {"frozenInput", "p6InputHash"}} {
			name := get(row, pair[0])
			if filepath.Base(name) != name {
				t.Fatal("unsafe parent name")
			}
			raw := read(filepath.Join(filepath.Dir(parent), name))
			if p6Digest(raw) != get(row, pair[1]) {
				t.Fatal("parent artifact SHA", name)
			}
			artifacts[name] = raw
		}
	}
	svc := p6OriginalTables(t)
	for number, parentIndex := range []int{10, 12} {
		index := 14 + number
		row := cases[parentIndex]
		var prior model.FrozenBattleInput
		if json.Unmarshal(artifacts[get(row, "frozenInput")], &prior) != nil {
			t.Fatal("parent frozen decode")
		}
		attacker := proto.Clone(prior.Attacker).(*pb.BattlePlayer)
		attacker.Formation = nil
		attacker.Inst.Anger = proto.Float64(88.25)
		for i, id := range []string{"hero_047", "hero_002", "hero_013", "hero_001"} {
			actor, e := svc.actor(svc.table("Hero", "id", id), fmt.Sprintf("p16-c%d-h%d", index, i+1), []int32{3, 7, 1, 9}[i], false)
			if e != nil {
				t.Fatal(e)
			}
			actor.Level, actor.BloodNumber = proto.Int32(1), proto.Int32(1)
			if i == 0 {
				actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp() / 2)
			}
			attacker.Formation = append(attacker.Formation, actor)
		}
		heroID := []string{"guard01_npc", "hero_002"}[number]
		hero, e := svc.actor(svc.table("Hero", "id", heroID), prior.Monster.Formation[0].GetGuid(), 3, false)
		if e != nil {
			t.Fatal(e)
		}
		hero.Level, hero.BloodNumber = proto.Int32(1), proto.Int32(1)
		monster := proto.Clone(prior.Monster).(*pb.BattleMonster)
		monster.Formation = []*pb.BattleHero{hero}
		monster.Inst.Anger = proto.Float64(0)
		input, e := svc.FreezeBattleInput(model.FrozenBattleInput{Attacker: attacker, Monster: monster, Seed: prior.Seed, MaxRound: 40, BattleType: 14, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1), ClientVersion: prior.ClientVersion, AppliedProperties: map[string][]string{"prospective-party16": {"explicit mathematical mechanism probe; no match/quote/ownership/result/authority"}}})
		if e != nil {
			t.Fatal(e)
		}
		if e = validateP6UFGuardMechanismScope(input); e != nil {
			t.Fatal("intrinsic party scope", e)
		}
		if e = ValidateP6FirstOrder(input); e != nil {
			t.Fatal("original first order", e)
		}
		q, e := svc.ValidateP6GenericMechanisms(t.Context(), input)
		if e != nil || q == nil || q.NativeCertified || q.ActorCount != 5 {
			t.Fatal("original Generic mechanism qualification", q, e)
		}
		fightID := get(row, "fightId")
		wire := &pb.SCFightPlayback{Fightid: proto.String(fightID), RandSeed: proto.Int64(input.Seed), BattleType: proto.Int32(14), Attacker: input.Attacker, Defender2: input.Monster, AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1), MaxRound: proto.Int32(40)}
		typed := &pb.SCUFPveFightRecordReplay{FightId: proto.String(fightID), RandomSeed: proto.Int64(input.Seed), Player: input.Attacker, Monster: input.Monster, MaxRound: proto.Int32(40), AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)}
		prefix := fmt.Sprintf("%03d", index)
		wireName := prefix + ".pb"
		typedName := prefix + ".SCUFPveFightRecordReplay.prospective.pb"
		frozenName := prefix + ".frozen-input.json"
		wireRaw, e := proto.MarshalOptions{Deterministic: true}.Marshal(wire)
		if e != nil {
			t.Fatal(e)
		}
		typedRaw, e := proto.MarshalOptions{Deterministic: true}.Marshal(typed)
		if e != nil {
			t.Fatal(e)
		}
		frozenRaw, e := json.Marshal(input)
		if e != nil {
			t.Fatal(e)
		}
		frozenRaw = append(frozenRaw, '\n')
		artifacts[wireName], artifacts[typedName], artifacts[frozenName] = wireRaw, typedRaw, frozenRaw
		actorMeta := func(heroes []*pb.BattleHero) []map[string]any {
			v := []map[string]any{}
			for _, h := range heroes {
				r, _ := proto.MarshalOptions{Deterministic: true}.Marshal(h)
				v = append(v, map[string]any{"guid": h.GetGuid(), "heroID": h.GetId(), "pose": h.GetPose(), "bloodNumber": h.GetBloodNumber(), "hp": h.Inst.GetCurHp(), "maxHP": h.Prop.GetHp(), "sha256": p6Digest(r)})
			}
			return v
		}
		meta := map[string]any{"case": "prospective-party4-full-" + heroID, "guardHeroID": heroID, "parentCase": get(row, "case"), "derivation": "full-original-actor-party4", "wire": wireName, "wireSHA256": p6Digest(wireRaw), "frozenInput": frozenName, "p6InputHash": p6Digest(frozenRaw), "originalCallback": "SCUFPveFightRecordReplay", "originalCallbackFile": typedName, "originalCallbackSHA256": p6Digest(typedRaw), "typedCallbackSourceOnly": true, "typedCallbackExecuted": false, "fightId": fightID, "battleType": 14, "seed": input.Seed, "nativeScope": "original-pve-automatic-ai", "originalWrapperScope": "original-uf14-playerai-monster", "attacker": map[string]any{"anger": 88.25, "actors": actorMeta(attacker.Formation)}, "monster": map[string]any{"anger": 0, "actors": actorMeta(monster.Formation)}, "parentPins": map[string]string{filepath.ToSlash(filepath.Join("analysis/p6/guild-guard-prospective-inputs-20261026-r1", get(row, "wire"))): get(row, "wireSHA256"), filepath.ToSlash(filepath.Join("analysis/p6/guild-guard-prospective-inputs-20261026-r1", get(row, "frozenInput"))): get(row, "p6InputHash"), filepath.ToSlash(filepath.Join("analysis/p6/guild-guard-prospective-inputs-20261026-r1", get(row, "originalCallbackFile"))): get(row, "originalCallbackSHA256")}, "diagnosticOnly": true, "prospectiveDerived": true, "nativeCompared": false, "authoritative": false, "warApplied": false, "status": "PROSPECTIVE_AWAITING_ORIGINAL_ENGINE", "assumption": "Original historical fight/player/guard IDs retained only as provenance. Complete actors rebuilt from original Hero rows, not a business roster or quote. No result or winner supplied.", "mechanismQualification": q}
		raw, _ := json.Marshal(meta)
		var derived map[string]json.RawMessage
		_ = json.Unmarshal(raw, &derived)
		cases = append(cases, derived)
	}
	pins := map[string]string{}
	for _, name := range []string{"hyjs-server/service/p6UFGuardParty16Export_test.go", "hyjs-server/service/copyService.go", "hyjs-server/service/p6UFGuardService.go", "hyjs-server/service/p6GenericMechanismService.go", "analysis/tables/TableHero.tsv"} {
		pins[name] = p6Digest(read(filepath.Join(root, name)))
	}
	manifest := map[string]any{"schema": "guild-guard-party16-input-v1", "caseCount": 16, "cases": cases, "parentManifest": parent, "parentManifestSHA256": parentSHA, "parentMetadata": parentManifest, "first14ArtifactsByteExact": true, "first14MetadataPreserved": true, "prospectiveDerived": true, "naturalWarContinuation": false, "diagnosticOnly": true, "nativeCompared": false, "authoritative": false, "warApplied": false, "sourcePins": pins, "goComparisonRequirements": []string{"Compare full original events, all actor initial SHA/terminal HP/AP/operations/finalPB with tolerance0; ai_notify.stack platform difference separate.", "New parties are mathematical mechanism anchors only, no actual war/ownership/authority asserted."}}
	if err = os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for name, raw := range artifacts {
		if err = os.WriteFile(filepath.Join(out, name), raw, 0644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err = os.WriteFile(filepath.Join(out, "manifest.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("party16 manifest=%s SHA256=%s", filepath.Join(out, "manifest.json"), p6Digest(raw))
}
