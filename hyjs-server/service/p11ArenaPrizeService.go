package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"math/big"
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

type p11ArenaPrizeOption struct {
	ID     string `json:"id"`
	Num    int32  `json:"num"`
	Weight int64  `json:"weight"`
}
type p11ArenaPrizeDraw struct {
	Index int   `json:"index"`
	Value int64 `json:"value"`
	Total int64 `json:"total"`
}

type p11ArenaRegularCoinDraw struct {
	Source string  `json:"source"`
	Reason string  `json:"reason"`
	Values []int64 `json:"values"`
	Min    int64   `json:"min,omitempty"`
	Max    int64   `json:"max,omitempty"`
}

// Every card uses its own unbiased crypto draw. Repeated quantities are valid;
// the first card is the original consumer's hit and the other two are previews.
func p11ArenaRegularCoinCards(reason string, supplied *p11ArenaRegularCoinDraw) ([]*pb.BaseResource, *p11ArenaRegularCoinDraw, error) {
	switch reason {
	case constUtil.P11ArenaPrizeDisabled, constUtil.P11ArenaPrizeClosed, constUtil.P11ArenaPrizeLevel, constUtil.P11ArenaPrizeDaily, constUtil.P11ArenaPrizeTotal:
	default:
		return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
	}
	minimum, maximum := constUtil.P11ArenaPrizeRegularCoinMin, constUtil.P11ArenaPrizeRegularCoinMax
	if supplied != nil {
		switch supplied.Source {
		case constUtil.P11ArenaPrizeRegularCoinSource:
			if supplied.Min != 0 || supplied.Max != 0 {
				return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
			}
		case constUtil.P11ArenaPrizeConfiguredCoinSource:
			minimum, maximum = supplied.Min, supplied.Max
			if minimum < 1 || maximum < minimum || maximum > constUtil.P11ArenaPrizeCoinMaximum {
				return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
			}
		default:
			return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
		}
	}
	width := maximum - minimum + 1
	draw := &p11ArenaRegularCoinDraw{Source: constUtil.P11ArenaPrizeRegularCoinSource, Reason: reason}
	if supplied == nil {
		for card := 0; card < 3; card++ {
			value, err := rand.Int(rand.Reader, big.NewInt(width))
			if err != nil {
				return nil, nil, err
			}
			draw.Values = append(draw.Values, value.Int64())
		}
	} else {
		if supplied.Reason != reason || len(supplied.Values) != 3 {
			return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
		}
		draw.Source, draw.Min, draw.Max = supplied.Source, supplied.Min, supplied.Max
		draw.Values = append([]int64(nil), supplied.Values...)
	}
	var cards []*pb.BaseResource
	for _, value := range draw.Values {
		if value < 0 || value >= width {
			return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
		}
		cards = append(cards, &pb.BaseResource{Id: proto.String("coin"), Num: proto.Int32(int32(minimum + value))})
	}
	return cards, draw, nil
}

// Existing pre-configuration Go fixtures may carry zero fields; persisted
// schema defaults and admin saves always store the explicit configured range.
func p11ArenaPolicyRegularCoinRange(policy *db.P11ArenaPrizePolicy) (int64, int64, error) {
	minimum, maximum := policy.RegularCoinMin, policy.RegularCoinMax
	if minimum == 0 && maximum == 0 {
		return constUtil.P11ArenaPrizeRegularCoinMin, constUtil.P11ArenaPrizeRegularCoinMax, nil
	}
	if minimum < 1 || maximum < minimum || maximum > constUtil.P11ArenaPrizeCoinMaximum {
		return 0, 0, errors.New(constUtil.P11ArenaPrizeConfigError)
	}
	return minimum, maximum, nil
}

func p11ArenaConfiguredCoinCards(reason string, policy *db.P11ArenaPrizePolicy) ([]*pb.BaseResource, *p11ArenaRegularCoinDraw, error) {
	minimum, maximum, err := p11ArenaPolicyRegularCoinRange(policy)
	if err != nil {
		return nil, nil, err
	}
	draw := &p11ArenaRegularCoinDraw{Source: constUtil.P11ArenaPrizeConfiguredCoinSource, Reason: reason, Min: minimum, Max: maximum}
	for card := 0; card < 3; card++ {
		value, err := rand.Int(rand.Reader, big.NewInt(maximum-minimum+1))
		if err != nil {
			return nil, nil, err
		}
		draw.Values = append(draw.Values, value.Int64())
	}
	return p11ArenaRegularCoinCards(reason, draw)
}

