package service

import (
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

type p7ExpeditionQuote struct {
	Grade       string                  `json:"grade,omitempty"`
	Hours       int32                   `json:"hours"`
	Energy      int32                   `json:"energy"`
	Exp         int32                   `json:"exp"`
	FailExp     int32                   `json:"failExp"`
	MaxHeroes   int32                   `json:"maxHeroes"`
	Reward      string                  `json:"reward"`
	Source      string                  `json:"source"`
	Events      *p7ExpeditionEventRules `json:"events,omitempty"`
	Success     *p7ExpeditionSuccess    `json:"success,omitempty"`
	SuccessRule string                  `json:"successRule,omitempty"`
}

type p7ExpeditionSuccessHero struct {
	GUID         string  `json:"guid"`
	HeroID       string  `json:"heroId"`
	Level        int32   `json:"level"`
	Talent       int32   `json:"talent"`
	Expertise    string  `json:"expertise"`
	Contribution float32 `json:"contribution"`
}

type p7ExpeditionSuccess struct {
	Rule        string                    `json:"rule"`
	Source      string                    `json:"source"`
	Grade       string                    `json:"grade"`
	MissionType string                    `json:"missionType"`
	Difficulty  int32                     `json:"difficulty"`
	MaxHeroes   int32                     `json:"maxHeroes"`
	Heroes      []p7ExpeditionSuccessHero `json:"heroes"`
	Chance      float32                   `json:"chance"`
	Draw        int64                     `json:"draw"`
	Succeeded   bool                      `json:"succeeded"`
}

// p7ExpeditionSuccessChance 保留原客户端float逐步运算及缺员除最大人数的行为。
func p7ExpeditionSuccessChance(missionType string, difficulty, maximum int32, heroes []p7ExpeditionSuccessHero) (float32, error) {
	if maximum < 1 || maximum > 3 || difficulty < 0 || difficulty > 100 || len(heroes) > int(maximum) {
		return 0, errors.New("远征原成功率档位无效")
	}
	switch missionType {
	case "guard", "hunt", "collect", "kill", "war", "search":
	default:
		return 0, errors.New("远征原任务专长类型无效")
	}
	if len(heroes) == 0 {
		return 0, nil
	}
	var total float32
	seen := map[string]bool{}
	for i := range heroes {
		hero := &heroes[i]
		if hero.GUID == "" || hero.HeroID == "" || seen[hero.GUID] || hero.Level < 1 || hero.Talent < 0 {
			return 0, errors.New("远征成功率伙伴快照无效")
		}
		seen[hero.GUID] = true
		var bonus float32
		for _, key := range strings.Split(hero.Expertise, ",") {
			if key == missionType {
				bonus = 0.5
			}
		}
		hero.Contribution = (float32(hero.Level)+float32(hero.Talent)*5)/100 + bonus - float32(difficulty)/100
		total += hero.Contribution
	}
	return min(float32(1), max(float32(0.1), total/float32(maximum))), nil
}

func (s *GameService) p7FreezeExpeditionSuccess(mission db.P7ExpeditionMission, quote p7ExpeditionQuote, selected []*pb.Warrior) (*p7ExpeditionSuccess, error) {
	row := s.table("ExpeditionMission", "missionId", mission.MissionID)
	if row == nil || row["level"] != quote.Grade {
		return nil, errors.New("远征任务档位与冻结报价不一致")
	}
	difficulty, err := p7Integer(s.table("ExpeditionLevel", "level", quote.Grade)["difficulty"], 0, 100)
	if err != nil {
		return nil, err
	}
	frozen := &p7ExpeditionSuccess{Rule: constUtil.P7ExpeditionSuccessRule, Source: constUtil.P7ExpeditionSuccessSource, Grade: quote.Grade, MissionType: row["missionType"], Difficulty: difficulty, MaxHeroes: quote.MaxHeroes}
	for _, hero := range selected {
		frozen.Heroes = append(frozen.Heroes, p7ExpeditionSuccessHero{GUID: hero.GetGuid(), HeroID: hero.GetId(), Level: hero.GetLevel(), Talent: hero.GetTalent(), Expertise: s.table("Hero", "id", hero.GetId())["expeditionSkill"]})
	}
	frozen.Chance, err = p7ExpeditionSuccessChance(frozen.MissionType, frozen.Difficulty, frozen.MaxHeroes, frozen.Heroes)
	if err != nil {
		return nil, err
	}
	draw, err := rand.Int(rand.Reader, big.NewInt(constUtil.P7ExpeditionSuccessDrawRange))
	if err != nil {
		return nil, err
	}
	frozen.Draw = draw.Int64()
	frozen.Succeeded = float64(frozen.Draw)/float64(constUtil.P7ExpeditionSuccessDrawRange) < float64(frozen.Chance)
	return frozen, nil
}

// 只验接受时冻结的资料，后续等级/原表调整不会重算旧任务或改变一次随机结果。
func p7ValidateExpeditionSuccess(quote p7ExpeditionQuote) error {
	frozen := quote.Success
	if frozen == nil {
		return nil
	}
	if quote.SuccessRule != constUtil.P7ExpeditionSuccessRule || frozen.Rule != constUtil.P7ExpeditionSuccessRule || frozen.Source != constUtil.P7ExpeditionSuccessSource || frozen.Grade != quote.Grade || frozen.MaxHeroes != quote.MaxHeroes || frozen.Draw < 0 || frozen.Draw >= constUtil.P7ExpeditionSuccessDrawRange || len(frozen.Heroes) < 1 {
		return errors.New("远征冻结成功率来源无效")
	}
	heroes := append([]p7ExpeditionSuccessHero(nil), frozen.Heroes...)
	chance, err := p7ExpeditionSuccessChance(frozen.MissionType, frozen.Difficulty, frozen.MaxHeroes, heroes)
	if err != nil || chance != frozen.Chance || frozen.Succeeded != (float64(frozen.Draw)/float64(constUtil.P7ExpeditionSuccessDrawRange) < float64(chance)) {
		return errors.New("远征冻结成功率结果冲突")
	}
	for i := range heroes {
		if heroes[i].Contribution != frozen.Heroes[i].Contribution {
			return errors.New("远征冻结伙伴成功率贡献冲突")
		}
	}
	return nil
}

func (s *GameService) LoadP7ExpeditionTables(root string) error {
	for _, name := range []string{"ExpeditionMission", "ExpeditionLevel", "ExpeditionStory"} {
		if err := s.loadP7Table(root, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) InitializeP7Expedition(ctx context.Context) error {
	if !s.P1Ready || len(s.Tables["ExpeditionMission"]) == 0 || len(s.Tables["ExpeditionLevel"]) == 0 || len(s.Tables["ExpeditionStory"]) == 0 {
		return errors.New("远征原表与P1尚未初始化")
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P7ExpeditionPolicy{}, &db.P7ExpeditionState{}, &db.P7ExpeditionMission{}, &db.P7ExpeditionHero{}); err != nil {
		return err
	}
	if err := tx.AutoMigrate(&db.P7RushReceipt{}); err != nil {
		return err
	}
	if err := s.initializeP7ExpeditionEvents(tx); err != nil {
		return err
	}
	if err := tx.AutoMigrate(&db.P7ExpeditionCataloguePolicy{}); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P7ExpeditionCataloguePolicy{ID: constUtil.P7ExpeditionCataloguePolicyID, Source: constUtil.P7ExpeditionCatalogueSource}).Error; err != nil {
		return err
	}
	rewardID := s.table("ExpeditionStory", "storyId", "stroy_001")["rewardId"]
	if rewardID == "" {
		return errors.New("远征原常规故事资源缺失")
	}
	cfg := db.P7ExpeditionPolicy{ID: constUtil.P7ExpeditionPolicyID, Enabled: false, Reward: rewardID + ",1", Source: constUtil.P7ExpeditionSource}
	if ok, err := s.openServerRewardSupported(cfg.Reward); err != nil || !ok {
		return errors.New("远征常规奖励资源未恢复")
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

func (s *GameService) p7ExpeditionPolicy(tx *gorm.DB, p *pb.Player, requireEnabled bool) (db.P7ExpeditionPolicy, error) {
	var cfg db.P7ExpeditionPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, constUtil.P7ExpeditionPolicyID).Error; err != nil {
		return cfg, err
	}
	if !requireEnabled {
		return cfg, nil
	}
	if !cfg.Enabled {
		return cfg, errors.New(constUtil.P7ExpeditionClosed)
	}
	level, err := p7Integer(s.table("GameConfigure", "StaticName", "SailingLevel")["NameValue"], 1, math.MaxInt32)
	if err != nil {
		return cfg, err
	}
	if p.GetLevel() < level {
		return cfg, errors.New("角色尚未达到原32级远征开放条件")
	}
	if ok, err := s.openServerRewardSupported(cfg.Reward); err != nil || !ok {
		return cfg, errors.New("远征奖励未恢复")
	}
	return cfg, nil
}

func (s *GameService) p7ExpeditionState(tx *gorm.DB, id int64, now time.Time) (*db.P7ExpeditionState, error) {
	state := &db.P7ExpeditionState{PlayerID: id, Level: "D", Day: p1Day(LocalPlayAt(tx.Statement.Context, now))}
	err := tx.Where("player_id=?", id).First(state).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if state.Exp < 0 || state.RefreshUsed < 0 || s.table("ExpeditionLevel", "level", state.Level) == nil {
		return nil, errors.New("远征等级或次数存档无效")
	}
	if err := refreshP7ExpeditionCalendar(tx, state, now); err != nil {
		return nil, err
	}
	return state, saveP7ExpeditionCalendar(tx, state, now)
}

func (s *GameService) p7ExpeditionSetting(name string) (int32, error) {
	return p7Integer(s.table("GameConfigure", "StaticName", name)["NameValue"], 0, math.MaxInt32)
}

func (s *GameService) p7ExpeditionQuote(row map[string]string, cfg db.P7ExpeditionPolicy) (p7ExpeditionQuote, error) {
	return s.p7ExpeditionQuoteForCatalogue(row, cfg, false)
}

func (s *GameService) p7ExpeditionQuoteForCatalogue(row map[string]string, cfg db.P7ExpeditionPolicy, advanced bool) (p7ExpeditionQuote, error) {
	var quote p7ExpeditionQuote
	if row == nil || (row["level"] != "D" && !advanced) {
		return quote, errors.New("仅恢复原D级常规远征")
	}
	grade := row["level"]
	if grade != "D" && grade != "C" && grade != "B" && grade != "A" && grade != "S" {
		return quote, errors.New("特殊远征等级尚未恢复")
	}
	hours, err := p7Integer(row["time"], 4, 12)
	if err != nil || hours != 4 && hours != 8 && hours != 12 {
		return quote, errors.New("远征时长未恢复")
	}
	energy, err := s.p7ExpeditionSetting("ExpeditionEnergyCostPerHour")
	if err != nil {
		return quote, err
	}
	rate, err := p7Integer(s.table("ExpeditionLevel", "level", grade)["expPerHour"], 1, 10000)
	if err != nil {
		return quote, err
	}
	heroes, err := p7Integer(s.table("ExpeditionLevel", "level", grade)["maxHeroNum"], 1, 3)
	if err != nil {
		return quote, err
	}
	fail, err := s.p7ExpeditionSetting("ExpeditionFailExp")
	if err != nil || fail > 100 {
		return quote, errors.New("原远征失败经验比例无效")
	}
	if int64(hours)*int64(energy) > math.MaxInt32 {
		return quote, errors.New("远征耐力成本溢出")
	}
	return p7ExpeditionQuote{Grade: grade, Hours: hours, Energy: hours * energy, Exp: hours * rate, FailExp: hours * rate * fail / 100, MaxHeroes: heroes, Reward: cfg.Reward, Source: cfg.Source, SuccessRule: constUtil.P7ExpeditionSuccessRule}, nil
}

func (s *GameService) p7CreateExpeditionOffers(tx *gorm.DB, id int64, cfg db.P7ExpeditionPolicy) error {
	// Read the actual earned rank; each published quote remains independent of
	// later rank changes, policy closure or reward edits.
	var policy db.P7ExpeditionCataloguePolicy
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&policy, constUtil.P7ExpeditionCataloguePolicyID).Error; err != nil {
		return err
	}
	grade, slots := "D", int32(2)
	if policy.Enabled {
		if strings.TrimSpace(policy.Source) == "" {
			return errors.New("远征等级投放来源缺失")
		}
		var state db.P7ExpeditionState
		if err := tx.Where("player_id = ?", id).First(&state).Error; err != nil {
			return err
		}
		grade = state.Level
		if grade != "D" && grade != "C" && grade != "B" && grade != "A" && grade != "S" {
			return errors.New("特殊远征等级尚未恢复")
		}
		var err error
		slots, err = p7Integer(s.table("ExpeditionLevel", "level", grade)["maxMissionNum"], 1, 6)
		if err != nil {
			return err
		}
		cfg.Source += "; " + policy.Source
	}
	weights := map[int32]int64{}
	for _, entry := range strings.Split(s.table("GameConfigure", "StaticName", "ExpeditionTimeRate")["NameValue"], ";") {
		pair := strings.Split(entry, ",")
		if len(pair) != 2 {
			return errors.New("原远征时间权重无效")
		}
		h, err := strconv.ParseInt(pair[0], 10, 32)
		if err != nil {
			return err
		}
		weight, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || weight < 1 {
			return errors.New("原远征权重无效")
		}
		weights[int32(h)] = weight
	}
	var candidates []map[string]string
	for _, row := range s.Tables["ExpeditionMission"] {
		if row["level"] == grade {
			candidates = append(candidates, row)
		}
	}
	if len(candidates) < int(slots) {
		return errors.New("原等级远征内容不足")
	}
	for slot := int32(0); slot < slots; slot++ {
		var total int64
		for _, row := range candidates {
			quote, err := s.p7ExpeditionQuoteForCatalogue(row, cfg, policy.Enabled)
			if err != nil {
				return err
			}
			weight := weights[quote.Hours]
			if weight < 1 || total > math.MaxInt64-weight {
				return errors.New("远征抽取权重溢出")
			}
			total += weight
		}
		pick, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return err
		}
		cursor := pick.Int64()
		selected := -1
		for i, row := range candidates {
			quote, err := s.p7ExpeditionQuoteForCatalogue(row, cfg, policy.Enabled)
			if err != nil {
				return err
			}
			cursor -= weights[quote.Hours]
			if cursor < 0 {
				selected = i
				break
			}
		}
		if selected < 0 {
			return errors.New("远征发布抽取失败")
		}
		row := candidates[selected]
		quote, err := s.p7ExpeditionQuoteForCatalogue(row, cfg, policy.Enabled)
		if err != nil {
			return err
		}
		quote.Events, err = s.p7FreezeExpeditionEventRules(tx, quote.Reward)
		if err != nil {
			return err
		}
		if quote.Events != nil {
			quote.Source += "; " + quote.Events.Source
		}
		raw, err := json.Marshal(quote)
		if err != nil {
			return err
		}
		var random [16]byte
		if _, err = rand.Read(random[:]); err != nil {
			return err
		}
		mission := db.P7ExpeditionMission{Serial: "p7e-" + hex.EncodeToString(random[:]), PlayerID: id, MissionID: row["missionId"], State: constUtil.P7ExpeditionOffered, Quote: raw, QuoteHash: p6Digest(raw)}
		if err = tx.Create(&mission).Error; err != nil {
			return err
		}
		candidates = append(candidates[:selected], candidates[selected+1:]...)
	}
	return nil
}

func p7ReadExpeditionQuote(m db.P7ExpeditionMission) (p7ExpeditionQuote, error) {
	var quote p7ExpeditionQuote
	if p6Digest(m.Quote) != m.QuoteHash || json.Unmarshal(m.Quote, &quote) != nil || quote.Hours < 4 || quote.Hours > 12 || quote.Energy < 0 || quote.Exp < 1 || quote.MaxHeroes < 1 || quote.MaxHeroes > 3 || quote.Reward == "" {
		return quote, errors.New("远征冻结报价损坏")
	}
	return quote, p7ValidateExpeditionSuccess(quote)
}

func p7ProjectExpedition(m db.P7ExpeditionMission, now time.Time) (*pb.ExpeditionMissionInfo, error) {
	q, err := p7ReadExpeditionQuote(m)
	if err != nil {
		return nil, err
	}
	var heroes []string
	if len(m.Heroes) > 0 && json.Unmarshal(m.Heroes, &heroes) != nil {
		return nil, errors.New("远征派遣存档损坏")
	}
	state := m.State
	if state == constUtil.P7ExpeditionRunning && now.UnixMilli() >= m.EndAt {
		state = constUtil.P7ExpeditionReady
	}
	preview, err := p7ExpeditionPreviewIDs(q.Reward)
	if err != nil {
		return nil, err
	}
	// Original task descriptions accept ID lists, while the frozen ledger and
	// final settlement keep ID,quantity. Explicit empty optional strings avoid
	// the original unconditional Split on a null rare-reward field.
	return &pb.ExpeditionMissionInfo{MissionSerial: proto.String(m.Serial), MissionId: proto.String(m.MissionID), MainHeroId: proto.String(m.MainHeroID), State: proto.Int32(state), StartTime: proto.Int64(m.StartAt), EndTime: proto.Int64(m.EndAt), HeroGuids: heroes, Rewards: proto.String(preview), HeroReward: proto.String(""), BigReward: proto.String(""), Exp: proto.Int32(q.Exp)}, nil
}

func p7ExpeditionPreviewIDs(reward string) (string, error) {
	var ids []string
	seen := map[string]bool{}
	for _, entry := range strings.Split(reward, ";") {
		pair := strings.Split(entry, ",")
		if len(pair) != 2 || pair[0] == "" {
			return "", errors.New("远征冻结奖励预览格式无效")
		}
		n, err := strconv.ParseInt(pair[1], 10, 64)
		if err != nil || n < 1 {
			return "", errors.New("远征冻结奖励预览数量无效")
		}
		if !seen[pair[0]] {
			ids = append(ids, pair[0])
			seen[pair[0]] = true
		}
	}
	return strings.Join(ids, ";"), nil
}

// GuardP7ExpeditionSave 由主代理共享保存钩子调用，派遣中伙伴不能被成长/装备变更或回收绕过。
func (s *GameService) GuardP7ExpeditionSave(tx *gorm.DB, id int64, g *model.GameState) error {
	if !gameSchemaHasTable(tx, &db.P7ExpeditionHero{}) {
		return nil
	}
	var holds []db.P7ExpeditionHero
	if err := tx.Where("player_id=?", id).Find(&holds).Error; err != nil {
		return err
	}
	for _, hold := range holds {
		old := &pb.Warrior{}
		if proto.Unmarshal(hold.Snapshot, old) != nil || old.GetGuid() != hold.GUID {
			return errors.New("远征伙伴占用快照损坏")
		}
		found := false
		for _, hero := range g.Warriors {
			if hero.GetGuid() == hold.GUID {
				found = true
				if !proto.Equal(old, hero) {
					return errors.New("派遣中的伙伴请先完成远征并领取奖励")
				}
				break
			}
		}
		if !found {
			return errors.New("派遣伙伴尚未释放，不能回收或消耗")
		}
	}
	return nil
}

func p7ExpeditionInFormation(g *model.GameState, guid string) bool {
	for _, slot := range g.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetPos() == 0 && thing.GetGuid() == guid {
				return true
			}
		}
	}
	return false
}

