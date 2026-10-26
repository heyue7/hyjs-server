package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strings"
)

// validateP6UFGuardCandidate validates the current exported input family only.
// It grants no authority and deliberately excludes future wounded/AP continuations.
func validateP6UFGuardCandidate(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil || input.Attacker == nil || input.Monster == nil || input.Defender != nil || input.BattleType != 14 || input.IsAttToDef || input.DefenderFirst || input.MaxRound != 40 || input.Seed < 0 || input.EngineVersion != constUtil.P6EngineVersion || input.ClientVersion != constUtil.P6OriginalClientHash || input.KFReplay != nil || input.KFReplayViewer != nil || input.ConfigurationHash != constUtil.P6LinearPhysicalTables || len(input.WinConditions) != 0 || input.DiagnosticAIDriverPolicy != "" || input.PVECompatibilityRule != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return reject()
	}
	if input.AttackerDamageFactor == nil || input.DefenderDamageFactor == nil || *input.AttackerDamageFactor != 1 || *input.DefenderDamageFactor != 1 || input.Attacker.Inst == nil || input.Monster.Inst == nil || input.Attacker.Inst.GetAnger() != 0 || input.Monster.Inst.GetAnger() != 0 || len(input.Attacker.Formation) != 1 || len(input.Monster.Formation) != 1 || len(input.Attacker.Inst.ExtBattleAdds) != 0 {
		return reject()
	}
	guardIDs := map[string]string{"tower_guard_001": "guard01_npc", "tower_guard_002": "guard01_npc", "tower_guard_003": "monster_016", "tower_guard_004": "monster_016", "tower_guard_005": "guard01_npc", "tower_guard_006": "monster_016", "factory_guard_001": "monster_016", "factory_guard_002": "guard01_npc", "baseCamp_guard_001": "hero_002", "baseCamp_guard_002": "hero_013"}
	a, g := input.Attacker.Formation[0], input.Monster.Formation[0]
	if a == nil || g == nil || a.Prop == nil || g.Prop == nil || a.Inst == nil || g.Inst == nil || a.GetId() != "hero_047" || g.GetId() != guardIDs[input.Monster.GetTaskId()] || !strings.HasPrefix(g.GetGuid(), "guard/") || a.GetPose() != 3 || g.GetPose() != 3 || a.GetBloodNumber() != 1 || g.GetBloodNumber() != 1 || a.GetInst().GetCurHp() != a.GetProp().GetHp() || a.GetProp().GetHp() != 2054 || g.GetInst().GetCurHp() != 1000 || g.GetProp().GetHp() != 1000 {
		return reject()
	}
	if err := validateP6UFGuardPhoneInput(input); err != nil {
		return err
	}
	return validateFrozenP6(input)
}

