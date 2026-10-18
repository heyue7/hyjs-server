package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"math/big"
	"slices"
	"time"
)

type p11PlunderSweepChild struct {
	Stamp        int64  `json:"stamp"`
	FightID      string `json:"fightId"`
	MatchID      string `json:"matchId"`
	RecordID     string `json:"recordId"`
	ResponseHash string `json:"responseHash"`
	TerminalHash string `json:"terminalHash"`
}

type p11PlunderSweepPlan struct {
	PlayerID       int64                  `json:"playerId"`
	Stamp          int64                  `json:"stamp"`
	RequestHash    string                 `json:"requestHash"`
	FragmentID     string                 `json:"fragmentId"`
	RequestedCount int32                  `json:"requestedCount"`
	Source         string                 `json:"source"`
	SourceNote     string                 `json:"sourceNote"`
	Peers          []int64                `json:"peers"`
	OfferVersion   string                 `json:"offerVersion"`
	OfferHash      string                 `json:"offerHash"`
	Stop           string                 `json:"stop"`
	Children       []p11PlunderSweepChild `json:"children"`
}

// InitializeP11PlunderSweep adds no reward, win, immunity or publication. The
// existing independently certified plunder policy remains defaultoff.
func (s *GameService) InitializeP11PlunderSweep(ctx context.Context) error {
	for key, want := range map[string]int32{"TreaSweepLevel": 15, "Plunder5MinLevel": 30, "Plunder5VipMinLevel": 3, "TreaSweepCostEnergy": 2} {
		got, err := s.p10Setting(key)
		if err != nil || got != want {
			return errors.New(constUtil.P11PlunderSweepConflict)
		}
	}
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P11PlunderSweepReceipt{})
}

