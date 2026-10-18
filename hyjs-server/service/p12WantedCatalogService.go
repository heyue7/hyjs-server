package service

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p12WantedCatalogIDs(def p12WantedDefinition) ([]string, error) {
	ids := []string{}
	seen, slots := map[string]bool{}, map[string]bool{}
	for _, row := range def.Tables["WantedLevel"] {
		id, err := strconv.Atoi(row["WantedID"])
		level, e2 := strconv.Atoi(row["Level"])
		difficulty, e3 := strconv.Atoi(row["Difficulty"])
		key := row["Level"] + "/" + row["Difficulty"]
		if err != nil || id <= 0 || strconv.Itoa(id) != row["WantedID"] || e2 != nil || level < 1 || e3 != nil || difficulty < 1 || difficulty > 3 || row["FightRoomId"] == "" || seen[row["WantedID"]] || slots[key] {
			return nil, errors.New(constUtil.P12WantedInvalid)
		}
		seen[row["WantedID"]], slots[key] = true, true
		ids = append(ids, row["WantedID"])
	}
	if len(ids) != 347 {
		return nil, errors.New(constUtil.P12WantedInvalid)
	}
	sort.Slice(ids, func(i, j int) bool { a, _ := strconv.Atoi(ids[i]); b, _ := strconv.Atoi(ids[j]); return a < b })
	return ids, nil
}

func p12WantedDefinitionSelectionValid(def p12WantedDefinition) bool {
	if def.SelectionRule == "" {
		return len(def.SupportedWantedIDs) == 1 && def.SupportedWantedIDs[0] == "1"
	}
	if def.SelectionRule != constUtil.P12WantedCatalogSelectionRule {
		return false
	}
	ids, err := p12WantedCatalogIDs(def)
	if err != nil || len(ids) != len(def.SupportedWantedIDs) {
		return false
	}
	for i, id := range ids {
		if id != def.SupportedWantedIDs[i] {
			return false
		}
	}
	_, err = p12WantedCatalogWeights(def)
	return err == nil
}

func p12WantedCatalogWeights(def p12WantedDefinition) (map[int]int64, error) {
	weights := map[int]int64{}
	for _, row := range def.Tables["WantedTimeLimit"] {
		difficulty, err := strconv.Atoi(row["Difficulty"])
		if err != nil {
			return nil, errors.New(constUtil.P12WantedInvalid)
		}
		if difficulty < 1 || difficulty > 3 {
			continue
		}
		weight, err := strconv.ParseInt(row["Odd"], 10, 64)
		if err != nil || weight != map[int]int64{1: 50, 2: 22, 3: 22}[difficulty] || weights[difficulty] != 0 {
			return nil, errors.New(constUtil.P12WantedInvalid)
		}
		weights[difficulty] = weight
	}
	if len(weights) != 3 {
		return nil, errors.New(constUtil.P12WantedInvalid)
	}
	return weights, nil
}

// User-approved temporary choice: same level, otherwise nearest lower level;
// difficulty1/2/3 uses original50:22:22 among rows present at that chosen level.
// One crypto draw is committed with the resulting original instance, never on retry.
func p12WantedChooseCatalog(def p12WantedDefinition, playerLevel int32, draw func(int64) (int64, error)) (string, error) {
	if def.SelectionRule != constUtil.P12WantedCatalogSelectionRule || playerLevel < 1 || draw == nil {
		return "", errors.New(constUtil.P12WantedInvalid)
	}
	weights, err := p12WantedCatalogWeights(def)
	if err != nil {
		return "", err
	}
	allowed := map[string]bool{}
	for _, id := range def.SupportedWantedIDs {
		allowed[id] = true
	}
	level := 0
	byDifficulty := map[int]string{}
	for _, row := range def.Tables["WantedLevel"] {
		candidate, e1 := strconv.Atoi(row["Level"])
		difficulty, e2 := strconv.Atoi(row["Difficulty"])
		if e1 != nil || e2 != nil {
			return "", errors.New(constUtil.P12WantedInvalid)
		}
		if !allowed[row["WantedID"]] || candidate > int(playerLevel) || candidate < level || difficulty < 1 || difficulty > 3 {
			continue
		}
		if candidate > level {
			level = candidate
			byDifficulty = map[int]string{}
		}
		if byDifficulty[difficulty] != "" {
			return "", errors.New(constUtil.P12WantedInvalid)
		}
		byDifficulty[difficulty] = row["WantedID"]
	}
	if level == 0 {
		return "", errors.New(constUtil.P12WantedUnavailable)
	}
	total := int64(0)
	for difficulty := 1; difficulty <= 3; difficulty++ {
		if byDifficulty[difficulty] != "" {
			total += weights[difficulty]
		}
	}
	value, err := draw(total)
	if err != nil {
		return "", err
	}
	if value < 0 || value >= total {
		return "", errors.New(constUtil.P12WantedInvalid)
	}
	for difficulty := 1; difficulty <= 3; difficulty++ {
		if id := byDifficulty[difficulty]; id != "" {
			value -= weights[difficulty]
			if value < 0 {
				return id, nil
			}
		}
	}
	return "", errors.New(constUtil.P12WantedInvalid)
}

