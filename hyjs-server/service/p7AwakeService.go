package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p7AwakeContent 仅接受显式来源的节点，禁止把模型表当地图生成表。
type p7AwakeContent struct {
	Boxes    []*pb.AwakeCopyBox     `json:"boxes"`
	Monsters []*pb.AwakeCopyMonster `json:"monsters"`
	Source   string                 `json:"source"`
}

type p7AwakeFrozen struct {
	BusinessDay       string              `json:"businessDay,omitempty"`
	BusinessAt        int64               `json:"businessAt,omitempty"`
	Rule              db.P7AwakeStageRule `json:"rule"`
	Content           p7AwakeContent      `json:"content"`
	Scene             string              `json:"scene"`
	BattleMap         string              `json:"battleMap"`
	BornPos           string              `json:"bornPos"`
	BornAngle         int32               `json:"bornAngle"`
	DurationMinutes   int32               `json:"durationMinutes"`
	Revives           int32               `json:"revives"`
	ReviveCost        int32               `json:"reviveCost"`
	StaminaCost       int32               `json:"staminaCost"`
	ActorHash         string              `json:"actorHash"`
	GuildPrayerSource string              `json:"guildPrayerSource,omitempty"`
	GuildTechSource   string              `json:"guildTechSource,omitempty"`
	BoxLoot           map[string]string   `json:"boxLoot,omitempty"`
	MonsterLoot       map[string]string   `json:"monsterLoot,omitempty"`
	BattleSource      string              `json:"battleSource,omitempty"`
	ScoreReward       string              `json:"scoreReward,omitempty"`
	FrameHash         string              `json:"frameHash"`
}

