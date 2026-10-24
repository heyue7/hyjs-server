package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeP13GuildDonateBox installs an opt-in rule without granting historic donation points.
func (s *GameService) InitializeP13GuildDonateBox(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13GuildDonateBoxPolicy{}, &db.P13GuildDonateBoxDay{}, &db.P13GuildDonateBoxSource{}, &db.P13GuildDonateBoxClaim{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13GuildDonateBoxPolicy{ID: 1, Version: constUtil.P13GuildDonateBoxVersion, Source: constUtil.P13GuildDonateBoxSource}).Error
}

func p13GuildDonateBoxRule(ordinal int64) (int32, string, error) {
	thresholds := [...]int32{250, 400, 500, 600}
	rewards := [...]string{"coin,1000", "coin,2000", "coin,3000", "coin,5000"}
	if ordinal < 1 || ordinal > constUtil.P13GuildDonateBoxCount {
		return 0, "", errors.New("公会捐献宝箱档位无效")
	}
	return thresholds[ordinal-1], rewards[ordinal-1], nil
}

func p13GuildDonateBoxID(guild, day string, ordinal int64) string {
	return p6Digest([]byte(fmt.Sprintf("%s/%s/%s/%d", constUtil.P13GuildDonateBoxVersion, guild, day, ordinal)))
}

func p13GuildDonateBoxRules() string {
	var rules []struct {
		Threshold int32  `json:"threshold"`
		Reward    string `json:"reward"`
	}
	for ordinal := int64(1); ordinal <= constUtil.P13GuildDonateBoxCount; ordinal++ {
		threshold, reward, _ := p13GuildDonateBoxRule(ordinal)
		rules = append(rules, struct {
			Threshold int32  `json:"threshold"`
			Reward    string `json:"reward"`
		}{threshold, reward})
	}
	raw, _ := json.Marshal(rules)
	return string(raw)
}

// p13GuildDonateBoxReadRules 读取已冻结四档，旧空覆盖继续使用原规则。
func p13GuildDonateBoxReadRules(raw string) ([]model.AdminGuildDonateBoxTier, error) {
	if raw == "" {
		raw = p13GuildDonateBoxRules()
	}
	var tiers []model.AdminGuildDonateBoxTier
	if len(raw) > 16384 || json.Unmarshal([]byte(raw), &tiers) != nil || len(tiers) != int(constUtil.P13GuildDonateBoxCount) {
		return nil, errors.New("公会捐献宝箱档位无效")
	}
	var previous int32
	for _, tier := range tiers {
		if tier.Threshold <= previous || tier.Reward == "" {
			return nil, errors.New("公会捐献宝箱积分必须递增且奖励不能为空")
		}
		if _, err := p1RewardResources(tier.Reward); err != nil {
			return nil, err
		}
		previous = tier.Threshold
	}
	return tiers, nil
}
func p13GuildDonateBoxFrozenRule(day *db.P13GuildDonateBoxDay, ordinal int64) (int32, string, error) {
	tiers, err := p13GuildDonateBoxReadRules(day.RulesJSON)
	if err != nil || ordinal < 1 || ordinal > int64(len(tiers)) {
		return 0, "", errors.New("公会捐献宝箱冻结档位无效")
	}
	return tiers[ordinal-1].Threshold, tiers[ordinal-1].Reward, nil
}

func p13GuildDonateBoxSourceHash(source db.P13GuildDonateBoxSource) string {
	source.SourceHash = ""
	raw, _ := json.Marshal(source)
	return p6Digest(raw)
}

func p13GuildDonateBoxPolicy(tx *gorm.DB, now time.Time) (*db.P13GuildDonateBoxPolicy, error) {
	// Isolated legacy domains can omit this optional schema; the main initializer creates it.
	if !gameSchemaHasTable(tx, &db.P13GuildDonateBoxPolicy{}) {
		return nil, nil
	}
	var policy db.P13GuildDonateBoxPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return nil, err
	}
	if !policy.Enabled {
		return nil, nil
	}
	if policy.Version != constUtil.P13GuildDonateBoxVersion || policy.Source != constUtil.P13GuildDonateBoxSource || policy.PublishedAt <= 0 || policy.PublishedAt > now.UnixMilli() {
		return nil, errors.New("公会捐献宝箱缺提前发布的具名规则")
	}
	return &policy, nil
}

