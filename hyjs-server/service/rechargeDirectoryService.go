package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

const (
	rechargeDirectoryP1 = "p1"
	rechargeDirectoryP3 = "p3"
)

type rechargeDirectoryCandidate struct {
	domain, id, title     string
	start, end, rewardEnd int64
}

func rechargeDirectoryPrefix(id string) bool {
	return strings.HasPrefix(id, "rechargeTotal_") && id != constUtil.RechargeTotalDirectoryID
}

// The login binding is a session-scoped client declaration used only for UI
// compatibility. It does not replace the original authorization or receipts.
func rechargeDirectoryClientSupported(tx *gorm.DB, playerID int64) (bool, error) {
	if !clientUpdatesEnabled() || tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
		return false, nil
	}
	session, ok := tx.Statement.Context.Value(ggoNativeSessionKey{}).([]byte)
	if !ok || len(session) != 16 {
		return false, nil
	}
	var binding db.ClientUpdateSession
	err := tx.Table("client_update_sessions AS caps").
		Select("caps.player_id, caps.session_hash, caps.build_code, caps.client_version").
		Joins("JOIN profiles AS rp ON rp.id=caps.player_id").
		Where("caps.player_id=? AND caps.session_hash=? AND rp.session=?", playerID, clientUpdateSessionHash(session), session).
		Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return binding.BuildCode >= constUtil.RechargeTotalDirectoryMinBuild && ClientUpdateVersionValid(binding.ClientVersion) && !clientUpdateVersionBelow(binding.ClientVersion, constUtil.RechargeTotalDirectoryMinVersion), nil
}

// A reserved ID collision must fail closed before routing a combined request.
func rechargeDirectoryCheckReserved(tx *gorm.DB) error {
	for _, table := range []any{&db.PeriodInstance{}, &db.PaymentActivityRule{}} {
		if !gameSchemaHasTable(tx, table) {
			continue
		}
		var count int64
		if err := tx.Model(table).Where("id = ?", constUtil.RechargeTotalDirectoryID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return constUtil.ErrAdminConfigConflict
		}
	}
	return nil
}

