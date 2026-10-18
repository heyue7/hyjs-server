package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p7ExpeditionEventRules struct {
	RiskPercent int32                   `json:"riskPercent"`
	HelpReward  string                  `json:"helpReward"`
	Source      string                  `json:"source"`
	Stories     *p7ExpeditionStoryRules `json:"stories,omitempty"`
}

type p7ExpeditionStoryChoice struct {
	StoryID string `json:"storyId"`
	Reward  string `json:"reward"`
}

type p7ExpeditionStoryRules struct {
	BaseReward   string                    `json:"baseReward"`
	LuckyPercent int32                     `json:"luckyPercent"`
	Ordinary     []p7ExpeditionStoryChoice `json:"ordinary"`
	Lucky        []p7ExpeditionStoryChoice `json:"lucky"`
	Source       string                    `json:"source"`
}

// P7ExpeditionPendingHelp 是真实已到时、未解除的派遣事件引用。
type P7ExpeditionPendingHelp struct {
	OwnerID       int64
	MissionSerial string
	EventSerial   string
	StoryID       string
	EventHash     string
}

// P7ExpeditionClockContextKey 供隔离测试main注入同一有界时钟，正式请求不设置。
type P7ExpeditionClockContextKey struct{}

func (s *GameService) initializeP7ExpeditionEvents(tx *gorm.DB) error {
	if err := tx.AutoMigrate(&db.P7ExpeditionEventPolicy{}, &db.P7ExpeditionEvent{}, &db.P7ExpeditionStoryPolicy{}); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P7ExpeditionStoryPolicy{ID: constUtil.P7ExpeditionStoryPolicyID, LuckyPercent: 20, Source: constUtil.P7ExpeditionStorySource}).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P7ExpeditionEventPolicy{ID: constUtil.P7ExpeditionEventPolicyID, RiskPercent: 20, HelpReward: "energy,2", Source: constUtil.P7ExpeditionEventSource}).Error
}