func (s *GameService) LoadP7AwakeTables(root string) error {
	for _, name := range []string{"AwakeCopyChapter", "AwakeCopyBox", "AwakeCopyMonsterOutside"} {
		if err := s.loadP7Table(root, name); err != nil {
			return err
		}
	}
	return nil
}
func (s *GameService) InitializeP7Awake(ctx context.Context) error {
	if !s.P1Ready || len(s.Tables["AwakeCopyChapter"]) != 5 || len(s.Tables["AwakeCopyBox"]) == 0 || len(s.Tables["AwakeCopyMonsterOutside"]) == 0 {
		return errors.New("觉醒之地原章节/箱型/怪型尚未初始化")
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P7AwakePolicy{}, &db.P7AwakeStageRule{}, &db.P7AwakeState{}, &db.P7AwakeRun{}, &db.P7AwakeExploration{}, &db.P7AwakeFight{}); err != nil {
		return err
	}
	cfg := db.P7AwakePolicy{ID: constUtil.P7AwakePolicyID, MinLevel: 70, Source: constUtil.P7AwakeSource}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
		return err
	}
	// 本地实例单独具名，旧禁用策略和诊断目录保持各自含义。
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P7AwakePolicy{ID: constUtil.P7AwakeExplorePolicyID, MinLevel: 70, Source: constUtil.P7AwakeExploreSource}).Error; err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P7AwakePolicy{ID: constUtil.P7AwakeBattlePolicyID, MinLevel: 70, Source: constUtil.P7AwakeBattleSource}).Error; err != nil {
		return err
	}
	local, err := p7AwakeLocalCatalogue(false)
	if err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&local).Error
}
func p7AwakeOpenDay(raw string, now time.Time) (bool, error) {
	day := int(now.In(time.FixedZone("UTC+8", 8*3600)).Weekday())
	if day == 0 {
		day = 7
	}
	open := false
	for _, token := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(token)
		if err != nil || n < 1 || n > 7 {
			return false, errors.New("原觉醒星期配置无效")
		}
		if n == day {
			open = true
		}
	}
	return open, nil
}
func (s *GameService) p7AwakeState(tx *gorm.DB, id int64, now time.Time) (*db.P7AwakeState, int32, error) {
	budget, err := s.p7ExpeditionSetting("awakeCopyPlayTimes")
	if err != nil {
		return nil, 0, err
	}
	state := &db.P7AwakeState{PlayerID: id, Day: p1Day(now)}
	err = tx.Where("player_id=?", id).First(state).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, 0, err
	}
	if state.Used < 0 {
		return nil, 0, errors.New("觉醒日次数存档无效")
	}
	if err = refreshP7AwakeCalendar(tx, state, now); err != nil {
		return nil, 0, err
	}
	return state, max(0, budget-state.Used), saveP7AwakeCalendar(tx, state, now)
}
func (s *GameService) p7AwakeCatalogue(ctx context.Context, id int64, now time.Time) ([]proto.Message, error) {
	var result *pb.SCAwakeCopyData
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, left, err := s.p7AwakeState(tx, id, now)
		if err != nil {
			return err
		}
		ledger, run, err := p7AwakeActive(tx, id)
		if err != nil {
			return err
		}
		autoSettled := int32(0)
		if ledger != nil && now.UnixMilli() >= run.EndAt {
			if _, err = s.p7AwakeSettle(tx, p, g, run, ledger, now); err != nil {
				return err
			}
			ledger = nil
			run = nil
			autoSettled = 1
		}
		var rules []db.P7AwakeStageRule
		if err = tx.Order("copy_id").Find(&rules).Error; err != nil {
			return err
		}
		var battlePolicy db.P7AwakePolicy
		if e := tx.First(&battlePolicy, constUtil.P7AwakeBattlePolicyID).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		battlePreview := battlePolicy.Enabled && battlePolicy.Source == constUtil.P7AwakeBattleSource
		if run != nil {
			frozen, e := p7AwakeReadFrozen(run)
			if e != nil {
				return e
			}
			battlePreview = frozen.BattleSource == constUtil.P7AwakeBattleSource
		}
		chapters := append([]map[string]string(nil), s.Tables["AwakeCopyChapter"]...)
		sort.Slice(chapters, func(i, j int) bool { return chapters[i]["chapterId"] < chapters[j]["chapterId"] })
		result = &pb.SCAwakeCopyData{LastNum: proto.Int32(left), IsAutoSettled: proto.Int32(autoSettled)}
		// 已支付实例即使随后关闭策略也按原冻结目录继续履约。
		if run != nil {
			frozen, e := p7AwakeReadFrozen(run)
			if e != nil {
				return e
			}
			found := false
			for i := range rules {
				if rules[i].CopyID == run.CopyID {
					rules[i] = frozen.Rule
					found = true
				}
			}
			if !found {
				rules = append(rules, frozen.Rule)
			}
		}
		for _, row := range chapters {
			open, err := p7AwakeOpenDay(row["openDay"], LocalPlayAt(tx.Statement.Context, now))
			if err != nil {
				return err
			}
			chapter := &pb.AwakeCopyChapter{ChapterId: proto.String(row["chapterId"]), OpenDay: proto.String(row["openDay"]), IsOpen: proto.Int32(0)}
			if open {
				chapter.IsOpen = proto.Int32(1)
			}
			for _, rule := range rules {
				// 原客户端会Int32.Parse(copyId)；兼容导入也必须保留原数字字符串。
				if rule.ChapterID != row["chapterId"] || !rule.ContentRestored || (rule.Source == constUtil.P7AwakeExploreSource && !rule.Enabled && (run == nil || run.CopyID != rule.CopyID)) {
					continue
				}
				if _, err = p7Integer(rule.CopyID, 1, math.MaxInt32); err != nil {
					return errors.New("觉醒难度编号必须为原数字编号")
				}
				task := &pb.AwakeCopyTask{CopyId: proto.String(rule.CopyID), FightPower: proto.Int64(rule.FightPower), IsRunning: proto.Int32(0)}
				// 原BaoxiangClick按分号拆资源ID，不能发送数量串或缺省null。
				if rule.Source == constUtil.P7AwakeExploreSource {
					preview := constUtil.P7AwakeExplorePreview
					if battlePreview {
						preview += constUtil.P7AwakeBattlePreview
					}
					task.DropReward = proto.String(preview)
				}
				if run != nil && run.CopyID == rule.CopyID {
					task.IsRunning = proto.Int32(1)
					task.EndTime = proto.Int64(run.EndAt)
				}
				chapter.SmallCopys = append(chapter.SmallCopys, task)
			}
			result.Copys = append(result.Copys, chapter)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}
func p7AwakePosition(raw string) bool {
	p := strings.Split(raw, ",")
	if len(p) != 3 {
		return false
	}
	for _, part := range p {
		n, err := strconv.ParseFloat(part, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
	}
	return true
}
func (s *GameService) p7AwakeFreeze(rule db.P7AwakeStageRule) (p7AwakeFrozen, error) {
	var f p7AwakeFrozen
	row := s.table("AwakeCopyChapter", "chapterId", rule.ChapterID)
	if row == nil || !rule.ContentRestored || rule.Source == "" || rule.FightPower < 0 {
		return f, errors.New("原觉醒章节/节点来源未恢复")
	}
	if _, err := p7Integer(rule.CopyID, 1, math.MaxInt32); err != nil {
		return f, errors.New("原觉醒难度编号缺失")
	}
	var content p7AwakeContent
	if json.Unmarshal(rule.Content, &content) != nil || content.Source == "" || len(content.Boxes)+len(content.Monsters) == 0 || len(content.Boxes)+len(content.Monsters) > 500 {
		return f, errors.New("原觉醒节点数据缺失")
	}
	seen := map[string]bool{}
	for _, box := range content.Boxes {
		if box == nil || box.GetGuid() == "" || len(box.GetGuid()) > 64 || seen[box.GetGuid()] || box.GetIsOpen() != 0 || !p7AwakePosition(box.GetPos()) || math.IsNaN(box.GetAngle()) || math.IsInf(box.GetAngle(), 0) || s.table("AwakeCopyBox", "BoxId", box.GetBoxId()) == nil {
			return f, errors.New("原觉醒宝箱节点无效")
		}
		seen[box.GetGuid()] = true
	}
	for _, mon := range content.Monsters {
		if mon == nil || mon.GetGuid() == "" || len(mon.GetGuid()) > 64 || seen[mon.GetGuid()] || mon.GetHpPercent() != 1 || !p7AwakePosition(mon.GetPos()) || math.IsNaN(mon.GetAngle()) || math.IsInf(mon.GetAngle(), 0) || s.table("AwakeCopyMonsterOutside", "monId", mon.GetMonId()) == nil {
			return f, errors.New("原觉醒怪物节点无效")
		}
		seen[mon.GetGuid()] = true
	}
	duration, err := s.p7ExpeditionSetting("awakeCopyTimeLimit")
	if err != nil || duration < 1 || duration > 1440 {
		return f, errors.New("原觉醒时限无效")
	}
	revives, err := s.p7ExpeditionSetting("awakeCopyReviveCount")
	if err != nil {
		return f, err
	}
	cost, err := s.p7ExpeditionSetting("Awake_Revive_Cost")
	if err != nil {
		return f, err
	}
	stamina, err := s.p7ExpeditionSetting("awakeCopyStaminaCost")
	if err != nil {
		return f, err
	}
	angle, err := p7Integer(row["bornAngle"], 0, 360)
	if err != nil || !p7AwakePosition(row["bornPos"]) || row["sceneId"] == "" || row["BattleMap"] == "" {
		return f, errors.New("原觉醒场景/安全点配置无效")
	}
	return p7AwakeFrozen{Rule: rule, Content: content, Scene: row["sceneId"], BattleMap: row["BattleMap"], BornPos: row["bornPos"], BornAngle: angle, DurationMinutes: duration, Revives: revives, ReviveCost: cost, StaminaCost: stamina}, nil
}

// PrepareP7AwakeDiagnostic 冻结受支持原帧；无公开路由，不消耗次数/材料/道具，不产生正式奖励。
func (s *GameService) PrepareP7AwakeDiagnostic(ctx context.Context, id int64, copyID string, now time.Time) (string, *pb.SCEnterAwakeCopy, error) {
	var runID string
	var reply *pb.SCEnterAwakeCopy
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var rule db.P7AwakeStageRule
		if err := tx.Where("copy_id=?", copyID).First(&rule).Error; err != nil {
			return err
		}
		frozen, err := s.p7AwakeFreeze(rule)
		if err != nil {
			return err
		}
		actor, err := s.battlePlayer(p, g)
		if err != nil {
			return err
		}
		actorRaw, err := proto.Marshal(actor)
		if err != nil {
			return err
		}
		end := now.Add(time.Duration(frozen.DurationMinutes) * time.Minute).UnixMilli()
		reply = &pb.SCEnterAwakeCopy{Boxes: frozen.Content.Boxes, Monsters: frozen.Content.Monsters, RefreshData: &pb.AwakeCopyRefreshData{LeftReviveCount: proto.Int32(frozen.Revives), ReviveCost: proto.Int32(0), EndTime: proto.Int64(end), HpPercent: proto.String("1"), Score: proto.Int32(0), Anger: proto.Float64(0)}, IsOver: proto.Int32(0)}
		frame, err := proto.Marshal(reply)
		if err != nil {
			return err
		}
		frozen.ActorHash = p6Digest(actorRaw)
		frozen.FrameHash = p6Digest(frame)
		raw, err := json.Marshal(frozen)
		if err != nil {
			return err
		}
		var random [16]byte
		if _, err = rand.Read(random[:]); err != nil {
			return err
		}
		runID = "p7-awake-" + hex.EncodeToString(random[:])
		run := db.P7AwakeRun{ID: runID, PlayerID: id, CopyID: copyID, Diagnostic: true, StartAt: now.UnixMilli(), EndAt: end, Freeze: raw, FreezeHash: p6Digest(raw), Actor: actorRaw, EnterFrame: frame}
		return tx.Create(&run).Error
	})
	return runID, reply, err
}
func (s *GameService) ReadP7AwakeDiagnostic(ctx context.Context, id int64, runID string) (*pb.SCEnterAwakeCopy, error) {
	var run db.P7AwakeRun
	if err := db.GetIns().WithContext(ctx).Where("id=? AND player_id=? AND diagnostic=?", runID, id, true).First(&run).Error; err != nil {
		return nil, err
	}
	var frozen p7AwakeFrozen
	if p6Digest(run.Freeze) != run.FreezeHash || json.Unmarshal(run.Freeze, &frozen) != nil || p6Digest(run.EnterFrame) != frozen.FrameHash || p6Digest(run.Actor) != frozen.ActorHash {
		return nil, errors.New("觉醒冻结帧被篡改")
	}
	if run.EndAt-run.StartAt != int64(frozen.DurationMinutes)*60000 {
		return nil, errors.New("觉醒冻结时限被篡改")
	}
	reply := &pb.SCEnterAwakeCopy{}
	if err := proto.Unmarshal(run.EnterFrame, reply); err != nil {
		return nil, err
	}
	return reply, nil
}
func (s *GameService) DispatchP7Awake(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSAwakeCopyData:
		reply, err := s.p7AwakeCatalogue(ctx, id, time.Now())
		return reply, true, err
	case *pb.CSEnterAwakeCopy, *pb.CSAwakeCopyOpenBox, *pb.CSAwakeCopySettle:
		if _, settle := request.(*pb.CSAwakeCopySettle); settle && stamp <= 0 {
			return nil, true, errors.New("觉醒结算时间标识无效")
		}
		reply, err := s.p7AwakeExplore(ctx, id, request, time.Now(), stamp)
		return reply, true, err
	case *pb.CSAwakeCopyFightBegin:
		reply, err := s.p7AwakeFightBegin(ctx, id, request.(*pb.CSAwakeCopyFightBegin), time.Now())
		return reply, true, err
	case *pb.CSAwakeCopyFightEnd:
		reply, err := s.p7AwakeFightEnd(ctx, id, request.(*pb.CSAwakeCopyFightEnd), time.Now())
		return reply, true, err
	case *pb.CSAwakeUseItem, *pb.CSAwakeRevice, *pb.CSAwakeFightRecords, *pb.CSAwakeFightRecordReplay:
		// 改运营SQL也不能制造原节点/胜利/完成度；诊断函数从未注册为网络入口。
		return nil, true, errors.New(constUtil.P7AwakeClosed)
	}
	return nil, false, nil
}

