package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeP11PlunderFight seeds no publication, target, victory or fragment.
func (s *GameService) InitializeP11PlunderFight(ctx context.Context) error {
	for key, expected := range map[string]int32{"RobTreasureLevel": 15, "TreaSweepCostEnergy": 2, "TreaSweepCoinPerLevel": 55, "TreaSweepExpPerLevel": 2} {
		value, err := s.p10Setting(key)
		if err != nil || value != expected {
			return errors.New(constUtil.P11PlunderFightConflict)
		}
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P11PlunderFightPolicy{}, &db.P11PlunderFightOffer{}, &db.P11PlunderFightMatch{}, &db.P11PlunderFightSettlement{}); err != nil {
		return err
	}
	cfg := db.P11PlunderFightPolicy{ID: 1, Version: "local-plunder-type3-v1", MatchCount: 3, ScanLimit: 512, LevelDifference: 20, MatchMillis: 300000, ChanceTier: 3, DropPPM: 250000, Source: constUtil.P11PlunderFightPolicySource}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

func p11PlunderFightPolicyValid(cfg *db.P11PlunderFightPolicy) bool {
	return cfg != nil && cfg.ID == 1 && cfg.Enabled && strings.TrimSpace(cfg.Version) != "" && len(cfg.Version) <= 64 && cfg.Source == constUtil.P11PlunderFightPolicySource && cfg.PublishedAt > 0 && cfg.PublishedAt <= cfg.StartAt && cfg.EndAt > cfg.StartAt && cfg.EndAt-cfg.StartAt <= int64(31*24*time.Hour/time.Millisecond) && cfg.MatchCount >= 1 && cfg.MatchCount <= 3 && cfg.ScanLimit >= cfg.MatchCount && cfg.ScanLimit <= 4096 && cfg.LevelDifference >= 0 && cfg.LevelDifference <= 100 && cfg.MatchMillis >= 1000 && cfg.MatchMillis <= 300000 && cfg.ChanceTier >= 1 && cfg.ChanceTier <= 6 && cfg.DropPPM >= 0 && cfg.DropPPM <= 1000000
}

func p11PlunderFightOfferRules(offer *db.P11PlunderFightOffer) (*db.P11PlunderFightPolicy, error) {
	var cfg db.P11PlunderFightPolicy
	if offer == nil || p6Digest([]byte(offer.Snapshot)) != offer.SourceHash || json.Unmarshal([]byte(offer.Snapshot), &cfg) != nil || !p11PlunderFightPolicyValid(&cfg) || cfg.Version != offer.Version || offer.CapturedAt < cfg.StartAt || offer.CapturedAt >= cfg.EndAt {
		return nil, errors.New(constUtil.P11PlunderFightConflict)
	}
	return &cfg, nil
}

// p11PlunderFightOffer locks one local policy after the ordered profile locks.
// Its existing immutable version supplies all parameters; live field edits never
// alter pending candidates or the chance already displayed by the original UI.
func p11PlunderFightOffer(tx *gorm.DB, now time.Time) (*db.P11PlunderFightOffer, *db.P11PlunderFightPolicy, error) {
	var live db.P11PlunderFightPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, 1).Error; err != nil {
		return nil, nil, err
	}
	if !live.Enabled {
		return nil, nil, errors.New(constUtil.P11PlunderFightUnavailable)
	}
	var offer db.P11PlunderFightOffer
	err := tx.First(&offer, "version=?", live.Version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !p11PlunderFightPolicyValid(&live) || now.UnixMilli() < live.StartAt || now.UnixMilli() >= live.EndAt {
			return nil, nil, errors.New(constUtil.P11PlunderFightUnavailable)
		}
		raw, err := json.Marshal(live)
		if err != nil {
			return nil, nil, err
		}
		offer = db.P11PlunderFightOffer{Version: live.Version, Snapshot: string(raw), SourceHash: p6Digest(raw), CapturedAt: now.UnixMilli()}
		if err = tx.Create(&offer).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	cfg, err := p11PlunderFightOfferRules(&offer)
	if err != nil {
		return nil, nil, err
	}
	if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
		return nil, nil, errors.New(constUtil.P11PlunderFightUnavailable)
	}
	return &offer, cfg, nil
}