func (s *GameService) p7FreezeExpeditionEventRules(tx *gorm.DB, reward string) (*p7ExpeditionEventRules, error) {
	var policy db.P7ExpeditionEventPolicy
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&policy, constUtil.P7ExpeditionEventPolicyID).Error; err != nil {
		return nil, err
	}
	if !policy.Enabled {
		return nil, nil
	}
	rules := &p7ExpeditionEventRules{RiskPercent: policy.RiskPercent, HelpReward: policy.HelpReward, Source: policy.Source}
	var stories db.P7ExpeditionStoryPolicy
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&stories, constUtil.P7ExpeditionStoryPolicyID).Error; err != nil {
		return nil, err
	}
	if stories.Enabled {
		cost, err := s.p4ParseCost(reward)
		if err != nil || cost.GetNum() < 1 || cost.GetNum() > math.MaxInt32/2 {
			return nil, errors.New("远征剧情冻结奖励数量无效")
		}
		pool := &p7ExpeditionStoryRules{BaseReward: reward, LuckyPercent: stories.LuckyPercent, Source: stories.Source}
		for _, row := range s.Tables["ExpeditionStory"] {
			if row["rewardId"] != cost.GetId() {
				continue
			}
			if row["storyId"] >= "stroy_001" && row["storyId"] <= "stroy_073" {
				pool.Ordinary = append(pool.Ordinary, p7ExpeditionStoryChoice{StoryID: row["storyId"], Reward: reward})
			} else if row["storyId"] >= "stroy_074" && row["storyId"] <= "stroy_092" {
				pool.Lucky = append(pool.Lucky, p7ExpeditionStoryChoice{StoryID: row["storyId"], Reward: cost.GetId() + "," + strconv.FormatInt(int64(cost.GetNum())*2, 10)})
			}
		}
		sort.Slice(pool.Ordinary, func(i, j int) bool { return pool.Ordinary[i].StoryID < pool.Ordinary[j].StoryID })
		sort.Slice(pool.Lucky, func(i, j int) bool { return pool.Lucky[i].StoryID < pool.Lucky[j].StoryID })
		rules.Stories = pool
		rules.Source += "; " + pool.Source
	}
	if err := s.validateP7ExpeditionEventRules(rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (s *GameService) validateP7ExpeditionEventRules(rules *p7ExpeditionEventRules) error {
	if rules == nil {
		return nil
	}
	if rules.RiskPercent < 0 || rules.RiskPercent > 100 || rules.Source == "" {
		return errors.New("远征事件发布参数无效")
	}
	cost, err := s.p4ParseCost(rules.HelpReward)
	if err != nil || cost.GetId() != "energy" || cost.GetNum() < 1 || cost.GetNum() > 100 {
		return errors.New("远征好友协助冻结数量无效")
	}
	if pool := rules.Stories; pool != nil {
		base, err := s.p4ParseCost(pool.BaseReward)
		if err != nil || base.GetNum() < 1 || base.GetNum() > math.MaxInt32/2 || pool.Source == "" || pool.LuckyPercent < 0 || pool.LuckyPercent > 100 || len(pool.Ordinary) < 1 || len(pool.Ordinary) > 73 || len(pool.Lucky) < 1 || len(pool.Lucky) > 19 {
			return errors.New("远征冻结剧情池无效")
		}
		seen := map[string]bool{}
		for _, group := range []struct {
			choices     []p7ExpeditionStoryChoice
			first, last string
			quantity    int32
		}{{pool.Ordinary, "stroy_001", "stroy_073", base.GetNum()}, {pool.Lucky, "stroy_074", "stroy_092", base.GetNum() * 2}} {
			for _, choice := range group.choices {
				row := s.table("ExpeditionStory", "storyId", choice.StoryID)
				if seen[choice.StoryID] || choice.StoryID < group.first || choice.StoryID > group.last || row == nil || row["rewardId"] != base.GetId() || choice.Reward != base.GetId()+","+strconv.FormatInt(int64(group.quantity), 10) {
					return errors.New("远征冻结剧情与原奖励身份不符")
				}
				if ok, err := s.openServerRewardSupported(choice.Reward); err != nil || !ok {
					return errors.New("远征冻结剧情奖励不支持")
				}
				seen[choice.StoryID] = true
			}
		}
	}
	return nil
}

func p7ExpeditionEventHash(event db.P7ExpeditionEvent) string {
	event.SourceHash = ""
	raw, _ := json.Marshal(event)
	return p6Digest(raw)
}

func p7NewExpeditionEventSerial() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "p7ev-" + hex.EncodeToString(value[:]), nil
}

func p7SaveExpeditionEvent(tx *gorm.DB, event *db.P7ExpeditionEvent, create bool) error {
	event.SourceHash = p7ExpeditionEventHash(*event)
	if create {
		return tx.Create(event).Error
	}
	return tx.Save(event).Error
}

