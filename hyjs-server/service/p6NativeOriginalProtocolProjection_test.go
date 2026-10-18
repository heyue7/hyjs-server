package service

import (
	"bytes"
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

// Original playback evidence supplies its actual player record explicitly.
// Automatic nil-record cases retain their existing cache and comparison path;
// source-record variants run independently and never share the automatic cache.
func p6NativeGoldenComputeProjection(t *testing.T, s *GameService, input *model.FrozenBattleInput, wireSHA, fixtureSHA string, record *pb.BattleOperate, observer func(*combatRoundEngine)) (*model.CombatOutcome, error) {
	t.Helper()
	if grab, ok := p6NativeGrabFixtureInput(t, s, input, wireSHA, fixtureSHA); ok {
		return p6NativeGoldenCompute(t, s, grab, wireSHA, observer)
	}
	if kf, ok := p6NativeKFRealmFixtureInput(t, s, input, wireSHA, fixtureSHA); ok {
		return p6NativeGoldenCompute(t, s, kf, wireSHA, observer)
	}
	if manual, ok := p6NativeManualAutoFixtureInput(t, input, wireSHA, fixtureSHA); ok {
		return p6NativeGoldenCompute(t, s, manual, wireSHA, observer)
	}
	// Preserve the actual old caller's explicitly shape-selected UF14 guard
	// probe. Its exact PB identity has CopyEnemyAI evidence, and does not certify
	// the original BattleManager UF14 wrapper's both-PlayerAI selection.
	legacyGuardFixture := fixtureSHA == "03b850f2f0b2026b1b4726b441371804445cdcb85aff714d5c2b9b54dcff4d78" || fixtureSHA == "4423c5cd9cd1b2f8ed15c091681bf563bc7c78cd3a074334053f8fcbd3c41e82" || fixtureSHA == "6e6a6c050588f956ad41ddb1c7f1bd24c14ab83a46781390e00d965705af6e16"
	if legacyGuardFixture && wireSHA == "3d69ec109c3d37c6817c871c8e83d14b5924ad96eb843163838b56fd75b5aea8" {
		cloned, err := cloneP6CalculationInput(input)
		if err != nil {
			return nil, err
		}
		cloned.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticUF14CopyAI
		input = cloned
	}
	// The named no-target controller proof is separately SHA-bound. Sharing the
	// same paid wire with strict original-auto evidence never enables recovery.
	compatFixture := fixtureSHA == "a008992ebf9df8f6556289880e4ded460d6b32af4c103877085c8ce45f76cbd0" || fixtureSHA == "a7365bafa691146eed55a7c11de33cc70c59e06a038dd08ef49d2de8d338405d" || fixtureSHA == "931fd0de11a7c809fc4754d4af325e5baaffa324c4c4564d54c84bcfdbbaaaf4" || fixtureSHA == "45db90ac490109975123ac7e14dccc749205aef5925f295efd3847bd7661769d" || fixtureSHA == "fa5a135e6bbad41390ca8254215196c35a5538b5dd2e9557a3b2ca4621f907c6" || fixtureSHA == "e2369e881cb4bf1b2b59a8ecbf1876502cddae45ffce352741d852f13179d575"
	if compatFixture && wireSHA == "f2497a9f8031b3b2c103a3db1aa1b4d57d267849758f960c6990aa392b0668ed" {
		cloned, err := cloneP6CalculationInput(input)
		if err != nil {
			return nil, err
		}
		cloned.PVECompatibilityRule = constUtil.P6LocalPVENoTargetFinish
		input = cloned
	}
	// Automatic PVP captures can carry historical optRecord fields while the
	// original caller deliberately runs PlayerAI. Presence is not driver scope.
	// Only explicitly SHA-bound original-pve-playback-ai captures use the source
	// record. Every earlier automatic fixture retains its established nil/cache.
	recordedScope := wireSHA == "e846c859ec4df2d538aedb3048509b910727d2655f46b1888aec81428f5559e2" || wireSHA == "20917042f2bf5722178eecf85fd59810713f6615e0bb5a2285d5a702071864aa" || wireSHA == "d66b69bc5463cbc29c2ef1c1bfca9f18f60b0ce1576c7b1f08d5b54ca6f69eba" || wireSHA == "2df71d8f04ec04217163e024aa4b158d99605aed6dc3cb33913cab7ed05a720b"
	if record == nil || !recordedScope {
		return p6NativeGoldenCompute(t, s, input, wireSHA, observer)
	}
	return s.computeFrozenBattleRecordObserved(t.Context(), input, record, observer, nil, true)
}

func TestP6NativeAutomaticDriverIgnoresIncidentalSourceRecord(t *testing.T) {
	s := p6OriginalTables(t)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Inst: &pb.BattlePlayerInst{}, Formation: []*pb.BattleHero{p6Actor("hero_047", "scope-player", "047_1", 3, 20000)}}, Defender: &pb.BattlePlayer{PlayerId: proto.Int64(43), Inst: &pb.BattlePlayerInst{}, Formation: []*pb.BattleHero{p6Actor("hero_047", "scope-enemy", "047_1", 7, 20000)}}, Seed: 42, MaxRound: 2, BattleType: 2})
	if err != nil {
		t.Fatal(err)
	}
	want, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Automatic original captures ignore optRecord, including stale source
	// fields that cannot be executed as a current playback operation stream.
	incidental := &pb.BattleOperate{FinalRoundNum: proto.Int32(99), SkillOpts: []*pb.BattleSkillOperate{{Camp: proto.Int32(2), ActorUid: proto.String("stale-actor")}}}
	got, err := p6NativeGoldenComputeProjection(t, s, input, "unbound-original-automatic-scope", "unbound-fixture", incidental, nil)
	if err != nil || !p6CopySeriesJSONEqual(t, want, got) {
		t.Fatal("automatic native scope changed due to incidental optRecord", err)
	}
}

