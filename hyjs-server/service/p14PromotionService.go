package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type p14PromotionRules struct {
	GrantHour        int64  `json:"grantHour"`
	DailyPoints      int64  `json:"dailyPoints"`
	MaxPoints        int64  `json:"maxPoints"`
	InitialScore     int64  `json:"initialScore"`
	FirstGrantSource string `json:"firstGrantSource"`
}

func p14PromotionDecode(raw, hash string) (*p14PromotionRules, error) {
	var rules p14PromotionRules
	if p6Digest([]byte(raw)) != hash || json.Unmarshal([]byte(raw), &rules) != nil || rules.GrantHour != 12 || rules.DailyPoints != 20 || rules.MaxPoints != 20 || rules.InitialScore != 1 || rules.FirstGrantSource != constUtil.P14PromotionFirstGrantSource {
		return nil, errors.New("原晋级赛匹配点冻结规则不符")
	}
	return &rules, nil
}

// InitializeP14Promotion 冻结原12点/20点/初始1分，未知首次发点边界具名且独立默认关闭。
func (s *GameService) InitializeP14Promotion(ctx context.Context) error {
	values := make(map[string]int64)
	for _, key := range []string{"kuafuGiveMatchPointTime", "kuafuGiveMatchPoint", "kuafuInitScore"} {
		row := s.table("GameConfigure", "StaticName", key)
		if row == nil {
			return errors.New("原晋级赛匹配点配置缺失")
		}
		value, err := strconv.ParseInt(row["NameValue"], 10, 64)
		if err != nil {
			return err
		}
		values[key] = value
	}
	rules := p14PromotionRules{GrantHour: values["kuafuGiveMatchPointTime"], DailyPoints: values["kuafuGiveMatchPoint"], MaxPoints: 20, InitialScore: values["kuafuInitScore"], FirstGrantSource: constUtil.P14PromotionFirstGrantSource}
	raw, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	hash := p6Digest(raw)
	if _, err = p14PromotionDecode(string(raw), hash); err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14PromotionPolicy{}, &db.P14PromotionState{}, &db.P14PromotionGrant{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14PromotionPolicy{ID: 1, Source: constUtil.P14PromotionSource, RulesJSON: string(raw), RulesHash: hash}).Error
}
func p14PromotionStateHash(row *db.P14PromotionState) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14PromotionGrantHash(row *db.P14PromotionGrant) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// p14PromotionNextNoon 首次取不早于真实阶段和资格的12点；延期观察补足既到期节点，余额仍封顶20。
func p14PromotionNextNoon(state *db.P14PromotionState, rules *p14PromotionRules) time.Time {
	location := time.FixedZone("UTC+8", 8*3600)
	if state.LastGrantAt > 0 {
		return time.UnixMilli(state.LastGrantAt).In(location).AddDate(0, 0, 1)
	}
	eligible := time.UnixMilli(state.EligibleAt).In(location)
	at := time.Date(eligible.Year(), eligible.Month(), eligible.Day(), int(rules.GrantHour), 0, 0, 0, location)
	if at.Before(eligible) {
		at = at.AddDate(0, 0, 1)
	}
	return at
}

// p14PromotionVerifyGrants 从零点账重建所有原应发时刻，丢账/改余额不能被下一次查询掩盖。
func p14PromotionVerifyGrants(tx *gorm.DB, state *db.P14PromotionState, rules *p14PromotionRules, now time.Time) error {
	var grants []db.P14PromotionGrant
	if err := tx.Where("season_index=? AND server_id=? AND player_id=?", state.SeasonIndex, state.ServerID, state.PlayerID).Order("grant_at").Find(&grants).Error; err != nil {
		return err
	}
	rebuilt := *state
	rebuilt.Points = 0
	rebuilt.LastGrantAt = 0
	rebuilt.UpdatedAt = rebuilt.CreatedAt
	rebuilt.SourceHash = p14PromotionStateHash(&rebuilt)
	for _, grant := range grants {
		next := p14PromotionNextNoon(&rebuilt, rules).UnixMilli()
		if grant.SourceHash != p14PromotionGrantHash(&grant) || grant.GrantAt != next || grant.GrantAt >= state.StageEnd || grant.ObservedAt < grant.GrantAt || grant.ObservedAt > now.UnixMilli() || grant.RulesHash != state.RulesHash || grant.QualificationHash != state.QualificationHash || grant.PointsBefore != rebuilt.Points || grant.StateBeforeHash != rebuilt.SourceHash {
			return errors.New("晋级赛真实匹配点发放账目损坏或缺口")
		}
		rebuilt.Points += rules.DailyPoints
		if rebuilt.Points > rules.MaxPoints {
			rebuilt.Points = rules.MaxPoints
		}
		rebuilt.LastGrantAt = grant.GrantAt
		rebuilt.UpdatedAt = grant.ObservedAt
		rebuilt.SourceHash = p14PromotionStateHash(&rebuilt)
		if grant.PointsAfter != rebuilt.Points || grant.StateAfterHash != rebuilt.SourceHash {
			return errors.New("晋级赛匹配点与封顶后存档不符")
		}
	}
	if rebuilt.SourceHash != state.SourceHash {
		return errors.New("晋级赛匹配点缺少真实发放来源")
	}
	return nil
}