func (s *GameService) p7CreateExpeditionEvents(tx *gorm.DB, mission db.P7ExpeditionMission, quote p7ExpeditionQuote) error {
	if quote.Events == nil {
		return nil
	}
	if err := s.validateP7ExpeditionEventRules(quote.Events); err != nil {
		return err
	}
	if quote.Events.Stories != nil && quote.Events.Stories.BaseReward != quote.Reward {
		return errors.New("远征剧情基础奖与任务冻结报价不符")
	}
	for hour := int32(1); hour < quote.Hours; hour++ {
		roll, err := rand.Int(rand.Reader, big.NewInt(100))
		if err != nil {
			return err
		}
		kind, story, reward := constUtil.P7ExpeditionNormalEvent, "stroy_001", quote.Reward
		if roll.Int64() < int64(quote.Events.RiskPercent) {
			pick, err := rand.Int(rand.Reader, big.NewInt(6))
			if err != nil {
				return err
			}
			kind, story, reward = constUtil.P7ExpeditionRiskEvent, "stroy_"+strconv.Itoa(98+int(pick.Int64())), "nothing,0"
			if pick.Int64() < 2 {
				story = "stroy_0" + strconv.Itoa(98+int(pick.Int64()))
			}
		} else if pool := quote.Events.Stories; pool != nil {
			choices := pool.Ordinary
			lucky, err := rand.Int(rand.Reader, big.NewInt(100))
			if err != nil {
				return err
			}
			if lucky.Int64() < int64(pool.LuckyPercent) {
				choices, kind = pool.Lucky, constUtil.P7ExpeditionLuckyEvent
			}
			pick, err := rand.Int(rand.Reader, big.NewInt(int64(len(choices))))
			if err != nil {
				return err
			}
			story, reward = choices[pick.Int64()].StoryID, choices[pick.Int64()].Reward
		}
		if s.table("ExpeditionStory", "storyId", story) == nil {
			return errors.New("远征原事件剧情不存在")
		}
		serial, err := p7NewExpeditionEventSerial()
		if err != nil {
			return err
		}
		event := &db.P7ExpeditionEvent{Serial: serial, MissionSerial: mission.Serial, PlayerID: mission.PlayerID, StoryID: story, Kind: kind, EventAt: mission.StartAt + int64(hour)*time.Hour.Milliseconds(), Reward: reward}
		if err := p7SaveExpeditionEvent(tx, event, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) p7ExpeditionEvents(tx *gorm.DB, mission db.P7ExpeditionMission, now time.Time) ([]db.P7ExpeditionEvent, error) {
	quote, err := p7ReadExpeditionQuote(mission)
	if err != nil {
		return nil, err
	}
	if quote.Events == nil {
		// 新成功率任务可在无小时事件策略时失败，结算后仍展示原失败故事。
		// 旧nil任务保持旧投影，不为其补建事件或重新抽成败。
		if quote.Success == nil || quote.Success.Succeeded || mission.State != constUtil.P7ExpeditionClaimed {
			return nil, nil
		}
		var events []db.P7ExpeditionEvent
		if err := tx.Where("player_id=? AND mission_serial=?", mission.PlayerID, mission.Serial).Find(&events).Error; err != nil {
			return nil, err
		}
		if len(events) != 1 {
			return nil, errors.New("远征基础失败故事缺失")
		}
		event := events[0]
		if event.SourceHash != p7ExpeditionEventHash(event) || event.Kind != constUtil.P7ExpeditionFailEvent || event.StoryID != "stroy_105" || event.EventAt != mission.EndAt || event.Reward != "nothing,0" || event.ParentSerial != "" || event.HelperID != 0 || event.ResolvedAt < mission.EndAt {
			return nil, errors.New("远征基础失败故事与冻结任务不符")
		}
		return events, nil
	}
	if mission.State == constUtil.P7ExpeditionOffered {
		return nil, nil
	}
	if err := s.validateP7ExpeditionEventRules(quote.Events); err != nil {
		return nil, err
	}
	if quote.Events.Stories != nil && quote.Events.Stories.BaseReward != quote.Reward {
		return nil, errors.New("远征剧情基础奖与任务冻结报价不符")
	}
	var events []db.P7ExpeditionEvent
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND mission_serial=?", mission.PlayerID, mission.Serial).Order("serial").Find(&events).Error; err != nil {
		return nil, err
	}
	if len(events) > int(quote.Hours)*2+1 {
		return nil, errors.New("远征事件数量损坏")
	}
	var base int
	slots := map[int64]bool{}
	risk := map[string]db.P7ExpeditionEvent{}
	helped := map[string]bool{}
	for _, event := range events {
		if event.SourceHash != p7ExpeditionEventHash(event) || event.EventAt <= mission.StartAt || event.EventAt > mission.EndAt || event.ResolvedAt < 0 || event.HelperID < 0 || s.table("ExpeditionStory", "storyId", event.StoryID) == nil {
			return nil, errors.New("远征事件来源或时刻损坏")
		}
		if event.Kind == constUtil.P7ExpeditionNormalEvent || event.Kind == constUtil.P7ExpeditionLuckyEvent || event.Kind == constUtil.P7ExpeditionRiskEvent {
			offset := event.EventAt - mission.StartAt
			if offset%time.Hour.Milliseconds() != 0 || offset >= mission.EndAt-mission.StartAt || slots[offset] || event.ParentSerial != "" {
				return nil, errors.New("远征冻结事件时槽损坏")
			}
			slots[offset] = true
			if event.Kind != constUtil.P7ExpeditionRiskEvent {
				match := event.Kind == constUtil.P7ExpeditionNormalEvent && event.StoryID == "stroy_001" && event.Reward == quote.Reward
				if pool := quote.Events.Stories; pool != nil {
					match = false
					choices := pool.Ordinary
					if event.Kind == constUtil.P7ExpeditionLuckyEvent {
						choices = pool.Lucky
					}
					for _, choice := range choices {
						match = match || choice.StoryID == event.StoryID && choice.Reward == event.Reward
					}
					if event.Kind == constUtil.P7ExpeditionLuckyEvent && pool.LuckyPercent == 0 || event.Kind == constUtil.P7ExpeditionNormalEvent && pool.LuckyPercent == 100 {
						match = false
					}
				}
				if !match || event.HelperID != 0 || event.ResolvedAt != 0 {
					return nil, errors.New("远征常规事件与冻结报价不符")
				}
			} else {
				if event.StoryID < "stroy_098" || event.StoryID > "stroy_103" || event.Reward != "nothing,0" || (event.HelperID == 0) != (event.ResolvedAt == 0) || event.ResolvedAt > mission.EndAt || event.ResolvedAt > 0 && event.ResolvedAt < event.EventAt {
					return nil, errors.New("远征风险事件或解除时刻损坏")
				}
				risk[event.Serial] = event
			}
			base++
		} else if event.Kind != constUtil.P7ExpeditionHelpEvent && event.Kind != constUtil.P7ExpeditionFailEvent {
			return nil, errors.New("远征事件类型损坏")
		} else if event.Kind == constUtil.P7ExpeditionHelpEvent {
			if event.StoryID != "stroy_104" || event.Reward != quote.Events.HelpReward || event.ParentSerial == "" || helped[event.ParentSerial] || event.HelperID < 1 || event.ResolvedAt != event.EventAt {
				return nil, errors.New("远征协助事件与冻结报价不符")
			}
			helped[event.ParentSerial] = true
		} else if event.StoryID != "stroy_105" || event.Reward != "nothing,0" || event.EventAt != mission.EndAt || event.HelperID != 0 || event.ParentSerial != "" || event.ResolvedAt < mission.EndAt || mission.State != constUtil.P7ExpeditionClaimed {
			return nil, errors.New("远征失败事件与真实截止状态不符")
		}
	}
	for _, event := range events {
		if event.Kind == constUtil.P7ExpeditionHelpEvent {
			parent, ok := risk[event.ParentSerial]
			if !ok || parent.HelperID != event.HelperID || parent.ResolvedAt != event.EventAt {
				return nil, errors.New("远征协助事件没有对应已解除风险")
			}
		}
	}
	for serial, event := range risk {
		if (event.ResolvedAt > 0) != helped[serial] {
			return nil, errors.New("远征风险与协助记录不完整")
		}
	}
	if base != int(quote.Hours)-1 {
		return nil, errors.New("远征已接受任务的事件缺失")
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].EventAt != events[j].EventAt {
			return events[i].EventAt < events[j].EventAt
		}
		return events[i].Serial < events[j].Serial
	})
	missionNow, err := s.p7ExpeditionMissionNow(tx, mission, now)
	if err != nil {
		return nil, err
	}
	visible := events[:0]
	for _, event := range events {
		if event.EventAt <= missionNow.UnixMilli() {
			visible = append(visible, event)
		}
	}
	return visible, nil
}