// The list uses both the live switch and the player's already-frozen rule. No
// P3 state is created until the combined page actually observes the instance.
func (s *GameService) rechargeDirectoryCandidates(tx *gorm.DB, playerID, now int64) ([]rechargeDirectoryCandidate, error) {
	var result []rechargeDirectoryCandidate
	if s.P1Ready {
		var rows []db.PeriodInstance
		if err := tx.Where("enabled = ? AND id LIKE ? ESCAPE '!'", true, "rechargeTotal!_%").
			Where("owner_player_id IN ? AND realm_id IN ?", []int64{0, playerID}, []int64{0, s.OriginalServerID()}).
			Order("id ASC").Find(&rows).Error; err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		progress := map[string]db.PeriodProgress{}
		if len(ids) > 0 {
			var saved []db.PeriodProgress
			if err := tx.Where("player_id = ? AND instance_id IN ?", playerID, ids).Find(&saved).Error; err != nil {
				return nil, err
			}
			for _, row := range saved {
				progress[row.InstanceID] = row
			}
		}
		for _, live := range rows {
			if !rechargeDirectoryPrefix(live.ID) || !s.IsP1Activity(live.ID) {
				continue
			}
			allowed, err := localPlayActivityVisible(tx, live.ID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
			cfg, err := s.freezeP1Period(tx, live, false)
			if err != nil || cfg.Kind != "payment_amount" || now < cfg.StartAt || now >= cfg.RewardEndAt {
				continue
			}
			if saved, ok := progress[live.ID]; ok {
				var frozen db.PeriodInstance
				if json.Unmarshal(saved.Snapshot, &frozen) != nil || frozen.ID != cfg.ID || frozen.RewardSourceID != cfg.RewardSourceID || saved.Value < 0 || frozen.Kind != "payment_amount" || s.validateP1Period(frozen) != nil {
					continue
				}
				cfg = frozen
			}
			if now < cfg.StartAt || now >= cfg.RewardEndAt {
				continue
			}
			result = append(result, rechargeDirectoryCandidate{rechargeDirectoryP1, cfg.ID, cfg.Title, cfg.StartAt, cfg.EndAt, cfg.RewardEndAt})
		}
	}
	if !gameSchemaHasTable(tx, &db.PaymentActivityRule{}) || !gameSchemaHasTable(tx, &db.PaymentActivityState{}) {
		return result, nil
	}
	var rules []db.PaymentActivityRule
	// In a page/claim transaction, prevent an admin edit from changing the live
	// P3 switch between eligibility, first observation and the final receipt.
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("enabled = ? AND kind = ? AND start_at <= ? AND reward_end_at > ? AND id LIKE ? ESCAPE '!'", true, "total", now, now, "rechargeTotal!_%").Order("id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	states := map[string]db.PaymentActivityState{}
	if len(ids) > 0 {
		var saved []db.PaymentActivityState
		if err := tx.Where("player_id = ? AND activity_id IN ?", playerID, ids).Find(&saved).Error; err != nil {
			return nil, err
		}
		for _, row := range saved {
			states[row.ActivityID] = row
		}
	}
	for _, live := range rules {
		if !rechargeDirectoryPrefix(live.ID) || !utf8.ValidString(live.ID) || utf8.RuneCountInString(live.ID) > 128 || now < live.StartAt || now >= live.RewardEndAt {
			continue
		}
		allowed, err := localPlayActivityVisible(tx, live.ID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		cfg := live
		if saved, ok := states[live.ID]; ok {
			if json.Unmarshal([]byte(saved.Snapshot), &cfg) != nil {
				continue
			}
		}
		if cfg.ID != live.ID || cfg.Kind != "total" || !cfg.Enabled || cfg.StartAt <= 0 || cfg.StartAt >= cfg.EndAt || cfg.EndAt > cfg.RewardEndAt || cfg.Threshold <= 0 || cfg.Threshold > math.MaxInt32 || cfg.Source == "" || now < cfg.StartAt || now >= cfg.RewardEndAt {
			continue
		}
		if _, err := p3Resources(cfg.Reward); err != nil {
			continue
		}
		if supported, err := s.openServerRewardSupported(cfg.Reward); err != nil || !supported {
			continue
		}
		result = append(result, rechargeDirectoryCandidate{rechargeDirectoryP3, cfg.ID, cfg.Name, cfg.StartAt, cfg.EndAt, cfg.RewardEndAt})
	}
	return result, nil
}

// Replace only cumulative-recharge tabs; the other native categories keep
// their existing ordering and visibility.
func (s *GameService) rechargeDirectoryTabs(tx *gorm.DB, playerID, now int64, tabs []*pb.ActivityTab) ([]*pb.ActivityTab, error) {
	var hasRecharge bool
	for _, tab := range tabs {
		if strings.HasPrefix(tab.GetActivityId(), "rechargeTotal_") {
			hasRecharge = true
			break
		}
	}
	if !hasRecharge {
		return tabs, nil
	}
	supported, err := rechargeDirectoryClientSupported(tx, playerID)
	if err != nil {
		return nil, err
	}
	if !supported {
		return tabs, nil
	}
	if err := rechargeDirectoryCheckReserved(tx); err != nil {
		return nil, err
	}
	candidates, err := s.rechargeDirectoryCandidates(tx, playerID, now)
	if err != nil {
		return nil, err
	}
	eligible := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		eligible[candidate.id] = true
	}
	var merged *pb.ActivityTab
	out := make([]*pb.ActivityTab, 0, len(tabs))
	for _, tab := range tabs {
		id := tab.GetActivityId()
		if !strings.HasPrefix(id, "rechargeTotal_") {
			out = append(out, tab)
			continue
		}
		if !eligible[id] {
			continue
		}
		if merged == nil {
			merged = &pb.ActivityTab{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Name: proto.String("累计充值"), Order: proto.Int32(tab.GetOrder()), Status: proto.Int32(tab.GetStatus())}
			out = append(out, merged)
		} else if tab.GetStatus() > merged.GetStatus() {
			merged.Status = proto.Int32(tab.GetStatus())
		}
	}
	return out, nil
}

func rechargeDirectoryIndex(tx *gorm.DB, candidate rechargeDirectoryCandidate, child int32) (int32, error) {
	if (candidate.domain != rechargeDirectoryP1 && candidate.domain != rechargeDirectoryP3) || !rechargeDirectoryPrefix(candidate.id) || len(candidate.id) > 512 || child <= 0 {
		return 0, errors.New("累计充值目录绑定无效")
	}
	row := db.RechargeDirectoryMember{Domain: candidate.domain, InstanceID: candidate.id, ChildIndex: child}
	if err := tx.Where("domain = ? AND instance_id = ? AND child_index = ?", row.Domain, row.InstanceID, row.ChildIndex).First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return 0, err
		}
		if err = tx.Where("domain = ? AND instance_id = ? AND child_index = ?", candidate.domain, candidate.id, child).First(&row).Error; err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	if row.IndexID <= 0 || row.IndexID > math.MaxInt32 {
		return 0, errors.New("累计充值目录索引超限")
	}
	return int32(row.IndexID), nil
}

func rechargeDirectoryCaption(candidate rechargeDirectoryCandidate) string {
	zone := time.FixedZone("UTC+8", 8*3600)
	title := strings.TrimSpace(candidate.title)
	if title == "" {
		title = "累计充值"
	}
	return fmt.Sprintf("%s · 活动截止 %s · 领奖截止 %s", title, time.UnixMilli(candidate.end).In(zone).Format("2006-01-02 15:04:05"), time.UnixMilli(candidate.rewardEnd).In(zone).Format("2006-01-02 15:04:05"))
}

func (s *GameService) rechargeDirectoryInfo(tx *gorm.DB, playerID int64, player *pb.Player, game *model.GameState, candidates []rechargeDirectoryCandidate, now int64) (*pb.SCActivityInfo, error) {
	result := &pb.SCActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Param: proto.String("1"), Information: proto.String("各活动独立统计和领取，截止时间见每档说明。")}
	for _, candidate := range candidates {
		request := &pb.CSActivityInfo{ActivityId: proto.String(candidate.id)}
		var reply proto.Message
		var err error
		if candidate.domain == rechargeDirectoryP1 {
			reply, err = s.p1ActivityApply(tx, playerID, player, game, candidate.id, request, now)
		} else {
			reply, err = s.paymentActivityApply(tx, nil, playerID, player, game, request, time.UnixMilli(now), false)
		}
		if err != nil {
			return nil, err
		}
		child, ok := reply.(*pb.SCActivityInfo)
		if !ok {
			return nil, errors.New("累计充值子活动响应无效")
		}
		for _, item := range child.ItemList {
			index, err := rechargeDirectoryIndex(tx, candidate, item.GetIndex())
			if err != nil {
				return nil, err
			}
			item.Index = proto.Int32(index)
			if candidate.domain == rechargeDirectoryP3 {
				// P3's original direct page formats yuan; the combined page uses cents.
				item.Value = proto.String(strconv.FormatInt(int64(item.GetTotal()), 10))
			}
			item.Message = proto.String(rechargeDirectoryCaption(candidate))
			item.Param = proto.String(constUtil.RechargeTotalDirectoryItemParam)
			result.ItemList = append(result.ItemList, item)
		}
		if result.GetStartTime() == 0 || candidate.start < result.GetStartTime() {
			result.StartTime = proto.Int64(candidate.start)
		}
		if candidate.end > result.GetEndTime() {
			result.EndTime = proto.Int64(candidate.end)
		}
		if candidate.rewardEnd > result.GetRewardEndTime() {
			result.RewardEndTime = proto.Int64(candidate.rewardEnd)
		}
	}
	if len(result.ItemList) == 0 {
		return nil, errors.New("累计充值活动未开放或领奖截止")
	}
	return result, nil
}