// p7AwakeLocalCatalogue 显式选择原可行走多边形中心；本地点位不冒充原服地图生成。
func p7AwakeLocalCatalogue(enabled bool) (db.P7AwakeStageRule, error) {
	content := p7AwakeContent{Source: constUtil.P7AwakeExploreSource}
	positions := []string{"-1.683333397,-0.078914553,3.266666889", "-2.866666794,-0.060859005,1.400000095", "-0.673333359,-0.096414578,5.273333549", "4.283333619,-0.102525691,3.727777918"}
	for i, pos := range positions {
		content.Boxes = append(content.Boxes, &pb.AwakeCopyBox{BoxId: proto.String(fmt.Sprintf("box%d", i+1)), Guid: proto.String(fmt.Sprintf("local-forest-box-%d", i+1)), Pos: proto.String(pos), IsOpen: proto.Int32(0), Angle: proto.Float64(0)})
	}
	content.Monsters = []*pb.AwakeCopyMonster{{MonId: proto.String("mon_030"), Guid: proto.String("local-forest-mon-1"), Pos: proto.String("-4.238889058,-0.021970133,-1.994444529"), HpPercent: proto.Float64(1), Angle: proto.Float64(0), MonsterType: proto.Int32(1)}}
	raw, err := json.Marshal(content)
	return db.P7AwakeStageRule{CopyID: constUtil.P7AwakeExploreCopy, ChapterID: "5", Enabled: enabled, ContentRestored: true, FightPower: 100, Content: raw, Source: constUtil.P7AwakeExploreSource}, err
}

type p7AwakeExplorationState struct {
	Opened       map[string]bool    `json:"opened"`
	Score        int32              `json:"score"`
	Defeated     map[string]bool    `json:"defeated,omitempty"`
	MonsterHP    map[string]float64 `json:"monsterHp,omitempty"`
	ActiveFight  string             `json:"activeFight,omitempty"`
	LastFight    string             `json:"lastFight,omitempty"`
	CurrentActor []byte             `json:"currentActor,omitempty"`
}

// p7AwakeRollBox 使用具名本地的千分权重解释；原表达式不是直接资源串。
// 仅接收 box1-4 的简单 item:quantity,weight 语法，职业选择标记保持关闭。
func (s *GameService) p7AwakeRollBox(boxID string, draw func(int64) (int64, error)) (string, error) {
	row := s.table("AwakeCopyBox", "BoxId", boxID)
	if row == nil || (boxID != "box1" && boxID != "box2" && boxID != "box3" && boxID != "box4") {
		return "", errors.New("本地箱型的概率语法未恢复")
	}
	var rewards []string
	for _, group := range strings.Split(row["dropAward"], "|") {
		pair := strings.Split(group, ":")
		if len(pair) != 2 || !strings.HasPrefix(pair[0], "item_") || s.table("Item", "itemId", pair[0]) == nil {
			return "", errors.New("觉醒箱奖资源无效")
		}
		type choice struct {
			n      int32
			weight int64
		}
		var choices []choice
		total := int64(0)
		for _, token := range strings.Split(pair[1], ";") {
			values := strings.Split(token, ",")
			if len(values) != 2 {
				return "", errors.New("觉醒箱奖权重无效")
			}
			quantity, err := p7Integer(values[0], 0, 10000)
			if err != nil {
				return "", err
			}
			weight, err := p7Integer(values[1], 0, 1000)
			if err != nil {
				return "", err
			}
			choices = append(choices, choice{quantity, int64(weight)})
			total += int64(weight)
		}
		if total != 1000 {
			return "", errors.New("觉醒箱奖权重合计必须为1000")
		}
		value, err := draw(total)
		if err != nil {
			return "", err
		}
		if value < 0 || value >= total {
			return "", errors.New("觉醒箱奖随机源越界")
		}
		for _, c := range choices {
			if value < c.weight {
				if c.n > 0 {
					rewards = append(rewards, fmt.Sprintf("%s,%d", pair[0], c.n))
				}
				break
			}
			value -= c.weight
		}
	}
	return strings.Join(rewards, ";"), nil
}

