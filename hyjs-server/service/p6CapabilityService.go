package service

import (
	"encoding/json"
	"hyjs-server/model"
)

// P6Capabilities separates executable mechanisms from native-device parity evidence.
// NativeCompared is intentionally false until a complete original battle has matched.
func P6Capabilities() []model.CombatCapability {
	return []model.CombatCapability{
		{Name: "original-jrandom-48", Implemented: true, Evidence: "analysis/p6/dynamic.il.txt RandomUtil.Range; TestCombatJRandomGoldenValues"},
		{Name: "whole-round-ai-target-cooldown-anger", Implemented: true, Evidence: "analysis/p6/core.il.txt; TestP6OriginalWholeBattleAndOperationReplay"},
		{Name: "hit-cast-critical-buff-lifetimes", Implemented: true, Evidence: "analysis/p6/dynamic.il.txt FBuffProc; TestP6RedSkillLifestealOrderAndRoundReset"},
		{Name: "property-stacks-shield-state", Implemented: true, Evidence: "analysis/p6/input-buffs.il.txt DataAttribute/FBuffManager; TestP6BuffStacksCoverDurationShieldAndStaticNoDoubleApply"},
		{Name: "talent-triggers-conditions-aura", Implemented: true, Evidence: "combatRoundBuffService.go supportedP6Timer/Condition/Factor explicit scope"},
		{Name: "native-round-recovery-debuff-reaction-shield-cover", Implemented: true, Evidence: "analysis/p6/conditions.il.txt ProcMyRoundStart/CanBuffCover; TestP6OriginalRoundRecoveryShieldCoverAndDebuffTrigger"},
		{Name: "named-camp-hp-and-cross-property-condition", Implemented: true, Evidence: "analysis/p6/conditions.il.txt HasActorHpPercentCondition/PropertyValCondition; TestP6OriginalRoundRecoveryShieldCoverAndDebuffTrigger"},
		{Name: "critical-float-fall-magicstone-delay", Implemented: true, Evidence: "analysis/p6/release.il.txt ProcSPEState/ProcMagicStone; original timer0 panel remains frozen"},
		{Name: "poison-fire-periodic-state", Implemented: true, Evidence: "analysis/p6/periodic-state.il.txt BuffOfState/ProcMyRoundStart; TestP6OriginalFirePoisonWholeFightAndOperationReplay"},
		{Name: "normal-hit-counterstrike-original-target", Implemented: true, Evidence: "analysis/p6/release.il.txt FRelease 10a0-12e8; supported counter selection 2 only; native parity pending"},
		{Name: "one-use-die-talent-revive-invisible", Implemented: true, Evidence: "analysis/p6/periodic-state.il.txt BuffOfProperty 198d-1a2b/DoDead/CanBuffCover; only timer16 self-select"},
		{Name: "original-projectile-launch-fly-hit", Implemented: true, Evidence: "analysis/p6/core.il.txt FSkill init01bf-0211; FRelease isCast=false calls only SkillProject; original FlyTime"},
		{Name: "native-awakening-skill-slots-frozen-panel", Implemented: true, Evidence: "analysis/battle_actor.il.txt 018b-0285; HeroAwakenAdd frozen; individual dynamic factors retain explicit gate"},
		{Name: "normal-hit-physical-bleeding", Implemented: true, Evidence: "analysis/p6/release.il.txt FRelease 12e9-13f2; percentage property condition, shields and Buff expiry"},
		{Name: "source-maxhp-immediate-typed-damage-state-conditions", Implemented: true, Evidence: "analysis/p6/periodic-state.il.txt BuffOfProperty 1f6f-1fd4; original awakening Hero001"},
		{Name: "native-multiple-bloodbar-total-hp", Implemented: true, Evidence: "analysis/p6/blood-rush-references.il.txt only BattleCharacter.CreateBloodBar consumes bloodNum; InActorData omits it"},
		{Name: "native-operation-wire-guid-manual-wait-queue", Implemented: true, Evidence: "analysis/p6/native-operation-wire.il.txt ParseOptRecord strips camp prefix; PlaybackAI reconstructs it and waits roundTime"},
		{Name: "original-special-state-condition-positive-anger-rate", Implemented: true, Evidence: "analysis/p6/conditions.il.txt GetValueByType speState; analysis/p6/anger.il.txt FPlayer.AddAnger positive gains only"},
		{Name: "native-held-first-weakdeath-release-flow", Implemented: true, Evidence: "analysis/p6/core.il.txt FSkill.Do excludes weak death; FRelease 01de-0215 includes it; FIRST retains selected target"},
		{Name: "native-cast-hit-critical-buff-target-boundaries", Implemented: true, Evidence: "analysis/p6/dynamic.il.txt FBuffProc.LoadAttBuff expands castTargets; Hit/Cri/End use reference; LimitConditions(1,1,!isDebuff)"},
		{Name: "native-target-buff-temporary-suppression", Implemented: true, Evidence: "analysis/p6/release.il.txt FRelease 06ef-070d/155f-1574; FBuffManager.uneffectBuffsOnce/reeffectBuffsOnce; unsupported reactivation factors explicitly reject"},
		{Name: "original-hero-impos-state-float-break", Implemented: true, Evidence: "analysis/p6/conditions.il.txt FActor.init comma list; getFloatTimeBySkill/getFallTimeBySkill Contains(2/3)"},
		{Name: "original-before-cast-area-talent-targets", Implemented: true, Evidence: "analysis/p6/dynamic.il.txt LoadAttBuff timers4/5/6 selects castTargets for SAME_AS_TARGET; PetTalentSkill original IDs"},
		{Name: "original-random-buff-child-clone", Implemented: true, Evidence: "analysis/p6/random-buff.il.txt BuffOfRandom.Clone; Range(0,Count-1) exclusive integer bound; child-specific cover/lifetime"},
		{Name: "original-hp-staircase-property-transfers", Implemented: true, Evidence: "analysis/p6/release.il.txt BuffOfPropertyTransfer.Effect/UnEffect; load-time HP snapshot, native lower/upper Args3 bounds"},
		{Name: "original-source-attack-shield-reactivation", Implemented: true, Evidence: "analysis/p6/release.il.txt BuffOfPropertyTransfer.Effect0c00-0d15/UnEffect0453-04a8; source attack reread and whole typed shield reset"},
		{Name: "original-maxhp-percent-current-skill-cap", Implemented: true, Evidence: "analysis/p6/input-buffs.il.txt BuffOfProperty.Effect05ca-079a; source current skill physical/magic cap, AddHp physical type1; target maxHp unchanged"},
		{Name: "original-anger-combo-critical-source-transfers", Implemented: true, Evidence: "analysis/p6/hp-transfer.il.txt Effect097d-0b5a/0d1a-0d7d/0ffa-1051/1462-1508; eight native rows, snapshot and ordered reactivation"},
		{Name: "original-block-roll-damage-hurt-timer", Implemented: true, Evidence: "analysis/p6/periodic-state.il.txt BuffOfProperty Effect1156-119c/UnEffect0930-0957; FRelease0e1c-0e66; native overwrite/lifetime and timer23"},
		{Name: "original-maxhp-self-round-capped-physical-change", Implemented: true, Evidence: "analysis/p6/input-buffs.il.txt BuffOfProperty.Effect1d4b-1f07; ProcMyRoundStart0465-04dc; target maxHp/source typed attack snapshot"},
		{Name: "original-buff722-class0-no-effect-identity", Implemented: true, Evidence: "analysis/p6/input-buffs.il.txt FBuffFactory.CreateBuff class0 Property; full Effect/UnEffect unknown-factor LogError retains identity with no shield"},
		{Name: "original-all-debuff-exact-id-dispel", Implemented: true, Evidence: "analysis/p6/dispel-full.il.txt BuffOfDispel/FBuffManager both Dispel overloads; clearability, snapshot-all and mutating exact-ID iteration"},
		{Name: "original-anger-steal-lowest-hp-heal-mana-burn", Implemented: true, Evidence: "analysis/p6/input-buffs.il.txt BuffOfProperty.Effect0ca7-1150/UnEffect no-op; raw AP consumption, source physical heal, signed cap and physical HP application"},
		{Name: "original-damage-dual-bonus-and-two-ended-reduction-transfer", Implemented: true, Evidence: "analysis/p6/input-buffs.il.txt class0 Effect225e-2304/UnEffect0ca3-0d49; target-owned source contribution follows same suppression/identity"},
		{Name: "original-both-camps-once-cast-clear", Implemented: true, Evidence: "analysis/p6/release.il.txt FRelease1661-16f0 clears source then opposite camp after AttEnd"},
		{Name: "native-empty-original-skills-idle-and-green-state-guard", Implemented: true, Evidence: "analysis/p6/empty-skill-player-ai.il.txt BattleActorData skips empty slots and EnginePlayerAI null guards; original 13 NPCs use ImposState0; six green rows retain original empty ImposState rejection"},
		{Name: "snapshot-ownership-operation-replay", Implemented: true, Evidence: "battleReplayService.go profile transaction/immutable hash"},
		{Name: "multi-group-native-playback", Implemented: false, Evidence: "SCFightPlayback has one defender2; original multi-group operation mapping needs audit"},
		{Name: "original-unused-playback-victory-condition-transport", Implemented: true, Evidence: "26946 original method bodies/0 parse failures: no SCFightPlayback.get_winConditions consumer; BuildInFightData carries no victory-condition field; original subject0 default rule remains"},
		{Name: "custom-victory-condition-transformation", Implemented: false, Evidence: "Original unused playback list defines no custom victory rules; special play conditions and transform operations require separate original mechanism evidence"},
		{Name: "native-full-battle-parity", Implemented: false, Evidence: "root owns actual device acceptance; native golden capture pending"},
	}
}
func P6CapabilityHash() string { raw, _ := json.Marshal(P6Capabilities()); return p6Digest(raw) }
