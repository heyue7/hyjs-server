package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p11GrabRandomOption struct {
	ID     string `json:"id"`
	Num    int32  `json:"num"`
	Weight int64  `json:"weight"`
}

// InitializeP11GrabRandom creates no awards and preserves published operator rows.
func (s *GameService) InitializeP11GrabRandom(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P11GrabRandomPolicy{}, &db.P11GrabRandomOffer{}, &db.P11GrabRandomAward{}); err != nil {
		return err
	}
	cfg := db.P11GrabRandomPolicy{ID: 1, Version: "local-grab-random-v1", IntervalSeconds: 1800, DailyLimit: 6, PoolJSON: constUtil.P11GrabRandomDefaultPool, Source: constUtil.P11GrabRandomSource}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

func p11GrabRandomSchema(tx *gorm.DB) bool {
	return tx.Migrator().HasTable(&db.P11GrabRandomPolicy{}) && tx.Migrator().HasTable(&db.P11GrabRandomOffer{}) && tx.Migrator().HasTable(&db.P11GrabRandomAward{})
}

func (s *GameService) p11GrabRandomPool(cfg *db.P11GrabRandomPolicy) ([]p11GrabRandomOption, error) {
	bad := errors.New(constUtil.P11GrabRandomInvalid)
	if cfg == nil || cfg.ID != 1 || !cfg.Enabled || strings.TrimSpace(cfg.Version) == "" || len(cfg.Version) > 64 || cfg.Source != constUtil.P11GrabRandomSource || cfg.PublishedAt <= 0 || cfg.PublishedAt > cfg.StartAt || cfg.StartAt >= cfg.EndAt || cfg.IntervalSeconds < 1 || cfg.IntervalSeconds > 10800 || cfg.DailyLimit < 1 || cfg.DailyLimit > 1000 || len(cfg.PoolJSON) > 16384 {
		return nil, bad
	}
	month, _ := p11ArenaPrizeMonth(time.UnixMilli(cfg.StartAt))
	endMonth, _ := p11ArenaPrizeMonth(time.UnixMilli(cfg.EndAt - 1))
	if month != endMonth {
		return nil, bad
	}
	var options []p11GrabRandomOption
	if json.Unmarshal([]byte(cfg.PoolJSON), &options) != nil || len(options) < 1 || len(options) > 32 {
		return nil, bad
	}
	seen := map[string]bool{}
	var total int64
	for _, v := range options {
		key := fmt.Sprintf("%s:%d", v.ID, v.Num)
		allowed := v.ID == "coin" || v.ID == "popularity" || strings.HasPrefix(v.ID, "item_") && s.table("Item", "itemId", v.ID) != nil
		if !allowed || v.Num < 1 || v.Weight < 1 || v.Weight > 1000000000 || seen[key] || total > math.MaxInt64-v.Weight {
			return nil, bad
		}
		seen[key] = true
		total += v.Weight
	}
	return options, nil
}

func (s *GameService) p11GrabRandomOfferRules(offer *db.P11GrabRandomOffer) (*db.P11GrabRandomPolicy, []p11GrabRandomOption, error) {
	bad := errors.New(constUtil.P11GrabRandomInvalid)
	var cfg db.P11GrabRandomPolicy
	if offer == nil || p6Digest([]byte(offer.Snapshot)) != offer.SourceHash || json.Unmarshal([]byte(offer.Snapshot), &cfg) != nil {
		return nil, nil, bad
	}
	pool, err := s.p11GrabRandomPool(&cfg)
	if err != nil {
		return nil, nil, err
	}
	month, _ := p11ArenaPrizeMonth(time.UnixMilli(cfg.StartAt))
	if month != offer.Month || offer.CapturedAt < cfg.StartAt || offer.CapturedAt >= cfg.EndAt {
		return nil, nil, bad
	}
	return &cfg, pool, nil
}