func (s *GameService) p11PlunderFragment(id string) bool {
	frag := s.table("Frag", "id", id)
	return strings.HasPrefix(id, "frag_trea_") && frag != nil && frag["num"] == "1" && s.table("Treasure", "id", frag["target"]) != nil
}

func p11PlunderFragmentCount(game *model.GameState, id string) (int32, error) {
	var count int32
	seen := false
	for _, item := range game.Items {
		if item.GetId() != id {
			continue
		}
		if seen || item.GetNum() < 0 {
			return 0, errors.New(constUtil.P11PlunderFightConflict)
		}
		seen = true
		count = item.GetNum()
	}
	return count, nil
}

func p11PlunderMatchHash(match *db.P11PlunderFightMatch) string {
	copy := *match
	copy.SourceHash = ""
	raw, err := json.Marshal(copy)
	if err != nil {
		return ""
	}
	return p6Digest(raw)
}

// p11PlunderListAt publishes only real profiles holding this original fragment.
// Peer snapshots use ordinary MVCC reads. The later fight acquires both profiles
// in ID order and rechecks their actual inventory/protection/complete team bytes.
func (s *GameService) p11PlunderListAt(ctx context.Context, id int64, fragment string, now time.Time) ([]proto.Message, error) {
	if !s.p11PlunderFragment(fragment) {
		return nil, errors.New(constUtil.P11PlunderFightRefresh)
	}
	out := &pb.SCTreasureBattleList{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var owner db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, id).Error; err != nil {
			return err
		}
		player, _, err := decodeProfile(&owner)
		if err != nil {
			return err
		}
		minimum, err := s.p10Setting("RobTreasureLevel")
		if err != nil {
			return err
		}
		if player.GetLevel() < minimum {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		offer, cfg, err := p11PlunderFightOffer(tx, now)
		if err != nil {
			return err
		}
		_, attacker, err := s.p11ArenaProjection(&owner, tx)
		if err != nil {
			return err
		}
		config, err := s.p11ArenaConfigurationHash()
		if err != nil {
			return err
		}
		var cursor int64
		var scanned int32
		for scanned < cfg.ScanLimit && len(out.InfoList) < int(cfg.MatchCount) {
			var peers []db.Profile
			if err = tx.Where("id>? AND id<>?", cursor, id).Order("id").Limit(int(min(int32(64), cfg.ScanLimit-scanned))).Find(&peers).Error; err != nil {
				return err
			}
			if len(peers) == 0 {
				break
			}
			for _, peer := range peers {
				cursor = peer.Id
				scanned++
				other, game, err := decodeProfile(&peer)
				if err != nil || other.GetLevel() < minimum || int64(other.GetLevel())-int64(player.GetLevel()) > int64(cfg.LevelDifference) || int64(player.GetLevel())-int64(other.GetLevel()) > int64(cfg.LevelDifference) {
					continue
				}
				count, err := p11PlunderFragmentCount(game, fragment)
				if err != nil || count < 1 {
					continue
				}
				protected, err := P11PlunderProtected(tx, peer.Id, now)
				if err != nil {
					return err
				}
				blocked, err := p10Blocked(tx, id, peer.Id)
				if err != nil {
					return err
				}
				if protected || blocked {
					continue
				}
				info, defender, err := s.p11ArenaProjection(&peer, tx)
				if err != nil {
					continue
				}
				attack := proto.Clone(attacker).(*pb.BattlePlayer)
				defenderFirst := attack.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1
				if err = SetP6FirstOrder(attack, defender, defenderFirst); err != nil {
					return err
				}
				for _, hero := range defender.Formation {
					if hero.GetGuid() == other.GetHeroWarId() {
						info.WeaponId = hero.WeaponId
					}
				}
				entry := &pb.TreasureBattleInfo{SimplePlayerInfo: info, IsRobot: proto.Int32(0), GetChance: proto.Int32(cfg.ChanceTier)}
				infoRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(entry)
				if err != nil {
					return err
				}
				a, err := proto.MarshalOptions{Deterministic: true}.Marshal(attack)
				if err != nil {
					return err
				}
				d, err := proto.MarshalOptions{Deterministic: true}.Marshal(defender)
				if err != nil {
					return err
				}
				seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
				if err != nil {
					return err
				}
				match := db.P11PlunderFightMatch{ID: uuid.NewString(), PlayerID: id, OpponentID: peer.Id, FragmentID: fragment, OfferVersion: offer.Version, OfferHash: offer.SourceHash, Info: infoRaw, Attacker: a, Defender: d, Seed: seed.Int64(), ConfigurationHash: config, CapabilityHash: P6CapabilityHash(), CreatedAt: now.UnixMilli(), ExpiresAt: min(now.UnixMilli()+cfg.MatchMillis, cfg.EndAt)}
				match.SourceHash = p11PlunderMatchHash(&match)
				if err = tx.Create(&match).Error; err != nil {
					return err
				}
				out.InfoList = append(out.InfoList, entry)
				if len(out.InfoList) >= int(cfg.MatchCount) {
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{out}, nil
}

// p11PlunderFightAt holds both real profiles before policy/match locks. Every
// mutation follows the independent type3 certificate; unsupported computation
// rolls back its captured P6 record as well as all resource/cost/result writes.
func (s *GameService) p11PlunderFightAt(ctx context.Context, id int64, request *pb.CSFightTreasureStartResult, stamp int64, now time.Time) ([]proto.Message, error) {
	if stamp <= 0 || id == request.GetFightPlayerId() || !s.p11PlunderFragment(request.GetTargetFlagId()) {
		return nil, errors.New(constUtil.P11PlunderFightRefresh)
	}
	var response *pb.SCFightTreasureStartResult
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var e error
		response, e = s.p11PlunderFightTx(ctx, tx, id, request, stamp, now, "")
		return e
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

// p11PlunderFightTx shares only the actual original type3 settlement transaction
// with the new batch caller. The caller owns ordered real profile locks and the
// transaction; every failure rolls back all captures, resources and facts.
func (s *GameService) p11PlunderFightTx(ctx context.Context, tx *gorm.DB, id int64, request *pb.CSFightTreasureStartResult, stamp int64, now time.Time, matchID string) (*pb.SCFightTreasureStartResult, error) {
	var response *pb.SCFightTreasureStartResult
	var accepted *db.P11PlunderFightSettlement
	err := func() error {
		if _, err := p10LockProfiles(tx, id, request.GetFightPlayerId()); err != nil {
			return err
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
		if err != nil {
			return err
		}
		var previous db.P11PlunderFightSettlement
		err = tx.First(&previous, "player_id=? AND stamp=?", id, stamp).Error
		if err == nil {
			if previous.RequestHash != p6Digest(raw) || previous.OpponentID != request.GetFightPlayerId() || previous.FragmentID != request.GetTargetFlagId() {
				return errors.New(constUtil.P11PlunderFightConflict)
			}
			accepted = &previous
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if s.P12WantedReady {
			for _, pid := range []int64{id, request.GetFightPlayerId()} {
				if err := P12WantedRequireIdle(tx, pid, now); err != nil {
					return err
				}
			}
		}
		if s.P12FieldPVPReady {
			for _, pid := range []int64{id, request.GetFightPlayerId()} {
				if err := P12FieldPVPRequireIdle(tx, pid, now); err != nil {
					return err
				}
			}
		}
		offer, cfg, err := p11PlunderFightOffer(tx, now)
		if err != nil {
			return err
		}
		var owner, peer db.Profile
		if err = tx.First(&owner, id).Error; err != nil {
			return err
		}
		if err = tx.First(&peer, request.GetFightPlayerId()).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &owner)
		if err != nil {
			return err
		}
		other, otherGame, err := decodeProfileForCalendarUpdate(tx, &peer)
		if err != nil {
			return err
		}
		minimum, err := s.p10Setting("RobTreasureLevel")
		if err != nil {
			return err
		}
		energy, err := s.p10Setting("TreaSweepCostEnergy")
		if err != nil || energy != 2 || player.GetLevel() < minimum || other.GetLevel() < minimum || player.GetEnergy() < energy {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		protected, err := P11PlunderProtected(tx, peer.Id, now)
		if err != nil {
			return err
		}
		blocked, err := p10Blocked(tx, id, peer.Id)
		if err != nil {
			return err
		}
		holding, err := p11PlunderFragmentCount(otherGame, request.GetTargetFlagId())
		if err != nil {
			return err
		}
		if protected || blocked || holding < 1 {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		var match db.P11PlunderFightMatch
		matchQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND opponent_id=? AND fragment_id=? AND expires_at>?", id, peer.Id, request.GetTargetFlagId(), now.UnixMilli())
		if matchID != "" {
			matchQuery = matchQuery.Where("id=?", matchID)
		}
		if err = matchQuery.Order("created_at DESC,id DESC").First(&match).Error; err != nil {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		configuration, err := s.p11ArenaConfigurationHash()
		if err != nil {
			return err
		}
		if match.SourceHash != p11PlunderMatchHash(&match) || match.OfferVersion != offer.Version || match.OfferHash != offer.SourceHash || match.ConfigurationHash != configuration || match.CapabilityHash != P6CapabilityHash() {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		attacker, defender := &pb.BattlePlayer{}, &pb.BattlePlayer{}
		if proto.Unmarshal(match.Attacker, attacker) != nil || proto.Unmarshal(match.Defender, defender) != nil || attacker.GetPlayerId() != id || defender.GetPlayerId() != peer.Id {
			return errors.New(constUtil.P11PlunderFightConflict)
		}
		_, actualA, err := s.p11ArenaProjection(&owner, tx)
		if err != nil {
			return err
		}
		_, actualD, err := s.p11ArenaProjection(&peer, tx)
		if err != nil {
			return err
		}
		defenderFirst := attacker.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1
		if err = SetP6FirstOrder(actualA, actualD, defenderFirst); err != nil {
			return err
		}
		if !proto.Equal(actualA, attacker) || !proto.Equal(actualD, defender) {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		var already int64
		if err = tx.Model(&db.P11PlunderFightSettlement{}).Where("match_id=?", match.ID).Count(&already).Error; err != nil {
			return err
		}
		if already != 0 {
			return errors.New(constUtil.P11PlunderFightRefresh)
		}
		fightID := uuid.NewString()
		input := model.FrozenBattleInput{Attacker: attacker, Defender: defender, BattleType: 3, MaxRound: 20, Seed: match.Seed, DefenderFirst: defenderFirst, AppliedProperties: map[string][]string{"source": {constUtil.P11PlunderFightSource}, "plunderTarget": {request.GetTargetFlagId()}, "plunderMatch": {match.ID, match.SourceHash}, "plunderOffer": {offer.Version, offer.SourceHash}, "plunderRequest": {p6Digest(raw)}}}
		recordID, err := s.CaptureBattleReplay(ctx, tx, id, constUtil.P11PlunderFightSource, fightID, input)
		if err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err = tx.First(&record, "id=?", recordID).Error; err != nil {
			return err
		}
		frozen, err := decodeP6Record(&record)
		if err != nil {
			return err
		}
		terminal, err := s.ComputeP6PlunderSettlement(ctx, frozen)
		if err != nil {
			return err
		}
		if terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || !p6KnownPlunderSettlementProof(frozen, terminal.Outcome.CapabilityHash) || terminal.Outcome.Winner < 0 || terminal.Outcome.Winner > 1 {
			return errors.New(constUtil.P11PlunderFightUnavailable)
		}
		won := terminal.Outcome.Winner == 0
		draw := int64(-1)
		transferred := false
		gain := &pb.ResultObject{}
		var coins, experience int32
		player.Energy = proto.Int32(player.GetEnergy() - energy)
		if won {
			coinRate, err := s.p10Setting("TreaSweepCoinPerLevel")
			if err != nil {
				return err
			}
			expRate, err := s.p10Setting("TreaSweepExpPerLevel")
			if err != nil || coinRate != 55 || expRate != 2 || player.GetLevel() > math.MaxInt32/coinRate || player.GetLevel() > math.MaxInt32/expRate {
				return errors.New(constUtil.P11PlunderFightConflict)
			}
			coins, experience = player.GetLevel()*coinRate, player.GetLevel()*expRate
			// The original simple-reward primitive owns coin and inventory. Player
			// experience follows the recovered UserExp thresholds and level rewards.
			if player.GetExp() < 0 || player.GetExp() > math.MaxInt64-int64(experience) {
				return errors.New(constUtil.P11PlunderFightConflict)
			}
			gain, err = s.GrantFixedReward(player, game, fmt.Sprintf("coin,%d", coins))
			if err != nil {
				return err
			}
			if err = s.addCopySweepExperience(player, int64(experience)); err != nil {
				return err
			}
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String("exp"), Num: proto.Int32(experience)})
			choice, err := rand.Int(rand.Reader, big.NewInt(1000000))
			if err != nil {
				return err
			}
			draw = choice.Int64()
			transferred = draw < cfg.DropPPM
			if transferred {
				if err = deductPoolCost(other, otherGame, &pb.BaseResource{Id: request.TargetFlagId, Num: proto.Int32(1)}); err != nil {
					return err
				}
				fragment, err := s.GrantFixedReward(player, game, request.GetTargetFlagId()+",1")
				if err != nil {
					return err
				}
				proto.Merge(gain, fragment)
				if err = saveProfile(tx, &peer, other, otherGame); err != nil {
					return err
				}
			}
		}
		if err = saveProfile(tx, &owner, player, game); err != nil {
			return err
		}
		winner, fragmentFlag := int32(0), int32(0)
		reward := "coin,0"
		if won {
			winner = 1
		}
		if transferred {
			fragmentFlag, reward = 1, request.GetTargetFlagId()+",1"
		}
		response = &pb.SCFightTreasureStartResult{FightId: proto.String(fightID), RoomId: proto.String("map_arena_001"), RandSeed: proto.Int64(frozen.Seed), PlayerList: []*pb.BattlePlayer{proto.Clone(frozen.Attacker).(*pb.BattlePlayer), proto.Clone(frozen.Defender).(*pb.BattlePlayer)}, IsWin: proto.Int32(winner), MaxRoundNum: proto.Int32(20), Get: gain, Cost: []*pb.BaseResource{{Id: proto.String("energy"), Num: proto.Int32(energy)}}, IsGetFrag: proto.Int32(fragmentFlag), Reward: proto.String(reward), RewardFake1: proto.String("coin,0"), RewardFake2: proto.String("coin,0")}
		body, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
		if err != nil {
			return err
		}
		final, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		if err = p11StorePlunderP6(tx, &record, frozen, terminal, now); err != nil {
			return err
		}
		row := db.P11PlunderFightSettlement{PlayerID: id, Stamp: stamp, OpponentID: peer.Id, FragmentID: request.GetTargetFlagId(), RequestHash: p6Digest(raw), FightID: fightID, MatchID: match.ID, MatchHash: match.SourceHash, P6RecordID: recordID, ScopeProof: terminal.Outcome.CapabilityHash, OfferHash: offer.SourceHash, AttackerWon: won, Draw: draw, DropPPM: cfg.DropPPM, FragmentTransferred: transferred, EnergySpent: energy, RewardCoin: coins, RewardExp: experience, Response: body, ResponseHash: p6Digest(body), Terminal: final, TerminalHash: p6Digest(final), CreatedAt: now.UnixMilli()}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		// Record the original activity attempt only for an already open operator
		// publication, in the same transaction as this accepted paid challenge.
		if err = s.recordP4VerifiedPlunderAttempt(tx, &row, response, now); err != nil {
			return err
		}
		if len(s.Tables["ActivityNewOpen7Day"]) == 249 && tx.Migrator().HasTable(&db.OpenServerActivityRule{}) {
			if err = RecordOpenServerCounter(tx, id, constUtil.OpenServerActivityCounterPredatory+p1Day(LocalPlayNow(tx.Statement.Context)), 1); err != nil {
				return err
			}
			if err = s.RefreshOpenServerProgress(tx, player, game); err != nil {
				return err
			}
		}
		if s.P1Ready {
			if err = s.RecordP1Event(tx, id, "p11-plunder-attempt:"+fightID, "treasure_fight_accepted", 1, now); err != nil {
				return err
			}
			if transferred {
				return s.RecordP1Event(tx, id, "p11-plunder-fragment:"+fightID, "treasure_fragment_transferred", 1, now)
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	if accepted != nil {
		return p11ReadPlunderReceipt(tx, accepted)
	}
	return response, nil
}

func p11StorePlunderP6(tx *gorm.DB, record *db.P6BattleRecord, input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome, now time.Time) error {
	if record.Status != constUtil.P6BattleCaptured || record.Source != constUtil.P11PlunderFightSource || terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || !p6KnownPlunderSettlementProof(input, terminal.Outcome.CapabilityHash) {
		return errors.New(constUtil.P11PlunderFightConflict)
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
	if err != nil {
		return err
	}
	body, err := json.Marshal(terminal.Outcome)
	if err != nil {
		return err
	}
	result := p6PlaybackResult(terminal.Outcome.Winner)
	saved := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=?", record.ID, constUtil.P6BattleCaptured).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": operations, "outcome": body, "outcome_hash": p6Digest(body), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), operations...), byte(result))), "authoritative": true, "finished_at": now.UnixMilli()})
	if saved.Error != nil {
		return saved.Error
	}
	if saved.RowsAffected != 1 {
		return errors.New(constUtil.P11PlunderFightConflict)
	}
	return nil
}

// p11ReadPlunderReceipt reads only immutable accepted match/offer/P6 material.
// Replays keep the original fragment transfer, cost and exact frozen teams;
// current team changes and operator closure never execute another transfer.
func p11ReadPlunderReceipt(tx *gorm.DB, row *db.P11PlunderFightSettlement) (*pb.SCFightTreasureStartResult, error) {
	bad := errors.New(constUtil.P11PlunderFightConflict)
	if row == nil || row.EnergySpent != 2 || p6Digest(row.Response) != row.ResponseHash || p6Digest(row.Terminal) != row.TerminalHash {
		return nil, bad
	}
	var match db.P11PlunderFightMatch
	if err := tx.First(&match, "id=?", row.MatchID).Error; err != nil {
		return nil, err
	}
	var offer db.P11PlunderFightOffer
	if err := tx.First(&offer, "version=?", match.OfferVersion).Error; err != nil {
		return nil, err
	}
	cfg, err := p11PlunderFightOfferRules(&offer)
	if err != nil {
		return nil, err
	}
	if match.SourceHash != p11PlunderMatchHash(&match) || match.SourceHash != row.MatchHash || match.PlayerID != row.PlayerID || match.OpponentID != row.OpponentID || match.FragmentID != row.FragmentID || match.OfferHash != offer.SourceHash || row.OfferHash != offer.SourceHash || row.CreatedAt < match.CreatedAt || row.CreatedAt >= match.ExpiresAt || row.DropPPM != cfg.DropPPM {
		return nil, bad
	}
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, err
	}
	matchedA, matchedD := &pb.BattlePlayer{}, &pb.BattlePlayer{}
	if proto.Unmarshal(match.Attacker, matchedA) != nil || proto.Unmarshal(match.Defender, matchedD) != nil || !proto.Equal(input.Attacker, matchedA) || !proto.Equal(input.Defender, matchedD) {
		return nil, bad
	}
	var final model.CombatFinalHPOutcome
	out := &pb.SCFightTreasureStartResult{}
	if json.Unmarshal(row.Terminal, &final) != nil || proto.Unmarshal(row.Response, out) != nil || final.Outcome == nil || !final.Outcome.Authoritative || !p6KnownPlunderSettlementProof(input, row.ScopeProof) {
		return nil, bad
	}
	// Revalidate the independent type3 actor and terminal boundaries from the
	// immutable accepted input before returning the original resource receipt.
	if err = p11SettlementScope(input, row.ScopeProof, validateP6PlunderScope); err != nil {
		return nil, err
	}
	if err = validateP6PlunderTerminal(input, &final); err != nil {
		return nil, err
	}
	terminalBody, err := json.Marshal(final.Outcome)
	if err != nil {
		return nil, err
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(final.Outcome.Operates)
	if err != nil {
		return nil, err
	}
	closure := *input
	closure.Tables = nil
	inputBody, err := json.Marshal(closure)
	if err != nil {
		return nil, err
	}
	request, err := proto.MarshalOptions{Deterministic: true}.Marshal(&pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(row.OpponentID), TargetFlagId: proto.String(row.FragmentID)})
	if err != nil {
		return nil, err
	}
	meta := input.AppliedProperties
	if !slices.Equal(meta["plunderTarget"], []string{row.FragmentID}) || !slices.Equal(meta["plunderMatch"], []string{match.ID, match.SourceHash}) || !slices.Equal(meta["plunderOffer"], []string{offer.Version, offer.SourceHash}) || !slices.Equal(meta["plunderRequest"], []string{row.RequestHash}) || row.RequestHash != p6Digest(request) {
		return nil, bad
	}
	if record.PlayerID != row.PlayerID || record.Source != constUtil.P11PlunderFightSource || record.SourceFight != row.FightID || record.Status != constUtil.P6BattleComputed || !record.Authoritative || record.OutcomeHash != p6Digest(terminalBody) || p6Digest(record.Outcome) != record.OutcomeHash || p6Digest(operations) != p6Digest(record.Operates) || record.ReplayHash != p6Digest(append(append([]byte(nil), operations...), byte(record.ServerResult))) || record.ClientResult != record.ServerResult || record.ServerResult != p6PlaybackResult(final.Outcome.Winner) || final.CalculationSourceSHA256 != p6Digest(inputBody) || final.ConfigurationHash != record.ConfigurationHash || final.EngineVersion != record.EngineVersion || final.ClientVersion != record.ClientVersion || final.Outcome.CapabilityHash != row.ScopeProof || input.BattleType != 3 || input.MaxRound != 20 || input.Attacker.GetPlayerId() != row.PlayerID || input.Defender.GetPlayerId() != row.OpponentID {
		return nil, bad
	}
	won := final.Outcome.Winner == 0
	if final.Outcome.Winner < 0 || final.Outcome.Winner > 1 || row.AttackerWon != won || won && (row.Draw < 0 || row.Draw >= 1000000) || !won && row.Draw != -1 || row.FragmentTransferred != (won && row.Draw < row.DropPPM) {
		return nil, bad
	}
	expected := map[string]int32{}
	if won {
		level := int64(input.Attacker.GetLevel())
		if level < 15 || level > math.MaxInt32/55 || row.RewardCoin != int32(level*55) || row.RewardExp != int32(level*2) {
			return nil, bad
		}
		expected["coin"], expected["exp"] = row.RewardCoin, row.RewardExp
	} else if row.RewardCoin != 0 || row.RewardExp != 0 {
		return nil, bad
	}
	winner, fragmentFlag, reward := int32(0), int32(0), "coin,0"
	if won {
		winner = 1
	}
	if row.FragmentTransferred {
		fragmentFlag, reward = 1, row.FragmentID+",1"
		expected[row.FragmentID] = 1
	}
	gain := out.GetGet()
	if gain == nil || len(gain.GetWars())+len(gain.GetEquips())+len(gain.GetTreas())+len(gain.GetPets()) != 0 || len(gain.Simps) != len(expected) {
		return nil, bad
	}
	for _, item := range gain.Simps {
		if expected[item.GetId()] != item.GetNum() || item.GetNum() < 1 {
			return nil, bad
		}
		delete(expected, item.GetId())
	}
	if len(expected) != 0 || out.GetFightId() != row.FightID || out.GetRoomId() != "map_arena_001" || out.GetRandSeed() != input.Seed || out.GetMaxRoundNum() != 20 || len(out.PlayerList) != 2 || !proto.Equal(out.PlayerList[0], input.Attacker) || !proto.Equal(out.PlayerList[1], input.Defender) || out.GetIsWin() != winner || out.GetIsGetFrag() != fragmentFlag || out.GetReward() != reward || out.GetRewardFake1() != "coin,0" || out.GetRewardFake2() != "coin,0" || len(out.Cost) != 1 || out.Cost[0].GetId() != "energy" || out.Cost[0].GetNum() != row.EnergySpent {
		return nil, bad
	}
	return out, nil
}