// validateP6UFGuardPhoneInput registers only the ten unchanged phone inputs.
// Full party protobufs bind identities, every stat/skill/talent and initial HP/AP;
// seed and canonical tables are bound separately. This is evidence registration,
// not a playable mechanism domain or execution of the typed UF callback.
func validateP6UFGuardPhoneInput(input *model.FrozenBattleInput) error {
	type sample struct {
		seed              int64
		attacker, monster string
	}
	pins := map[string]sample{
		"baseCamp_guard_001": {2127764594, "95299e933502b6f7affaf6dfbb1ca5b56315fa88c9e6c46deaf9141cd9febaf4", "e1b751a47b47180a0000ee6ec27a5e5793dac5c39968ad8b83ddf2d4947e0b38"},
		"baseCamp_guard_002": {696284978, "343b02193cfa4b6d9ba4d2c89a6809a2681cc62d22b7288192c776da3269b97b", "38fad18d8ce675e031c1c9fd1d22d5564799af8f54bc39b793c2208853314846"},
		"factory_guard_001":  {1878063167, "343b02193cfa4b6d9ba4d2c89a6809a2681cc62d22b7288192c776da3269b97b", "004038ec784add22bd0ac26fcd0c62f5b73878c0d65af2832d50047e37221a96"},
		"factory_guard_002":  {1256795199, "95299e933502b6f7affaf6dfbb1ca5b56315fa88c9e6c46deaf9141cd9febaf4", "dfc43e0674c0ab8d82bd9db312faf9af99ed740bdfced8ec0a6c3439d1d36264"},
		"tower_guard_001":    {1161180788, "95299e933502b6f7affaf6dfbb1ca5b56315fa88c9e6c46deaf9141cd9febaf4", "ff45e36a9cd76bb34f800f3b522307de71a9f2dbab080a166675dccd601ab5f1"},
		"tower_guard_002":    {1871104161, "95299e933502b6f7affaf6dfbb1ca5b56315fa88c9e6c46deaf9141cd9febaf4", "129255b835b77628e30d08007acfe4504a962e56c5f539b9db276544f427d3c0"},
		"tower_guard_003":    {2000386153, "343b02193cfa4b6d9ba4d2c89a6809a2681cc62d22b7288192c776da3269b97b", "3482e458feb17ddc34bffeeb858302a19432a59d1bd1899461174ad65a656fa9"},
		"tower_guard_004":    {2141140117, "343b02193cfa4b6d9ba4d2c89a6809a2681cc62d22b7288192c776da3269b97b", "efb7eff1bb68ae568b8c3550fb3158fe349760eebe7eeb8c9a5269edac5bcc12"},
		"tower_guard_005":    {1302635446, "95299e933502b6f7affaf6dfbb1ca5b56315fa88c9e6c46deaf9141cd9febaf4", "168ab988447420d5e7e693733a3428c6064e55b6d45428bc2101d2266ad56052"},
		"tower_guard_006":    {1444176619, "343b02193cfa4b6d9ba4d2c89a6809a2681cc62d22b7288192c776da3269b97b", "46252e1ff2ce586137e09c58a1b3b388a064f1bb435486d97e2cd2c1599a72bf"},
	}
	tableRaw, tableErr := json.Marshal(input.Tables)
	if tableErr != nil || p6Digest(tableRaw) != constUtil.P6LinearPhysicalTables {
		return errors.New(constUtil.CombatUnsupported)
	}
	pin, ok := pins[input.Monster.GetTaskId()]
	attacker, errA := proto.MarshalOptions{Deterministic: true}.Marshal(input.Attacker)
	monster, errM := proto.MarshalOptions{Deterministic: true}.Marshal(input.Monster)
	if !ok || input.Seed != pin.seed || errA != nil || errM != nil || p6Digest(attacker) != pin.attacker || p6Digest(monster) != pin.monster {
		return errors.New(constUtil.CombatUnsupported)
	}
	return nil
}

// ComputeP6UFGuardSettlement requires the independent UF14 native certificate.
// Capture additionally binds this mechanism domain to current producer frames.
func (s *GameService) ComputeP6UFGuardSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := validateP6UFGuardMechanismScope(input); err != nil {
		return nil, err
	}
	if constUtil.P6UFGuardNativeProof == "" {
		return nil, errors.New(constUtil.P13UFGuardProofMissing)
	}
	out, _, err := s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6UFGuardTerminalValue(input, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P6UFGuardNativeProof
	return out, nil
}

func validateP6UFGuardTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if constUtil.P6UFGuardNativeProof == "" || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P6UFGuardNativeProof {
		return errors.New(constUtil.P13UFGuardProofMissing)
	}
	return validateP6UFGuardTerminalValue(input, out)
}