func (s *GameService) rechargeDirectoryApply(tx *gorm.DB, playerID int64, player *pb.Player, game *model.GameState, request proto.Message, now int64) (proto.Message, error) {
	if err := rechargeDirectoryCheckReserved(tx); err != nil {
		return nil, err
	}
	supported, err := rechargeDirectoryClientSupported(tx, playerID)
	if err != nil {
		return nil, err
	}
	if !supported {
		return nil, errors.New("当前客户端不支持合并累计充值")
	}
	candidates, err := s.rechargeDirectoryCandidates(tx, playerID, now)
	if err != nil {
		return nil, err
	}
	info, err := s.rechargeDirectoryInfo(tx, playerID, player, game, candidates, now)
	if err != nil {
		return nil, err
	}
	claim, ok := request.(*pb.CSActivityReward)
	if !ok {
		return info, nil
	}
	var member db.RechargeDirectoryMember
	if err := tx.Where("index_id = ?", claim.GetIndex()).First(&member).Error; err != nil {
		return nil, errors.New("累计充值领奖选项不存在")
	}
	var selected *rechargeDirectoryCandidate
	for i := range candidates {
		if candidates[i].domain == member.Domain && candidates[i].id == member.InstanceID {
			selected = &candidates[i]
			break
		}
	}
	if selected == nil || member.ChildIndex <= 0 || !rechargeDirectoryPrefix(member.InstanceID) || (member.Domain != rechargeDirectoryP1 && member.Domain != rechargeDirectoryP3) {
		return nil, errors.New("累计充值领奖选项无效或已截止")
	}
	var item *pb.ActivityItem
	for _, current := range info.ItemList {
		if current.GetIndex() == claim.GetIndex() {
			item = current
			break
		}
	}
	if item == nil || !p1SameResources(item.RewardList, claim.GetGetResources()) || (member.Domain == rechargeDirectoryP3 && !p3ResourcesEqual(claim.GetGetResources(), item.RewardList)) {
		return nil, errors.New("累计充值领奖资源无效")
	}
	child := proto.Clone(claim).(*pb.CSActivityReward)
	child.ActivityId = proto.String(member.InstanceID)
	child.Index = proto.Int32(member.ChildIndex)
	gain := &pb.ResultObject{}
	// P3's historical direct endpoint rejects duplicate claims. The combined
	// page returns an empty delta while retaining the same frozen P3 receipt.
	if member.Domain != rechargeDirectoryP3 || item.GetStatus() != 2 {
		var reply proto.Message
		if member.Domain == rechargeDirectoryP1 {
			reply, err = s.p1ActivityApply(tx, playerID, player, game, member.InstanceID, child, now)
		} else {
			reply, err = s.paymentActivityApply(tx, nil, playerID, player, game, child, time.UnixMilli(now), false)
		}
		if err != nil {
			return nil, err
		}
		result, ok := reply.(*pb.SCActivityReward)
		if !ok {
			return nil, errors.New("累计充值领奖响应无效")
		}
		gain = result.GetGet()
	}
	info, err = s.rechargeDirectoryInfo(tx, playerID, player, game, candidates, now)
	if err != nil {
		return nil, err
	}
	return &pb.SCActivityReward{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), ItemList: info.ItemList, Get: gain}, nil
}