// PrepareP11GrabRandom runs after the existing GrabPolicy lock. Its first open
// offer starts future earning; existing occupancy before CapturedAt is excluded.
func (s *GameService) PrepareP11GrabRandom(tx *gorm.DB, now time.Time) error {
	if !p11GrabRandomSchema(tx) {
		return nil
	}
	_, err := s.p11GrabRandomOffer(tx, now)
	return err
}

func (s *GameService) p11GrabRandomOffer(tx *gorm.DB, now time.Time) (*db.P11GrabRandomOffer, error) {
	month, _ := p11ArenaPrizeMonth(now)
	var cfg db.P11GrabRandomPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, 1).Error; err != nil {
		return nil, err
	}
	var offer db.P11GrabRandomOffer
	err := tx.First(&offer, "month=?", month).Error
	if err == nil {
		if _, _, err = s.p11GrabRandomOfferRules(&offer); err != nil {
			return nil, err
		}
		return &offer, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, nil
	}
	if _, err = s.p11GrabRandomPool(&cfg); err != nil {
		return nil, err
	}
	if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
		return nil, nil
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	offer = db.P11GrabRandomOffer{Month: month, Snapshot: string(raw), SourceHash: p6Digest(raw), CapturedAt: now.UnixMilli()}
	if err = tx.Create(&offer).Error; err != nil {
		return nil, err
	}
	return &offer, nil
}

// p11GrabRandomMillis intersects the real immutable occupancy segments with this
// offer. It never mutates sessions, accelerates time or infers a battle winner.
func p11GrabRandomMillis(tx *gorm.DB, id int64, offer *db.P11GrabRandomOffer, cfg *db.P11GrabRandomPolicy, now time.Time) (int64, error) {
	var sessions []db.P11GrabSession
	if err := tx.Where("player_id=? AND expires_at>? AND started_at<?", id, offer.CapturedAt, min(now.UnixMilli(), cfg.EndAt)).Order("started_at,id").Find(&sessions).Error; err != nil {
		return 0, err
	}
	var total, lastEnd int64
	for _, session := range sessions {
		if session.ID == "" || session.Position < 1 || session.Position > 18 || session.StartedAt <= 0 || session.ExpiresAt <= session.StartedAt || session.EndedAt < 0 || session.EndedAt > session.ExpiresAt || session.EndedAt > 0 && session.EndedAt < session.StartedAt {
			return 0, errors.New(constUtil.P11GrabRandomInvalid)
		}
		start := max(session.StartedAt, offer.CapturedAt, cfg.StartAt)
		end := min(session.ExpiresAt, now.UnixMilli(), cfg.EndAt)
		if session.EndedAt > 0 {
			end = min(end, session.EndedAt)
		}
		if end <= start {
			continue
		}
		if start < lastEnd || total > math.MaxInt64-(end-start) {
			return 0, errors.New(constUtil.P11GrabRandomInvalid)
		}
		total += end - start
		lastEnd = end
	}
	return total, nil
}

func p11GrabRandomAwardHash(award *db.P11GrabRandomAward) string {
	copy := *award
	copy.IntegrityHash = ""
	raw, err := json.Marshal(copy)
	if err != nil {
		return ""
	}
	return p6Digest(raw)
}

func (s *GameService) p11GrabRandomAwardReply(tx *gorm.DB, award *db.P11GrabRandomAward) (*pb.SCGetRandReward, error) {
	var offer db.P11GrabRandomOffer
	if err := tx.First(&offer, "month=?", award.Month).Error; err != nil {
		return nil, err
	}
	cfg, pool, err := s.p11GrabRandomOfferRules(&offer)
	if err != nil {
		return nil, err
	}
	return p11GrabRandomFrozenReply(award, &offer, cfg, pool)
}