// Check immutable initial identities, all formation HP/AP and whole outcome
// protobufs. The engine terminal stays distinct from NPC failure reset policy.
func validateP6UFGuardTerminalValue(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.P13UFGuardConflict) }
	if err := validateP6UFGuardMechanismScope(input); err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.Outcome.Winner < 0 || out.Outcome.Winner > 1 || out.Outcome.Attacker == nil || out.Outcome.Monster == nil || out.Outcome.Defender != nil {
		return reject()
	}
	closure := *input
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return reject()
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return err
	}
	attacker := proto.Clone(input.Attacker).(*pb.BattlePlayer)
	monster := proto.Clone(input.Monster).(*pb.BattleMonster)
	attacker.Inst.Anger = proto.Float64(out.Outcome.Attacker.GetInst().GetAnger())
	monster.Inst.Anger = proto.Float64(out.Outcome.Monster.GetInst().GetAnger())
	parties := [][]*pb.BattleHero{attacker.Formation, monster.Formation}
	for _, actor := range out.FinalActors {
		if actor.Camp < 0 || actor.Camp > 1 {
			return reject()
		}
		found := false
		for _, hero := range parties[actor.Camp] {
			if hero.GetGuid() == actor.GUID {
				if actor.BloodNumber != hero.GetBloodNumber() || actor.MaxHP != hero.Prop.GetHp() {
					return reject()
				}
				hero.Inst.CurHp = proto.Float64(actor.CurrentHP)
				found = true
				break
			}
		}
		if !found {
			return reject()
		}
	}
	if !proto.Equal(attacker, out.Outcome.Attacker) || !proto.Equal(monster, out.Outcome.Monster) {
		return reject()
	}
	for _, ap := range []float64{attacker.Inst.GetAnger(), monster.Inst.GetAnger()} {
		if !combatFinite(ap) || ap < 0 || ap > 300 {
			return reject()
		}
	}
	return nil
}

// validateP6UFGuardMechanismScope admits the original UF14 driver and complete
// current-life parties. Producer identity and published NPC templates are bound
// separately by ValidateP6UFGuardMechanisms inside the capture/verification TX.
func validateP6UFGuardMechanismScope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil || input.Attacker == nil || input.Monster == nil || input.Defender != nil || input.BattleType != 14 || input.MaxRound != 40 || input.Seed < 0 || input.Seed > math.MaxInt32 || input.IsAttToDef || input.DefenderFirst || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.ConfigurationHash != constUtil.P6LinearPhysicalTables || input.EngineVersion != constUtil.P6EngineVersion || input.ClientVersion != constUtil.P6OriginalClientHash || input.AttackerDamageFactor == nil || input.DefenderDamageFactor == nil || *input.AttackerDamageFactor != 1 || *input.DefenderDamageFactor != 1 {
		return reject()
	}
	if input.Attacker.GetPlayerId() <= 0 || input.Attacker.Inst == nil || input.Monster.Inst == nil || len(input.Attacker.Formation) < 1 || len(input.Attacker.Formation) > 4 || len(input.Monster.Formation) != 1 || len(input.Attacker.Inst.ExtBattleAdds) != 0 || p6ScopedUnknown(input.Attacker) || p6ScopedUnknown(input.Monster) || p6ScopedUnknown(input.Attacker.Inst) || p6ScopedUnknown(input.Monster.Inst) {
		return reject()
	}
	ids := map[string]bool{"tower_guard_001": true, "tower_guard_002": true, "tower_guard_003": true, "tower_guard_004": true, "tower_guard_005": true, "tower_guard_006": true, "factory_guard_001": true, "factory_guard_002": true, "baseCamp_guard_001": true, "baseCamp_guard_002": true}
	if !ids[input.Monster.GetTaskId()] || input.Monster.GetMonsterId() <= 0 {
		return reject()
	}
	raw, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(raw) != constUtil.P6LinearPhysicalTables {
		return reject()
	}
	for _, ap := range []float64{input.Attacker.Inst.GetAnger(), input.Monster.Inst.GetAnger()} {
		if !combatFinite(ap) || ap < 0 || ap > 300 {
			return reject()
		}
	}
	seenGUID, seenPose := map[string]bool{}, map[int32]bool{}
	alive := false
	for _, actor := range input.Attacker.Formation {
		if actor == nil || actor.Prop == nil || actor.Inst == nil || !strings.HasPrefix(actor.GetId(), "hero_") || actor.GetBloodNumber() != 1 || actor.GetGuid() == "" || len(actor.GetGuid()) > 128 || strings.Contains(actor.GetGuid(), "_") || seenGUID[actor.GetGuid()] || seenPose[actor.GetPose()] || p6ScopedUnknown(actor) || p6ScopedUnknown(actor.Prop) || p6ScopedUnknown(actor.Inst) || !combatFinite(actor.Inst.GetCurHp()) || actor.Inst.GetCurHp() < 0 || actor.Inst.GetCurHp() > actor.Prop.GetHp() {
			return reject()
		}
		seenGUID[actor.GetGuid()] = true
		seenPose[actor.GetPose()] = true
		alive = alive || actor.Inst.GetCurHp() > 0
		var hero map[string]string
		for _, row := range input.Tables["Hero"] {
			if row["id"] == actor.GetId() {
				hero = row
				break
			}
		}
		if hero == nil || hero["heroClass"] == "99" {
			return reject()
		}
		normal := strings.Join([]string{hero["SkillId1"], hero["SkillId2"], hero["UltSkillId"]}, ",")
		swapped := strings.Join([]string{hero["SkillId2"], hero["SkillId1"], hero["UltSkillId"]}, ",")
		if actor.GetSkillOrder() != normal && actor.GetSkillOrder() != swapped {
			return reject()
		}
	}
	guard := input.Monster.Formation[0]
	if !alive || guard == nil || guard.Prop == nil || guard.Inst == nil || !strings.HasPrefix(guard.GetGuid(), "guard/") || guard.GetBloodNumber() != 1 || !combatFinite(guard.Inst.GetCurHp()) || guard.Inst.GetCurHp() <= 0 || guard.Inst.GetCurHp() > guard.Prop.GetHp() {
		return reject()
	}
	if err := validateFrozenP6(input); err != nil {
		return err
	}
	return ValidateP6FirstOrder(input)
}

