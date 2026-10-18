package service

import (
	"errors"
	"strconv"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Zero is the legacy migration marker. Trust the better existing witnessed rank
// and current rank, without issuing retrospective rewards for unknown history.
func p11ArenaRankRewardBoundary(member db.P11ArenaMember) int64 {
	best := min(member.Rank, constUtil.P11ArenaVirtualSlots+1)
	for _, known := range []int64{member.BestWonRank, member.RankRewardBest} {
		if known > 0 && known < best {
			best = known
		}
	}
	return best
}

// Reward positions are capped independently of actual ladder/history ranks.
func p11ArenaRankRewardDelta(before, achieved int64, won bool) (after, diamond int64) {
	before = min(before, constUtil.P11ArenaVirtualSlots+1)
	after = before
	if won && achieved > 0 && achieved < before {
		after = achieved
	}
	return after, before - after
}

func p11ArenaRankRewardFacts(tx *gorm.DB, row *db.P11ArenaFightSettlement) (before, after, diamond int64, current bool, err error) {
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return
	}
	var input *model.FrozenBattleInput
	input, err = decodeP6Record(&record)
	if err != nil {
		return
	}
	meta := input.AppliedProperties["arenaRankReward"]
	if len(meta) == 0 {
		return
	}
	current = true
	if len(meta) != 2 || meta[0] != constUtil.P11ArenaRankRewardSource {
		err = errors.New(constUtil.P11ArenaPrizeConflict)
		return
	}
	before, err = strconv.ParseInt(meta[1], 10, 64)
	if err != nil || before < 1 || before > row.AttackerOldRank || before > constUtil.P11ArenaVirtualSlots+1 {
		err = errors.New(constUtil.P11ArenaPrizeConflict)
		return
	}
	after, diamond = p11ArenaRankRewardDelta(before, row.AttackerNewRank, row.WinnerID == row.PlayerID)
	if after < 1 {
		err = errors.New(constUtil.P11ArenaPrizeConflict)
		return
	}
	return
}

func (s *GameService) applyP11ArenaRankReward(tx *gorm.DB, row *db.P11ArenaFightSettlement, award *db.P11ArenaPrizeAward, player *pb.Player, game *model.GameState, response *pb.SCPvpFightResult) error {
	before, after, diamond, current, err := p11ArenaRankRewardFacts(tx, row)
	if err != nil || !current {
		return err
	}
	award.RankSource = constUtil.P11ArenaRankRewardSource
	award.RankBestBefore, award.RankBestAfter, award.RankDiamond = before, after, diamond
	// This boundary, the balance and the original response commit atomically.
	if err = tx.Model(&db.P11ArenaMember{}).Where("player_id=?", row.PlayerID).Update("rank_reward_best", after).Error; err != nil {
		return err
	}
	if diamond == 0 {
		return nil
	}
	gain, err := s.GrantFixedReward(player, game, "diamond,"+strconv.FormatInt(diamond, 10))
	if err != nil {
		return err
	}
	expected := &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("diamond"), Num: proto.Int32(int32(diamond))}}}
	if !proto.Equal(gain, expected) {
		return errors.New(constUtil.P11ArenaPrizeConflict)
	}
	p1AppendGain(response.Result, gain)
	return nil
}