// p14PromotionStatusTX 原阶段6查询只承认真实本服资格；既有非权威对局不能产生资格、计分或扣匹配点。
func (s *GameService) p14PromotionStatusTX(ctx context.Context, tx *gorm.DB, policy *db.P14SeasonPolicy, id int64, now time.Time) (int32, int32, error) {
	var gate db.P14PromotionPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
		return 0, 0, err
	}
	if !gate.Enabled {
		return 0, 0, nil
	}
	if gate.Source != constUtil.P14PromotionSource {
		return 0, 0, errors.New("晋级赛匹配点规则来源尚未发布")
	}
	if _, err := p14PromotionDecode(gate.RulesJSON, gate.RulesHash); err != nil {
		return 0, 0, err
	}
	qualified, err := s.p14AuditionQualificationRead(ctx, tx, policy.SeasonIndex, s.OriginalServerID())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	var ids []int64
	if json.Unmarshal(qualified.Players, &ids) != nil {
		return 0, 0, errors.New("晋级赛本服真实资格名单损坏")
	}
	eligible := false
	for _, playerID := range ids {
		if playerID == id {
			eligible = true
		}
	}
	if !eligible {
		return 0, 0, nil
	}
	boundary, _, err := p14AuditionRead(tx, policy.SeasonIndex, s.OriginalServerID())
	if err != nil {
		return 0, 0, err
	}
	slots, err := s.p14SeasonCalendar(tx, policy.SeasonIndex)
	if err != nil {
		return 0, 0, err
	}
	var stage *db.P14TimeSlot
	for i := range slots {
		if slots[i].Stage == constUtil.P14PromotionStage {
			if stage != nil {
				return 0, 0, errors.New("晋级赛原阶段重复")
			}
			stage = &slots[i]
		}
	}
	if stage == nil || stage.EndAt-stage.StartAt > constUtil.P14PromotionMaxStageMillis || now.UnixMilli() < stage.StartAt || now.UnixMilli() >= stage.EndAt {
		return 0, 0, errors.New("当前不在真实原晋级赛窗口")
	}
	eligibleAt := stage.StartAt
	if qualified.QualifiedAt > eligibleAt {
		eligibleAt = qualified.QualifiedAt
	}
	if eligibleAt >= stage.EndAt {
		return 0, 0, errors.New("晋级资格产生晚于原晋级赛截止")
	}
	var state db.P14PromotionState
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "season_index=? AND server_id=? AND player_id=?", policy.SeasonIndex, s.OriginalServerID(), id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		state = db.P14PromotionState{SeasonIndex: policy.SeasonIndex, ServerID: s.OriginalServerID(), PlayerID: id, QualificationHash: qualified.SourceHash, CalendarHash: boundary.CalendarHash, StageStart: stage.StartAt, StageEnd: stage.EndAt, EligibleAt: eligibleAt, RulesJSON: gate.RulesJSON, RulesHash: gate.RulesHash, Score: 1, CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli()}
		state.SourceHash = p14PromotionStateHash(&state)
		if err := tx.Create(&state).Error; err != nil {
			return 0, 0, err
		}
	} else if err != nil {
		return 0, 0, err
	}
	if state.SourceHash != p14PromotionStateHash(&state) || state.QualificationHash != qualified.SourceHash || state.CalendarHash != boundary.CalendarHash || state.StageStart != stage.StartAt || state.StageEnd != stage.EndAt || state.EligibleAt != eligibleAt || state.Score != 1 || state.Points < 0 || state.Points > 20 || state.CreatedAt < eligibleAt || state.CreatedAt > now.UnixMilli() {
		return 0, 0, errors.New("晋级赛匹配点来源/余额/初始分损坏")
	}
	rules, err := p14PromotionDecode(state.RulesJSON, state.RulesHash)
	if err != nil {
		return 0, 0, err
	}
	if err := p14PromotionVerifyGrants(tx, &state, rules, now); err != nil {
		return 0, 0, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		at := p14PromotionNextNoon(&state, rules)
		if at.UnixMilli() > now.UnixMilli() || at.UnixMilli() >= state.StageEnd {
			break
		}
		grant := db.P14PromotionGrant{SeasonIndex: state.SeasonIndex, ServerID: state.ServerID, PlayerID: state.PlayerID, GrantAt: at.UnixMilli(), ObservedAt: now.UnixMilli(), PointsBefore: state.Points, RulesHash: state.RulesHash, QualificationHash: state.QualificationHash, StateBeforeHash: state.SourceHash}
		state.Points += rules.DailyPoints
		if state.Points > rules.MaxPoints {
			state.Points = rules.MaxPoints
		}
		state.LastGrantAt = at.UnixMilli()
		state.UpdatedAt = now.UnixMilli()
		state.SourceHash = p14PromotionStateHash(&state)
		grant.PointsAfter = state.Points
		grant.StateAfterHash = state.SourceHash
		grant.SourceHash = p14PromotionGrantHash(&grant)
		if err := tx.Create(&grant).Error; err != nil {
			return 0, 0, err
		}
		if err := tx.Save(&state).Error; err != nil {
			return 0, 0, err
		}
	}
	points, _, err := s.p14PromotionFightBalanceTX(tx, &state, now)
	return 1, int32(points), err
}