// RecordP13GuildDonateBox runs after the original donation debit, under the guild/profile locks.
// Disabling the local policy stops new score; already earned daily boxes remain claimable.
func (s *GameService) RecordP13GuildDonateBox(tx *gorm.DB, member *db.P13GuildMember, request *pb.CSUnionDonate, stamp int64, amount, score int32, now time.Time) error {
	policy, err := p13GuildDonateBoxPolicy(tx, now)
	if err != nil || policy == nil {
		return err
	}
	if member.GuildID == "" || member.JoinedAt <= 0 || member.JoinedAt > now.UnixMilli() || stamp <= 0 || amount <= 0 || score <= 0 || request.GetDonateType() != "coin" && request.GetDonateType() != "badge" {
		return errors.New("公会捐献宝箱实际捐献来源无效")
	}
	day := p13GuildDay(now)
	var frozen db.P13GuildDonateBoxDay
	err = tx.First(&frozen, "guild_id=? AND day=?", member.GuildID, day).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		rules := policy.RulesJSON
		if rules == "" {
			rules = p13GuildDonateBoxRules()
		}
		frozen = db.P13GuildDonateBoxDay{GuildID: member.GuildID, Day: day, Version: policy.Version, Source: policy.Source, RulesJSON: rules, RulesHash: p6Digest([]byte(rules)), PublishedAt: policy.PublishedAt, CreatedAt: now.UnixMilli()}
		if err = tx.Create(&frozen).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if frozen.Version != constUtil.P13GuildDonateBoxVersion || frozen.Source != constUtil.P13GuildDonateBoxSource || frozen.RulesHash != p6Digest([]byte(frozen.RulesJSON)) || frozen.PublishedAt <= 0 || frozen.PublishedAt > frozen.CreatedAt || frozen.CreatedAt > now.UnixMilli() || frozen.Score < 0 || int64(frozen.Score)+int64(score) > math.MaxInt32 {
		return errors.New("公会捐献宝箱当日冻结规则无效")
	}
	if _, err := p13GuildDonateBoxReadRules(frozen.RulesJSON); err != nil {
		return err
	}
	source := db.P13GuildDonateBoxSource{PlayerID: member.PlayerID, Stamp: stamp, GuildID: member.GuildID, Day: day, JoinedAt: member.JoinedAt, DonateType: request.GetDonateType(), Amount: amount, Score: score, CreatedAt: now.UnixMilli()}
	source.SourceHash = p13GuildDonateBoxSourceHash(source)
	if err = tx.Create(&source).Error; err != nil {
		return err
	}
	frozen.Score += score
	return tx.Save(&frozen).Error
}