// p7Expedition 以服务端时钟恢复离线计时，接收/刷新/领奖沿原协议和角色行锁回执。
func (s *GameService) p7Expedition(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var result proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var domain string
		switch request.(type) {
		case *pb.CSAcceptMission:
			domain = "exp-accept"
			result = &pb.SCAcceptMission{}
		case *pb.CSRefreshMissionList:
			domain = "exp-refresh"
			result = &pb.SCRefreshMissionList{}
		case *pb.CSGetFinalReward:
			domain = "exp-final"
			result = &pb.SCGetFinalReward{}
		}
		if domain != "" {
			if found, err := p7ReadReceipt(tx, id, domain, stamp, request, result); err != nil || found {
				return err
			}
		}
		// Restore only this player's existing accepted task after publication
		// closes. Recovery reads neither publish offers nor accept new tasks.
		_, finalClaim := request.(*pb.CSGetFinalReward)
		recoveryRead := false
		var recoveryCount int64
		switch b := request.(type) {
		case *pb.CSInterExpedition, *pb.CSGetAcceptedMissions:
			if err := tx.Model(&db.P7ExpeditionMission{}).Where("player_id=? AND state IN ?", id, []int32{constUtil.P7ExpeditionRunning, constUtil.P7ExpeditionReady, constUtil.P7ExpeditionClaimed}).Count(&recoveryCount).Error; err != nil {
				return err
			}
		case *pb.CSExpeditionMissionDetail:
			if err := tx.Model(&db.P7ExpeditionMission{}).Where("serial=? AND player_id=? AND state IN ?", b.GetMissionSerial(), id, []int32{constUtil.P7ExpeditionRunning, constUtil.P7ExpeditionReady, constUtil.P7ExpeditionClaimed}).Count(&recoveryCount).Error; err != nil {
				return err
			}
		case *pb.CSGetExpeditionIdleHeros:
			if err := tx.Model(&db.P7ExpeditionMission{}).Where("serial=? AND player_id=? AND state IN ?", b.GetMissionSerial(), id, []int32{constUtil.P7ExpeditionRunning, constUtil.P7ExpeditionReady, constUtil.P7ExpeditionClaimed}).Count(&recoveryCount).Error; err != nil {
				return err
			}
		}
		recoveryRead = recoveryCount > 0
		cfg, err := s.p7ExpeditionPolicy(tx, p, !finalClaim && !recoveryRead)
		if err != nil {
			return err
		}
		state, err := s.p7ExpeditionState(tx, id, now)
		if err != nil {
			return err
		}
		free, err := s.p7ExpeditionSetting("ExpeditionRefreshFreeTime")
		if err != nil {
			return err
		}
		var offers int64
		if err = tx.Model(&db.P7ExpeditionMission{}).Where("player_id=? AND state=?", id, constUtil.P7ExpeditionOffered).Count(&offers).Error; err != nil {
			return err
		}
		if offers == 0 && !finalClaim && !recoveryRead {
			if err = s.p7CreateExpeditionOffers(tx, id, cfg); err != nil {
				return err
			}
		}
		switch b := request.(type) {
		case *pb.CSInterExpedition:
			result = &pb.SCInterExpedition{Level: proto.String(state.Level), Exp: proto.Int32(state.Exp), FreeRefreshTime: proto.Int32(max(0, free-state.RefreshUsed))}
		case *pb.CSGetAcceptedMissions, *pb.CSGetRefreshedMissions, *pb.CSRefreshMissionList:
			cost := &pb.BaseResource{}
			if _, ok := request.(*pb.CSRefreshMissionList); ok {
				if state.RefreshUsed >= free {
					cost, err = s.p4ParseCost(s.table("GameConfigure", "StaticName", "ExpeditionRefreshCost")["NameValue"])
					if err != nil {
						return err
					}
					if err = deductPoolCost(p, g, cost); err != nil {
						return err
					}
					if err = s.RecordP1Cost(tx, id, fmt.Sprintf("exp-refresh:%d", stamp), []*pb.BaseResource{cost}, LocalPlayAt(ctx, now)); err != nil {
						return err
					}
				}
				if state.RefreshUsed == math.MaxInt32 {
					return errors.New("远征刷新计数溢出")
				}
				state.RefreshUsed++
				if err = tx.Where("player_id=? AND state=?", id, constUtil.P7ExpeditionOffered).Delete(&db.P7ExpeditionMission{}).Error; err != nil {
					return err
				}
				if err = s.p7CreateExpeditionOffers(tx, id, cfg); err != nil {
					return err
				}
			}
			var rows []db.P7ExpeditionMission
			query := tx.Where("player_id=?", id)
			if _, ok := request.(*pb.CSGetAcceptedMissions); ok {
				// 优先保留仍占用伙伴的任务，避免长期游玩后被已领奖历史挤出原列表。
				// 历史仅限制本次展示，原任务和冻结领奖回执继续保留。
				query = query.Where("state IN ?", []int32{constUtil.P7ExpeditionRunning, constUtil.P7ExpeditionReady, constUtil.P7ExpeditionClaimed}).Order("CASE WHEN state = 3 THEN 1 ELSE 0 END").Order("start_at DESC")
			} else {
				query = query.Where("state=?", 0)
			}
			if err = query.Order("serial").Limit(50).Find(&rows).Error; err != nil {
				return err
			}
			var list []*pb.ExpeditionMissionInfo
			for _, row := range rows {
				item, err := s.p7ProjectExpeditionCalendar(tx, row, now)
				if err != nil {
					return err
				}
				list = append(list, item)
			}
			if _, ok := request.(*pb.CSGetAcceptedMissions); ok {
				result = &pb.SCGetAcceptedMissions{AcceptedMissions: list}
			} else if _, ok := request.(*pb.CSGetRefreshedMissions); ok {
				result = &pb.SCGetRefreshedMissions{RefreshedMissions: list}
			} else {
				response := &pb.SCRefreshMissionList{FreeRefreshTime: proto.Int32(max(0, free-state.RefreshUsed)), RefreshedMissions: list}
				if cost.GetNum() > 0 {
					response.Cost = cost
				}
				result = response
			}
		case *pb.CSAcceptMission:
			var mission db.P7ExpeditionMission
			if err = tx.Where("serial=? AND player_id=?", b.GetMissionSerial(), id).First(&mission).Error; err != nil {
				return err
			}
			if mission.State != 0 {
				return errors.New("远征任务已接受或已完成")
			}
			q, err := p7ReadExpeditionQuote(mission)
			if err != nil {
				return err
			}
			if len(b.GetHeroGuids()) < 1 || len(b.GetHeroGuids()) > int(q.MaxHeroes) {
				return errors.New("派遣伙伴数超出冻结原等级额度")
			}
			limit, err := p7Integer(s.table("ExpeditionLevel", "level", state.Level)["maxMissionNum"], 1, 6)
			if err != nil {
				return err
			}
			var active int64
			if err = tx.Model(&db.P7ExpeditionMission{}).Where("player_id=? AND state IN ?", id, []int32{1, 2}).Count(&active).Error; err != nil {
				return err
			}
			if active >= int64(limit) {
				return errors.New("远征同时派遣数量已满")
			}
			seen := map[string]bool{}
			var selected []*pb.Warrior
			for _, guid := range b.GetHeroGuids() {
				if seen[guid] || guid == p.GetHeroWarId() {
					return errors.New("远征伙伴重复或选中主角")
				}
				seen[guid] = true
				var hero *pb.Warrior
				for _, w := range g.Warriors {
					if w.GetGuid() == guid {
						hero = w
						break
					}
				}
				if hero == nil || strings.HasPrefix(hero.GetId(), "hero_exp_") || s.table("Hero", "id", hero.GetId()) == nil {
					return errors.New("远征伙伴不存在或属于经验材料")
				}
				selected = append(selected, hero)
				if mission.MainHeroID == "" {
					mission.MainHeroID = hero.GetId()
				}
				var busy int64
				if err = tx.Model(&db.P7ExpeditionHero{}).Where("player_id=? AND guid=?", id, guid).Count(&busy).Error; err != nil {
					return err
				}
				if busy > 0 {
					return errors.New("伙伴已在其他远征中")
				}
				snapshot, err := proto.Marshal(hero)
				if err != nil {
					return err
				}
				if err = tx.Create(&db.P7ExpeditionHero{PlayerID: id, GUID: guid, MissionSerial: mission.Serial, Snapshot: snapshot}).Error; err != nil {
					return err
				}
			}
			if p.GetEnergy() < q.Energy {
				return errors.New("远征耐力不足")
			}
			// 新报价显式登记成功率版本；旧报价/已接受任务的nil合同保持原样。
			if q.SuccessRule != "" {
				if q.SuccessRule != constUtil.P7ExpeditionSuccessRule {
					return errors.New("远征成功率报价版本无效")
				}
				q.Success, err = s.p7FreezeExpeditionSuccess(mission, q, selected)
				if err != nil {
					return err
				}
			}
			mission.Quote, err = json.Marshal(q)
			if err != nil {
				return err
			}
			mission.QuoteHash = p6Digest(mission.Quote)
			p.Energy = proto.Int32(p.GetEnergy() - q.Energy)
			mission.Heroes, err = json.Marshal(b.GetHeroGuids())
			if err != nil {
				return err
			}
			mission.StartAt = now.UnixMilli()
			mission.EndAt = now.Add(time.Duration(q.Hours) * time.Hour).UnixMilli()
			mission.State = constUtil.P7ExpeditionRunning
			if err = tx.Save(&mission).Error; err != nil {
				return err
			}
			if err = freezeP7ExpeditionCalendar(tx, mission, now); err != nil {
				return err
			}
			if err = s.p7CreateExpeditionEvents(tx, mission, q); err != nil {
				return err
			}
			if err = s.RecordP1Event(tx, id, "exp-accept:"+mission.Serial, "expedition_accept", 1, LocalPlayAt(ctx, now)); err != nil {
				return err
			}
			result = &pb.SCAcceptMission{}
		case *pb.CSGetExpeditionIdleHeros:
			var mission db.P7ExpeditionMission
			if err = tx.Where("serial=? AND player_id=?", b.GetMissionSerial(), id).First(&mission).Error; err != nil {
				return err
			}
			q, err := p7ReadExpeditionQuote(mission)
			if err != nil {
				return err
			}
			var holds []db.P7ExpeditionHero
			if err = tx.Where("player_id=?", id).Find(&holds).Error; err != nil {
				return err
			}
			busy := map[string]bool{}
			for _, hold := range holds {
				busy[hold.GUID] = true
			}
			var idle, selected []string
			// Match the original panel: every valid companion is either available
			// or shown as executing. Formation membership does not block dispatch.
			for _, hero := range g.Warriors {
				if hero.GetGuid() == p.GetHeroWarId() || strings.HasPrefix(hero.GetId(), "hero_exp_") || s.table("Hero", "id", hero.GetId()) == nil {
					continue
				}
				if busy[hero.GetGuid()] {
					selected = append(selected, hero.GetGuid())
				} else {
					idle = append(idle, hero.GetGuid())
				}
			}
			sort.Strings(idle)
			sort.Strings(selected)
			preview, err := p7ExpeditionPreviewIDs(q.Reward)
			if err != nil {
				return err
			}
			result = &pb.SCGetExpeditionIdleHeros{CanSendGuids: idle, SendGuids: selected, PossibleReward: proto.String(preview), BigReward: proto.String("")}
		case *pb.CSExpeditionMissionDetail:
			var mission db.P7ExpeditionMission
			if err = tx.Where("serial=? AND player_id=?", b.GetMissionSerial(), id).First(&mission).Error; err != nil {
				return err
			}
			q, err := p7ReadExpeditionQuote(mission)
			if err != nil {
				return err
			}
			events, err := s.p7ExpeditionEvents(tx, mission, now)
			if err != nil {
				return err
			}
			response := &pb.SCExpeditionMissionDetail{FailExp: proto.Int32(q.FailExp)}
			for _, event := range events {
				item, err := s.p7ProjectExpeditionCalendarEvent(tx, mission, event)
				if err != nil {
					return err
				}
				response.Events = append(response.Events, item)
			}
			result = response
		case *pb.CSGetFinalReward:
			var mission db.P7ExpeditionMission
			if err = tx.Where("serial=? AND player_id=?", b.GetMissionSerial(), id).First(&mission).Error; err != nil {
				return err
			}
			if mission.State == constUtil.P7ExpeditionClaimed {
				response := &pb.SCGetFinalReward{}
				if err = proto.Unmarshal(mission.Result, response); err != nil {
					return err
				}
				result = response
				break
			}
			missionNow, err := s.p7ExpeditionMissionNow(tx, mission, now)
			if err != nil {
				return err
			}
			if mission.State != constUtil.P7ExpeditionRunning && mission.State != constUtil.P7ExpeditionReady || missionNow.UnixMilli() < mission.EndAt {
				return errors.New("远征尚未完成或任务已失败")
			}
			q, err := p7ReadExpeditionQuote(mission)
			if err != nil {
				return err
			}
			events, err := s.p7ExpeditionEvents(tx, mission, now)
			if err != nil {
				return err
			}
			failed := q.Success != nil && !q.Success.Succeeded
			for _, event := range events {
				if event.Kind == constUtil.P7ExpeditionRiskEvent && event.ResolvedAt == 0 {
					failed = true
				}
			}
			earnedExp := q.Exp
			if failed {
				earnedExp = q.FailExp
			}
			if state.Exp > math.MaxInt32-earnedExp {
				return errors.New("远征经验溢出")
			}
			gain := &pb.ResultObject{}
			if !failed {
				rewards := []string{q.Reward}
				for _, event := range events {
					if event.Kind == constUtil.P7ExpeditionNormalEvent || event.Kind == constUtil.P7ExpeditionLuckyEvent || event.Kind == constUtil.P7ExpeditionHelpEvent {
						rewards = append(rewards, event.Reward)
					}
				}
				gain, err = s.GrantFixedReward(p, g, strings.Join(rewards, ";"))
				if err != nil {
					return err
				}
			} else {
				serial, err := p7NewExpeditionEventSerial()
				if err != nil {
					return err
				}
				failure := &db.P7ExpeditionEvent{Serial: serial, MissionSerial: mission.Serial, PlayerID: id, StoryID: "stroy_105", Kind: constUtil.P7ExpeditionFailEvent, EventAt: mission.EndAt, Reward: "nothing,0", ResolvedAt: missionNow.UnixMilli()}
				if err = p7SaveExpeditionEvent(tx, failure, true); err != nil {
					return err
				}
			}
			state.Exp += earnedExp
			for {
				row := s.table("ExpeditionLevel", "level", state.Level)
				if row == nil {
					return errors.New("远征等级表缺失")
				}
				if row["nextLevel"] == "-1" {
					break
				}
				threshold, err := p7Integer(row["exp"], 1, math.MaxInt32)
				if err != nil {
					return err
				}
				if state.Exp < threshold {
					break
				}
				state.Exp -= threshold
				state.Level = row["nextLevel"]
			}
			if err = tx.Where("player_id=? AND mission_serial=?", id, mission.Serial).Delete(&db.P7ExpeditionHero{}).Error; err != nil {
				return err
			}
			result = &pb.SCGetFinalReward{Get: gain, Level: proto.String(state.Level), Exp: proto.Int32(state.Exp)}
			mission.State = constUtil.P7ExpeditionClaimed
			mission.Result, err = proto.Marshal(result)
			if err != nil {
				return err
			}
			if err = tx.Save(&mission).Error; err != nil {
				return err
			}
			kind := "expedition_complete"
			if failed {
				kind = "expedition_failed"
			}
			if err = s.RecordP1Event(tx, id, "exp-finish:"+mission.Serial, kind, 1, LocalPlayAt(ctx, now)); err != nil {
				return err
			}
		case *pb.CSFriendBadEvents, *pb.CSHelpFriendEvent:
			return errors.New("远征风险与好友帮助待P10真实关系/事件联动")
		default:
			return errors.New("远征请求尚未恢复")
		}
		if err = saveP7ExpeditionCalendar(tx, state, now); err != nil {
			return err
		}
		if domain != "" {
			return p7SaveReceipt(tx, id, domain, stamp, request, result)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	replies := []proto.Message{result}
	switch request.(type) {
	case *pb.CSAcceptMission, *pb.CSInterExpedition:
		// Publish the current absolute balance after commit, including receipt replay.
		// The original UI resets its recovery countdown from SCPlayer, without a local debit.
		current, _, readErr := s.resourceProfile(ctx, id)
		if readErr != nil {
			return nil, readErr
		}
		replies = append(replies, &pb.SCPlayer{Player: current})
	}
	return replies, nil
}

func (s *GameService) DispatchP7Expedition(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSInterExpedition, *pb.CSGetAcceptedMissions, *pb.CSGetRefreshedMissions, *pb.CSAcceptMission, *pb.CSExpeditionMissionDetail, *pb.CSRefreshMissionList, *pb.CSGetFinalReward, *pb.CSGetExpeditionIdleHeros, *pb.CSFriendBadEvents, *pb.CSHelpFriendEvent:
		now := time.Now()
		if at, ok := ctx.Value(P7ExpeditionClockContextKey{}).(time.Time); ok && !at.IsZero() {
			now = at
		}
		r, e := s.p7Expedition(ctx, id, request, stamp, now)
		return r, true, e
	}
	return nil, false, nil
}