// InitializeP11ArenaPrizes preserves published operator values. Creating the
// schema grants nothing and leaves the explicitly local prize policy disabled.
func (s *GameService) InitializeP11ArenaPrizes(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P11ArenaPrizePolicy{}, &db.P11ArenaPrizeOffer{}, &db.P11ArenaPrizeAward{}); err != nil {
		return err
	}
	policy := db.P11ArenaPrizePolicy{ID: constUtil.P11ArenaPrizePolicyID, Version: "local-arena-win-prize-v1", Source: constUtil.P11ArenaPrizeSource, PoolJSON: constUtil.P11ArenaPrizeDefaultPool, MinLevel: 10, DailyWins: 5, WindowWins: 30, RegularCoinMin: constUtil.P11ArenaPrizeRegularCoinMin, RegularCoinMax: constUtil.P11ArenaPrizeRegularCoinMax}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&policy).Error
}

func p11ArenaPrizePool(policy *db.P11ArenaPrizePolicy) ([]p11ArenaPrizeOption, error) {
	reject := func() ([]p11ArenaPrizeOption, error) { return nil, errors.New(constUtil.P11ArenaPrizeConfigError) }
	if policy.ID != constUtil.P11ArenaPrizePolicyID || strings.TrimSpace(policy.Version) == "" || len(policy.Version) > 64 || policy.Source != constUtil.P11ArenaPrizeSource || policy.PublishedAt <= 0 || policy.PublishedAt > policy.StartAt || policy.StartAt >= policy.EndAt || policy.MinLevel < 10 || policy.MinLevel > math.MaxInt32 || policy.DailyWins < 1 || policy.DailyWins > 100 || policy.WindowWins < 1 || policy.WindowWins > 3100 || policy.DailyWins > policy.WindowWins {
		return reject()
	}
	var pool []p11ArenaPrizeOption
	if len(policy.PoolJSON) > 16384 || json.Unmarshal([]byte(policy.PoolJSON), &pool) != nil || len(pool) < 1 || len(pool) > 32 {
		return reject()
	}
	seen := map[string]bool{}
	var total int64
	for _, item := range pool {
		allowed := item.ID == "coin" || item.ID == "popularity"
		for _, original := range strings.Split(constUtil.P11ArenaPrizeOriginalItems, ",") {
			allowed = allowed || item.ID == original
		}
		key := item.ID + ":" + strconv.FormatInt(int64(item.Num), 10)
		if !allowed || item.Num <= 0 || item.Weight <= 0 || item.Weight > 1000000000 || seen[key] || total > math.MaxInt64-item.Weight {
			return reject()
		}
		total += item.Weight
		seen[key] = true
	}
	return pool, nil
}

func p11ArenaPrizeMonth(now time.Time) (string, string) {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	return local.Format("200601"), local.Format("20060102")
}

// p11ArenaPrizeOfferRules checks the original publication and its finite month
// window without consulting mutable current operator settings on receipt reads.
func p11ArenaPrizeOfferRules(offer *db.P11ArenaPrizeOffer) (*db.P11ArenaPrizePolicy, []p11ArenaPrizeOption, error) {
	bad := errors.New(constUtil.P11ArenaPrizeConflict)
	var policy db.P11ArenaPrizePolicy
	if offer == nil || p6Digest([]byte(offer.Snapshot)) != offer.SourceHash || json.Unmarshal([]byte(offer.Snapshot), &policy) != nil || !policy.Enabled {
		return nil, nil, bad
	}
	pool, err := p11ArenaPrizePool(&policy)
	if err != nil {
		return nil, nil, err
	}
	month, _ := p11ArenaPrizeMonth(time.UnixMilli(policy.StartAt))
	endMonth, _ := p11ArenaPrizeMonth(time.UnixMilli(policy.EndAt - 1))
	if month != offer.Month || endMonth != offer.Month || offer.CapturedAt < policy.StartAt || offer.CapturedAt >= policy.EndAt {
		return nil, nil, bad
	}
	return &policy, pool, nil
}