// p11PlunderSweepAt uses only already real matched profiles. Preparing a query
// freezes qualification without granting resources; the later single transaction
// locks every possible peer in ID order before policy and all child mutations.
func (s *GameService) p11PlunderSweepAt(ctx context.Context, id int64, req *pb.CSSweepTreasureFight, stamp int64, now time.Time) ([]proto.Message, error) {
	if stamp <= 0 || !s.p11PlunderFragment(req.GetFragId()) || (req.GetSweepNumber() != 1 && req.GetSweepNumber() != 5) {
		return nil, errors.New(constUtil.P11PlunderFightRefresh)
	}
	if req.GetAutoAddEnergy() != 0 {
		return nil, errors.New(constUtil.P11PlunderSweepUnavailable)
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(req)
	if err != nil {
		return nil, err
	}
	// The existing successful packet is read before a new query, including after
	// operator closure. It cannot allocate a new match or repeat any child fight.
	var previous db.P11PlunderSweepReceipt
	err = db.GetIns().WithContext(ctx).First(&previous, "player_id=? AND stamp=?", id, stamp).Error
	if err == nil {
		response, e := p11ReadPlunderSweepReceipt(db.GetIns().WithContext(ctx), &previous, p6Digest(raw))
		if e != nil {
			return nil, e
		}
		current, _, e := s.resourceProfile(ctx, id)
		if e != nil {
			return nil, e
		}
		return []proto.Message{response, &pb.SCPlayer{Player: current}}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	listing, err := s.p11PlunderListAt(ctx, id, req.GetFragId(), now)
	if err != nil {
		return nil, err
	}
	peers := []int64{}
	seen := map[int64]bool{}
	for _, entry := range listing[0].(*pb.SCTreasureBattleList).InfoList {
		peer := entry.GetSimplePlayerInfo().GetPlayerId()
		if peer > 0 && peer != id && !seen[peer] {
			peers = append(peers, peer)
			seen[peer] = true
		}
	}
	if len(peers) == 0 {
		return nil, errors.New(constUtil.P11PlunderFightRefresh)
	}
	var response *pb.SCSweepTreasureFight
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		ids := append([]int64{id}, peers...)
		slices.Sort(ids)
		ids = slices.Compact(ids)
		for _, profileID := range ids {
			var locked db.Profile
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, profileID).Error; e != nil {
				return e
			}
		}
		var old db.P11PlunderSweepReceipt
		e := tx.First(&old, "player_id=? AND stamp=?", id, stamp).Error
		if e == nil {
			response, e = p11ReadPlunderSweepReceipt(tx, &old, p6Digest(raw))
			return e
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var owner db.Profile
		if e = tx.First(&owner, id).Error; e != nil {
			return e
		}
		player, game, e := decodeProfileForCalendarUpdate(tx, &owner)
		if e != nil {
			return e
		}
		ordinary, e := s.p10Setting("TreaSweepLevel")
		if e != nil {
			return e
		}
		minLevel, e := s.p10Setting("Plunder5MinLevel")
		if e != nil {
			return e
		}
		minVIP, e := s.p10Setting("Plunder5VipMinLevel")
		if e != nil {
			return e
		}
		if player.GetLevel() < ordinary || req.GetSweepNumber() == 5 && (player.GetLevel() < minLevel && player.GetVip() < minVIP) {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		held, e := p11PlunderFragmentCount(game, req.GetFragId())
		if e != nil {
			return e
		}
		if held > 0 {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		offer, cfg, e := p11PlunderFightOffer(tx, now)
		if e != nil {
			return e
		}
		if !cfg.SweepEnabled {
			return errors.New(constUtil.P11PlunderSweepUnavailable)
		}
		plan := p11PlunderSweepPlan{PlayerID: id, Stamp: stamp, RequestHash: p6Digest(raw), FragmentID: req.GetFragId(), RequestedCount: req.GetSweepNumber(), Source: constUtil.P11PlunderSweepSource, SourceNote: constUtil.P11PlunderSweepSourceNote, Peers: peers, OfferVersion: offer.Version, OfferHash: offer.SourceHash}
		response = &pb.SCSweepTreasureFight{IsLevelUp: proto.Int32(0), Get: &pb.ResultObject{}}
		initialLevel := player.GetLevel()
		for step := int32(0); step < req.GetSweepNumber(); step++ {
			if e = tx.First(&owner, id).Error; e != nil {
				return e
			}
			player, game, e = decodeProfileForCalendarUpdate(tx, &owner)
			if e != nil {
				return e
			}
			held, e = p11PlunderFragmentCount(game, req.GetFragId())
			if e != nil {
				return e
			}
			if held > 0 {
				plan.Stop = "target-obtained"
				break
			}
			if player.GetEnergy() < 2 {
				plan.Stop = "energy-exhausted"
				break
			}
			var chosen int64
			for _, peerID := range peers {
				var peer db.Profile
				if e = tx.First(&peer, peerID).Error; e != nil {
					return e
				}
				other, otherGame, e := decodeProfileForCalendarUpdate(tx, &peer)
				if e != nil {
					return e
				}
				n, e := p11PlunderFragmentCount(otherGame, req.GetFragId())
				if e != nil {
					return e
				}
				protected, e := P11PlunderProtected(tx, peerID, now)
				if e != nil {
					return e
				}
				blocked, e := p10Blocked(tx, id, peerID)
				if e != nil {
					return e
				}
				difference := int64(player.GetLevel()) - int64(other.GetLevel())
				if n > 0 && !protected && !blocked && other.GetLevel() >= ordinary && difference <= int64(cfg.LevelDifference) && -difference <= int64(cfg.LevelDifference) {
					chosen = peerID
					break
				}
			}
			if chosen == 0 {
				plan.Stop = "real-holders-exhausted"
				break
			}
			match, e := s.p11PlunderSweepMatch(tx, id, chosen, req.GetFragId(), offer, cfg, now)
			if e != nil {
				return e
			}
			childStamp, e := p11PlunderSweepChildStamp(tx, id)
			if e != nil {
				return e
			}
			child, e := s.p11PlunderFightTx(ctx, tx, id, &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(chosen), TargetFlagId: req.FragId}, childStamp, now, match.ID)
			if e != nil {
				return e
			}
			var accepted db.P11PlunderFightSettlement
			if e = tx.First(&accepted, "player_id=? AND stamp=?", id, childStamp).Error; e != nil {
				return e
			}
			plan.Children = append(plan.Children, p11PlunderSweepChild{Stamp: childStamp, FightID: accepted.FightID, MatchID: accepted.MatchID, RecordID: accepted.P6RecordID, ResponseHash: accepted.ResponseHash, TerminalHash: accepted.TerminalHash})
			p1AppendGain(response.Get, child.GetGet())
			response.RewardList = append(response.RewardList, &pb.TreasureFightReward{Exp: proto.Int32(accepted.RewardExp), Coin: proto.Int32(accepted.RewardCoin), Reward: child.Reward, IsGetFrag: child.IsGetFrag})
		}
		if len(plan.Children) == 0 {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		if plan.Stop == "" {
			plan.Stop = "requested-count"
		}
		if e = tx.First(&owner, id).Error; e != nil {
			return e
		}
		current, _, e := decodeProfileForCalendarUpdate(tx, &owner)
		if e != nil {
			return e
		}
		if current.GetLevel() != initialLevel {
			response.IsLevelUp = proto.Int32(1)
		}
		response.Cost = []*pb.BaseResource{{Id: proto.String("energy"), Num: proto.Int32(int32(len(plan.Children)) * 2)}}
		body, e := proto.MarshalOptions{Deterministic: true}.Marshal(response)
		if e != nil {
			return e
		}
		planBody, e := json.Marshal(plan)
		if e != nil {
			return e
		}
		return tx.Create(&db.P11PlunderSweepReceipt{PlayerID: id, Stamp: stamp, RequestHash: p6Digest(raw), FragmentID: req.GetFragId(), RequestedCount: req.GetSweepNumber(), AcceptedCount: int32(len(plan.Children)), Plan: planBody, SourceHash: p6Digest(planBody), Response: body, ResponseHash: p6Digest(body), Source: constUtil.P11PlunderSweepSource, CreatedAt: now.UnixMilli(), FinishedAt: now.UnixMilli()}).Error
	})
	if err != nil {
		return nil, err
	}
	current, _, err := s.resourceProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	return []proto.Message{response, &pb.SCPlayer{Player: current}}, nil
}

// p11PlunderSweepChildStamp allocates an internal negative receipt key. It never
// changes a packet timestamp or bypasses the positive originalHTTP guard.
func p11PlunderSweepChildStamp(tx *gorm.DB, id int64) (int64, error) {
	for range 8 {
		n, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			return 0, err
		}
		key := -(n.Int64() + 1)
		var exists int64
		if err = tx.Model(&db.P11PlunderFightSettlement{}).Where("player_id=? AND stamp=?", id, key).Count(&exists).Error; err != nil {
			return 0, err
		}
		if exists == 0 {
			return key, nil
		}
	}
	return 0, errors.New(constUtil.P11PlunderSweepConflict)
}

// p11PlunderSweepMatch freezes the actual locked profiles again for each real
// child. A level-up or earlier fragment transfer cannot reuse an old actor body.
func (s *GameService) p11PlunderSweepMatch(tx *gorm.DB, id, peerID int64, fragment string, offer *db.P11PlunderFightOffer, cfg *db.P11PlunderFightPolicy, now time.Time) (*db.P11PlunderFightMatch, error) {
	var owner, peer db.Profile
	if err := tx.First(&owner, id).Error; err != nil {
		return nil, err
	}
	if err := tx.First(&peer, peerID).Error; err != nil {
		return nil, err
	}
	_, attacker, err := s.p11ArenaProjection(&owner, tx)
	if err != nil {
		return nil, err
	}
	peerInfo, defender, err := s.p11ArenaProjection(&peer, tx)
	if err != nil {
		return nil, err
	}
	if err = SetP6FirstOrder(attacker, defender, false); err != nil {
		return nil, err
	}
	info := &pb.TreasureBattleInfo{SimplePlayerInfo: peerInfo, IsRobot: proto.Int32(0), GetChance: proto.Int32(cfg.ChanceTier)}
	infoBody, err := proto.MarshalOptions{Deterministic: true}.Marshal(info)
	if err != nil {
		return nil, err
	}
	a, err := proto.MarshalOptions{Deterministic: true}.Marshal(attacker)
	if err != nil {
		return nil, err
	}
	d, err := proto.MarshalOptions{Deterministic: true}.Marshal(defender)
	if err != nil {
		return nil, err
	}
	seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}
	configuration, err := s.p11ArenaConfigurationHash()
	if err != nil {
		return nil, err
	}
	match := &db.P11PlunderFightMatch{ID: uuid.NewString(), PlayerID: id, OpponentID: peerID, FragmentID: fragment, OfferVersion: offer.Version, OfferHash: offer.SourceHash, Info: infoBody, Attacker: a, Defender: d, Seed: seed.Int64(), ConfigurationHash: configuration, CapabilityHash: P6CapabilityHash(), CreatedAt: now.UnixMilli(), ExpiresAt: min(now.UnixMilli()+cfg.MatchMillis, cfg.EndAt)}
	match.SourceHash = p11PlunderMatchHash(match)
	if err = tx.Create(match).Error; err != nil {
		return nil, err
	}
	return match, nil
}