func p12WantedRandomCatalog(def p12WantedDefinition, level int32) (string, error) {
	return p12WantedChooseCatalog(def, level, func(total int64) (int64, error) {
		n, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return 0, err
		}
		return n.Int64(), nil
	})
}

// Only the exact former default definition is upgraded. Enabled, customized
// policies, all instances, discoveries, pending battles and receipts are retained.
func p12WantedDefaultCatalogUpgrade(old *dbWantedCatalogPolicyView, next p12WantedDefinition) (bool, error) {
	if old == nil || old.Source != constUtil.P12WantedLocalSource || old.Hash != p6Digest([]byte(old.JSON)) {
		return false, nil
	}
	legacy := next
	legacy.SelectionRule = ""
	legacy.SupportedWantedIDs = []string{"1"}
	raw, err := json.Marshal(legacy)
	if err != nil {
		return false, err
	}
	return p6Digest(raw) == old.Hash, nil
}

// Small immutable view avoids adding any persistence schema to this update.
type dbWantedCatalogPolicyView struct{ Source, Hash, JSON string }

func p12WantedCatalogMarker(input *model.FrozenBattleInput) bool {
	if input == nil || input.BattleType != 8 {
		return false
	}
	_, found := input.AppliedProperties[constUtil.P12WantedCatalogScope]
	return found
}

func p12MechanismIdentityForInput(input *model.FrozenBattleInput) (string, string) {
	if p12WantedCatalogMarker(input) {
		return constUtil.P12WantedCatalogMechanismProof, "local-original-wanted8-catalog347-auto-v2"
	}
	if input == nil {
		return "", ""
	}
	return p12MechanismIdentity(input.BattleType)
}

// Every original field comes from this frozen WantedEnemy row; only current HP
// comes from the shared instance. Order, GUID, skills, talents and panels stay exact.
func validateP12WantedCatalogNPCs(input *model.FrozenBattleInput) error {
	marker := input.AppliedProperties[constUtil.P12WantedCatalogScope]
	if len(marker) != 2 || marker[0] != constUtil.P12WantedCatalogSelectionRule || marker[1] == "" {
		return errors.New(constUtil.CombatUnsupported)
	}
	expected := []*pb.BattleHero{}
	s := &GameService{Tables: input.Tables}
	for _, row := range input.Tables["WantedEnemy"] {
		if row["WantedID"] != marker[1] {
			continue
		}
		pose, err := strconv.ParseInt(row["Location"], 10, 32)
		if err != nil || pose < 1 || pose > 15 || row["GroupIndex"] != "1" {
			return errors.New(constUtil.CombatUnsupported)
		}
		actor, err := s.actor(row, "wanted-"+strings.TrimPrefix(row["WantedEnemyId"], "wanted_"), int32(pose), true)
		if err != nil {
			return err
		}
		expected = append(expected, actor)
	}
	if len(expected) != 4 || input.Monster == nil || len(input.Monster.Formation) != len(expected) {
		return fmt.Errorf("%s: original WantedID %s complete four members", constUtil.CombatUnsupported, marker[1])
	}
	living := false
	for index, source := range input.Monster.Formation {
		if source == nil || source.Prop == nil || source.Inst == nil || !combatFinite(source.Inst.GetCurHp()) || source.Inst.GetCurHp() < 0 || source.Inst.GetCurHp() > expected[index].Prop.GetHp() {
			return errors.New(constUtil.CombatUnsupported)
		}
		living = living || source.Inst.GetCurHp() > 0
		copy := proto.Clone(source).(*pb.BattleHero)
		copy.Inst.CurHp = proto.Float64(expected[index].Prop.GetHp())
		if !proto.Equal(copy, expected[index]) {
			return fmt.Errorf("%s: WantedID %s original member %d", constUtil.CombatUnsupported, marker[1], index)
		}
	}
	if !living {
		return errors.New(constUtil.CombatUnsupported)
	}
	return nil
}