// p11ArenaPrizeCards records crypto draws. Local previews exhaust distinct pool
// entries before reusing them; their quantities never reach GrantFixedReward.
func p11ArenaPrizeCards(pool []p11ArenaPrizeOption, supplied []p11ArenaPrizeDraw) ([]*pb.BaseResource, []p11ArenaPrizeDraw, error) {
	var cards []*pb.BaseResource
	var draws []p11ArenaPrizeDraw
	used := map[int]bool{}
	for card := 0; card < 3; card++ {
		if len(used) == len(pool) {
			used = map[int]bool{}
		}
		var total int64
		for i, p := range pool {
			if !used[i] {
				total += p.Weight
			}
		}
		if total <= 0 {
			return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
		}
		var value int64
		if supplied == nil {
			v, err := rand.Int(rand.Reader, big.NewInt(total))
			if err != nil {
				return nil, nil, err
			}
			value = v.Int64()
		} else {
			if len(supplied) != 3 || supplied[card].Total != total {
				return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
			}
			value = supplied[card].Value
		}
		if value < 0 || value >= total {
			return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
		}
		index := -1
		remaining := value
		for i, p := range pool {
			if !used[i] {
				if remaining < p.Weight {
					index = i
					break
				}
				remaining -= p.Weight
			}
		}
		if index < 0 || (supplied != nil && supplied[card].Index != index) {
			return nil, nil, errors.New(constUtil.P11ArenaPrizeConflict)
		}
		p := pool[index]
		used[index] = true
		cards = append(cards, &pb.BaseResource{Id: proto.String(p.ID), Num: proto.Int32(p.Num)})
		draws = append(draws, p11ArenaPrizeDraw{Index: index, Value: value, Total: total})
	}
	return cards, draws, nil
}

func p11ArenaPrizeBase(response *pb.SCPvpFightResult) *pb.SCPvpFightResult {
	base := proto.Clone(response).(*pb.SCPvpFightResult)
	base.BoxList = nil
	base.Result = &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("energy"), Num: proto.Int32(-constUtil.P11ArenaEnergy)}}}
	return base
}
func p11ArenaPrizeAwardHash(award db.P11ArenaPrizeAward) string {
	award.SourceHash = ""
	// SQL drivers may decode an empty BLOB as nil or an empty slice. Normalize
	// that zero-grant representation before hashing accepted prize facts.
	if len(award.GrantPB) == 0 {
		award.GrantPB = nil
	}
	raw, _ := json.Marshal(award)
	return p6Digest(raw)
}
func p11ArenaPrizeLegacy(response *pb.SCPvpFightResult) bool {
	return response != nil && len(response.BoxList) == 0 && proto.Equal(response.GetResult(), p11ArenaPrizeBase(response).Result)
}