// Qualification covers mechanisms and exact producer frames; it does not add
// native authority to an audit result. Current identities never need new pins.
func (s *GameService) ValidateP6UFGuardMechanisms(ctx context.Context, input *model.FrozenBattleInput, playerID int64, player *pb.InstUFPlayer, guard *pb.InstUFGuard, offer *db.P13UnionFightGuardAuditOffer) (*model.P6MechanismQualification, error) {
	reject := func() (*model.P6MechanismQualification, error) { return nil, errors.New(constUtil.CombatUnsupported) }
	if err := validateP6UFGuardMechanismScope(input); err != nil {
		return nil, err
	}
	if offer != nil && offer.GuardRule == constUtil.P13UFGuardMirrorRule {
		if err := validateP6UFGuardMirrorScope(input); err != nil {
			return nil, err
		}
	}
	if player == nil || guard == nil || offer == nil || offer.SourceHash != p13UFGuardOfferHash(offer) || playerID <= 0 || player.GetGuid() != playerID || input.Attacker.GetPlayerId() != playerID || input.Monster.GetMonsterId() != guard.GetGuid() {
		return reject()
	}
	expected, err := p13UFGuardAuditMonster(offer, guard)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(expected, input.Monster) {
		return reject()
	}
	original := map[string]string{"tower_guard_001": "guard01_npc", "tower_guard_002": "guard01_npc", "tower_guard_003": "monster_016", "tower_guard_004": "monster_016", "tower_guard_005": "guard01_npc", "tower_guard_006": "monster_016", "factory_guard_001": "monster_016", "factory_guard_002": "guard01_npc", "baseCamp_guard_001": "hero_002", "baseCamp_guard_002": "hero_013"}
	expectedID, err := p13UFGuardTemplateHeroID(offer.GuardID, original[offer.GuardID], offer.GuardRule)
	if err != nil || expectedID == "" || input.Monster.Formation[0].GetId() != expectedID {
		return reject()
	}
	fight := player.FightInfo
	if fight == nil || fight.GetGuid() != playerID || len(fight.Index) != len(input.Attacker.Formation) || len(fight.HpPos) != len(fight.Index) || len(fight.MaxHpPos) != len(fight.Index) || fight.GetAnger() != input.Attacker.Inst.GetAnger() {
		return reject()
	}
	poses := map[int32]int{}
	for index, pose := range fight.Index {
		if _, exists := poses[pose]; exists {
			return reject()
		}
		poses[pose] = index
	}
	for _, actor := range input.Attacker.Formation {
		index, ok := poses[actor.GetPose()]
		if !ok || fight.HpPos[index] != actor.Inst.GetCurHp() || fight.MaxHpPos[index] != actor.Prop.GetHp() {
			return reject()
		}
	}
	return s.ValidateP6GenericMechanisms(ctx, input)
}