func p7AwakeReadFrozen(run *db.P7AwakeRun) (*p7AwakeFrozen, error) {
	var frozen p7AwakeFrozen
	if run.Diagnostic || p6Digest(run.Freeze) != run.FreezeHash || json.Unmarshal(run.Freeze, &frozen) != nil || p6Digest(run.Actor) != frozen.ActorHash || p6Digest(run.EnterFrame) != frozen.FrameHash || run.EndAt-run.StartAt != int64(frozen.DurationMinutes)*60000 || frozen.Rule.Source != constUtil.P7AwakeExploreSource || frozen.Rule.CopyID != constUtil.P7AwakeExploreCopy || frozen.Content.Source != constUtil.P7AwakeExploreSource || len(frozen.BoxLoot) != len(frozen.Content.Boxes) {
		return nil, errors.New("觉醒实际实例冻结来源无效")
	}
	if frozen.BattleSource != "" && (frozen.BattleSource != constUtil.P7AwakeBattleSource || frozen.ScoreReward != constUtil.P7AwakeScoreReward || len(frozen.MonsterLoot) != len(frozen.Content.Monsters)) {
		return nil, errors.New("觉醒冻结战斗奖池来源无效")
	}
	if _, err := p7AwakeFrozenBusinessDay(run, &frozen); err != nil {
		return nil, err
	}
	return &frozen, nil
}
func p7AwakeReadProgress(ledger *db.P7AwakeExploration, frozen *p7AwakeFrozen) (*p7AwakeExplorationState, error) {
	var state p7AwakeExplorationState
	if p6Digest(ledger.State) != ledger.StateHash || json.Unmarshal(ledger.State, &state) != nil || state.Opened == nil {
		return nil, errors.New("觉醒探索账本损坏")
	}
	count := int32(0)
	for guid, open := range state.Opened {
		if _, ok := frozen.BoxLoot[guid]; !ok || !open {
			return nil, errors.New("觉醒探索记录包含外来节点")
		}
		count++
	}
	defeated := int32(0)
	for guid, dead := range state.Defeated {
		known := false
		for _, node := range frozen.Content.Monsters {
			if node.GetGuid() == guid {
				known = true
			}
		}
		if !known || !dead || frozen.BattleSource != constUtil.P7AwakeBattleSource || state.LastFight == "" {
			return nil, errors.New("觉醒击杀记录缺少真实战斗来源")
		}
		defeated++
	}
	for guid, hp := range state.MonsterHP {
		known := false
		for _, node := range frozen.Content.Monsters {
			if node.GetGuid() == guid {
				known = true
			}
		}
		if !known || !combatFinite(hp) || hp < 0 || hp > 1 || state.LastFight == "" {
			return nil, errors.New("觉醒怪物血量来源无效")
		}
	}
	if (state.LastFight == "") != (len(state.CurrentActor) == 0) {
		return nil, errors.New("觉醒战后队伍来源不完整")
	}
	if state.Score != count*5+defeated*80 || state.Score > 100 {
		return nil, errors.New("觉醒探索评分来源无效")
	}
	return &state, nil
}
func p7AwakeSaveProgress(tx *gorm.DB, ledger *db.P7AwakeExploration, state *p7AwakeExplorationState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	ledger.State = raw
	ledger.StateHash = p6Digest(raw)
	return tx.Save(ledger).Error
}
func p7AwakeRefresh(run *db.P7AwakeRun, frozen *p7AwakeFrozen, state *p7AwakeExplorationState) (*pb.AwakeCopyRefreshData, error) {
	// 原ChaCopyHeroPanel按 current/max 文本更新血条；取已冻结camp0完整队伍，重登不重算当前配置。
	actor := &pb.BattlePlayer{}
	actorRaw := run.Actor
	if len(state.CurrentActor) > 0 {
		actorRaw = state.CurrentActor
	}
	if proto.Unmarshal(actorRaw, actor) != nil || len(actor.Formation) == 0 {
		return nil, errors.New("觉醒冻结队伍血量缺失")
	}
	var current, maximum float64
	for _, hero := range actor.Formation {
		if hero == nil || hero.Prop == nil || hero.Inst == nil {
			return nil, errors.New("觉醒冻结队伍血量不完整")
		}
		hp, maxHP := hero.Inst.GetCurHp(), hero.Prop.GetHp()
		if math.IsNaN(hp) || math.IsInf(hp, 0) || math.IsNaN(maxHP) || math.IsInf(maxHP, 0) || maxHP <= 0 || hp < 0 || hp > maxHP {
			return nil, errors.New("觉醒冻结队伍血量无效")
		}
		current += hp
		maximum += maxHP
	}
	if math.IsNaN(current) || math.IsInf(current, 0) || math.IsNaN(maximum) || math.IsInf(maximum, 0) || maximum <= 0 {
		return nil, errors.New("觉醒队伍总血量无效")
	}
	hpText := strconv.FormatFloat(current, 'f', -1, 64) + "/" + strconv.FormatFloat(maximum, 'f', -1, 64)
	return &pb.AwakeCopyRefreshData{LeftReviveCount: proto.Int32(frozen.Revives), ReviveCost: proto.Int32(0), EndTime: proto.Int64(run.EndAt), HpPercent: proto.String(hpText), Score: proto.Int32(state.Score), Anger: proto.Float64(actor.GetInst().GetAnger())}, nil
}
func p7AwakePending(frozen *p7AwakeFrozen, state *p7AwakeExplorationState) (string, []*pb.BaseResource, error) {
	var tokens []string
	// 固定节点顺序，使冷恢复、并发重试的回包顺序保持一致。
	for _, box := range frozen.Content.Boxes {
		if state.Opened[box.GetGuid()] && frozen.BoxLoot[box.GetGuid()] != "" {
			tokens = append(tokens, frozen.BoxLoot[box.GetGuid()])
		}
	}
	for _, mon := range frozen.Content.Monsters {
		if state.Defeated[mon.GetGuid()] && frozen.MonsterLoot[mon.GetGuid()] != "" {
			tokens = append(tokens, frozen.MonsterLoot[mon.GetGuid()])
		}
	}
	text := strings.Join(tokens, ";")
	if text == "" {
		return "", nil, nil
	}
	rewards, err := p1RewardResources(text)
	return text, rewards, err
}
func p7AwakeResume(run *db.P7AwakeRun, frozen *p7AwakeFrozen, state *p7AwakeExplorationState) (*pb.SCEnterAwakeCopy, error) {
	reply := &pb.SCEnterAwakeCopy{}
	if err := proto.Unmarshal(run.EnterFrame, reply); err != nil {
		return nil, err
	}
	for _, box := range reply.Boxes {
		if state.Opened[box.GetGuid()] {
			box.IsOpen = proto.Int32(1)
		}
	}
	remaining := []*pb.AwakeCopyMonster{}
	for _, mon := range reply.Monsters {
		if !state.Defeated[mon.GetGuid()] {
			if hp, ok := state.MonsterHP[mon.GetGuid()]; ok {
				mon.HpPercent = proto.Float64(hp)
			}
			remaining = append(remaining, mon)
		}
	}
	reply.Monsters = remaining
	_, rewards, err := p7AwakePending(frozen, state)
	if err != nil {
		return nil, err
	}
	reply.Rewards = rewards
	reply.RefreshData, err = p7AwakeRefresh(run, frozen, state)
	if err != nil {
		return nil, err
	}
	return reply, nil
}
func p7AwakeActive(tx *gorm.DB, id int64) (*db.P7AwakeExploration, *db.P7AwakeRun, error) {
	var ledger db.P7AwakeExploration
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND status=?", id, constUtil.P7AwakeExploreActive).Order("run_id").First(&ledger).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var run db.P7AwakeRun
	if err = tx.Where("id=? AND player_id=? AND diagnostic=?", ledger.RunID, id, false).First(&run).Error; err != nil {
		return nil, nil, err
	}
	return &ledger, &run, nil
}
func (s *GameService) p7AwakeSettle(tx *gorm.DB, p *pb.Player, g *model.GameState, run *db.P7AwakeRun, ledger *db.P7AwakeExploration, now time.Time, explicitExit ...bool) (*pb.SCAwakeCopySettle, error) {
	if ledger.Status == constUtil.P7AwakeExploreSettled || ledger.Status == constUtil.P7AwakeExploreAbandoned {
		reply := &pb.SCAwakeCopySettle{}
		if p6Digest(ledger.Receipt) != ledger.ReceiptHash {
			return nil, errors.New("觉醒结算回执损坏")
		}
		err := proto.Unmarshal(ledger.Receipt, reply)
		return reply, err
	}
	frozen, err := p7AwakeReadFrozen(run)
	if err != nil {
		return nil, err
	}
	state, err := p7AwakeReadProgress(ledger, frozen)
	if err != nil {
		return nil, err
	}
	// 原显式退出可放弃失败回调后仍未提交的战斗；日目录自动结算仍只在到期时执行。
	abandoned := state.ActiveFight != "" && len(explicitExit) > 0 && explicitExit[0]
	if state.ActiveFight != "" {
		if now.UnixMilli() < run.EndAt && !abandoned {
			return nil, errors.New("请先完成当前觉醒战斗")
		}
		// 只关闭未提交pending，不补战斗结果、怪物分数或掉落；已有箱奖保持原结算。
		if err = tx.Model(&db.P7AwakeFight{}).Where("id=? AND player_id=? AND run_id=? AND status=?", state.ActiveFight, run.PlayerID, run.ID, constUtil.P7AwakeFightPending).Update("status", constUtil.P7AwakeFightAbandoned).Error; err != nil {
			return nil, err
		}
		state.ActiveFight = ""
		if err = p7AwakeSaveProgress(tx, ledger, state); err != nil {
			return nil, err
		}
	}
	text, rewards, err := p7AwakePending(frozen, state)
	if err != nil {
		return nil, err
	}
	gain := &pb.ResultObject{}
	if text != "" {
		gain, err = s.GrantFixedReward(p, g, text)
		if err != nil {
			return nil, err
		}
	}
	var scoreRewards []*pb.BaseResource
	if state.Score == 100 {
		if frozen.BattleSource != constUtil.P7AwakeBattleSource || frozen.ScoreReward != constUtil.P7AwakeScoreReward || len(state.Defeated) != 1 {
			return nil, errors.New("觉醒100分缺少真实完成来源")
		}
		day, eventAt, err := p7AwakeScoreClock(tx, run, frozen, now)
		if err != nil {
			return nil, err
		}
		dayKey := "awake-score-day:" + day
		var count int64
		if err = tx.Model(&db.EventFact{}).Where("player_id=? AND source_key=? AND kind=?", run.PlayerID, dayKey, constUtil.P7AwakeCompleteDailyEvent).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			current, err := s.GrantFixedReward(p, g, frozen.ScoreReward)
			if err != nil {
				return nil, err
			}
			p1AppendGain(gain, current)
			scoreRewards, err = p1RewardResources(frozen.ScoreReward)
			if err != nil {
				return nil, err
			}
			if err = s.RecordP1Event(tx, run.PlayerID, dayKey, constUtil.P7AwakeCompleteDailyEvent, 100, eventAt); err != nil {
				return nil, err
			}
		}
		if err = s.RecordP1Event(tx, run.PlayerID, "awake-complete:"+run.ID, constUtil.P7AwakeCompleteEvent, 100, eventAt); err != nil {
			return nil, err
		}
	}
	// 部分探索仅结算已取得资源；完整真100分才按具名规则领取本日首次评分奖。
	reply := &pb.SCAwakeCopySettle{BoxRewards: rewards, ScoreRewards: scoreRewards, TotalRewards: gain, Score: proto.Int32(state.Score), CoinRewards: proto.Int32(0), Exp: proto.Int32(0)}
	raw, err := proto.Marshal(reply)
	if err != nil {
		return nil, err
	}
	ledger.Status = constUtil.P7AwakeExploreSettled
	if abandoned {
		ledger.Status = constUtil.P7AwakeExploreAbandoned
	}
	ledger.SettledAt = now.UnixMilli()
	ledger.Receipt = raw
	ledger.ReceiptHash = p6Digest(raw)
	return reply, tx.Save(ledger).Error
}