// ApplyP11ArenaFightPrize runs after the real accepted P6 settlement was saved,
// under its existing profile and arena-policy locks, in the very same transaction.
// There is no reward-claim endpoint: the original flip callback only draws icons.
func (s *GameService) ApplyP11ArenaFightPrize(ctx context.Context, tx *gorm.DB, row *db.P11ArenaFightSettlement, profile *db.Profile, player *pb.Player, game *model.GameState, response *pb.SCPvpFightResult, now time.Time) error {
	if tx == nil || row == nil || profile == nil || player == nil || game == nil || response == nil || row.PlayerID != profile.Id || row.PlayerID != player.GetPlayerId() || !p11ArenaPrizeLegacy(response) || row.CreatedAt != now.UnixMilli() {
		return errors.New(constUtil.P11ArenaPrizeConflict)
	}
	tx = tx.WithContext(ctx)
	// The existing reader validates the actual source, native certificate and
	// computed winner before any policy draw or inventory mutation is permitted.
	accepted, err := p11ReadArenaFightReceipt(tx, row)
	if err != nil {
		return err
	}
	if !proto.Equal(accepted, response) {
		return errors.New(constUtil.P11ArenaPrizeConflict)
	}
	if err = s.recordP4VerifiedArenaWin(tx, row, accepted, now); err != nil {
		return err
	}
	var prior int64
	if err = tx.Model(&db.P11ArenaPrizeAward{}).Where("fight_id=?", row.FightID).Count(&prior).Error; err != nil {
		return err
	}
	if prior != 0 {
		return errors.New(constUtil.P11ArenaPrizeConflict)
	}
	month, day := p11ArenaPrizeMonth(now)
	award := db.P11ArenaPrizeAward{FightID: row.FightID, PlayerID: row.PlayerID, Stamp: row.Stamp, PlayerLevel: int64(player.GetLevel()), WinnerID: row.WinnerID, P6RecordID: row.P6RecordID, TerminalHash: row.TerminalHash, ScopeProof: row.ScopeProof, BaseResponseHash: row.ResponseHash, Month: month, Day: day, Status: constUtil.P11ArenaPrizeLost, DrawJSON: "[]", BoxesJSON: "[]", CreatedAt: now.UnixMilli()}
	grant := &pb.ResultObject{}
	var policy db.P11ArenaPrizePolicy
	if row.WinnerID == row.PlayerID {
		response.BoxList = []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(0)}, {Id: proto.String("coin"), Num: proto.Int32(0)}, {Id: proto.String("coin"), Num: proto.Int32(0)}}
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, constUtil.P11ArenaPrizePolicyID).Error; err != nil {
			return err
		}
		award.Status = constUtil.P11ArenaPrizeDisabled
		if policy.Enabled {
			award.Status = constUtil.P11ArenaPrizeClosed
			var offer db.P11ArenaPrizeOffer
			err = tx.First(&offer, "month=?", month).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if _, err = p11ArenaPrizePool(&policy); err != nil {
					return err
				}
				if now.UnixMilli() >= policy.StartAt && now.UnixMilli() < policy.EndAt {
					raw, e := json.Marshal(policy)
					if e != nil {
						return e
					}
					offer = db.P11ArenaPrizeOffer{Month: month, Snapshot: string(raw), SourceHash: p6Digest(raw), CapturedAt: now.UnixMilli()}
					if _, _, e = p11ArenaPrizeOfferRules(&offer); e != nil {
						return e
					}
					if err = tx.Create(&offer).Error; err != nil {
						return err
					}
				}
			} else if err != nil {
				return err
			}
			if offer.Month != "" {
				frozen, pool, e := p11ArenaPrizeOfferRules(&offer)
				if e != nil {
					return e
				}
				award.OfferHash = offer.SourceHash
				if now.UnixMilli() >= frozen.StartAt && now.UnixMilli() < frozen.EndAt {
					award.Status = constUtil.P11ArenaPrizeLevel
					if int64(player.GetLevel()) >= frozen.MinLevel {
						var daily, window int64
						q := tx.Model(&db.P11ArenaPrizeAward{}).Where("player_id=? AND month=? AND status=?", row.PlayerID, month, constUtil.P11ArenaPrizeAwarded)
						if err = q.Count(&window).Error; err != nil {
							return err
						}
						if err = q.Where("day=?", day).Count(&daily).Error; err != nil {
							return err
						}
						award.Status = constUtil.P11ArenaPrizeDaily
						if daily < frozen.DailyWins {
							award.Status = constUtil.P11ArenaPrizeTotal
							if window < frozen.WindowWins {
								cards, draws, e := p11ArenaPrizeCards(pool, nil)
								if e != nil {
									return e
								}
								selected := cards[0]
								gain, e := s.GrantFixedReward(player, game, selected.GetId()+","+strconv.FormatInt(int64(selected.GetNum()), 10))
								if e != nil {
									return e
								}
								expected := &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String(selected.GetId()), Num: proto.Int32(selected.GetNum())}}}
								if !proto.Equal(gain, expected) {
									return errors.New(constUtil.P11ArenaPrizeConflict)
								}
								grant = gain
								p1AppendGain(response.Result, gain)
								response.BoxList = cards
								encoded, e := json.Marshal(draws)
								if e != nil {
									return e
								}
								award.DrawJSON = string(encoded)
								award.DayOrdinal = daily + 1
								award.WindowOrdinal = window + 1
								award.Status = constUtil.P11ArenaPrizeAwarded
							}
						}
					}
				}
			}
		}
	}
	// User-confirmed regular victory coins replace only the old zero-card
	// fallback. Valid operator awards keep their own quantities and quota facts.
	if row.WinnerID == row.PlayerID && award.Status != constUtil.P11ArenaPrizeAwarded {
		cards, draw, e := p11ArenaConfiguredCoinCards(award.Status, &policy)
		if e != nil {
			return e
		}
		selected := cards[0]
		gain, e := s.GrantFixedReward(player, game, "coin,"+strconv.FormatInt(int64(selected.GetNum()), 10))
		if e != nil {
			return e
		}
		expected := &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("coin"), Num: selected.Num}}}
		if !proto.Equal(gain, expected) {
			return errors.New(constUtil.P11ArenaPrizeConflict)
		}
		encoded, e := json.Marshal(draw)
		if e != nil {
			return e
		}
		grant = gain
		p1AppendGain(response.Result, gain)
		response.BoxList = cards
		award.DrawJSON, award.Status = string(encoded), constUtil.P11ArenaPrizeRegularCoin
	}
	if err = s.applyP11ArenaRankReward(tx, row, &award, player, game, response); err != nil {
		return err
	}
	boxes, err := json.Marshal(response.BoxList)
	if err != nil {
		return err
	}
	award.BoxesJSON = string(boxes)
	award.GrantPB, err = proto.MarshalOptions{Deterministic: true}.Marshal(grant)
	if err != nil {
		return err
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
	if err != nil {
		return err
	}
	award.FinalResponseHash = p6Digest(raw)
	award.SourceHash = p11ArenaPrizeAwardHash(award)
	if err = tx.Create(&award).Error; err != nil {
		return err
	}
	if err = tx.Model(row).Updates(map[string]any{"response": raw, "response_hash": award.FinalResponseHash}).Error; err != nil {
		return err
	}
	row.Response = raw
	row.ResponseHash = award.FinalResponseHash
	if err = saveProfile(tx, profile, player, game); err != nil {
		return err
	}
	return ValidateP11ArenaFightPrizeReceipt(tx, row, response)
}