// Rule selection never falls back to another domain's native certificate.
func p6UFGuardProofForRule(rule string) (string, error) {
	var proof string
	switch rule {
	case constUtil.P13UFGuardMirrorRule:
		proof = constUtil.P6UFGuardMirrorNativeProof
	case "", constUtil.P13UFGuardLocalRule:
		proof = constUtil.P6UFGuardNativeProof
	default:
		return "", errors.New(constUtil.CombatUnsupported)
	}
	if proof == "" {
		return "", errors.New(constUtil.P13UFGuardProofMissing)
	}
	return proof, nil
}

// Only the complete original mirror actor is in the new certificate's domain.
// Actual entity IDs vary per match; every other protobuf field stays exact.
func validateP6UFGuardMirrorScope(input *model.FrozenBattleInput) error {
	if err := validateP6UFGuardMechanismScope(input); err != nil {
		return err
	}
	expected, err := p6UFGuardMirrorTemplate(input)
	if err != nil {
		return err
	}
	expected.MonsterId = input.Monster.MonsterId
	expected.Formation[0].Guid = input.Monster.Formation[0].Guid
	if !proto.Equal(expected, input.Monster) {
		return errors.New(constUtil.CombatUnsupported)
	}
	return nil
}

// Canonical combat tables omit the field-only UnionFightGuard directory.
// Supply only its known identity dispatch row to the existing template builder;
// every actor stat/skill still comes from the unchanged canonical input tables.
func p6UFGuardMirrorTemplate(input *model.FrozenBattleInput) (*pb.BattleMonster, error) {
	if input == nil || input.Monster == nil {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	id := input.Monster.GetTaskId()
	if _, err := p13UFGuardTemplateHeroID(id, "", constUtil.P13UFGuardMirrorRule); err != nil {
		return nil, err
	}
	tables := make(map[string][]map[string]string, len(input.Tables)+1)
	for name, rows := range input.Tables {
		tables[name] = rows
	}
	tables["UnionFightGuard"] = []map[string]string{{"UFGuardId": id}}
	original := &GameService{Tables: tables, ServerID: int64(input.Monster.GetServerId())}
	return original.P13UnionFightMirrorGuardTemplate(id)
}

// ComputeP6UFGuardMirrorSettlement is closed until the independent mirror
// native evidence is approved; full player and NPC mechanisms remain required.
func (s *GameService) ComputeP6UFGuardMirrorSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := validateP6UFGuardMirrorScope(input); err != nil {
		return nil, err
	}
	proof, err := p6UFGuardProofForRule(constUtil.P13UFGuardMirrorRule)
	if err != nil {
		return nil, err
	}
	out, _, err := s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6UFGuardTerminalValue(input, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	return out, nil
}

func validateP6UFGuardMirrorTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	proof, err := p6UFGuardProofForRule(constUtil.P13UFGuardMirrorRule)
	if err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof {
		return errors.New(constUtil.P13UFGuardProofMissing)
	}
	if err = validateP6UFGuardMirrorScope(input); err != nil {
		return err
	}
	return validateP6UFGuardTerminalValue(input, out)
}

func p13UFGuardRulesProof(rules *p13UFGuardBattleRules) error {
	if rules == nil {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	proof, err := p6UFGuardProofForRule(rules.LocalRule)
	if err != nil {
		return err
	}
	if rules.Proof != proof {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	return nil
}

func validateP6UFGuardTerminalForRule(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome, rules *p13UFGuardBattleRules) error {
	if err := p13UFGuardRulesProof(rules); err != nil {
		return err
	}
	if rules.LocalRule == constUtil.P13UFGuardMirrorRule {
		return validateP6UFGuardMirrorTerminal(input, out)
	}
	return validateP6UFGuardTerminal(input, out)
}

func (s *GameService) computeP6UFGuardForRule(ctx context.Context, input *model.FrozenBattleInput, rules *p13UFGuardBattleRules) (*model.CombatFinalHPOutcome, error) {
	if err := p13UFGuardRulesProof(rules); err != nil {
		return nil, err
	}
	if rules.LocalRule == constUtil.P13UFGuardMirrorRule {
		return s.ComputeP6UFGuardMirrorSettlement(ctx, input)
	}
	return s.ComputeP6UFGuardSettlement(ctx, input)
}
