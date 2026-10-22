package service

import (
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
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

// p11ArenaRankRewardCenterKey is the reward-centre source identity of one
// accepted fight, so a settlement retry returns the original unclaimed or
// already claimed attachment instead of paying twice.
func p11ArenaRankRewardCenterKey(fightID string) string {
	return constUtil.P11ArenaRankRewardCenterSourceKey + fightID
}

func p11ArenaRankRewardCenterRewards(diamond int64) string {
	return "diamond," + strconv.FormatInt(diamond, 10)
}

// p11ArenaRankRewardPaysDirectly selects the receipt shape from the persisted
// award marker. Awards settled before the split keep the direct settlement
// balance; awards settled after it only have a reward-centre attachment.
func p11ArenaRankRewardPaysDirectly(source string) (bool, error) {
	switch source {
	case constUtil.P11ArenaRankRewardSource:
		return true, nil
	case constUtil.P11ArenaRankRewardCenterSource:
		return false, nil
	}
	return false, errors.New(constUtil.P11ArenaPrizeConflict)
}

// applyP11ArenaRankReward advances the paid best-rank boundary and freezes the
// diamond attachment in the caller's settlement transaction. The response and
// the character balance stay untouched; the original reward centre pays once.
func (s *GameService) applyP11ArenaRankReward(tx *gorm.DB, row *db.P11ArenaFightSettlement, award *db.P11ArenaPrizeAward) error {
	before, after, diamond, current, err := p11ArenaRankRewardFacts(tx, row)
	if err != nil || !current {
		return err
	}
	award.RankSource = constUtil.P11ArenaRankRewardCenterSource
	award.RankBestBefore, award.RankBestAfter, award.RankDiamond = before, after, diamond
	// This boundary, the attachment and the fight receipt commit atomically.
	if err = tx.Model(&db.P11ArenaMember{}).Where("player_id=?", row.PlayerID).Update("rank_reward_best", after).Error; err != nil {
		return err
	}
	if diamond == 0 {
		return nil
	}
	_, err = s.EnqueueReward(tx, row.PlayerID, p11ArenaRankRewardCenterKey(row.FightID), constUtil.P11ArenaRankRewardCenterTitle, fmt.Sprintf(constUtil.P11ArenaRankRewardCenterBodyFormat, after), p11ArenaRankRewardCenterRewards(diamond), 0)
	return err
}

// validateP11ArenaRankRewardCenterReceipt checks the attachment identity of a
// split award. A claimed row keeps satisfying the receipt; the claim itself is
// the original reward centre's idempotent step and never re-runs here.
func validateP11ArenaRankRewardCenterReceipt(tx *gorm.DB, row *db.P11ArenaFightSettlement, award *db.P11ArenaPrizeAward) error {
	bad := errors.New(constUtil.P11ArenaPrizeConflict)
	var delivery db.RewardDelivery
	err := tx.First(&delivery, "player_id=? AND source_key=?", row.PlayerID, p11ArenaRankRewardCenterKey(row.FightID)).Error
	if award.RankDiamond == 0 {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return bad
	}
	if err != nil {
		return err
	}
	if delivery.Rewards != p11ArenaRankRewardCenterRewards(award.RankDiamond) || delivery.Title != constUtil.P11ArenaRankRewardCenterTitle || delivery.Content != fmt.Sprintf(constUtil.P11ArenaRankRewardCenterBodyFormat, award.RankBestAfter) || delivery.ExpiresAt != 0 {
		return bad
	}
	return nil
}
