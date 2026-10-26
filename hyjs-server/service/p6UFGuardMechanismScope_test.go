package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p6GuardMechanismFixture struct {
	Input  *model.FrozenBattleInput
	Player *pb.InstUFPlayer
	Guard  *pb.InstUFGuard
	Offer  *db.P13UnionFightGuardAuditOffer
}

func p6GuardMechanismFixtures(t *testing.T) []p6GuardMechanismFixture {
	t.Helper()
	root := "../../analysis/p6/guild-guard-inputs-20261025-raw"
	raw, err := os.ReadFile(filepath.Join(root, "source-export.json"))
	if os.IsNotExist(err) {
		t.Skip("retained original producer input/offer/frames required")
	}
	if err != nil {
		t.Fatal(err)
	}
	if p6Digest(raw) != "1e000e88490fbbbf833a19df39f721e7a6da73578babc173fd11f7a3dfcb8478" {
		t.Fatal("original source export pin drift")
	}
	var source struct {
		Cases []struct {
			Prefix string                          `json:"prefix"`
			Files  map[string]string               `json:"files"`
			Quote  db.P13UnionFightGuardAuditOffer `json:"quote"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &source) != nil || len(source.Cases) != 10 {
		t.Fatal("ten original source cases required")
	}
	result := []p6GuardMechanismFixture{}
	for _, row := range source.Cases {
		read := func(suffix string) []byte {
			t.Helper()
			name := row.Prefix + "." + suffix
			raw, err := os.ReadFile(filepath.Join(root, name))
			if err != nil || row.Files[name] == "" || p6Digest(raw) != row.Files[name] {
				t.Fatal("source SHA drift", name, err)
			}
			return raw
		}
		offer := row.Quote
		fixture := p6GuardMechanismFixture{Input: &model.FrozenBattleInput{}, Player: &pb.InstUFPlayer{}, Guard: &pb.InstUFGuard{}, Offer: &offer}
		if json.Unmarshal(read("frozen-input.json"), fixture.Input) != nil || proto.Unmarshal(read("player-frame.pb"), fixture.Player) != nil || proto.Unmarshal(read("guard-frame.pb"), fixture.Guard) != nil {
			t.Fatal("original source decode")
		}
		if fixture.Offer.SourceHash != p13UFGuardOfferHash(fixture.Offer) || fixture.Offer.MonsterHash != p6Digest(fixture.Offer.Monster) {
			t.Fatal("original offer SHA drift")
		}
		result = append(result, fixture)
	}
	return result
}

func p6GuardMechanismClone(t *testing.T, source p6GuardMechanismFixture) p6GuardMechanismFixture {
	t.Helper()
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var out p6GuardMechanismFixture
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("fixture clone")
	}
	return out
}

// This exercises intrinsic qualification and exact source/frame binding only.
// New parties and mirror templates here are prospective test inputs, without a
// claim that these parties have been compared with the original phone engine.
func TestP6UFGuardMechanismScopeRealSourcesAndMirrorParties(t *testing.T) {
	for _, seed := range p6GuardMechanismFixtures(t) {
		t.Run(seed.Offer.GuardID, func(t *testing.T) {
			engine := &GameService{Tables: seed.Input.Tables}
			q, err := engine.ValidateP6UFGuardMechanisms(context.Background(), seed.Input, seed.Input.Attacker.GetPlayerId(), seed.Player, seed.Guard, seed.Offer)
			if err != nil || q == nil || q.NativeCertified {
				t.Fatal("real producer qualification", err)
			}
			for n := 1; n <= 4; n++ {
				t.Run(fmt.Sprintf("party-%d", n), func(t *testing.T) {
					f := p6GuardMechanismClone(t, seed)
					f.Input.Attacker.Formation = nil
					heroes := []string{"hero_047", "hero_002", "hero_013", "hero_001"}
					poses := []int32{3, 7, 1, 9}
					for i := 0; i < n; i++ {
						actor, err := engine.actor(engine.table("Hero", "id", heroes[i]), fmt.Sprintf("party-%d-hero-%d", f.Input.Attacker.GetPlayerId(), i+1), poses[i], false)
						if err != nil {
							t.Fatal(err)
						}
						actor.Level, actor.BloodNumber = proto.Int32(1), proto.Int32(1)
						actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp() * 0.625)
						if n == 4 && i == 3 {
							actor.Inst.CurHp = proto.Float64(0)
						}
						f.Input.Attacker.Formation = append(f.Input.Attacker.Formation, actor)
					}
					f.Input.Attacker.Inst.Anger = proto.Float64(88.25)
					f.Player.FightInfo.Index = nil
					f.Player.FightInfo.HpPos = nil
					f.Player.FightInfo.MaxHpPos = nil
					f.Player.FightInfo.Anger = proto.Float64(88.25)
					// Reverse frame storage to prove binding follows real Pose, not slice order.
					for i := n - 1; i >= 0; i-- {
						hero := f.Input.Attacker.Formation[i]
						f.Player.FightInfo.Index = append(f.Player.FightInfo.Index, hero.GetPose())
						f.Player.FightInfo.HpPos = append(f.Player.FightInfo.HpPos, hero.Inst.GetCurHp())
						f.Player.FightInfo.MaxHpPos = append(f.Player.FightInfo.MaxHpPos, hero.Prop.GetHp())
					}
					// Mirror source is the complete original actor, never an HP-only rewrite.
					heroID, err := p13UFGuardTemplateHeroID(f.Offer.GuardID, f.Offer.HeroID, constUtil.P13UFGuardMirrorRule)
					if err != nil {
						t.Fatal(err)
					}
					hero, err := engine.actor(engine.table("Hero", "id", heroID), "mirror-source", 3, false)
					if err != nil {
						t.Fatal(err)
					}
					hero.Level, hero.BloodNumber = proto.Int32(1), proto.Int32(1)
					template := proto.Clone(f.Input.Monster).(*pb.BattleMonster)
					template.Formation = []*pb.BattleHero{hero}
					template.Inst.Anger = proto.Float64(0)
					f.Offer.GuardRule = constUtil.P13UFGuardMirrorRule
					f.Offer.HeroID = heroID
					f.Offer.InitialHP = hero.Prop.GetHp()
					f.Offer.Monster, _ = proto.MarshalOptions{Deterministic: true}.Marshal(template)
					f.Offer.MonsterHash = p6Digest(f.Offer.Monster)
					f.Offer.SourceHash = p13UFGuardOfferHash(f.Offer)
					f.Guard.FightInfo.HpPos[0] = hero.Prop.GetHp()
					f.Guard.FightInfo.MaxHpPos[0] = hero.Prop.GetHp()
					f.Guard.FightInfo.Anger = proto.Float64(0)
					f.Input.Monster, err = p13UFGuardAuditMonster(f.Offer, f.Guard)
					if err != nil {
						t.Fatal(err)
					}
					if err = validateP6UFGuardMechanismScope(f.Input); err != nil {
						t.Fatal("prospective intrinsic scope", err)
					}
					q, err = engine.ValidateP6UFGuardMechanisms(context.Background(), f.Input, f.Input.Attacker.GetPlayerId(), f.Player, f.Guard, f.Offer)
					if err != nil || q == nil || q.NativeCertified || q.ActorCount != n+1 {
						t.Fatal("prospective source/frame qualification", n, q, err)
					}
					negatives := map[string]func(*p6GuardMechanismFixture){
						"player-frame-owner": func(v *p6GuardMechanismFixture) { v.Player.Guid = proto.Int64(v.Player.GetGuid() + 1) },
						"fight-frame-owner":  func(v *p6GuardMechanismFixture) { v.Player.FightInfo.Guid = proto.Int64(v.Player.GetGuid() + 1) },
						"actor-source":       func(v *p6GuardMechanismFixture) { v.Input.Attacker.Formation[0].Id = proto.String("hero_missing") },
						"actor-skill":        func(v *p6GuardMechanismFixture) { v.Input.Attacker.Formation[0].SkillOrder = proto.String("injected") },
						"pose-binding":       func(v *p6GuardMechanismFixture) { v.Player.FightInfo.Index[0] = 99 },
						"HP-binding":         func(v *p6GuardMechanismFixture) { v.Player.FightInfo.HpPos[0]++ },
						"AP-binding":         func(v *p6GuardMechanismFixture) { v.Player.FightInfo.Anger = proto.Float64(0) },
						"guard-binding":      func(v *p6GuardMechanismFixture) { v.Guard.Guid = proto.Int64(v.Guard.GetGuid() + 1) },
						"guard-panel":        func(v *p6GuardMechanismFixture) { v.Input.Monster.Formation[0].Prop.Att = proto.Float64(99999) },
						"offer-source-hash":  func(v *p6GuardMechanismFixture) { v.Offer.SourceHash = "bad" },
						"source-rule": func(v *p6GuardMechanismFixture) {
							v.Offer.GuardRule = "unpublished"
							v.Offer.SourceHash = p13UFGuardOfferHash(v.Offer)
						},
						"factor":       func(v *p6GuardMechanismFixture) { v.Input.AttackerDamageFactor = proto.Float64(2) },
						"driver":       func(v *p6GuardMechanismFixture) { v.Input.DiagnosticAIDriverPolicy = "different-driver" },
						"table-source": func(v *p6GuardMechanismFixture) { v.Input.Tables["Hero"][0]["hp"] = "999999" },
					}
					if n > 1 {
						negatives["duplicate-pose"] = func(v *p6GuardMechanismFixture) {
							v.Input.Attacker.Formation[1].Pose = v.Input.Attacker.Formation[0].Pose
						}
						negatives["duplicate-guid"] = func(v *p6GuardMechanismFixture) {
							v.Input.Attacker.Formation[1].Guid = v.Input.Attacker.Formation[0].Guid
						}
					}
					for name, mutate := range negatives {
						t.Run(name, func(t *testing.T) {
							bad := p6GuardMechanismClone(t, f)
							mutate(&bad)
							if _, err := engine.ValidateP6UFGuardMechanisms(context.Background(), bad.Input, bad.Input.Attacker.GetPlayerId(), bad.Player, bad.Guard, bad.Offer); err == nil {
								t.Fatal("invalid mechanism/frame/source admitted")
							}
						})
					}
				})
			}
		})
	}
}