// p7AwakeExplore 每个动作持有同一角色事务锁；进度、库存与结算回执一起提交。
func (s *GameService) p7AwakeExplore(ctx context.Context, id int64, request proto.Message, now time.Time, stamps ...int64) ([]proto.Message, error) {
	var stamp int64
	if len(stamps) > 0 {
		stamp = stamps[0]
	}
	var response proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		// 原Settle没有实例ID；已提交stamp先回放所属旧实例，再查询当前活动轮次。
		if _, settle := request.(*pb.CSAwakeCopySettle); settle && stamp > 0 {
			var saved db.P7AwakeExploration
			e := tx.Where("player_id=? AND status IN ? AND settle_stamp=?", id, []string{constUtil.P7AwakeExploreSettled, constUtil.P7AwakeExploreAbandoned}, stamp).First(&saved).Error
			if e == nil {
				reply := &pb.SCAwakeCopySettle{}
				if p6Digest(saved.Receipt) != saved.ReceiptHash {
					return errors.New("觉醒旧结算回执损坏")
				}
				if e = proto.Unmarshal(saved.Receipt, reply); e != nil {
					return e
				}
				response = reply
				return nil
			}
			if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		ledger, run, err := p7AwakeActive(tx, id)
		if err != nil {
			return err
		}
		switch body := request.(type) {
		case *pb.CSEnterAwakeCopy:
			if len(body.GetItems()) != 0 {
				return errors.New("本地觉醒首实例尚未恢复携带料理，未扣道具")
			}
			if ledger != nil {
				if body.GetCopyId() != run.CopyID {
					return errors.New("请先退出当前觉醒探索")
				}
				if now.UnixMilli() >= run.EndAt {
					return errors.New("当前觉醒探索已到期，请结算")
				}
				frozen, err := p7AwakeReadFrozen(run)
				if err != nil {
					return err
				}
				state, err := p7AwakeReadProgress(ledger, frozen)
				if err != nil {
					return err
				}
				response, err = p7AwakeResume(run, frozen, state)
				return err
			}
			if g.Run != nil && !g.Run.Complete {
				return errors.New("请先完成当前副本")
			}
			if err = s.ensureNoActiveDailyCopy(tx, id, now); err != nil {
				return err
			}
			if tx.Migrator().HasTable(&db.P7RushState{}) {
				var active int64
				if err = tx.Model(&db.P7RushState{}).Where("player_id=? AND active_fight<>?", id, "").Count(&active).Error; err != nil {
					return err
				}
				if active > 0 {
					return errors.New("请先结束当前试炼战斗")
				}
			}
			var policy db.P7AwakePolicy
			if err = tx.Where("id=?", constUtil.P7AwakeExplorePolicyID).First(&policy).Error; err != nil {
				return err
			}
			if !policy.Enabled || policy.Source != constUtil.P7AwakeExploreSource || policy.MinLevel != 70 || p.GetLevel() < policy.MinLevel {
				return errors.New(constUtil.P7AwakeClosed)
			}
			expected, err := p7AwakeLocalCatalogue(true)
			if err != nil {
				return err
			}
			var rule db.P7AwakeStageRule
			if err = tx.Where("copy_id=?", body.GetCopyId()).First(&rule).Error; err != nil {
				return errors.New(constUtil.P7AwakeClosed)
			}
			a, _ := json.Marshal(rule)
			b, _ := json.Marshal(expected)
			if string(a) != string(b) {
				return errors.New("本地觉醒目录尚未完整发布")
			}
			frozen, err := s.p7AwakeFreeze(rule)
			if err != nil {
				return err
			}
			if err = freezeP7AwakeCalendar(tx, id, &frozen, now); err != nil {
				return err
			}
			open, err := p7AwakeOpenDay(s.table("AwakeCopyChapter", "chapterId", rule.ChapterID)["openDay"], LocalPlayAt(tx.Statement.Context, now))
			if err != nil {
				return err
			}
			if !open {
				return errors.New("今天未开放该觉醒章节")
			}
			daily, left, err := s.p7AwakeState(tx, id, now)
			if err != nil {
				return err
			}
			if left == 0 {
				return errors.New("今日觉醒探索次数已用完")
			}
			if frozen.StaminaCost != 0 {
				return errors.New("本地觉醒体力规则与原表不符")
			}
			// 在同一付次数事务捕获当前真实公会来源，不能漏算增益来绕过独立scope。
			if err = s.PrepareP5GuildPrayerProperties(ctx, tx, p, g, now); err != nil {
				return err
			}
			actor, err := s.battlePlayer(p, g)
			if err != nil {
				return err
			}
			actorRaw, err := proto.Marshal(actor)
			if err != nil {
				return err
			}
			// 原动作只携带节点GUID；每次实例采用独立GUID，旧轮延迟请求不能命中新轮。
			var random [16]byte
			if _, err = rand.Read(random[:]); err != nil {
				return err
			}
			runID := "p7-awake-" + hex.EncodeToString(random[:])
			for index, box := range frozen.Content.Boxes {
				box.Guid = proto.String(fmt.Sprintf("%s:b%d", runID, index+1))
			}
			for index, mon := range frozen.Content.Monsters {
				mon.Guid = proto.String(fmt.Sprintf("%s:m%d", runID, index+1))
			}
			frozen.BoxLoot = map[string]string{}
			for _, box := range frozen.Content.Boxes {
				loot, err := s.p7AwakeRollBox(box.GetBoxId(), func(limit int64) (int64, error) {
					n, err := rand.Int(rand.Reader, big.NewInt(limit))
					if err != nil {
						return 0, err
					}
					return n.Int64(), nil
				})
				if err != nil {
					return err
				}
				frozen.BoxLoot[box.GetGuid()] = loot
			}
			var battlePolicy db.P7AwakePolicy
			if e := tx.First(&battlePolicy, constUtil.P7AwakeBattlePolicyID).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if battlePolicy.Enabled {
				if battlePolicy.Source != constUtil.P7AwakeBattleSource || battlePolicy.MinLevel != 70 {
					return errors.New("本地觉醒战斗策略来源无效")
				}
				// 首次付次数前核对完整实际队伍；未覆盖阵容不能先付次数再发现战斗门禁。
				if frozen.BattleMap != "map_forest_003" {
					return errors.New("觉醒原战斗场景未覆盖")
				}
				enemyBytes, e := hex.DecodeString(constUtil.P7AwakeEnemyHex)
				if e != nil || p6Digest(enemyBytes) != constUtil.P7AwakeEnemySHA {
					return errors.New("觉醒原NPC来源无效")
				}
				enemy := &pb.BattleMonster{}
				if e = proto.Unmarshal(enemyBytes, enemy); e != nil {
					return e
				}
				admission, e := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: actor, Monster: enemy, Seed: 0, MaxRound: 40, BattleType: 16, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
				if e != nil {
					return e
				}
				if proof, e := s.p7AwakeBattleProof(ctx, admission); e != nil || len(proof) != 64 {
					return fmt.Errorf("当前觉醒队伍原机制不可用: %w", e)
				}
				frozen.BattleSource = constUtil.P7AwakeBattleSource
				frozen.ScoreReward = constUtil.P7AwakeScoreReward
				frozen.MonsterLoot = map[string]string{}
				for _, mon := range frozen.Content.Monsters {
					loot, e := s.p7AwakeRollMonster(mon.GetMonId())
					if e != nil {
						return e
					}
					frozen.MonsterLoot[mon.GetGuid()] = loot
				}
			}
			run = &db.P7AwakeRun{ID: runID, PlayerID: id, CopyID: rule.CopyID, StartAt: now.UnixMilli(), EndAt: now.Add(time.Duration(frozen.DurationMinutes) * time.Minute).UnixMilli(), Actor: actorRaw}
			state := &p7AwakeExplorationState{Opened: map[string]bool{}}
			refresh, err := p7AwakeRefresh(run, &frozen, state)
			if err != nil {
				return err
			}
			initial := &pb.SCEnterAwakeCopy{Boxes: frozen.Content.Boxes, Monsters: frozen.Content.Monsters, RefreshData: refresh, IsOver: proto.Int32(0)}
			run.EnterFrame, err = proto.Marshal(initial)
			if err != nil {
				return err
			}
			frozen.ActorHash = p6Digest(actorRaw)
			if g.GuildPrayerProjection != nil {
				frozen.GuildPrayerSource = g.GuildPrayerProjection.SourceHash
				frozen.GuildTechSource = g.GuildPrayerProjection.TechSourceHash
			}
			frozen.FrameHash = p6Digest(run.EnterFrame)
			run.Freeze, err = json.Marshal(frozen)
			if err != nil {
				return err
			}
			run.FreezeHash = p6Digest(run.Freeze)
			if err = tx.Create(run).Error; err != nil {
				return err
			}
			ledger = &db.P7AwakeExploration{RunID: run.ID, PlayerID: id, Status: constUtil.P7AwakeExploreActive, EnterStamp: stamp}
			if err = p7AwakeSaveProgress(tx, ledger, state); err != nil {
				return err
			}
			daily.Used++
			if err = saveP7AwakeCalendar(tx, daily, now); err != nil {
				return err
			}
			response = initial
		case *pb.CSAwakeCopyOpenBox:
			if ledger == nil {
				return errors.New("没有进行中的觉醒探索")
			}
			if now.UnixMilli() >= run.EndAt {
				return errors.New("觉醒探索已到期，请结算")
			}
			frozen, err := p7AwakeReadFrozen(run)
			if err != nil {
				return err
			}
			state, err := p7AwakeReadProgress(ledger, frozen)
			if err != nil {
				return err
			}
			if state.ActiveFight != "" {
				return errors.New("当前觉醒战斗尚未结束，不能开启其他节点")
			}
			text, ok := frozen.BoxLoot[body.GetBoxGuid()]
			if !ok {
				return errors.New("宝箱不属于当前觉醒实例")
			}
			var rewards []*pb.BaseResource
			if !state.Opened[body.GetBoxGuid()] {
				state.Opened[body.GetBoxGuid()] = true
				state.Score += 5
				if err = p7AwakeSaveProgress(tx, ledger, state); err != nil {
					return err
				}
				if text != "" {
					rewards, err = p1RewardResources(text)
					if err != nil {
						return err
					}
				}
			}
			refresh, err := p7AwakeRefresh(run, frozen, state)
			if err != nil {
				return err
			}
			response = &pb.SCAwakeCopyOpenBox{Rewards: rewards, RefreshData: refresh}
		case *pb.CSAwakeCopySettle:
			if ledger == nil {
				ledger = &db.P7AwakeExploration{}
				if err = tx.Where("player_id=? AND status IN ?", id, []string{constUtil.P7AwakeExploreSettled, constUtil.P7AwakeExploreAbandoned}).Order("settled_at DESC,run_id DESC").First(ledger).Error; err != nil {
					return errors.New("没有可结算的觉醒探索")
				}
				result := &pb.SCAwakeCopySettle{}
				if p6Digest(ledger.Receipt) != ledger.ReceiptHash {
					return errors.New("觉醒结算回执损坏")
				}
				if err = proto.Unmarshal(ledger.Receipt, result); err != nil {
					return err
				}
				if stamp > 0 && ledger.SettleStamp == 0 {
					ledger.SettleStamp = stamp
					if err = tx.Save(ledger).Error; err != nil {
						return err
					}
				}
				response = result
				return nil
			}
			if stamp > 0 && ledger.EnterStamp > 0 && stamp < ledger.EnterStamp {
				return errors.New("旧实例结算请求不能结算当前觉醒探索")
			}
			response, err = s.p7AwakeSettle(tx, p, g, run, ledger, now, true)
			if err != nil {
				return err
			}
			if stamp > 0 {
				ledger.SettleStamp = stamp
				return tx.Save(ledger).Error
			}
			return nil
		default:
			return errors.New("本地觉醒动作未恢复")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// p7AwakeRequireIdle 供共享战斗所有权入口使用；老隔离fixture未建本域表时维持原行为。
// 到期实例仍需经原目录/结算入口发放已得箱奖，不能在创建其他战斗时静默丢弃。
func p7AwakeRequireIdle(tx *gorm.DB, id int64) error {
	if !tx.Migrator().HasTable(&db.P7AwakeExploration{}) {
		return nil
	}
	var count int64
	if err := tx.Model(&db.P7AwakeExploration{}).Where("player_id=? AND status=?", id, constUtil.P7AwakeExploreActive).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("请先退出当前觉醒探索")
	}
	return nil
}

// p7AwakeRollMonster 沿具名本地千分规则解释原mon030怪池，入场冻结所有结果。
func (s *GameService) p7AwakeRollMonster(monID string) (string, error) {
	if monID != "mon_030" {
		return "", errors.New("本地觉醒怪池未恢复")
	}
	row := s.table("AwakeCopyMonsterOutside", "monId", monID)
	if row == nil {
		return "", errors.New("原觉醒怪型缺失")
	}
	var rewards []string
	for _, group := range strings.Split(row["rewardGroupId"], "|") {
		pair := strings.Split(group, ":")
		if len(pair) != 2 {
			return "", errors.New("原觉醒怪奖表达式无效")
		}
		ok, err := s.openServerRewardSupported(pair[0] + ",1")
		if err != nil || !ok {
			return "", errors.New("觉醒怪奖资源未恢复")
		}
		type choice struct {
			quantity int32
			weight   int64
		}
		var choices []choice
		total := int64(0)
		for _, token := range strings.Split(pair[1], ";") {
			values := strings.Split(token, ",")
			if len(values) != 2 {
				return "", errors.New("觉醒怪奖权重无效")
			}
			quantity, err := p7Integer(values[0], 0, 10000)
			if err != nil {
				return "", err
			}
			weight, err := p7Integer(values[1], 0, 1000)
			if err != nil {
				return "", err
			}
			total += int64(weight)
			choices = append(choices, choice{quantity, int64(weight)})
		}
		if total != 1000 {
			return "", errors.New("觉醒怪奖权重合计必须为1000")
		}
		n, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return "", err
		}
		draw := n.Int64()
		for _, choice := range choices {
			if draw < choice.weight {
				if choice.quantity > 0 {
					rewards = append(rewards, fmt.Sprintf("%s,%d", pair[0], choice.quantity))
				}
				break
			}
			draw -= choice.weight
		}
	}
	return strings.Join(rewards, ";"), nil
}
func p7AwakeFindMonster(frozen *p7AwakeFrozen, guid string) *pb.AwakeCopyMonster {
	for _, mon := range frozen.Content.Monsters {
		if mon.GetGuid() == guid {
			return proto.Clone(mon).(*pb.AwakeCopyMonster)
		}
	}
	return nil
}
func p7AwakeFightInput(tx *gorm.DB, fight *db.P7AwakeFight) (*model.FrozenBattleInput, *db.P6BattleRecord, error) {
	var record db.P6BattleRecord
	if err := tx.Where("id=? AND player_id=? AND source=? AND source_fight=?", fight.ReplayID, fight.PlayerID, constUtil.P7AwakeBattleReplaySource, fight.ID).First(&record).Error; err != nil {
		return nil, nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil || fight.InputHash != record.InputHash || p6Digest(fight.Start) != fight.StartHash {
		return nil, nil, errors.New("觉醒战斗冻结来源无效")
	}
	start := &pb.SCAwakeCopyFightBegin{}
	if proto.Unmarshal(fight.Start, start) != nil || start.GetFightId() != fight.ID || start.GetRandomSeed() != input.Seed || start.GetMaxRoundNum() != input.MaxRound || start.GetRoomId() != "map_forest_003" || !proto.Equal(start.BattlePlayer, input.Attacker) || !proto.Equal(start.BattleMonser, input.Monster) {
		return nil, nil, errors.New("觉醒typed16与冻结输入不一致")
	}
	var proof string
	if fight.ScopeProof == constUtil.P7Awake16GenericCompatibilityProof {
		validator := &GameService{Tables: input.Tables}
		proof, err = validator.p7Awake16GenericProof(tx.Statement.Context, input)
	} else {
		proof, err = P7Awake16ProofForInput(input)
	}
	if err != nil || proof == "" || proof != fight.ScopeProof {
		return nil, nil, errors.New("觉醒战斗独立证明无效")
	}
	return input, &record, nil
}

func (s *GameService) p7AwakeFightBegin(ctx context.Context, id int64, body *pb.CSAwakeCopyFightBegin, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCAwakeCopyFightBegin
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		ledger, run, err := p7AwakeActive(tx, id)
		if err != nil {
			return err
		}
		if ledger == nil {
			return errors.New("没有进行中的觉醒探索")
		}
		if now.UnixMilli() >= run.EndAt {
			return errors.New("觉醒探索已到期，请结算")
		}
		frozen, err := p7AwakeReadFrozen(run)
		if err != nil {
			return err
		}
		state, err := p7AwakeReadProgress(ledger, frozen)
		if err != nil {
			return err
		}
		mon := p7AwakeFindMonster(frozen, body.GetMonGuid())
		if mon == nil || frozen.BattleSource != constUtil.P7AwakeBattleSource || len(frozen.MonsterLoot) != 1 {
			return errors.New("觉醒真实战斗策略或节点尚未开放")
		}
		if state.ActiveFight != "" {
			var fight db.P7AwakeFight
			if err = tx.Where("id=? AND player_id=? AND run_id=? AND monster_guid=? AND status=?", state.ActiveFight, id, run.ID, body.GetMonGuid(), constUtil.P7AwakeFightPending).First(&fight).Error; err != nil {
				return errors.New("当前觉醒怪物战斗尚未结束")
			}
			if _, record, e := p7AwakeFightInput(tx, &fight); e != nil || record.Status != constUtil.P6BattleCaptured || record.Authoritative {
				return errors.New("觉醒待完成战斗来源无效")
			}
			reply = &pb.SCAwakeCopyFightBegin{}
			return proto.Unmarshal(fight.Start, reply)
		}
		if state.Defeated[body.GetMonGuid()] || state.LastFight != "" {
			return errors.New("本地首实例该怪已挑战，受伤续战和复活尚未恢复")
		}
		attacker := &pb.BattlePlayer{}
		if proto.Unmarshal(run.Actor, attacker) != nil {
			return errors.New("觉醒冻结队伍无效")
		}
		enemyRaw, err := hex.DecodeString(constUtil.P7AwakeEnemyHex)
		if err != nil || p6Digest(enemyRaw) != constUtil.P7AwakeEnemySHA {
			return errors.New("原觉醒本地NPC来源无效")
		}
		enemy := &pb.BattleMonster{}
		if err = proto.Unmarshal(enemyRaw, enemy); err != nil {
			return err
		}
		random, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32+1))
		if err != nil {
			return err
		}
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return err
		}
		fightID := "p7-awake-fight-" + hex.EncodeToString(nonce[:])
		input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: attacker, Monster: enemy, Seed: random.Int64(), MaxRound: 40, BattleType: 16, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
		if err != nil {
			return err
		}
		proof, err := s.p7AwakeBattleProof(ctx, input)
		if err != nil {
			return err
		}
		if len(proof) != 64 {
			return errors.New("觉醒独立证明未完成")
		}
		replayID, err := s.CaptureBattleReplay(ctx, tx, id, constUtil.P7AwakeBattleReplaySource, fightID, *input)
		if err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err = tx.First(&record, "id=?", replayID).Error; err != nil {
			return err
		}
		reply = &pb.SCAwakeCopyFightBegin{FightId: proto.String(fightID), RandomSeed: proto.Int64(input.Seed), BattlePlayer: input.Attacker, BattleMonser: input.Monster, MaxRoundNum: proto.Int32(40), RoomId: proto.String(frozen.BattleMap)}
		raw, err := proto.Marshal(reply)
		if err != nil {
			return err
		}
		fight := &db.P7AwakeFight{ID: fightID, PlayerID: id, RunID: run.ID, MonsterGUID: mon.GetGuid(), Status: constUtil.P7AwakeFightPending, ReplayID: replayID, ScopeProof: proof, Start: raw, StartHash: p6Digest(raw), InputHash: record.InputHash, CreatedAt: now.UnixMilli()}
		if err = tx.Create(fight).Error; err != nil {
			return err
		}
		state.ActiveFight = fightID
		return p7AwakeSaveProgress(tx, ledger, state)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}