func (s *GameService) ListP7ExpeditionHelp(tx *gorm.DB, ownerIDs []int64, now time.Time) ([]P7ExpeditionPendingHelp, error) {
	if len(ownerIDs) > 1000 {
		return nil, errors.New("远征帮助候选超过上限")
	}
	ids := append([]int64(nil), ownerIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var pending []P7ExpeditionPendingHelp
	for index, id := range ids {
		if id < 1 || index > 0 && id == ids[index-1] {
			return nil, errors.New("远征帮助所有者列表无效")
		}
		var missions []db.P7ExpeditionMission
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND state=?", id, constUtil.P7ExpeditionRunning).Order("serial").Find(&missions).Error; err != nil {
			return nil, err
		}
		for _, mission := range missions {
			missionNow, err := s.p7ExpeditionMissionNow(tx, mission, now)
			if err != nil {
				return nil, err
			}
			if missionNow.UnixMilli() >= mission.EndAt {
				continue
			}
			events, err := s.p7ExpeditionEvents(tx, mission, now)
			if err != nil {
				return nil, err
			}
			for _, event := range events {
				if event.Kind == constUtil.P7ExpeditionRiskEvent && event.ResolvedAt == 0 {
					pending = append(pending, P7ExpeditionPendingHelp{OwnerID: id, MissionSerial: mission.Serial, EventSerial: event.Serial, StoryID: event.StoryID, EventHash: event.SourceHash})
				}
			}
		}
	}
	return pending, nil
}

