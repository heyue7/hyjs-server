package service

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type awake85Chapter struct {
	copyID, chapter, monster string
	firstBox                 int
}

func awake85Chapters() []awake85Chapter {
	return []awake85Chapter{
		{"102", "1", "mon_024", 17}, {"202", "2", "mon_010", 13},
		{"302", "3", "mon_030", 9}, {"402", "4", "mon_016", 5}, {"502", "5", "mon_006", 1},
	}
}

func awake85Definition(copyID string) (awake85Chapter, bool) {
	for _, c := range awake85Chapters() {
		if c.copyID == copyID {
			return c, true
		}
	}
	return awake85Chapter{}, false
}

// The five first difficulties reuse the audited exploration budget and original
// element reward expressions. Their node placement is an explicit local layout.
func awake85Catalogue(c awake85Chapter, enabled bool) (db.P7AwakeStageRule, error) {
	content := p7AwakeContent{Source: constUtil.Awake85ExploreSource}
	positions := []string{"14,0,14", "-14,0,14", "14,0,-14", "-14,0,-14"}
	for i, pos := range positions {
		content.Boxes = append(content.Boxes, &pb.AwakeCopyBox{BoxId: proto.String(fmt.Sprintf("box%d", c.firstBox+i)), Guid: proto.String(fmt.Sprintf("awake85-template:b%d", i+1)), Pos: proto.String(pos), IsOpen: proto.Int32(0), Angle: proto.Float64(0)})
	}
	content.Monsters = []*pb.AwakeCopyMonster{{MonId: proto.String(c.monster), Guid: proto.String("awake85-template:m1"), Pos: proto.String("0,0,0"), HpPercent: proto.Float64(1), Angle: proto.Float64(0), MonsterType: proto.Int32(1)}}
	raw, err := json.Marshal(content)
	return db.P7AwakeStageRule{CopyID: c.copyID, ChapterID: c.chapter, Enabled: enabled, ContentRestored: true, FightPower: 100, Content: raw, Source: constUtil.Awake85ExploreSource}, err
}

func p7AwakeExpectedCatalogue85(copyID string) (db.P7AwakeStageRule, error) {
	if c, ok := awake85Definition(copyID); ok {
		return awake85Catalogue(c, true)
	}
	if copyID == constUtil.P7AwakeExploreCopy {
		return p7AwakeLocalCatalogue(true)
	}
	return db.P7AwakeStageRule{}, errors.New(constUtil.P7AwakeClosed)
}

func p7AwakeExploreSourceKnown85(source string) bool {
	return source == constUtil.P7AwakeExploreSource || source == constUtil.Awake85ExploreSource
}
func p7AwakeBattleSourceKnown85(source string) bool {
	return source == constUtil.P7AwakeBattleSource || source == constUtil.Awake85BattleSource
}

func p7AwakeRuleEqual85(a, b db.P7AwakeStageRule) bool {
	return a.CopyID == b.CopyID && a.ChapterID == b.ChapterID && a.Enabled == b.Enabled && a.ContentRestored == b.ContentRestored && a.FightPower == b.FightPower && a.Source == b.Source && bytes.Equal(a.Content, b.Content)
}

func p7AwakeFrozenSource85(run *db.P7AwakeRun, f *p7AwakeFrozen) bool {
	if f.Rule.Source == constUtil.P7AwakeExploreSource {
		return f.Rule.CopyID == constUtil.P7AwakeExploreCopy && f.Content.Source == constUtil.P7AwakeExploreSource
	}
	c, ok := awake85Definition(f.Rule.CopyID)
	if !ok || run.CopyID != f.Rule.CopyID || f.Rule.Source != constUtil.Awake85ExploreSource || f.Content.Source != constUtil.Awake85ExploreSource || f.Rule.ChapterID != c.chapter || !strings.HasPrefix(run.ID, constUtil.Awake85RunPrefix) || len(f.Content.Boxes) != 4 || len(f.Content.Monsters) != 1 || len(f.MonsterSources85) != 1 || len(f.MonsterSourceHashes85) != 1 {
		return false
	}
	expected, err := awake85Catalogue(c, true)
	if err != nil {
		return false
	}
	actualRule := f.Rule
	actualRule.Content = f.RuleContent85
	if !p7AwakeRuleEqual85(actualRule, expected) {
		return false
	}
	var content p7AwakeContent
	if json.Unmarshal(expected.Content, &content) != nil {
		return false
	}
	for i, box := range content.Boxes {
		box.Guid = proto.String(fmt.Sprintf("%s:b%d", run.ID, i+1))
	}
	content.Monsters[0].Guid = proto.String(run.ID + ":m1")
	if !reflect.DeepEqual(content, f.Content) {
		return false
	}
	if len(f.BoxLoot) != len(content.Boxes) || len(f.MonsterLoot) != len(content.Monsters) {
		return false
	}
	for _, box := range content.Boxes {
		if _, ok := f.BoxLoot[box.GetGuid()]; !ok {
			return false
		}
	}
	for _, mon := range content.Monsters {
		if _, ok := f.MonsterLoot[mon.GetGuid()]; !ok {
			return false
		}
	}

	// These map bindings are the original AwakeCopyChapter rows used by the
	// explicitly named v85 catalogue; old accepted 501 instances stay frozen.
	scenes := map[string][4]string{
		"1": {"map_nightforestfield_001", "map_nightforest_001", "36,1,56", "0"},
		"2": {"map_snowfield_001", "map_ice_001", "59,15,77", "0"},
		"3": {"map_volcanofield_001", "map_volcano_001", "93,5,85", "0"},
		"4": {"map_desertfield_001", "map_desert_001", "-7,0,-6", "0"},
		"5": {"map_forestfield_001", "map_forest_003", "0,0,1", "0"},
	}
	raw := f.MonsterSources85[run.ID+":m1"]
	if len(raw) == 0 || f.MonsterSourceHashes85[run.ID+":m1"] != p6Digest(raw) {
		return false
	}
	metadata := scenes[c.chapter]
	angle, err := strconv.ParseInt(metadata[3], 10, 32)
	return err == nil && f.Scene == metadata[0] && f.BattleMap == metadata[1] && f.BornPos == metadata[2] && f.BornAngle == int32(angle) && f.BattleSource == constUtil.Awake85BattleSource

}

