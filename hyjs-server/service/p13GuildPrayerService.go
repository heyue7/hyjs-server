package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
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

type p13GuildPrayerChoice struct {
	Type   string  `json:"type"`
	Value  float64 `json:"value"`
	Weight int64   `json:"weight"`
}

func p13GuildPrayerPool(raw, hash string) ([]p13GuildPrayerChoice, int64, error) {
	var choices []p13GuildPrayerChoice
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if p6Digest([]byte(raw)) != hash || decoder.Decode(&choices) != nil || len(choices) == 0 || len(choices) > constUtil.P13GuildPrayerPoolLimit {
		return nil, 0, errors.New("公会祈祷配置无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, 0, errors.New("公会祈祷配置无效")
	}
	var total int64
	for _, choice := range choices {
		if choice.Type != "peratt" && choice.Type != "perhp" && choice.Type != "perpdef" && choice.Type != "permdef" || choice.Value <= 0 || choice.Value > constUtil.P13GuildPrayerPercentLimit || math.IsNaN(choice.Value) || math.IsInf(choice.Value, 0) || choice.Weight <= 0 || choice.Weight > constUtil.P13GuildPrayerWeightLimit {
			return nil, 0, errors.New("公会祈祷属性配置无效")
		}
		total += choice.Weight
	}
	return choices, total, nil
}

func p13GuildPrayerProp(choice p13GuildPrayerChoice) *pb.BaseProp {
	prop := &pb.BaseProp{}
	switch choice.Type {
	case "peratt":
		prop.Peratt = proto.Float64(choice.Value)
	case "perhp":
		prop.Perhp = proto.Float64(choice.Value)
	case "perpdef":
		prop.Perpdef = proto.Float64(choice.Value)
	case "permdef":
		prop.Permdef = proto.Float64(choice.Value)
	}
	return prop
}

func p13GuildPrayerRollHash(row *db.P13GuildPrayerRoll) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func p13GuildPrayerRollRead(row *db.P13GuildPrayerRoll) (*pb.BaseProp, error) {
	if row.PlayerID <= 0 || row.Stamp <= 0 || row.GuildID == "" || row.Ordinal < 1 || row.Ordinal > int64(row.DailyLimit) || row.AppliedAt <= 0 || row.PublishedAt <= 0 || row.PublishedAt > row.AppliedAt || row.AppliedAt > math.MaxInt64-int64(constUtil.P13GuildPrayerDurationHours*time.Hour/time.Millisecond) || row.ExpiresAt != row.AppliedAt+int64(constUtil.P13GuildPrayerDurationHours*time.Hour/time.Millisecond) || row.ContributionCost != constUtil.P13GuildPrayerContributionCost || row.DailyLimit != constUtil.P13GuildPrayerDailyLimit || row.DurationHours != constUtil.P13GuildPrayerDurationHours || row.Day != p13GuildDay(time.UnixMilli(row.AppliedAt)) || strings.TrimSpace(row.PoolVersion) == "" || row.Source != constUtil.P13GuildPrayerSource || row.SourceHash != p13GuildPrayerRollHash(row) {
		return nil, errors.New("公会祈祷实际扣费与冻结来源不符")
	}
	choices, total, err := p13GuildPrayerPool(row.PoolJSON, row.PoolHash)
	if err != nil {
		return nil, err
	}
	if row.Draw < 0 || row.Draw >= total || row.ChosenIndex < 0 || row.ChosenIndex >= int64(len(choices)) {
		return nil, errors.New("公会祈祷真实选取记录损坏")
	}
	var prefix int64
	chosen := int64(-1)
	for i, choice := range choices {
		prefix += choice.Weight
		if row.Draw < prefix {
			chosen = int64(i)
			break
		}
	}
	prop := &pb.BaseProp{}
	if chosen != row.ChosenIndex || p6Digest(row.Effect) != row.EffectHash || proto.Unmarshal(row.Effect, prop) != nil || !proto.Equal(prop, p13GuildPrayerProp(choices[chosen])) {
		return nil, errors.New("公会祈祷原属性帧与冻结选取不符")
	}
	return prop, nil
}

// InitializeP13GuildPrayer creates only a disabled local policy. Original costs,
// counts and duration are read from the recovered APK, never from the local pool.
func (s *GameService) InitializeP13GuildPrayer(ctx context.Context) error {
	if _, _, _, err := s.p13GuildPrayerConfig(); err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13GuildPrayerPolicy{}, &db.P13GuildPrayerRoll{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13GuildPrayerPolicy{ID: 1, Source: constUtil.P13GuildPrayerSource, PoolJSON: "[]", PoolHash: p6Digest([]byte("[]"))}).Error
}

func (s *GameService) p13GuildPrayerConfig() (int32, int32, int32, error) {
	cost, err := s.p10Setting("UnionPrayContributeCost")
	if err != nil {
		return 0, 0, 0, err
	}
	limit, err := s.p10Setting("UnionPrayDailyCount")
	if err != nil {
		return 0, 0, 0, err
	}
	duration, err := s.p10Setting("UnionPrayDuration")
	if err != nil {
		return 0, 0, 0, err
	}
	if cost != constUtil.P13GuildPrayerContributionCost || limit != constUtil.P13GuildPrayerDailyLimit || duration != constUtil.P13GuildPrayerDurationHours {
		return 0, 0, 0, errors.New("原祈祷贡献/次数/有效时长配置不兼容")
	}
	return cost, limit, duration, nil
}

// P13GuildPrayerProperties is a read-only source API for the P5 attribute
// assembler. It returns original percent points; multiply by .01 exactly once
// at the original combined-percent position. Membership and expiry are live.
func (s *GameService) P13GuildPrayerProperties(ctx context.Context, tx *gorm.DB, id int64, now time.Time) (*pb.BaseProp, string, int64, error) {
	tx = tx.WithContext(ctx)
	// P5-only fixtures and optional deployments can omit the entire guild schema.
	// These metadata reads never create tables or acquire guild mutation locks.
	for _, table := range []any{&db.P13GuildPrayerRoll{}, &db.P13GuildMember{}, &db.P13Guild{}} {
		if !tx.Migrator().HasTable(table) {
			return &pb.BaseProp{}, "", 0, nil
		}
	}
	member, err := p13GuildMember(tx, id)
	if err != nil {
		return nil, "", 0, err
	}
	if member.GuildID == "" {
		return &pb.BaseProp{}, "", 0, nil
	}
	var guild db.P13Guild
	err = tx.First(&guild, "id=?", member.GuildID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &pb.BaseProp{}, "", 0, nil
	}
	if err != nil {
		return nil, "", 0, err
	}
	var row db.P13GuildPrayerRoll
	err = tx.Where("player_id=?", id).Order("applied_at DESC,ordinal DESC,stamp DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &pb.BaseProp{}, "", 0, nil
	}
	if err != nil {
		return nil, "", 0, err
	}
	prop, err := p13GuildPrayerRollRead(&row)
	if err != nil {
		return nil, "", 0, err
	}
	if row.AppliedAt > now.UnixMilli() {
		return nil, "", 0, errors.New("公会祈祷实际发生时刻晚于当前属性快照")
	}
	// A later membership term cannot reactivate a prayer bought before leaving.
	if row.GuildID != member.GuildID || member.JoinedAt > row.AppliedAt || now.UnixMilli() >= row.ExpiresAt {
		return &pb.BaseProp{}, "", 0, nil
	}
	return proto.Clone(prop).(*pb.BaseProp), row.SourceHash, row.ExpiresAt, nil
}

func (s *GameService) p13GuildPrayerInfo(ctx context.Context, id int64, now time.Time) ([]proto.Message, error) {
	var response *pb.SCUnionLightChurchInfo
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if !gate.ManagementEnabled {
			return errors.New(constUtil.P13GuildClosed)
		}
		if _, _, err := p13GuildNeedMember(tx, id); err != nil {
			return err
		}
		var profile db.Profile
		if err := tx.First(&profile, id).Error; err != nil {
			return err
		}
		player, _, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		var count int64
		if err = tx.Model(&db.P13GuildPrayerRoll{}).Where("player_id=? AND day=?", id, p13GuildDay(now)).Count(&count).Error; err != nil {
			return err
		}
		if count < 0 || count > constUtil.P13GuildPrayerDailyLimit {
			return errors.New("公会祈祷实际每日次数不符")
		}
		prop, _, expires, err := s.P13GuildPrayerProperties(ctx, tx, id, now)
		if err != nil {
			return err
		}
		response = &pb.SCUnionLightChurchInfo{PrayTimes: proto.Int32(int32(count)), Contribution: proto.Int32(player.GetContribution()), AddProp: prop, InvalidTime: proto.Int64(expires)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// p13GuildPrayer uses the existing guild/profile transaction and original
// receipt. The personal spendable contribution changes; lifetime donation does not.
func (s *GameService) p13GuildPrayer(ctx context.Context, id int64, request *pb.CSUnionPray, stamp int64, now time.Time) ([]proto.Message, error) {
	return s.p13GuildMutate(ctx, id, request, &pb.SCUnionPray{}, stamp, now, true, func(tx *gorm.DB, profile *db.Profile, player *pb.Player, game *model.GameState) error {
		member, _, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		var policy db.P13GuildPrayerPolicy
		if err = tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled {
			return errors.New(constUtil.P13GuildPrayerClosed)
		}
		if policy.Source != constUtil.P13GuildPrayerSource || policy.PublishedAt <= 0 || policy.PublishedAt > now.UnixMilli() || strings.TrimSpace(policy.Version) == "" || len(policy.Version) > 128 {
			return errors.New("公会祈祷缺实际提前发布的具名池版本")
		}
		choices, total, err := p13GuildPrayerPool(policy.PoolJSON, policy.PoolHash)
		if err != nil {
			return err
		}
		cost, limit, duration, err := s.p13GuildPrayerConfig()
		if err != nil {
			return err
		}
		var count int64
		day := p13GuildDay(now)
		if err = tx.Model(&db.P13GuildPrayerRoll{}).Where("player_id=? AND day=?", id, day).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(limit) {
			return errors.New("今日公会祈祷次数已用完")
		}
		if player.GetContribution() < cost {
			return errors.New("个人贡献不足")
		}
		var latest db.P13GuildPrayerRoll
		err = tx.Where("player_id=?", id).Order("applied_at DESC,ordinal DESC,stamp DESC").First(&latest).Error
		if err == nil {
			if _, err = p13GuildPrayerRollRead(&latest); err != nil {
				return err
			}
			if latest.AppliedAt > now.UnixMilli() {
				return errors.New("祈祷时钟早于实际最近发生记录")
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		draw, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return err
		}
		var prefix int64
		chosen := int64(-1)
		for index, choice := range choices {
			prefix += choice.Weight
			if draw.Int64() < prefix {
				chosen = int64(index)
				break
			}
		}
		if chosen < 0 {
			return errors.New("公会祈祷真实随机选择失败")
		}
		effect, err := proto.Marshal(p13GuildPrayerProp(choices[chosen]))
		if err != nil {
			return err
		}
		if now.UnixMilli() > math.MaxInt64-int64(time.Duration(duration)*time.Hour/time.Millisecond) {
			return errors.New("公会祈祷有效时刻溢出")
		}
		row := db.P13GuildPrayerRoll{PlayerID: id, Stamp: stamp, GuildID: member.GuildID, Day: day, Ordinal: count + 1, AppliedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Duration(duration) * time.Hour).UnixMilli(), ContributionCost: cost, DailyLimit: limit, DurationHours: duration, PoolVersion: policy.Version, PoolJSON: policy.PoolJSON, PoolHash: policy.PoolHash, PublishedAt: policy.PublishedAt, Draw: draw.Int64(), ChosenIndex: chosen, Effect: effect, EffectHash: p6Digest(effect), Source: constUtil.P13GuildPrayerSource}
		row.SourceHash = p13GuildPrayerRollHash(&row)
		if _, err = p13GuildPrayerRollRead(&row); err != nil {
			return err
		}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		player.Contribution = proto.Int32(player.GetContribution() - cost)
		player.NeedRefreshUnionProp = proto.Int32(1)
		return nil
	})
}

// DispatchP13GuildPrayer precedes the legacy guild dispatcher after explicit
// initialization. The original empty HTTP messages and response fields remain.
func (s *GameService) DispatchP13GuildPrayer(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	now := p13UFNow(ctx)
	switch body := request.(type) {
	case *pb.CSUnionPray:
		messages, err := s.p13GuildPrayer(ctx, id, body, stamp, now)
		return messages, true, err
	case *pb.CSUnionLightChurchInfo:
		messages, err := s.p13GuildPrayerInfo(ctx, id, now)
		return messages, true, err
	case *pb.CSGetUnionAddBaseProp:
		var (
			prayer *pb.BaseProp
			tech   *pb.BaseProp
		)
		// Both current guild sources share one transaction and one time snapshot.
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var err error
			prayer, _, _, err = s.P13GuildPrayerProperties(ctx, tx, id, now)
			if err != nil {
				return err
			}
			tech, _, err = s.P13GuildTechProperties(ctx, tx, id, now)
			return err
		})
		if err != nil {
			return nil, true, err
		}
		return []proto.Message{&pb.SCGetUnionAddBaseProp{UnionAddBaseProp: tech, UnionPrayProp: prayer}}, true, nil

	default:
		return nil, false, nil
	}
}
