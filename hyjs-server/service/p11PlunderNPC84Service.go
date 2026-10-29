package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
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

type p11PlunderNPC84Candidate struct {
	Index  int
	Info   *pb.SimplePlayerInfo
	Battle *pb.BattlePlayer
}

func p11PlunderNPC84ID(id int64) bool {
	return id > constUtil.P11PlunderNPC84IDBase && id <= constUtil.P11PlunderNPC84IDBase+constUtil.P11PlunderNPC84Count
}

func (s *GameService) p11PlunderNPC84Allowed(fragment string) bool {
	row := s.table("Frag", "id", fragment)
	if row == nil {
		return false
	}
	treasure := s.table("Treasure", "id", row["target"])
	return treasure != nil && (treasure["quality"] == "13" || treasure["quality"] == "18" || treasure["quality"] == "20")
}

// The profile table never owns these virtual IDs. Refuse the domain on any
// collision rather than reclassifying or modifying a real account.
func p11PlunderNPC84CheckIdentityDomain(tx *gorm.DB) error {
	var count int64
	if err := tx.Model(&db.Profile{}).Where("id>? AND id<=?", constUtil.P11PlunderNPC84IDBase, constUtil.P11PlunderNPC84IDBase+constUtil.P11PlunderNPC84Count).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return errors.New(constUtil.P11PlunderFightConflict)
	}
	return nil
}

// Each local roster uses original heroes and formation calculations. Training
// provides a reproducible local panel near the requested tier; both the wire
// power and the frozen combat actors come from the same projected GameState.
func (s *GameService) p11PlunderNPC84Projection(index int) (*p11PlunderNPC84Candidate, error) {
	templates := constUtil.P11PlunderNPC84Templates()
	if index < 0 || index >= len(templates) {
		return nil, errors.New(constUtil.P11PlunderFightRefresh)
	}
	template := templates[index]
	if len(template.Heroes) == 0 || len(template.Heroes) > 4 {
		return nil, errors.New(constUtil.P11PlunderFightConflict)
	}
	id := constUtil.P11PlunderNPC84IDBase + int64(index) + 1
	name := fmt.Sprintf("夺宝挑战者·%d", index+1)
	player := &pb.Player{PlayerId: proto.Int64(id), Name: proto.String(name), Level: proto.Int32(100)}
	game := &model.GameState{}
	// The original power weights are att=6, hp=.25 and both defences=10.
	// This equal training mix is a local starting point; the actual result below
	// is always recalculated from the generated battle properties.
	unit := int32(math.Round(float64(template.TargetPower) * 20 / (27 * float64(len(template.Heroes)))))
	for i, heroID := range template.Heroes {
		guid := fmt.Sprintf("plunder84-v1-%d-%d", index+1, i+1)
		warrior := &pb.Warrior{Id: proto.String(heroID), Guid: proto.String(guid), Level: proto.Int32(100), Breached: &pb.Breached{Level: proto.Int32(1)}}
		if i == 0 {
			player.HeroWarId = warrior.Guid
		}
		game.Warriors = append(game.Warriors, warrior)
		game.Formation = append(game.Formation, &pb.FormationItem{FormatPos: proto.Int32(int32(i + 1)), BattlePos: proto.Int32(int32(i + 1)), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: warrior.Id, Guid: warrior.Guid}}})
	}
	setTraining := func(value int32) {
		for _, warrior := range game.Warriors {
			warrior.Training = &pb.Training{Hp: proto.Int32(value * 2), Att: proto.Int32(value / 2), Pdef: proto.Int32(value / 2), Mdef: proto.Int32(value / 2)}
		}
	}
	setTraining(unit)
	initialPower, err := s.formationFightPower(player, game)
	if err != nil || initialPower <= 0 {
		return nil, errors.New(constUtil.P11PlunderFightConflict)
	}
	// Relations affect a full original roster's panel. Calibrate the training
	// mix against the actual projected power, including those same bonuses.
	adjusted := math.Round(float64(unit) * float64(template.TargetPower) / float64(initialPower))
	if adjusted < 1 || adjusted > math.MaxInt32/2 {
		return nil, errors.New(constUtil.P11PlunderFightConflict)
	}
	setTraining(int32(adjusted))
	battle, err := s.battlePlayer(player, game)
	if err != nil {
		return nil, err
	}
	power, err := s.formationFightPower(player, game)
	if err != nil || power <= 0 || math.Abs(float64(power-template.TargetPower)) > float64(template.TargetPower)*0.05 {
		return nil, errors.New(constUtil.P11PlunderFightConflict)
	}
	info := &pb.SimplePlayerInfo{PlayerId: player.PlayerId, ServerId: proto.Int32(int32(s.OriginalServerID())), PlayerName: player.Name, Level: player.Level, HeroWarId: proto.String(template.Heroes[0]), FightPower: proto.Int32(power)}
	return &p11PlunderNPC84Candidate{Index: index, Info: info, Battle: battle}, nil
}

