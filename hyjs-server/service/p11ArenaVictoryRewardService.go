package service

import (
	"errors"
	"math"
	"strconv"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p11ArenaVictoryRewardAmounts derives the user-confirmed fixed values from the
// pre-settlement level basis. The experience must stay addressable by the
// original SimpleItem payload, so implausible levels are rejected instead of
// truncated.
func p11ArenaVictoryRewardAmounts(level int64) (popularity, exp int64, err error) {
	if level < 1 || level > math.MaxInt32/constUtil.P11ArenaVictoryExpPerLevel {
		return 0, 0, errors.New(constUtil.P11ArenaVictoryRewardConflict)
	}
	return constUtil.P11ArenaVictoryPopularity, level * constUtil.P11ArenaVictoryExpPerLevel, nil
}

// applyP11ArenaVictoryReward grants every actual accepted arena victory the
// confirmed popularity and level-scaled real experience inside the existing P6
// settlement transaction. It is independent of the operator three-card pool, its
// level/quota windows and its frozen offer, and it never touches GrantPB, which
// still describes only the original flip card. Fame and experience both reuse
// the original simple-resource consumer and UserExp progression, so a level-up
// keeps the inherited stamina/energy semantics.
func (s *GameService) applyP11ArenaVictoryReward(row *db.P11ArenaFightSettlement, award *db.P11ArenaPrizeAward, player *pb.Player, game *model.GameState, response *pb.SCPvpFightResult) error {
	bad := errors.New(constUtil.P11ArenaVictoryRewardConflict)
	if row == nil || award == nil || player == nil || game == nil || response == nil {
		return bad
	}
	// A challenger loss grants neither fame nor experience.
	if row.WinnerID != row.PlayerID {
		return nil
	}
	// The stored level was captured before any victory grant, so it is the real
	// pre-settlement basis even though earlier rewards already ran.
	if award.PlayerLevel < 1 || int64(player.GetLevel()) != award.PlayerLevel {
		return bad
	}
	popularity, exp, err := p11ArenaVictoryRewardAmounts(award.PlayerLevel)
	if err != nil || player.GetExp() < 0 || player.GetExp() > math.MaxInt64-exp {
		return bad
	}
	gain, err := s.GrantFixedReward(player, game, "popularity,"+strconv.FormatInt(popularity, 10))
	if err != nil {
		return err
	}
	expected := &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("popularity"), Num: proto.Int32(int32(popularity))}}}
	if !proto.Equal(gain, expected) {
		return bad
	}
	// Experience follows the recovered UserExp thresholds and level rewards; a
	// configuration or overflow failure rolls the whole settlement back.
	if err = s.addCopySweepExperience(player, exp); err != nil {
		return err
	}
	p1AppendGain(response.Result, &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("popularity"), Num: proto.Int32(int32(popularity))}, {Id: proto.String("exp"), Num: proto.Int32(int32(exp))}}})
	award.VictoryRewardSource = constUtil.P11ArenaVictoryRewardSource
	award.VictoryRewardLevel, award.VictoryRewardPopularity, award.VictoryRewardExp = award.PlayerLevel, popularity, exp
	return nil
}

// p11ArenaVictoryRewardFacts rebuilds the frozen victory projection for receipt
// validation. Awards written before this source existed keep their original hash
// and replay unchanged and are never backfilled; partially populated facts are
// treated as corruption.
func p11ArenaVictoryRewardFacts(row *db.P11ArenaFightSettlement, award *db.P11ArenaPrizeAward, expected *pb.ResultObject) error {
	bad := errors.New(constUtil.P11ArenaVictoryRewardConflict)
	if row == nil || award == nil || expected == nil {
		return bad
	}
	if award.VictoryRewardSource == "" {
		if award.VictoryRewardLevel != 0 || award.VictoryRewardPopularity != 0 || award.VictoryRewardExp != 0 {
			return bad
		}
		return nil
	}
	if award.VictoryRewardSource != constUtil.P11ArenaVictoryRewardSource || row.WinnerID != row.PlayerID || award.PlayerLevel < 1 || award.VictoryRewardLevel != award.PlayerLevel {
		return bad
	}
	popularity, exp, err := p11ArenaVictoryRewardAmounts(award.VictoryRewardLevel)
	if err != nil || award.VictoryRewardPopularity != popularity || award.VictoryRewardExp != exp {
		return bad
	}
	p1AppendGain(expected, &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("popularity"), Num: proto.Int32(int32(popularity))}, {Id: proto.String("exp"), Num: proto.Int32(int32(exp))}}})
	return nil
}