// p7AwakeFightEnd 在角色事务内按冻结输入独立重算完整原AI终局，再一次提交真实结果。
func (s *GameService) p7AwakeFightEnd(ctx context.Context, id int64, body *pb.CSAwakeCopyFightEnd, now time.Time) ([]proto.Message, error) {
	if body == nil || body.GetOptRecord() == nil || (body.GetIsWinClient() != 0 && body.GetIsWinClient() != 1) {
		return nil, errors.New("觉醒End记录不完整")
	}
	var reply *pb.SCAwakeCopyFightEnd
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var fight db.P7AwakeFight
		if err := tx.Where("id=? AND player_id=?", body.GetFightId(), id).First(&fight).Error; err != nil {
			return errors.New("觉醒战斗不属于当前角色")
		}
		input, record, err := p7AwakeFightInput(tx, &fight)
		if err != nil {
			return err
		}
		requestRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(body)
		if err != nil {
			return err
		}
		if fight.Status == constUtil.P7AwakeFightAccepted {
			if p6Digest(requestRaw) != fight.EndHash || p6Digest(fight.EndRequest) != fight.EndHash || p6Digest(fight.Terminal) != fight.TerminalHash || p6Digest(fight.Reply) != fight.ReplyHash || record.Status != constUtil.P6BattleComputed || !record.Authoritative {
				return errors.New("觉醒End回执或重试记录不一致")
			}
			var terminal model.CombatFinalHPOutcome
			if json.Unmarshal(fight.Terminal, &terminal) != nil {
				return errors.New("觉醒终局回执损坏")
			}
			if fight.ScopeProof == constUtil.P7Awake16GenericCompatibilityProof {
				err = validateP7Awake16GenericEnd(ctx, input, &terminal, body.GetOptRecord(), body.GetIsWinClient())
			} else {
				err = validateP7Awake16EndReceipt(input, &terminal, body.GetOptRecord(), body.GetIsWinClient())
			}
			if err != nil {
				return err
			}
			outcomeRaw, encodeErr := json.Marshal(terminal.Outcome)
			result := p6PlaybackResult(terminal.Outcome.Winner)
			if encodeErr != nil || !bytes.Equal(outcomeRaw, record.Outcome) || p6Digest(record.Outcome) != record.OutcomeHash || record.ClientResult != result || record.ServerResult != result || record.ReplayHash != p6Digest(append(append([]byte(nil), record.Operates...), byte(result))) {
				return errors.New("觉醒权威录像与完整终局回执不一致")
			}
			reply = &pb.SCAwakeCopyFightEnd{}
			return proto.Unmarshal(fight.Reply, reply)
		}
		ledger, run, err := p7AwakeActive(tx, id)
		if err != nil {
			return err
		}
		if ledger == nil || run.ID != fight.RunID || now.UnixMilli() >= run.EndAt || fight.Status != constUtil.P7AwakeFightPending || record.Status != constUtil.P6BattleCaptured || record.Authoritative {
			return errors.New("觉醒当前战斗已结束或到期")
		}
		frozen, err := p7AwakeReadFrozen(run)
		if err != nil {
			return err
		}
		state, err := p7AwakeReadProgress(ledger, frozen)
		if err != nil {
			return err
		}
		if state.ActiveFight != fight.ID || state.Defeated[fight.MonsterGUID] || frozen.BattleSource != constUtil.P7AwakeBattleSource {
			return errors.New("觉醒End跨实例或节点")
		}
		var terminal *model.CombatFinalHPOutcome
		if fight.ScopeProof == constUtil.P7Awake16GenericCompatibilityProof {
			terminal, err = s.ComputeP7Awake16GenericSettlement(ctx, input, body.GetOptRecord(), body.GetIsWinClient())
			if err != nil {
				return err
			}
		} else {
			terminal, err = s.ComputeP7Awake16ProtocolFinalHP(ctx, input)
			if err != nil {
				return err
			}
			if err = ValidateP7Awake16CertifiedReceipt(input, terminal); err != nil {
				return err
			}
			autoResult := int32(0)
			if terminal.Outcome.Winner == 0 {
				autoResult = 1
			}
			if autoResult != body.GetIsWinClient() || !proto.Equal(terminal.OriginalOperates, body.GetOptRecord()) {
				// 旧实例继续使用旧自动/原机手动族，不以新证书改写历史资格。
				terminal, err = s.ComputeP7Awake16ManualSettlement(ctx, input, body.GetOptRecord(), body.GetIsWinClient())
				if err != nil {
					return errors.New("觉醒完整原操作或实际胜负不一致")
				}
			}
		}
		// 手动驱动的真实结果可能不同于预先自动结果，得分/掉落只使用最终认可终局。
		actual := int32(0)
		if terminal.Outcome.Winner == 0 {
			actual = 1
		}
		currentActor := proto.Clone(input.Attacker).(*pb.BattlePlayer)
		if terminal.Outcome.Attacker == nil || terminal.Outcome.Attacker.Inst == nil {
			return errors.New("觉醒真实终局AP缺失")
		}
		currentActor.Inst = proto.Clone(terminal.Outcome.Attacker.Inst).(*pb.BattlePlayerInst)
		var enemyCurrent, enemyMaximum float64
		for _, actor := range terminal.FinalActors {
			if actor.Camp == 0 {
				found := false
				for _, hero := range currentActor.Formation {
					if hero.GetGuid() == actor.GUID {
						hero.Inst.CurHp = proto.Float64(actor.CurrentHP)
						found = true
					}
				}
				if !found {
					return errors.New("觉醒终局队员不一致")
				}
			}
			if actor.Camp == 1 {
				enemyCurrent += actor.CurrentHP
				enemyMaximum += actor.MaxHP
			}
		}
		if enemyMaximum <= 0 || !combatFinite(enemyCurrent) || enemyCurrent < 0 || enemyCurrent > enemyMaximum {
			return errors.New("觉醒怪物真实终局HP无效")
		}
		state.CurrentActor, err = proto.Marshal(currentActor)
		if err != nil {
			return err
		}
		if state.MonsterHP == nil {
			state.MonsterHP = map[string]float64{}
		}
		state.MonsterHP[fight.MonsterGUID] = enemyCurrent / enemyMaximum
		if state.Defeated == nil {
			state.Defeated = map[string]bool{}
		}
		var rewards []*pb.BaseResource
		if actual == 1 {
			state.Defeated[fight.MonsterGUID] = true
			state.Score += 80
			if loot := frozen.MonsterLoot[fight.MonsterGUID]; loot != "" {
				rewards, err = p1RewardResources(loot)
				if err != nil {
					return err
				}
			}
		}
		state.ActiveFight = ""
		state.LastFight = fight.ID
		refresh, err := p7AwakeRefresh(run, frozen, state)
		if err != nil {
			return err
		}
		mon := p7AwakeFindMonster(frozen, fight.MonsterGUID)
		if mon == nil {
			return errors.New("觉醒战斗原怪节点缺失")
		}
		mon.HpPercent = proto.Float64(state.MonsterHP[fight.MonsterGUID])
		reply = &pb.SCAwakeCopyFightEnd{IsWinServer: proto.Int32(actual), Rewards: rewards, RefreshData: refresh, FightMonster: mon, DropAngerBallNum: proto.Int32(0), DropFireBallNum: proto.Int32(0), AngerBallAdditionExp: proto.Int32(0), FireBallAdditionCoin: proto.Int32(0)}
		fight.Reply, err = proto.Marshal(reply)
		if err != nil {
			return err
		}
		fight.ReplyHash = p6Digest(fight.Reply)
		fight.Terminal, err = json.Marshal(terminal)
		if err != nil {
			return err
		}
		fight.TerminalHash = p6Digest(fight.Terminal)
		fight.EndRequest = requestRaw
		fight.EndHash = p6Digest(requestRaw)
		fight.Status = constUtil.P7AwakeFightAccepted
		fight.AcceptedAt = now.UnixMilli()
		outcome, err := json.Marshal(terminal.Outcome)
		if err != nil {
			return err
		}
		operates, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
		if err != nil {
			return err
		}
		result := p6PlaybackResult(terminal.Outcome.Winner)
		saved := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=? AND authoritative=?", record.ID, constUtil.P6BattleCaptured, false).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": operates, "outcome": outcome, "outcome_hash": p6Digest(outcome), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), operates...), byte(result))), "authoritative": true, "finished_at": now.UnixMilli()})
		if saved.Error != nil {
			return saved.Error
		}
		if saved.RowsAffected != 1 {
			return errors.New("觉醒战斗重复提交")
		}
		if err = tx.Save(&fight).Error; err != nil {
			return err
		}
		return p7AwakeSaveProgress(tx, ledger, state)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}