// ValidateP11ArenaFightPrizeReceipt is a pure same-transaction read of immutable
// prize facts. Legacy rank-only receipts retain their exact original empty cards.
func ValidateP11ArenaFightPrizeReceipt(tx *gorm.DB, row *db.P11ArenaFightSettlement, response *pb.SCPvpFightResult) error {
	bad := errors.New(constUtil.P11ArenaPrizeConflict)
	if tx == nil || row == nil || response == nil {
		return bad
	}
	legacy := func() error {
		if p11ArenaPrizeLegacy(response) {
			return nil
		}
		return bad
	}
	if !tx.Migrator().HasTable(&db.P11ArenaPrizeAward{}) {
		return legacy()
	}
	var award db.P11ArenaPrizeAward
	err := tx.First(&award, "fight_id=?", row.FightID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return legacy()
	}
	if err != nil {
		return err
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
	if err != nil {
		return err
	}
	base, err := proto.MarshalOptions{Deterministic: true}.Marshal(p11ArenaPrizeBase(response))
	if err != nil {
		return err
	}
	boxes, err := json.Marshal(response.BoxList)
	if err != nil {
		return err
	}
	month, day := p11ArenaPrizeMonth(time.UnixMilli(row.CreatedAt))
	if award.SourceHash != p11ArenaPrizeAwardHash(award) || award.PlayerID != row.PlayerID || award.Stamp != row.Stamp || award.WinnerID != row.WinnerID || award.P6RecordID != row.P6RecordID || award.TerminalHash != row.TerminalHash || award.ScopeProof != row.ScopeProof || award.CreatedAt != row.CreatedAt || award.Month != month || award.Day != day || award.BaseResponseHash != p6Digest(base) || award.FinalResponseHash != row.ResponseHash || award.FinalResponseHash != p6Digest(raw) || award.BoxesJSON != string(boxes) {
		return bad
	}
	grant := &pb.ResultObject{}
	if proto.Unmarshal(award.GrantPB, grant) != nil {
		return bad
	}
	expected := p11ArenaPrizeBase(response).Result
	if award.Status == constUtil.P11ArenaPrizeRegularCoin {
		var draw p11ArenaRegularCoinDraw
		if json.Unmarshal([]byte(award.DrawJSON), &draw) != nil {
			return bad
		}
		cards, _, e := p11ArenaRegularCoinCards(draw.Reason, &draw)
		if e != nil {
			return e
		}
		if row.WinnerID != row.PlayerID || award.DayOrdinal != 0 || award.WindowOrdinal != 0 || len(response.BoxList) != 3 {
			return bad
		}
		for i := range cards {
			if !proto.Equal(cards[i], response.BoxList[i]) {
				return bad
			}
		}
		actual := &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("coin"), Num: cards[0].Num}}}
		if !proto.Equal(grant, actual) {
			return bad
		}
		p1AppendGain(expected, grant)
	} else if award.Status == constUtil.P11ArenaPrizeAwarded {
		var offer db.P11ArenaPrizeOffer
		if err = tx.First(&offer, "month=?", month).Error; err != nil {
			return err
		}
		frozen, pool, e := p11ArenaPrizeOfferRules(&offer)
		if e != nil {
			return e
		}
		var draws []p11ArenaPrizeDraw
		if json.Unmarshal([]byte(award.DrawJSON), &draws) != nil {
			return bad
		}
		cards, _, e := p11ArenaPrizeCards(pool, draws)
		if e != nil {
			return e
		}
		if row.WinnerID != row.PlayerID || award.OfferHash != offer.SourceHash || row.CreatedAt < frozen.StartAt || row.CreatedAt >= frozen.EndAt || award.PlayerLevel < frozen.MinLevel || award.DayOrdinal < 1 || award.DayOrdinal > frozen.DailyWins || award.WindowOrdinal < 1 || award.WindowOrdinal > frozen.WindowWins || len(response.BoxList) != 3 {
			return bad
		}
		for i := range cards {
			if !proto.Equal(cards[i], response.BoxList[i]) {
				return bad
			}
		}
		selected := cards[0]
		actual := &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String(selected.GetId()), Num: proto.Int32(selected.GetNum())}}}
		if !proto.Equal(grant, actual) {
			return bad
		}
		p1AppendGain(expected, grant)
	} else {
		if !proto.Equal(grant, &pb.ResultObject{}) || award.DrawJSON != "[]" || award.DayOrdinal != 0 || award.WindowOrdinal != 0 {
			return bad
		}
		if award.Status == constUtil.P11ArenaPrizeLost {
			if row.WinnerID == row.PlayerID || len(response.BoxList) != 0 {
				return bad
			}
		} else {
			valid := award.Status == constUtil.P11ArenaPrizeDisabled || award.Status == constUtil.P11ArenaPrizeClosed || award.Status == constUtil.P11ArenaPrizeLevel || award.Status == constUtil.P11ArenaPrizeDaily || award.Status == constUtil.P11ArenaPrizeTotal
			if !valid || row.WinnerID != row.PlayerID || len(response.BoxList) != 3 {
				return bad
			}
			for _, card := range response.BoxList {
				if card.GetId() != "coin" || card.GetNum() != 0 {
					return bad
				}
			}
		}
	}
	before, after, diamond, current, e := p11ArenaRankRewardFacts(tx, row)
	if e != nil {
		return e
	}
	if current {
		if award.RankSource != constUtil.P11ArenaRankRewardSource || award.RankBestBefore != before || award.RankBestAfter != after || award.RankDiamond != diamond {
			return bad
		}
		if diamond > 0 {
			p1AppendGain(expected, &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("diamond"), Num: proto.Int32(int32(diamond))}}})
		}
	} else if award.RankSource != "" || award.RankBestBefore != 0 || award.RankBestAfter != 0 || award.RankDiamond != 0 {
		return bad
	}
	if !proto.Equal(response.Result, expected) {
		return bad
	}
	return nil
}