// Choose one complete original encounter containing the visible monster. The
// earliest original enemy row is stable across calls; its allies are retained.
func (s *GameService) p7Awake85Enemy(monID string) (*pb.BattleMonster, error) {
	outside := s.table("AwakeCopyMonsterOutside", "monId", monID)
	if outside == nil {
		return nil, errors.New("觉醒怪型缺失")
	}
	var candidates []map[string]string
	for _, r := range s.Tables["CopyEnemy"] {
		if r["HeroId"] == outside["heroId"] {
			candidates = append(candidates, r)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return tableNumber(candidates[i], "CopyEnemyId") < tableNumber(candidates[j], "CopyEnemyId")
	})
	if len(candidates) == 0 {
		return nil, errors.New("觉醒完整敌方来源缺失")
	}
	first := candidates[0]
	party := &pb.BattleMonster{MonsterId: proto.Int64(1000), ServerId: proto.Int32(1), TaskId: proto.String(first["TaskId"]), GroupIndex: proto.Int32(int32(tableNumber(first, "GroupIndex"))), Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
	var rows []map[string]string
	for _, r := range s.Tables["CopyEnemy"] {
		if r["TaskId"] == first["TaskId"] && r["GroupIndex"] == first["GroupIndex"] {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return tableNumber(rows[i], "CopyEnemyId") < tableNumber(rows[j], "CopyEnemyId") })
	for _, r := range rows {
		actor, err := s.actor(r, "monster-"+r["CopyEnemyId"], int32(tableNumber(r, "Location")), true)
		if err != nil {
			return nil, err
		}
		party.Formation = append(party.Formation, actor)
	}
	if len(party.Formation) == 0 || len(party.Formation) > 15 {
		return nil, errors.New("觉醒完整敌方编队无效")
	}
	return party, nil
}

func p7AwakeFrozenEnemy85(f *p7AwakeFrozen, mon *pb.AwakeCopyMonster) (*pb.BattleMonster, error) {
	var raw []byte
	if f.Rule.Source == constUtil.Awake85ExploreSource {
		raw = f.MonsterSources85[mon.GetGuid()]
		if len(raw) == 0 || f.MonsterSourceHashes85[mon.GetGuid()] != p6Digest(raw) {
			return nil, errors.New("觉醒冻结敌方摘要不符")
		}
	} else {
		var err error
		raw, err = hex.DecodeString(constUtil.P7AwakeEnemyHex)
		if err != nil || p6Digest(raw) != constUtil.P7AwakeEnemySHA {
			return nil, errors.New("原觉醒敌方来源无效")
		}
	}
	enemy := &pb.BattleMonster{}
	if len(raw) == 0 || proto.Unmarshal(raw, enemy) != nil || len(enemy.Formation) == 0 || enemy.Inst == nil {
		return nil, errors.New("觉醒冻结敌方无效")
	}
	return enemy, nil
}

func p7AwakeBoxKnown85(id string) bool {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "box"))
	return err == nil && n >= 1 && n <= 20 && id == fmt.Sprintf("box%d", n)
}

// Bracketed outer weights choose one original resource branch; the selected
// resource's quantity weights are then evaluated by the existing inner parser.
func p7AwakeBoxGroup85(group string, draw func(int64) (int64, error)) (string, error) {
	if !strings.ContainsAny(group, "[]") {
		return group, nil
	}
	type branch struct {
		weight int64
		text   string
	}
	var choices []branch
	total := int64(0)
	for group != "" {
		begin := strings.IndexByte(group, '[')
		end := strings.IndexByte(group, ']')
		if begin <= 0 || end <= begin+1 {
			return "", errors.New("觉醒箱奖分支无效")
		}
		text := group[:begin]
		if strings.ContainsAny(text, "[]") {
			return "", errors.New("觉醒箱奖资源分支无效")
		}
		weight, err := strconv.ParseInt(group[begin+1:end], 10, 64)
		if err != nil || weight <= 0 || weight > 1000 || total > 1000-weight {
			return "", errors.New("觉醒箱奖分支权重无效")
		}
		choices = append(choices, branch{weight, text})
		total += weight
		group = group[end+1:]
	}
	if total != 1000 {
		return "", errors.New("觉醒箱奖分支总权重无效")
	}
	value, err := draw(total)
	if err != nil {
		return "", err
	}
	if value < 0 || value >= total {
		return "", errors.New("觉醒箱奖随机范围无效")
	}
	for _, b := range choices {
		if value < b.weight {
			return b.text, nil
		}
		value -= b.weight
	}
	return "", errors.New("觉醒箱奖分支缺失")
}

func p7Awake85Preview(c awake85Chapter) string {
	material := map[string]string{"1": "item_322", "2": "item_324", "3": "item_323", "4": "item_321", "5": "item_320"}[c.chapter]
	return strings.Join([]string{material, "item_312", "item_313", "coin", "hero_exp_002"}, ";")
}