// p11GrabRandomFrozenReply validates one stored award using this transaction's
// already read offer. Listing many ordinals does not repeatedly query its rules.
func p11GrabRandomFrozenReply(award *db.P11GrabRandomAward, offer *db.P11GrabRandomOffer, cfg *db.P11GrabRandomPolicy, pool []p11GrabRandomOption) (*pb.SCGetRandReward, error) {
	bad := errors.New(constUtil.P11GrabRandomInvalid)
	if award.Ordinal < 1 || award.Stamp <= 0 || award.IntegrityHash == "" || p11GrabRandomAwardHash(award) != award.IntegrityHash {
		return nil, bad
	}
	var total int64
	for _, option := range pool {
		total += option.Weight
	}
	_, day := p11ArenaPrizeMonth(time.UnixMilli(award.CreatedAt))
	if award.Month != offer.Month || award.Day != day || award.SourceHash != offer.SourceHash || award.Draw < 0 || award.Draw >= total || award.DrawTotal != total || award.EligibleMillis/(cfg.IntervalSeconds*1000) < award.Ordinal || award.CreatedAt < offer.CapturedAt || award.CreatedAt >= cfg.EndAt {
		return nil, bad
	}
	var chosen *p11GrabRandomOption
	cursor := award.Draw
	for i := range pool {
		cursor -= pool[i].Weight
		if cursor < 0 {
			chosen = &pool[i]
			break
		}
	}
	if chosen == nil || chosen.ID != award.ResourceID || chosen.Num != award.Quantity {
		return nil, bad
	}
	out := &pb.SCGetRandReward{}
	if err := proto.Unmarshal(award.Response, out); err != nil {
		return nil, err
	}
	gain := out.GetAdd()
	if gain == nil || len(gain.Simps) != 1 || len(gain.Wars)+len(gain.Equips)+len(gain.Treas)+len(gain.Pets) != 0 || gain.Simps[0].GetId() != chosen.ID || gain.Simps[0].GetNum() != chosen.Num {
		return nil, bad
	}
	return out, nil
}

// p11GrabRandomReady counts committed ordinals under the same real profile/Grab
// lock, so redpoint79 and the following claim share the exact eligibility rule.
func (s *GameService) p11GrabRandomReady(tx *gorm.DB, id int64, offer *db.P11GrabRandomOffer, cfg *db.P11GrabRandomPolicy, pool []p11GrabRandomOption, now time.Time) (int64, int64, bool, error) {
	if now.UnixMilli() >= cfg.EndAt {
		return 0, 0, false, nil
	}
	elapsed, err := p11GrabRandomMillis(tx, id, offer, cfg, now)
	if err != nil {
		return 0, 0, false, err
	}
	var awards []db.P11GrabRandomAward
	if err = tx.Where("month=? AND player_id=?", offer.Month, id).Order("ordinal").Find(&awards).Error; err != nil {
		return 0, 0, false, err
	}
	var today int64
	_, day := p11ArenaPrizeMonth(now)
	for i := range awards {
		if awards[i].Ordinal != int64(i+1) {
			return 0, 0, false, errors.New(constUtil.P11GrabRandomInvalid)
		}
		if _, err = p11GrabRandomFrozenReply(&awards[i], offer, cfg, pool); err != nil {
			return 0, 0, false, err
		}
		if awards[i].Day == day {
			today++
		}
	}
	claimed := int64(len(awards))
	return elapsed, claimed, elapsed/(cfg.IntervalSeconds*1000) > claimed && today < cfg.DailyLimit, nil
}

