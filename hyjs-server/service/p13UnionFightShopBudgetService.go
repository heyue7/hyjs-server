package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Unspent is already purchased/carried inventory, including departed actors.
// There is no separate reservation write: the actual bag is the reservation.
type p13UFShopBudgetQuota struct {
	Enabled                         bool
	Remaining, Used, Unspent, Limit int64
}

// Decode identity only; the complete roster SHA still binds every original
// battle byte without allocating its repeated base64 battle payloads here.
type p13UFShopBudgetRosterMember struct {
	GuildID   string `json:"guildId"`
	PlayerID  int64  `json:"playerId"`
	AppliedAt int64  `json:"appliedAt"`
	FirstOn   bool   `json:"firstOn"`
}

type p13UFShopBudgetOffer struct {
	config p13UFItemBudgetConfig
	hash   string
}

func p13UFShopBudgetRemaining(tx *gorm.DB, match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer, item string) (remaining int64, enabled bool, err error) {
	quotas, err := p13UFShopBudgetBatch(tx, match, actor, []string{item})
	if err != nil {
		return 0, false, err
	}
	quota := quotas[item]
	return quota.Remaining, quota.Enabled, nil
}

// Caller owns the existing guild-policy lock (and the buyer's Profile lock for
// purchases). Mutable rows use current reads in playerID then counter order;
// an earlier RR snapshot cannot hide another buyer's committed bag reservation.
// All reads are bounded by four item IDs and the immutable FirstOn roster/map.
func p13UFShopBudgetBatch(tx *gorm.DB, match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer, items []string) (map[string]p13UFShopBudgetQuota, error) {
	reject := func() (map[string]p13UFShopBudgetQuota, error) { return nil, p13UFItemBudgetError() }
	if tx == nil || match == nil || actor == nil || len(match.ID) != 64 || actor.MatchID != match.ID || actor.PlayerID <= 0 || (actor.Camp != 0 && actor.Camp != 1) {
		return reject()
	}
	result := map[string]p13UFShopBudgetQuota{}
	gates, cars, keys := []string{}, []string{}, []string{}
	for _, item := range items {
		if _, seen := result[item]; seen {
			continue
		}
		result[item] = p13UFShopBudgetQuota{}
		switch item {
		case "ufitem_019", "ufitem_020":
			gates = append(gates, item)
			keys = append(keys, item)
		case "ufitem_017", "ufitem_018":
			cars = append(cars, item)
			keys = append(keys, item)
		}
	}
	if len(keys) == 0 {
		return result, nil
	}
	offers := map[string]p13UFShopBudgetOffer{}
	if len(gates) > 0 {
		var rows []db.P13UnionFightGateItemOffer
		if err := tx.Where("match_id=? AND item_id IN ?", match.ID, gates).Limit(len(gates) + 1).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			cfg := p13UFItemBudgetConfig{row.BudgetRule, row.TeamUseLimit, row.TargetCooldownMillis, row.EffectFamily}
			if row.MatchID != match.ID || row.SourceHash != p13UFGateItemOfferHash(&row) || row.Radius != 5 || row.CooldownMillis < 100 || row.CooldownMillis > 3600000 || row.HPPercent < 1 || row.HPPercent > 100 || row.Enemy != (row.ItemID == "ufitem_020") || strings.TrimSpace(row.Source) == "" || p13UFItemBudgetValidate(row.ItemID, cfg) != nil {
				return reject()
			}
			offers[row.ItemID] = p13UFShopBudgetOffer{cfg, row.SourceHash}
		}
	}
	if len(cars) > 0 {
		var rows []db.P13UnionFightCarItemOffer
		if err := tx.Where("match_id=? AND item_id IN ?", match.ID, cars).Limit(len(cars) + 1).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			cfg := p13UFItemBudgetConfig{row.BudgetRule, row.TeamUseLimit, row.TargetCooldownMillis, row.EffectFamily}
			if row.MatchID != match.ID || row.SourceHash != p13UFCarItemOfferHash(&row) || row.Radius != 5 || row.CooldownMillis < 100 || row.CooldownMillis > 3600000 || row.HPPercent < 1 || row.HPPercent > 100 || strings.TrimSpace(row.Source) == "" || p13UFItemBudgetValidate(row.ItemID, cfg) != nil {
				return reject()
			}
			offers[row.ItemID] = p13UFShopBudgetOffer{cfg, row.SourceHash}
		}
	}
	enabled := false
	for item, offer := range offers {
		if offer.config.Rule != "" {
			result[item] = p13UFShopBudgetQuota{Enabled: true, Limit: offer.config.Limit}
			enabled = true
		}
	}
	unspent := map[string]int64{}
	if enabled {
		if p6Digest(match.Roster) != match.SnapshotHash || match.LeftGuildID == "" || match.RightGuildID == "" || match.LeftGuildID == match.RightGuildID {
			return reject()
		}
		snapshot, err := p13UFFieldStoredProjection(tx, match.ID)
		if err != nil {
			return nil, err
		}
		// The original builder permits at most100 starters per camp. Actual frozen
		// roster cardinality gives the tighter query limit, not an unbounded Find.
		if snapshot.PlayerLimit < 2 || snapshot.PlayerLimit > 200 || snapshot.PlayerLimit%2 != 0 {
			return reject()
		}
		var roster struct{ Left, Right []p13UFShopBudgetRosterMember }
		if json.Unmarshal(match.Roster, &roster) != nil {
			return reject()
		}
		eligible := map[int64]string{}
		eligibleIDs := []int64{}
		seen := map[int64]bool{}
		total := 0
		for camp, side := range [][]p13UFShopBudgetRosterMember{roster.Left, roster.Right} {
			guild := match.LeftGuildID
			if camp == 1 {
				guild = match.RightGuildID
			}
			count := 0
			for _, fighter := range side {
				if fighter.PlayerID <= 0 || seen[fighter.PlayerID] || fighter.GuildID != guild {
					return reject()
				}
				seen[fighter.PlayerID] = true
				if !fighter.FirstOn || fighter.AppliedAt <= 0 {
					continue
				}
				count++
				total++
				if int64(camp) == actor.Camp {
					eligible[fighter.PlayerID] = guild
					eligibleIDs = append(eligibleIDs, fighter.PlayerID)
				}
			}
			if count > int(snapshot.PlayerLimit)/2 {
				return reject()
			}
		}
		if total > int(snapshot.PlayerLimit) || len(eligible) == 0 || eligible[actor.PlayerID] != actor.GuildID {
			return reject()
		}
		var peers []db.P13UnionFightFieldPlayer
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("match_id,player_id,guild_id,camp,frame,frame_hash,map_projection").Where("match_id=? AND (camp=? OR player_id IN ?)", match.ID, actor.Camp, eligibleIDs).Order("player_id").Limit(len(eligible) + 1).Find(&peers).Error; err != nil {
			return nil, err
		}
		if len(peers) > len(eligible) {
			return reject()
		}
		original, err := (&GameService{}).p13UFFieldOriginal(match, tx)
		if err != nil {
			return nil, err
		}
		found := false
		for _, peer := range peers {
			guild, rostered := eligible[peer.PlayerID]
			if !rostered || peer.PlayerID <= 0 || peer.GuildID != guild || peer.MatchID != match.ID || peer.Camp != actor.Camp || eligible[peer.PlayerID] != peer.GuildID || p6Digest(peer.Frame) != peer.FrameHash || p13UFFieldActorProjection(&peer, snapshot) != nil {
				return reject()
			}
			frame := &pb.InstUFPlayer{}
			if proto.Unmarshal(peer.Frame, frame) != nil || frame.GetGuid() != peer.PlayerID || int64(frame.GetCamp()) != peer.Camp || len(frame.Items) > int(constUtil.P13UnionFightShopMaxSlots) {
				return reject()
			}
			occupied, inventory := map[int32]bool{}, map[string]bool{}
			for _, item := range frame.Items {
				if item == nil || item.GetId() == "" || inventory[item.GetId()] || occupied[item.GetIndex()] || item.GetIndex() < 0 || item.GetIndex() >= constUtil.P13UnionFightShopMaxSlots || item.GetPlayerGuid() != peer.PlayerID || item.GetLeftUseNum() < 0 || item.GetTotalUseNum() < item.GetLeftUseNum() {
					return reject()
				}
				definition := original.table("UnionFightItem", "UFItemId", item.GetId())
				maximum, err := strconv.ParseInt(definition["ItemCatchMaxNum"], 10, 32)
				if err != nil || maximum < 1 || int64(item.GetLeftUseNum()) > maximum {
					return reject()
				}
				occupied[item.GetIndex()] = true
				inventory[item.GetId()] = true
				if result[item.GetId()].Enabled {
					unspent[item.GetId()] += int64(item.GetLeftUseNum())
				}
			}
			found = found || peer.PlayerID == actor.PlayerID
		}
		if !found {
			return reject()
		}
	}
	// Read counters even on the legacy path: a surviving budget counter with no
	// matching frozen budget offer is damaged provenance, not unrestricted stock.
	var uses []db.P13UnionFightItemTeamUse
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=? AND camp=? AND item_id IN ?", match.ID, actor.Camp, keys).Order("item_id").Limit(len(keys) + 1).Find(&uses).Error; err != nil {
		return nil, err
	}
	used := map[string]int64{}
	for _, row := range uses {
		offer, ok := offers[row.ItemID]
		if !ok || offer.config.Rule == "" || row.MatchID != match.ID || row.Camp != actor.Camp || row.SourceHash != p13UFItemTeamUseHash(&row) || row.OfferHash != offer.hash || row.Rule != offer.config.Rule || row.Limit != offer.config.Limit || row.Used < 0 || row.Used > row.Limit {
			return reject()
		}
		if _, duplicate := used[row.ItemID]; duplicate {
			return reject()
		}
		used[row.ItemID] = row.Used
	}
	for item, quota := range result {
		if quota.Enabled {
			quota.Used = used[item]
			quota.Unspent = unspent[item]
			quota.Remaining = max(int64(0), quota.Limit-quota.Used-quota.Unspent)
			result[item] = quota
		}
	}
	return result, nil
}

// A purchase check is intentionally side-effect free. Only successful real bag
// insertion reserves units; positive-stamp receipts must return before calling.
func p13UFShopBudgetCheck(quota p13UFShopBudgetQuota, quantity int64) error {
	if quantity <= 0 {
		return errors.New("公会战购买数量无效")
	}
	if quota.Enabled && quantity > quota.Remaining {
		return errors.New("本场本阵营该道具剩余额度不足")
	}
	return nil
}