// ApplyP7ExpeditionHelp 由已在同tx验证真实好友权限的P10消费端调用。
func (s *GameService) ApplyP7ExpeditionHelp(tx *gorm.DB, helperID, ownerID int64, missionSerial, eventSerial string, now time.Time) (*P7ExpeditionPendingHelp, error) {
	if helperID < 1 || ownerID < 1 || helperID == ownerID {
		return nil, errors.New("远征帮助双方身份无效")
	}
	var identities int64
	if err := tx.Model(&db.Profile{}).Where("id IN ? AND player IS NOT NULL AND game IS NOT NULL", []int64{helperID, ownerID}).Count(&identities).Error; err != nil {
		return nil, err
	}
	if identities != 2 {
		return nil, errors.New("远征帮助双方真实角色不存在")
	}
	var mission db.P7ExpeditionMission
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("serial=? AND player_id=?", missionSerial, ownerID).First(&mission).Error; err != nil {
		return nil, err
	}
	missionNow, err := s.p7ExpeditionMissionNow(tx, mission, now)
	if err != nil {
		return nil, err
	}
	if mission.State != constUtil.P7ExpeditionRunning || missionNow.UnixMilli() >= mission.EndAt {
		return nil, errors.New("远征已截止或任务状态不允许帮助")
	}
	events, err := s.p7ExpeditionEvents(tx, mission, now)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if event.Serial != eventSerial {
			continue
		}
		if event.Kind != constUtil.P7ExpeditionRiskEvent || event.ResolvedAt != 0 {
			return nil, errors.New("远征事件不可帮助或已被解除")
		}
		out := &P7ExpeditionPendingHelp{OwnerID: ownerID, MissionSerial: mission.Serial, EventSerial: event.Serial, StoryID: event.StoryID, EventHash: event.SourceHash}
		event.ResolvedAt, event.HelperID = missionNow.UnixMilli(), helperID
		if err := p7SaveExpeditionEvent(tx, &event, false); err != nil {
			return nil, err
		}
		quote, err := p7ReadExpeditionQuote(mission)
		if err != nil || quote.Events == nil {
			return nil, errors.New("远征帮助冻结规则缺失")
		}
		serial, err := p7NewExpeditionEventSerial()
		if err != nil {
			return nil, err
		}
		help := &db.P7ExpeditionEvent{Serial: serial, MissionSerial: mission.Serial, PlayerID: ownerID, StoryID: "stroy_104", Kind: constUtil.P7ExpeditionHelpEvent, EventAt: missionNow.UnixMilli(), Reward: quote.Events.HelpReward, HelperID: helperID, ResolvedAt: missionNow.UnixMilli(), ParentSerial: event.Serial}
		if err := p7SaveExpeditionEvent(tx, help, true); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, errors.New("远征事件未到时或不属于该任务")
}

func p7ProjectExpeditionEvent(event db.P7ExpeditionEvent) *pb.ExpeditionStoryEvent {
	return &pb.ExpeditionStoryEvent{StoryId: proto.String(event.StoryID), EventType: proto.Int32(event.Kind), EventTime: proto.Int64(event.EventAt), Reward: proto.String(event.Reward)}
}