// These two old diagnostic inputs had scalar-default factors0/0 and are kept
// as explicit zero-factor probes, separate from the corrected natural paid
// Copy factors1/1. Their actual original record exposes both-camp collection.
func TestP6NativeOriginalRecordedContainerBothCampsZeroFactor(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-actual-paid-copy-auto-original-records-v86.json", "012ed908ee4175d49644ce153d182af0806476637c4362287d8ff9b5f3d0137b", true)
}

func p6NativeOriginalProtocolRecordFixture(t *testing.T, name, expectedSHA string, requireEnemy bool, expectedCases ...int) {
	t.Helper()
	p6NativeOriginalProtocolFixtureWithTerminalHP(t, name, expectedSHA, requireEnemy, true, expectedCases...)
}

// Separate protocol-byte acceptance from a separately tracked terminal metadata
// mismatch. Existing fixtures always retain their complete terminal HP checks.
func p6NativeOriginalProtocolFixtureWithTerminalHP(t *testing.T, name, expectedSHA string, requireEnemy, compareTerminalHP bool, expectedCases ...int) {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p6/" + name)
	if err != nil || p6Digest(raw) != expectedSHA {
		t.Fatal("actual original final record fixture SHA", err)
	}
	var golden struct {
		Scope       string `json:"nativeScope"`
		Manifest    string `json:"sourceManifest"`
		ManifestSHA string `json:"sourceManifestSHA256"`
		WireSource  string `json:"wireSourceManifest"`
		WireSHA     string `json:"wireSourceManifestSHA256"`
		Cases       []struct {
			Name        string                     `json:"case"`
			Wire        string                     `json:"wire"`
			SHA         string                     `json:"wireSHA256"`
			Win         bool                       `json:"attackerWon"`
			FinalActors []model.CombatFinalActorHP `json:"originalFinalActorHP"`
			Record      struct {
				Attacker bool   `json:"recordAttacker"`
				Defender bool   `json:"recordDefender"`
				Binary   string `json:"protobufBase64"`
				SHA      string `json:"protobufSHA256"`
			} `json:"originalFinalRecord"`
		} `json:"cases"`
	}
	count := 2
	if len(expectedCases) > 0 {
		if len(expectedCases) != 1 || expectedCases[0] <= 0 {
			t.Fatal("exact original record case denominator", expectedCases)
		}
		count = expectedCases[0]
	}
	if err = json.Unmarshal(raw, &golden); err != nil || len(golden.Cases) != count {
		t.Fatal("exact original final record source case count", err)
	}
	for _, binding := range [][2]string{{golden.Manifest, golden.ManifestSHA}, {golden.WireSource, golden.WireSHA}} {
		raw, err = os.ReadFile(filepath.Join("../..", binding[0]))
		if err != nil || p6Digest(raw) != binding[1] {
			t.Fatal("actual native/input manifest binding", err)
		}
	}
	s := p6OriginalTables(t)
	for _, c := range golden.Cases {
		t.Run(c.Name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("../..", c.Wire))
			if err != nil || p6Digest(raw) != c.SHA {
				t.Fatal("original source wire SHA", err)
			}
			wire := &pb.SCFightPlayback{}
			if err = proto.Unmarshal(raw, wire); err != nil {
				t.Fatal(err)
			}
			// This input is an actual SC playback initialization, whose original
			// scalar defaults are0. The natural Copy input separately uses Clear1.
			input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), DefenderFirst: wire.Attacker.GetInst().GetFirstOrder() == 0 && wire.Defender1.GetInst().GetFirstOrder() == 1, AttackerDamageFactor: proto.Float64(wire.GetAttackerFactor()), DefenderDamageFactor: proto.Float64(wire.GetDefenderFactor())})
			if err != nil {
				t.Fatal(err)
			}
			if input.BattleType == 19 {
				var bound bool
				input, bound = p6NativeGrabFixtureInput(t, s, input, c.SHA, expectedSHA)
				if !bound {
					t.Fatal("independent Grab19 fixture binding missing")
				}
			}
			// The SHA-pinned native scope owns driver selection. Historical PVP
			// protobufs may contain old OptRecord fields that automatic AI ignores.
			var sourceRecord *pb.BattleOperate
			switch golden.Scope {
			case "original-wanted-automatic-ai":
				if !p12WantedNativeBoundInput(t, input, c.SHA, expectedSHA) && !p12WantedPartialNativeBoundInput(t, input, c.SHA, expectedSHA) {
					t.Fatal("independent complete typed8 source binding missing")
				}
			case "original-awake16-automatic-ai":
				if !p7AwakeNativeBoundInput(t, input, c.SHA, expectedSHA) {
					t.Fatal("independent complete typed16 source binding missing")
				}
			case "original-field-pvp5-field002-automatic-ai":
				if !p12FieldPVPNativeBoundInput(t, input, c.SHA, expectedSHA) {
					t.Fatal("independent complete typed5 source binding missing")
				}
			case "original-pvp-automatic-ai", "original-pve-automatic-ai", "original-plunder-automatic-ai":
			case "original-worldboss-automatic-ai":
				if !p12WorldBossNativeBoundInput(t, input, c.SHA, expectedSHA) && !p12WorldBossPartyNativeBoundInput(t, input, c.SHA, expectedSHA) {
					t.Fatal("independent complete typed18 diagnostic source binding missing")
				}
			case "local-pve-no-target-finish-v1":
				if c.SHA != "f2497a9f8031b3b2c103a3db1aa1b4d57d267849758f960c6990aa392b0668ed" || expectedSHA != "094b6de34f83d92dfc7b7782893c9344839cbe13ea7f0e127555a07f4c5cfa1d" {
					t.Fatal("named compatibility terminal source binding")
				}
				input.PVECompatibilityRule = constUtil.P6LocalPVENoTargetFinish
			case "local-kf-replay-joint-realm-player-viewer-v1":
				var bound bool
				input, bound = p6NativeKFRealmFixtureInput(t, s, input, c.SHA, expectedSHA)
				if !bound {
					t.Fatal("independent KF realm fixture binding missing")
				}
			case "original-pve-manual-then-auto-ai":
				var bound bool
				input, bound = p6NativeManualAutoFixtureInput(t, input, c.SHA, expectedSHA)
				if !bound {
					t.Fatal("manual original final source binding missing")
				}
			case "original-pve-playback-ai":
				sourceRecord = wire.OptRecord
				if sourceRecord == nil {
					t.Fatal("actual original playback source record is absent")
				}
			default:
				t.Fatal("unaudited original final protocol driver scope", golden.Scope)
			}
			var out *model.CombatOriginalOperateOutcome
			var audit *model.CombatFinalHPOutcome
			if compareTerminalHP && c.FinalActors != nil {
				// One calculation provides both the actual terminal actor snapshot
				// and original final protocol, preserving every native comparison.
				if input.BattleType == 14 && expectedSHA == "1b346084e9d0dddcac8f99b414fbdc77a8524397948c8556fd9f2c5daecd5ac1" && golden.Scope == "original-pvp-automatic-ai" && sourceRecord == nil {
					audit, err = s.ComputeOriginalP6UFPlayerProtocolFinalHP(t.Context(), input)
				} else if input.BattleType == 19 && golden.Scope == "original-pvp-automatic-ai" && sourceRecord == nil {
					audit, err = s.ComputeOriginalP6GrabProtocolFinalHP(t.Context(), input)
				} else if input.BattleType == 20 && golden.Scope == "original-pvp-automatic-ai" && (expectedSHA == "037edbc7abb4f1e18e278988b2ce66d59a1390e49be413dbdf21519add810047" || expectedSHA == "81b95216b3c7151554db5d8ae6ef056280f37c903058c5279aee26a6b610c48d" || expectedSHA == "844516367d45fa688ae110ec319992f55298b9e29be43989ced3afc94662629e" || expectedSHA == "33cb6fc2b787d2f7c9ed4d79af26287dd98bb64acacd70a0c609118981e4170b") && sourceRecord == nil {
					audit, err = s.ComputeOriginalP6LimitProtocolFinalHP(t.Context(), input)
				} else if input.KFReplay != nil && golden.Scope == "local-kf-replay-joint-realm-player-viewer-v1" && sourceRecord == nil {
					audit, err = s.ComputeOriginalP6KFRealmProtocolFinalHP(t.Context(), input)
				} else if input.BattleType == 17 && golden.Scope == "original-pvp-automatic-ai" && sourceRecord == nil {
					// Typed SCKuafuMatchPlayer owns an explicit automatic pair API;
					// generic recorded/KFReplayData modes retain their old gate.
					audit, err = s.ComputeOriginalP6Typed17ProtocolFinalHP(t.Context(), input)
				} else if input.BattleType == 8 && golden.Scope == "original-wanted-automatic-ai" && sourceRecord == nil {
					if !p12WantedNativeBoundInput(t, input, c.SHA, expectedSHA) && !p12WantedPartialNativeBoundInput(t, input, c.SHA, expectedSHA) {
						t.Fatal("independent typed8 terminal source binding missing")
					}
					audit, err = s.computeFrozenBattleFinalHP(t.Context(), input, nil, true)
				} else if input.BattleType == 16 && golden.Scope == "original-awake16-automatic-ai" && sourceRecord == nil {
					if !p7AwakeNativeBoundInput(t, input, c.SHA, expectedSHA) {
						t.Fatal("independent typed16 terminal source binding missing")
					}
					audit, err = s.computeFrozenBattleFinalHP(t.Context(), input, nil, true)
				} else if input.BattleType == 5 && golden.Scope == "original-field-pvp5-field002-automatic-ai" && sourceRecord == nil {
					if !p12FieldPVPNativeBoundInput(t, input, c.SHA, expectedSHA) {
						t.Fatal("independent typed5 terminal source binding missing")
					}
					audit, err = s.computeFrozenBattleFinalHP(t.Context(), input, nil, true)
				} else if input.BattleType == 18 && golden.Scope == "original-worldboss-automatic-ai" && sourceRecord == nil {
					// Only this test's independent typed18 bundle admits the original
					// automatic wrapper. Keep the production generic protocol gate.
					if !p12WorldBossNativeBoundInput(t, input, c.SHA, expectedSHA) && !p12WorldBossPartyNativeBoundInput(t, input, c.SHA, expectedSHA) {
						t.Fatal("independent complete typed18 terminal source binding missing")
					}
					audit, err = s.computeFrozenBattleFinalHP(t.Context(), input, nil, true)
				} else {
					audit, err = s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, sourceRecord)
				}
				if err == nil {
					out = &model.CombatOriginalOperateOutcome{Outcome: audit.Outcome, OriginalOperates: audit.OriginalOperates, RecordAttacker: OriginalP6RecordedCamp(input.BattleType, 0)}
				}
			} else {
				out, err = s.ComputeOriginalP6ProtocolBattle(t.Context(), input, sourceRecord)
			}
			if err != nil || out.Outcome.Authoritative || (out.Outcome.Winner == 0) != c.Win || out.RecordAttacker != c.Record.Attacker || out.RecordDefender != c.Record.Defender {
				t.Fatal("actual original record calculation/result/flags", err)
			}
			want, err := base64.StdEncoding.Strict().DecodeString(c.Record.Binary)
			if err != nil || p6Digest(want) != c.Record.SHA {
				t.Fatal("original protobuf-net byte proof", err)
			}
			got, err := proto.MarshalOptions{Deterministic: true}.Marshal(out.OriginalOperates)
			if err != nil || !bytes.Equal(want, got) {
				t.Fatalf("original final protocol byte drift: nativeSHA%s GoSHA%s error%v", c.Record.SHA, p6Digest(got), err)
			}
			if compareTerminalHP && c.FinalActors != nil {
				if audit == nil || audit.OriginalOperates == nil || len(audit.FinalActors) != len(c.FinalActors) {
					t.Fatal("actual terminal original actor denominator/protocol")
				}
				for index, native := range c.FinalActors {
					calculated := audit.FinalActors[index]
					if calculated.UID != native.UID || calculated.Camp != native.Camp || calculated.HeroID != native.HeroID || calculated.State != native.State || calculated.InitialActorSHA256 == "" {
						t.Fatal("actual terminal runtime actor identity/state", native, calculated)
					}
					if input.BattleType == 18 && golden.Scope == "original-worldboss-automatic-ai" {
						initial := append(append([]*pb.BattleHero{}, input.Attacker.Formation...), input.Monster.Formation...)
						if len(input.Monster.Formation) != 4 || (len(input.Attacker.Formation) != 1 && len(input.Attacker.Formation) != 2) || len(initial) != len(c.FinalActors) || index >= len(initial) {
							t.Fatal("typed18 full terminal actor source denominator")
						}
						source := initial[index]
						encoded, marshalErr := proto.MarshalOptions{Deterministic: true}.Marshal(source)
						if marshalErr != nil || calculated.GUID != source.GetGuid() || calculated.Pose != source.GetPose() || calculated.BloodNumber != source.GetBloodNumber() || calculated.InitialActorSHA256 != p6Digest(encoded) {
							t.Fatal("typed18 exact initial actor/pose/visual blood provenance", marshalErr)
						}
					}
					closeCombat(t, calculated.CurrentHP, native.CurrentHP)
					closeCombat(t, calculated.MaxHP, native.MaxHP)
				}
			}
			if requireEnemy {
				enemy := 0
				for _, op := range out.OriginalOperates.SkillOpts {
					if op.GetCamp() == 1 {
						enemy++
					}
				}
				if enemy == 0 {
					t.Fatal("native multi-round record must contain original NPC actions")
				}
			}
		})
	}
}