func (s *GameService) p11PlunderNPC84Candidates(tx *gorm.DB, ownerPower int32) ([]p11PlunderNPC84Candidate, error) {
	if err := p11PlunderNPC84CheckIdentityDomain(tx); err != nil {
		return nil, err
	}
	var candidates []p11PlunderNPC84Candidate
	templates := constUtil.P11PlunderNPC84Templates()
	if len(templates) != constUtil.P11PlunderNPC84Count {
		return nil, errors.New(constUtil.P11PlunderFightConflict)
	}
	for i := range templates {
		candidate, err := s.p11PlunderNPC84Projection(i)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, *candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := int64(candidates[i].Info.GetFightPower()), int64(candidates[j].Info.GetFightPower())
		da, db := a-int64(ownerPower), b-int64(ownerPower)
		if da < 0 {
			da = -da
		}
		if db < 0 {
			db = -db
		}
		if da != db {
			return da < db
		}
		if a != b {
			return a < b
		}
		return candidates[i].Index < candidates[j].Index
	})
	return candidates[:3], nil
}

func (s *GameService) p11PlunderNPC84Match(tx *gorm.DB, id int64, fragment string, attacker *pb.BattlePlayer, candidate *p11PlunderNPC84Candidate, offer *db.P11PlunderFightOffer, cfg *db.P11PlunderFightPolicy, configuration string, now time.Time) (*db.P11PlunderFightMatch, *pb.TreasureBattleInfo, error) {
	attack := proto.Clone(attacker).(*pb.BattlePlayer)
	defender := proto.Clone(candidate.Battle).(*pb.BattlePlayer)
	if err := SetP6FirstOrder(attack, defender, false); err != nil {
		return nil, nil, err
	}
	entry := &pb.TreasureBattleInfo{SimplePlayerInfo: candidate.Info, IsRobot: proto.Int32(1), GetChance: proto.Int32(cfg.ChanceTier)}
	info, err := proto.MarshalOptions{Deterministic: true}.Marshal(entry)
	if err != nil {
		return nil, nil, err
	}
	a, err := proto.MarshalOptions{Deterministic: true}.Marshal(attack)
	if err != nil {
		return nil, nil, err
	}
	d, err := proto.MarshalOptions{Deterministic: true}.Marshal(defender)
	if err != nil {
		return nil, nil, err
	}
	seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, nil, err
	}
	match := &db.P11PlunderFightMatch{ID: uuid.NewString(), PlayerID: id, OpponentID: candidate.Info.GetPlayerId(), FragmentID: fragment, OfferVersion: offer.Version, OfferHash: offer.SourceHash, Info: info, Attacker: a, Defender: d, Seed: seed.Int64(), ConfigurationHash: configuration, CapabilityHash: P6CapabilityHash(), CreatedAt: now.UnixMilli(), ExpiresAt: p11PlunderMatchExpires(cfg, now.UnixMilli())}
	match.SourceHash = p11PlunderMatchHash(match)
	if err = tx.Create(match).Error; err != nil {
		return nil, nil, err
	}
	return match, entry, nil
}