// p13GuildDonateBoxDayRead verifies the score against immutable original donation facts.
func p13GuildDonateBoxDayRead(tx *gorm.DB, member *db.P13GuildMember, now time.Time) (*db.P13GuildDonateBoxDay, bool, error) {
	if !gameSchemaHasTable(tx, &db.P13GuildDonateBoxPolicy{}) {
		return nil, false, nil
	}
	var day db.P13GuildDonateBoxDay
	err := tx.First(&day, "guild_id=? AND day=?", member.GuildID, p13GuildDay(now)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		policy, err := p13GuildDonateBoxPolicy(tx, now)
		if err != nil || policy == nil {
			return nil, false, err
		}
		rules := policy.RulesJSON
		if rules == "" {
			rules = p13GuildDonateBoxRules()
		}
		return &db.P13GuildDonateBoxDay{GuildID: member.GuildID, Day: p13GuildDay(now), Version: policy.Version, Source: policy.Source, RulesJSON: rules, RulesHash: p6Digest([]byte(rules)), PublishedAt: policy.PublishedAt}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if day.Version != constUtil.P13GuildDonateBoxVersion || day.Source != constUtil.P13GuildDonateBoxSource || day.RulesHash != p6Digest([]byte(day.RulesJSON)) || day.CreatedAt <= 0 || day.PublishedAt <= 0 || day.PublishedAt > day.CreatedAt || day.CreatedAt > now.UnixMilli() || day.Score < 0 {
		return nil, false, errors.New("公会捐献宝箱冻结日来源损坏")
	}
	if _, err := p13GuildDonateBoxReadRules(day.RulesJSON); err != nil {
		return nil, false, err
	}
	var sources []db.P13GuildDonateBoxSource
	if err = tx.Where("guild_id=? AND day=?", day.GuildID, day.Day).Find(&sources).Error; err != nil {
		return nil, false, err
	}
	var total int64
	eligible := false
	for _, source := range sources {
		if source.PlayerID <= 0 || source.Stamp <= 0 || source.JoinedAt <= 0 || source.JoinedAt > source.CreatedAt || source.CreatedAt < day.CreatedAt || source.CreatedAt > now.UnixMilli() || source.Day != p13GuildDay(time.UnixMilli(source.CreatedAt)) || source.Amount <= 0 || source.Score <= 0 || source.DonateType != "coin" && source.DonateType != "badge" || source.SourceHash != p13GuildDonateBoxSourceHash(source) {
			return nil, false, errors.New("公会捐献宝箱积分缺实际捐献来源")
		}
		total += int64(source.Score)
		if total > math.MaxInt32 {
			return nil, false, errors.New("公会捐献宝箱来源总额溢出")
		}
		if source.PlayerID == member.PlayerID && source.JoinedAt == member.JoinedAt {
			eligible = true
		}
	}
	if total != int64(day.Score) {
		return nil, false, errors.New("公会捐献宝箱当日积分与实际来源不符")
	}
	return &day, eligible, nil
}

// ProjectP13GuildDonateBoxes uses original four-box states and resource-string previews.
// It performs no writes, so reopening or reconnecting cannot create qualification.
func (s *GameService) ProjectP13GuildDonateBoxes(tx *gorm.DB, member *db.P13GuildMember, now time.Time) ([]*pb.UnionContributionBox, int32, bool, error) {
	day, eligible, err := p13GuildDonateBoxDayRead(tx, member, now)
	if err != nil || day == nil {
		return nil, 0, false, err
	}
	var claims []db.P13GuildDonateBoxClaim
	if err = tx.Where("player_id=? AND day=?", member.PlayerID, day.Day).Find(&claims).Error; err != nil {
		return nil, 0, false, err
	}
	claimed := map[int64]bool{}
	for _, claim := range claims {
		claimDay := day
		if claim.GuildID != day.GuildID {
			var prior db.P13GuildDonateBoxDay
			if err := tx.First(&prior, "guild_id=? AND day=?", claim.GuildID, day.Day).Error; err != nil {
				return nil, 0, false, err
			}
			if prior.RulesHash != p6Digest([]byte(prior.RulesJSON)) || prior.Version != claim.Version {
				return nil, 0, false, errors.New("公会捐献宝箱原公会档位损坏")
			}
			claimDay = &prior
		}
		threshold, reward, ruleErr := p13GuildDonateBoxFrozenRule(claimDay, claim.Ordinal)
		if ruleErr != nil || claim.Version != day.Version || claim.Threshold != threshold || claim.Reward != reward || claim.Score < threshold || claim.BoxID != p13GuildDonateBoxID(claim.GuildID, day.Day, claim.Ordinal) || claim.ResponseHash != p6Digest(claim.Response) {
			return nil, 0, false, errors.New("公会捐献宝箱领取来源损坏")
		}
		claimed[claim.Ordinal] = true
	}
	var boxes []*pb.UnionContributionBox
	for ordinal := int64(1); ordinal <= constUtil.P13GuildDonateBoxCount; ordinal++ {
		threshold, reward, ruleErr := p13GuildDonateBoxFrozenRule(day, ordinal)
		if ruleErr != nil {
			return nil, 0, false, ruleErr
		}
		state := int32(0)
		if claimed[ordinal] {
			state = 2
		} else if eligible && day.Score >= threshold {
			state = 1
		}
		boxes = append(boxes, &pb.UnionContributionBox{BoxId: proto.String(p13GuildDonateBoxID(day.GuildID, day.Day, ordinal)), Score: proto.Int32(threshold), Reward: proto.String(reward), State: proto.Int32(state)})
	}
	return boxes, day.Score, true, nil
}

func (s *GameService) p13GuildOpenDonateBox(ctx context.Context, id int64, request *pb.CSUnionOpenDonateBox, stamp int64, now time.Time) ([]proto.Message, error) {
	response := &pb.SCUnionOpenDonateBox{}
	return s.p13GuildMutate(ctx, id, request, response, stamp, now, true, func(tx *gorm.DB, profile *db.Profile, player *pb.Player, game *model.GameState) error {
		member, _, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		boxes, score, available, err := s.ProjectP13GuildDonateBoxes(tx, member, now)
		if err != nil {
			return err
		}
		if !available {
			return errors.New(constUtil.P13GuildDonateBoxClosed)
		}
		for index, box := range boxes {
			if box.GetBoxId() != request.GetBoxId() {
				continue
			}
			if box.GetState() != 1 {
				return errors.New("公会捐献宝箱未达领取资格或已经领取")
			}
			gain, err := s.GrantFixedReward(player, game, box.GetReward())
			if err != nil {
				return err
			}
			response.Get = gain
			raw, err := proto.Marshal(response)
			if err != nil {
				return err
			}
			return tx.Create(&db.P13GuildDonateBoxClaim{PlayerID: id, Day: p13GuildDay(now), Ordinal: int64(index + 1), GuildID: member.GuildID, JoinedAt: member.JoinedAt, BoxID: box.GetBoxId(), Stamp: stamp, Score: score, Threshold: box.GetScore(), Reward: box.GetReward(), Version: constUtil.P13GuildDonateBoxVersion, CreatedAt: now.UnixMilli(), Response: raw, ResponseHash: p6Digest(raw)}).Error
		}
		return errors.New("公会捐献宝箱编号不属于当前公会日期")
	})
}