// p11ReadPlunderSweepReceipt rebuilds only the immutable original aggregate
// from actual accepted children. No opponent selection or combat runs on retry.
func p11ReadPlunderSweepReceipt(tx *gorm.DB, row *db.P11PlunderSweepReceipt, requestHash string) (*pb.SCSweepTreasureFight, error) {
	bad := errors.New(constUtil.P11PlunderSweepConflict)
	if row == nil || row.Stamp <= 0 || row.RequestHash != requestHash || row.Source != constUtil.P11PlunderSweepSource || row.AcceptedCount < 1 || row.AcceptedCount > row.RequestedCount || row.RequestedCount != 1 && row.RequestedCount != 5 || p6Digest(row.Plan) != row.SourceHash || p6Digest(row.Response) != row.ResponseHash {
		return nil, bad
	}
	var plan p11PlunderSweepPlan
	if json.Unmarshal(row.Plan, &plan) != nil || plan.PlayerID != row.PlayerID || plan.Stamp != row.Stamp || plan.RequestHash != row.RequestHash || plan.FragmentID != row.FragmentID || plan.RequestedCount != row.RequestedCount || plan.Source != row.Source || plan.SourceNote != constUtil.P11PlunderSweepSourceNote || len(plan.Children) != int(row.AcceptedCount) || len(plan.Peers) < 1 || len(plan.Peers) > 3 {
		return nil, bad
	}
	expected := &pb.SCSweepTreasureFight{Get: &pb.ResultObject{}}
	peers := map[int64]bool{}
	for _, peer := range plan.Peers {
		if peer <= 0 || peer == row.PlayerID || peers[peer] {
			return nil, bad
		}
		peers[peer] = true
	}
	seen := map[int64]bool{}
	for _, child := range plan.Children {
		if child.Stamp >= 0 || seen[child.Stamp] {
			return nil, bad
		}
		seen[child.Stamp] = true
		var accepted db.P11PlunderFightSettlement
		if err := tx.First(&accepted, "player_id=? AND stamp=?", row.PlayerID, child.Stamp).Error; err != nil {
			return nil, err
		}
		if !peers[accepted.OpponentID] || accepted.FightID != child.FightID || accepted.MatchID != child.MatchID || accepted.P6RecordID != child.RecordID || accepted.ResponseHash != child.ResponseHash || accepted.TerminalHash != child.TerminalHash || accepted.FragmentID != row.FragmentID || accepted.OfferHash != plan.OfferHash || accepted.CreatedAt != row.CreatedAt {
			return nil, bad
		}
		actual, err := p11ReadPlunderReceipt(tx, &accepted)
		if err != nil {
			return nil, err
		}
		p1AppendGain(expected.Get, actual.GetGet())
		expected.RewardList = append(expected.RewardList, &pb.TreasureFightReward{Exp: proto.Int32(accepted.RewardExp), Coin: proto.Int32(accepted.RewardCoin), Reward: actual.Reward, IsGetFrag: actual.IsGetFrag})
	}
	actual := &pb.SCSweepTreasureFight{}
	if proto.Unmarshal(row.Response, actual) != nil || actual.Player != nil || actual.GetIsLevelUp() < 0 || actual.GetIsLevelUp() > 1 {
		return nil, bad
	}
	expected.IsLevelUp = actual.IsLevelUp
	expected.Cost = []*pb.BaseResource{{Id: proto.String("energy"), Num: proto.Int32(row.AcceptedCount * 2)}}
	if !proto.Equal(expected, actual) {
		return nil, bad
	}
	return actual, nil
}