// A receipt binds the local source to the actual published candidate and its
// complete frozen defender. The original real-player receipt stays unchanged.
func p11PlunderNPC84Meta(match *db.P11PlunderFightMatch) ([]string, error) {
	if match == nil || !p11PlunderNPC84ID(match.OpponentID) {
		return nil, errors.New(constUtil.P11PlunderFightConflict)
	}
	info := &pb.TreasureBattleInfo{}
	defender := &pb.BattlePlayer{}
	index := int(match.OpponentID - constUtil.P11PlunderNPC84IDBase - 1)
	templates := constUtil.P11PlunderNPC84Templates()
	if index < 0 || index >= len(templates) || proto.Unmarshal(match.Info, info) != nil || proto.Unmarshal(match.Defender, defender) != nil || info.GetIsRobot() != 1 || info.GetSimplePlayerInfo().GetPlayerId() != match.OpponentID || defender.GetPlayerId() != match.OpponentID || info.GetSimplePlayerInfo().GetFightPower() <= 0 || len(defender.Formation) != len(templates[index].Heroes) || defender.Formation[0].GetId() != info.GetSimplePlayerInfo().GetHeroWarId() || defender.GetServerId() != info.GetSimplePlayerInfo().GetServerId() {
		return nil, errors.New(constUtil.P11PlunderFightConflict)
	}
	for i, actor := range defender.Formation {
		if actor.GetId() != templates[index].Heroes[i] || actor.GetGuid() != fmt.Sprintf("plunder84-v1-%d-%d", index+1, i+1) {
			return nil, errors.New(constUtil.P11PlunderFightConflict)
		}
	}
	return []string{constUtil.P11PlunderNPC84Source, strconv.FormatInt(match.OpponentID, 10), p6Digest(match.Info), p6Digest(match.Defender)}, nil
}

// The original request has no match ID. Several lists created in the same
// millisecond can order by random UUID, so choose a still-unused frozen match
// whose complete teams actually match the currently locked actors.
func p11Plunder84ChooseMatch(tx *gorm.DB, id, peerID int64, fragment, matchID string, offer *db.P11PlunderFightOffer, configuration string, actualA, actualD *pb.BattlePlayer, now time.Time) (*db.P11PlunderFightMatch, *pb.BattlePlayer, *pb.BattlePlayer, error) {
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND opponent_id=? AND fragment_id=? AND created_at<=? AND expires_at>?", id, peerID, fragment, now.UnixMilli(), now.UnixMilli())
	if matchID != "" {
		q = q.Where("id=?", matchID)
	}
	var matches []db.P11PlunderFightMatch
	if err := q.Order("created_at DESC,id DESC").Limit(constUtil.P11Plunder84MatchScanLimit).Find(&matches).Error; err != nil {
		return nil, nil, nil, err
	}
	if len(matches) == 0 {
		return nil, nil, nil, errors.New(constUtil.P11PlunderFightRefresh)
	}
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.ID)
	}
	var usedIDs []string
	if err := tx.Model(&db.P11PlunderFightSettlement{}).Where("match_id IN ?", ids).Pluck("match_id", &usedIDs).Error; err != nil {
		return nil, nil, nil, err
	}
	used := make(map[string]bool, len(usedIDs))
	for _, usedID := range usedIDs {
		used[usedID] = true
	}
	for i := range matches {
		match := &matches[i]
		if match.SourceHash != p11PlunderMatchHash(match) {
			return nil, nil, nil, errors.New(constUtil.P11PlunderFightConflict)
		}
		if used[match.ID] || match.OfferVersion != offer.Version || match.OfferHash != offer.SourceHash || match.ConfigurationHash != configuration || match.CapabilityHash != P6CapabilityHash() {
			continue
		}
		attacker, defender := &pb.BattlePlayer{}, &pb.BattlePlayer{}
		if proto.Unmarshal(match.Attacker, attacker) != nil || proto.Unmarshal(match.Defender, defender) != nil || attacker.GetPlayerId() != id || defender.GetPlayerId() != peerID {
			return nil, nil, nil, errors.New(constUtil.P11PlunderFightConflict)
		}
		if p11PlunderNPC84ID(peerID) {
			if _, err := p11PlunderNPC84Meta(match); err != nil {
				return nil, nil, nil, err
			}
		}
		a := proto.Clone(actualA).(*pb.BattlePlayer)
		d := proto.Clone(actualD).(*pb.BattlePlayer)
		defenderFirst := attacker.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1
		if err := SetP6FirstOrder(a, d, defenderFirst); err != nil {
			return nil, nil, nil, err
		}
		if proto.Equal(a, attacker) && proto.Equal(d, defender) {
			return match, attacker, defender, nil
		}
	}
	return nil, nil, nil, errors.New(constUtil.P11PlunderFightRefresh)
}