func (s *GameService) p11GrabRandomAt(ctx context.Context, id int64, stamp int64, claim bool, now time.Time) ([]proto.Message, error) {
	var reply proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if !p11GrabRandomSchema(tx) {
			if claim {
				return errors.New(constUtil.P11GrabRandomUnavailable)
			}
			reply = &pb.SCRedPoint{}
			return nil
		}
		if claim {
			if stamp <= 0 {
				return errors.New("原奖杯随机奖HTTP缺少回执时间戳")
			}
			var old db.P11GrabRandomAward
			err = tx.First(&old, "player_id=? AND stamp=?", id, stamp).Error
			if err == nil {
				reply, err = s.p11GrabRandomAwardReply(tx, &old)
				return err
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if _, _, err = p11GrabConfig(tx); err != nil {
			return err
		}
		offer, err := s.p11GrabRandomOffer(tx, now)
		if err != nil {
			return err
		}
		ready := false
		var elapsed, claimed int64
		var cfg *db.P11GrabRandomPolicy
		var pool []p11GrabRandomOption
		if offer != nil {
			cfg, pool, err = s.p11GrabRandomOfferRules(offer)
			if err != nil {
				return err
			}
			elapsed, claimed, ready, err = s.p11GrabRandomReady(tx, id, offer, cfg, pool, now)
			if err != nil {
				return err
			}
		}
		if !claim {
			value := int32(0)
			if ready {
				value = 1
			}
			reply = &pb.SCRedPoint{RedPointList: []*pb.RedPoint{{Type: proto.String(constUtil.P11GrabRandomRedPoint), Value: proto.Int32(value)}}}
			return nil
		}
		if !ready {
			return errors.New(constUtil.P11GrabRandomUnavailable)
		}
		var total int64
		for _, option := range pool {
			total += option.Weight
		}
		random, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return err
		}
		draw := random.Int64()
		cursor := draw
		var selected p11GrabRandomOption
		for _, option := range pool {
			cursor -= option.Weight
			if cursor < 0 {
				selected = option
				break
			}
		}
		gain, err := s.GrantFixedReward(player, game, fmt.Sprintf("%s,%d", selected.ID, selected.Num))
		if err != nil {
			return err
		}
		out := &pb.SCGetRandReward{Add: gain}
		encoded, err := proto.Marshal(out)
		if err != nil {
			return err
		}
		_, day := p11ArenaPrizeMonth(now)
		award := db.P11GrabRandomAward{Month: offer.Month, PlayerID: id, Ordinal: claimed + 1, Stamp: stamp, Day: day, EligibleMillis: elapsed, ResourceID: selected.ID, Quantity: selected.Num, Draw: draw, DrawTotal: total, SourceHash: offer.SourceHash, Response: encoded, CreatedAt: now.UnixMilli()}
		award.IntegrityHash = p11GrabRandomAwardHash(&award)
		if err = tx.Create(&award).Error; err != nil {
			return err
		}
		if err = saveProfile(tx, &row, player, game); err != nil {
			return err
		}
		if s.P1Ready {
			source := fmt.Sprintf("p11-grab-random:%s:%d:%d", offer.Month, id, award.Ordinal)
			if err = s.RecordP1Event(tx, id, source, "grab_occupation_random_reward", 1, now); err != nil {
				return err
			}
		}
		reply = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}

// DispatchP11GrabRandom supplies the original public notification and claim;
// the existing Grab entry/position/payment/time-reward rules stay in their owner.
func (s *GameService) DispatchP11GrabRandom(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSGetGrabData:
		now := time.Now()
		result, err := s.p11GrabAt(ctx, id, request, stamp, now)
		if err == nil {
			// The original page refreshes82041 every minute and listens for20118
			// on its actual authenticated TCP session. Keep its HTTP frame unchanged.
			points, pointErr := s.p11GrabRandomAt(ctx, id, 0, false, now)
			if pointErr == nil && len(points) > 0 {
				s.publishP10Private(ctx, id, points[0])
			}
		}
		return result, true, err
	case *pb.CSRedPoint:
		result, err := s.p11GrabRandomAt(ctx, id, 0, false, time.Now())
		if err == nil {
			points, pointErr := s.InventoryMailRedPoints(ctx, id)
			if pointErr != nil {
				return nil, true, pointErr
			}
			result[0].(*pb.SCRedPoint).RedPointList = append(result[0].(*pb.SCRedPoint).RedPointList, points.RedPointList...)
		}
		return result, true, err
	case *pb.CSGetRandReward:
		result, err := s.p11GrabRandomAt(ctx, id, stamp, true, time.Now())
		return result, true, err
	}
	return nil, false, nil
}
