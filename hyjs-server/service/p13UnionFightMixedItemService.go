package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p13UFMixedItemUse struct {
	Key, ItemID                                            string
	PlayerID, Stamp, At, Before, After, ItemType, Cooldown int64
}
type p13UFMixedItemEvidence struct {
	Events []p13UFMixedEvent
	Frozen []p13UFMixedFrozenMeta
	Uses   []p13UFMixedItemUse
}
type p13UFMixedItemReceiptMeta struct {
	PlayerID, Stamp, CreatedAt int64
	Operation, RequestHash     string
}

func p13UFMixedItemReceiptMetadata(row *db.P13GuildReceipt) p13UFMixedItemReceiptMeta {
	return p13UFMixedItemReceiptMeta{row.PlayerID, row.Stamp, row.CreatedAt, row.Operation, p6Digest(row.Request)}
}
func p13UFMixedEffectEvent(kind, key string, at int64, data any) p13UFMixedEvent {
	return p13UFMixedEvent{Kind: kind, Key: key, CreatedAt: at, DueAt: at, AppliedAt: at, Meta: p13UFMixedMeta{Kind: kind, Key: key, CreatedAt: at, EffectAt: at, AppliedAt: at, Data: p13UFMixedJSON(data)}}
}

// Effects are successful cast facts; speed's per-target rows share one cast.
// They change no HP here. Actual healing comes only from recovery transitions.
func p13UFMixedEffects(tx *gorm.DB, match *db.P13UnionFightMatch, guilds map[int64]string) (*p13UFMixedItemEvidence, error) {
	out := &p13UFMixedItemEvidence{}
	var items []db.P13UnionFightItemOffer
	if err := tx.Where("match_id=?", match.ID).Limit(4).Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) > 3 {
		return nil, p13UFMixedError()
	}
	itemMap := map[string]*db.P13UnionFightItemOffer{}
	for i := range items {
		row := &items[i]
		if row.SourceHash != p13UFItemOfferHash(row) || strings.TrimSpace(row.Source) == "" || row.ItemID == "ufitem_016" && row.ItemType != 0 || row.ItemID != "ufitem_016" && row.ItemType != 1 || row.HPPercent < 1 || row.HPPercent > 100 || row.Maximum != 5 || row.ItemType < 0 || row.ItemType > 1 || row.CooldownMillis < 100 || row.CooldownMillis > 3600000 || row.ItemID != "ufitem_011" && row.ItemID != "ufitem_012" && row.ItemID != "ufitem_016" {
			return nil, p13UFMixedError()
		}
		itemMap[row.ItemID] = row
		out.Frozen = append(out.Frozen, p13UFMixedFrozenMeta{"life-offer", row.ItemID, row.SourceHash})
	}
	var auraQuotes []db.P13UnionFightHealAuraOffer
	if err := tx.Where("match_id=?", match.ID).Limit(2).Find(&auraQuotes).Error; err != nil {
		return nil, err
	}
	if len(auraQuotes) > 1 {
		return nil, p13UFMixedError()
	}
	var auraQuote *db.P13UnionFightHealAuraOffer
	if len(auraQuotes) > 0 {
		auraQuote = &auraQuotes[0]
		if !p13UFHealAuraQuoteValid(auraQuote) || strings.TrimSpace(auraQuote.Source) == "" {
			return nil, p13UFMixedError()
		}
		out.Frozen = append(out.Frozen, p13UFMixedFrozenMeta{"aura-offer", auraQuote.ItemID, auraQuote.SourceHash})
	}
	var speedQuotes []db.P13UnionFightSpeedItemOffer
	if err := tx.Where("match_id=?", match.ID).Limit(2).Find(&speedQuotes).Error; err != nil {
		return nil, err
	}
	if len(speedQuotes) > 1 {
		return nil, p13UFMixedError()
	}
	var speedQuote *db.P13UnionFightSpeedItemOffer
	if len(speedQuotes) > 0 {
		speedQuote = &speedQuotes[0]
		q := speedQuote
		if q.SourceHash != p13UFSpeedOfferHash(q) || strings.TrimSpace(q.Source) == "" || q.ItemID != "ufitem_015" || !combatFinite(q.Multiplier) || q.Multiplier <= 1 || q.Multiplier > 10 || q.AuthorityMask < 1 || q.AuthorityMask > 7 || q.Radius != 3 || q.DurationMillis != 5000 || q.CooldownMillis < 100 || q.CooldownMillis > 3600000 {
			return nil, p13UFMixedError()
		}
		out.Frozen = append(out.Frozen, p13UFMixedFrozenMeta{"speed-offer", q.ItemID, q.SourceHash})
	}
	var drumQuotes []db.P13UnionFightWarDrumOffer
	if err := tx.Where("match_id=?", match.ID).Limit(2).Find(&drumQuotes).Error; err != nil {
		return nil, err
	}
	if len(drumQuotes) > 1 {
		return nil, p13UFMixedError()
	}
	var drumQuote *db.P13UnionFightWarDrumOffer
	if len(drumQuotes) > 0 {
		drumQuote = &drumQuotes[0]
		if !p13UFDrumQuoteValid(drumQuote) || strings.TrimSpace(drumQuote.Source) == "" {
			return nil, p13UFMixedError()
		}
		out.Frozen = append(out.Frozen, p13UFMixedFrozenMeta{"drum-offer", drumQuote.ItemID, drumQuote.SourceHash})
	}
	var auras []db.P13UnionFightHealAuraEffect
	if err := tx.Where("match_id=?", match.ID).Order("guid").Find(&auras).Error; err != nil {
		return nil, err
	}
	auraMap := map[int64]*db.P13UnionFightHealAuraEffect{}
	for i := range auras {
		row := &auras[i]
		q := auraQuote
		if q == nil || row.SourceHash != p13UFHealAuraEffectHash(row) || row.OfferHash != q.SourceHash || guilds[row.CasterID] == "" || row.StartedAt < match.StartAt || row.StartedAt >= match.EndAt || row.ExpiresAt <= row.StartedAt || row.ExpiresAt > min(row.StartedAt+q.DurationMillis, match.EndAt) || row.NextTickAt < row.StartedAt+q.TickMillis || (row.NextTickAt-row.StartedAt)%q.TickMillis != 0 || row.NextTickAt > row.StartedAt+q.DurationMillis+q.TickMillis || row.EndedAt != 0 && (row.EndedAt < row.StartedAt || row.EndedAt > row.ExpiresAt) || row.GUID != p13UFFieldGUID(match.ID, "heal-aura", fmt.Sprintf("%d/%d", row.CasterID, row.StartedAt)) {
			return nil, p13UFMixedError()
		}
		auraMap[row.GUID] = row
		key := strconv.FormatInt(row.GUID, 10)
		out.Events = append(out.Events, p13UFMixedEffectEvent("aura", key, row.StartedAt, *row))
		out.Uses = append(out.Uses, p13UFMixedItemUse{Key: "aura:" + key, ItemID: "ufitem_013", PlayerID: row.CasterID, Stamp: -1, At: row.StartedAt, Before: -1, After: -1, ItemType: 1, Cooldown: q.CooldownMillis})
	}
	var speeds []db.P13UnionFightSpeedItemEffect
	if err := tx.Where("match_id=?", match.ID).Order("guid").Find(&speeds).Error; err != nil {
		return nil, err
	}
	speedCasts := map[string]bool{}
	for _, row := range speeds {
		q := speedQuote
		if q == nil || row.SourceHash != p13UFSpeedEffectHash(&row) || row.OfferHash != q.SourceHash || row.Multiplier != q.Multiplier || guilds[row.CasterID] == "" || guilds[row.TargetID] != guilds[row.CasterID] || row.StartedAt < match.StartAt || row.StartedAt >= match.EndAt || row.ExpiresAt <= row.StartedAt || row.ExpiresAt > min(row.StartedAt+q.DurationMillis, match.EndAt) || row.EndedAt != 0 && (row.EndedAt < row.StartedAt || row.EndedAt > row.ExpiresAt) || row.GUID != p13UFFieldGUID(match.ID, "speed-buff", fmt.Sprintf("%d/%d/%d", row.CasterID, row.StartedAt, row.TargetID)) {
			return nil, p13UFMixedError()
		}
		key := strconv.FormatInt(row.GUID, 10)
		out.Events = append(out.Events, p13UFMixedEffectEvent("speed", key, row.StartedAt, row))
		cast := fmt.Sprintf("%d/%d", row.CasterID, row.StartedAt)
		if !speedCasts[cast] {
			speedCasts[cast] = true
			out.Uses = append(out.Uses, p13UFMixedItemUse{Key: "speed:" + cast, ItemID: "ufitem_015", PlayerID: row.CasterID, Stamp: -1, At: row.StartedAt, Before: -1, After: -1, ItemType: 1, Cooldown: q.CooldownMillis})
		}
	}
	var drums []db.P13UnionFightWarDrumEffect
	if err := tx.Where("match_id=?", match.ID).Order("guid").Find(&drums).Error; err != nil {
		return nil, err
	}
	for _, row := range drums {
		q := drumQuote
		camp := int64(0)
		if guilds[row.CasterID] == match.RightGuildID {
			camp = 1
		}
		if q == nil || row.SourceHash != p13UFDrumEffectHash(&row) || row.OfferHash != q.SourceHash || row.GuildID != guilds[row.CasterID] || row.GuildID == "" || row.Camp != camp || !combatFinite(float64(row.X)) || !combatFinite(float64(row.Y)) || !combatFinite(float64(row.Z)) || row.StartedAt < match.StartAt || row.StartedAt >= match.EndAt || row.ExpiresAt <= row.StartedAt || row.ExpiresAt > min(row.StartedAt+q.DurationMillis, match.EndAt) || row.EndedAt != 0 && (row.EndedAt < row.StartedAt || row.EndedAt > row.ExpiresAt) || row.GUID != p13UFFieldGUID(match.ID, "war-drum", fmt.Sprintf("%d/%d", row.CasterID, row.StartedAt)) {
			return nil, p13UFMixedError()
		}
		key := strconv.FormatInt(row.GUID, 10)
		out.Events = append(out.Events, p13UFMixedEffectEvent("drum", key, row.StartedAt, row))
		out.Uses = append(out.Uses, p13UFMixedItemUse{Key: "drum:" + key, ItemID: "ufitem_014", PlayerID: row.CasterID, Stamp: -1, At: row.StartedAt, Before: -1, After: -1, ItemType: 1, Cooldown: q.CooldownMillis})
	}
	var directory []struct{ ID string }
	if err := tx.Model(&db.P13UnionFightPlayerRecovery{}).Select("id").Where("match_id=?", match.ID).Order("id").Find(&directory).Error; err != nil {
		return nil, err
	}
	for start := 0; start < len(directory); start += 128 {
		ids := []string{}
		for _, row := range directory[start:min(start+128, len(directory))] {
			ids = append(ids, row.ID)
		}
		var rows []db.P13UnionFightPlayerRecovery
		if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) != len(ids) {
			return nil, p13UFMixedError()
		}
		for _, row := range rows {
			before, after, err := p13UFPlayerRecoveryVerify(&row)
			if err != nil {
				return nil, err
			}
			if row.MatchID != match.ID || guilds[row.PlayerID] == "" || row.LogicalAt < match.StartAt || row.LogicalAt > match.EndAt || row.LogicalAt == match.EndAt && row.Kind != constUtil.P13UFRecoveryAura {
				return nil, p13UFMixedError()
			}
			if row.Kind == constUtil.P13UFRecoveryItem {
				q := itemMap[row.ItemID]
				if q == nil || row.OfferHash != q.SourceHash || row.HPPercent != q.HPPercent || row.LogicalAt != row.AppliedAt {
					return nil, p13UFMixedError()
				}
				out.Uses = append(out.Uses, p13UFMixedItemUse{Key: "recovery:" + row.ID, ItemID: row.ItemID, PlayerID: row.PlayerID, Stamp: row.Stamp, At: row.AppliedAt, Before: int64(row.InventoryBefore), After: int64(row.InventoryAfter), ItemType: q.ItemType, Cooldown: q.CooldownMillis})
			} else {
				q, effect := auraQuote, auraMap[row.EffectGUID]
				if q != nil && (q.EffectRule == constUtil.P13UFHealAuraPendingRule && row.PendingID == "" || q.EffectRule == "" && row.PendingID != "") {
					return nil, p13UFMixedError()
				}
				if q == nil || effect == nil || effect.CasterID != row.CasterID || row.OfferHash != q.SourceHash || row.HPPercent != q.HPPercent || guilds[row.CasterID] != guilds[row.PlayerID] || row.LogicalAt < effect.StartedAt+q.TickMillis || row.LogicalAt > effect.ExpiresAt || (row.LogicalAt-effect.StartedAt)%q.TickMillis != 0 || row.LogicalAt >= effect.NextTickAt || effect.EndedAt > 0 && row.LogicalAt > effect.EndedAt {
					return nil, p13UFMixedError()
				}
			}
			meta := p13UFMixedMeta{Kind: "recovery", Key: row.ID, CreatedAt: row.LogicalAt, EffectAt: row.LogicalAt, AppliedAt: row.AppliedAt, Data: p13UFMixedJSON(row)}
			out.Events = append(out.Events, p13UFMixedEvent{Kind: "recovery", Key: row.ID, CreatedAt: row.LogicalAt, DueAt: row.LogicalAt, AppliedAt: row.AppliedAt, Before: []*pb.InstUFPlayer{before}, After: []*pb.InstUFPlayer{after}, PlayerID: row.PlayerID, Meta: meta})
		}
	}
	return out, nil
}

type p13UFMixedInventoryMove struct {
	Item                      string
	Player, At, Before, After int64
	Inferred                  bool
	Key                       string
}

func p13UFMixedItemClosure(tx *gorm.DB, match *db.P13UnionFightMatch, actors []db.P13UnionFightFieldPlayer, events []p13UFMixedEvent, uses []p13UFMixedItemUse) ([]p13UFMixedEvent, []p13UFMixedFrozenMeta, error) {
	actorsByID := map[int64]*db.P13UnionFightFieldPlayer{}
	ids := []int64{}
	for i := range actors {
		actorsByID[actors[i].PlayerID] = &actors[i]
		ids = append(ids, actors[i].PlayerID)
	}
	moves := []p13UFMixedInventoryMove{}
	cooldowns := map[int64][2]int64{}
	keys := map[string]bool{}
	for _, event := range events {
		if event.Kind != "shop" {
			continue
		}
		var row db.P13UnionFightShopPurchase
		if json.Unmarshal(event.Meta.Data, &row) != nil {
			return nil, nil, p13UFMixedError()
		}
		moves = append(moves, p13UFMixedInventoryMove{Item: row.ItemID, Player: row.PlayerID, At: row.LogicalAt, Before: row.InventoryBefore, After: row.InventoryAfter, Key: "shop:" + row.ID})
	}
	for _, use := range uses {
		if keys[use.Key] || actorsByID[use.PlayerID] == nil || use.At < match.StartAt || use.At >= match.EndAt || use.Cooldown < 100 || use.Cooldown > 3600000 || use.At > math.MaxInt64-use.Cooldown || use.ItemType < 0 || use.ItemType > 1 {
			return nil, nil, p13UFMixedError()
		}
		keys[use.Key] = true
		if use.Before >= 0 && (use.Before < 1 || use.Before > 5 || use.After != use.Before-1) {
			return nil, nil, p13UFMixedError()
		}
		moves = append(moves, p13UFMixedInventoryMove{Item: use.ItemID, Player: use.PlayerID, At: use.At, Before: use.Before, After: use.After, Inferred: use.Before < 0, Key: use.Key})
		cd := cooldowns[use.PlayerID]
		cd[use.ItemType] = max(cd[use.ItemType], use.At+use.Cooldown)
		cooldowns[use.PlayerID] = cd
	}
	if err := p13UFMixedInventoryChain(moves, actorsByID); err != nil {
		return nil, nil, err
	}
	var states []db.P13UnionFightItemState
	if err := tx.Where("match_id=?", match.ID).Find(&states).Error; err != nil {
		return nil, nil, err
	}
	frozen := []p13UFMixedFrozenMeta{}
	seen := map[int64]bool{}
	for _, row := range states {
		cd := cooldowns[row.PlayerID]
		if actorsByID[row.PlayerID] == nil || row.SourceHash != p13UFItemStateHash(&row) || row.NormalUntil != cd[0] || row.DiamondUntil != cd[1] {
			return nil, nil, p13UFMixedError()
		}
		seen[row.PlayerID] = true
		frozen = append(frozen, p13UFMixedFrozenMeta{"item-state", strconv.FormatInt(row.PlayerID, 10), p6Digest(p13UFMixedJSON(row))})
	}
	for player, cd := range cooldowns {
		if (cd[0] > 0 || cd[1] > 0) && !seen[player] {
			return nil, nil, p13UFMixedError()
		}
	}
	receiptEvents := []p13UFMixedEvent{}
	matched := map[string]bool{}
	for start := 0; start < len(ids); start += 128 {
		// The request is at most match+NUL+itemID. Response is not an authority input;
		// the immutable HP/inventory journals own the real business effects.
		var rows []db.P13GuildReceipt
		if err := tx.Select("player_id,operation,stamp,request,created_at").Where("player_id IN ? AND operation=? AND created_at>=? AND created_at<? AND substr(request,1,?)=?", ids[start:min(start+128, len(ids))], constUtil.P13UnionFightItemUseOperation, match.StartAt, match.EndAt, len(match.ID)+1, []byte(match.ID+"\x00")).Order("player_id,stamp").Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			item := string(row.Request[len(match.ID)+1:])
			found := ""
			for _, use := range uses {
				if use.PlayerID == row.PlayerID && use.ItemID == item && use.At == row.CreatedAt && (use.Stamp == row.Stamp || use.Stamp == -1) {
					if found != "" && found != use.Key {
						return nil, nil, p13UFMixedError()
					}
					found = use.Key
				}
			}
			if found == "" || matched[found] || row.Stamp <= 0 {
				return nil, nil, p13UFMixedError()
			}
			matched[found] = true
			receiptEvents = append(receiptEvents, p13UFMixedEffectEvent("item-receipt", fmt.Sprintf("%d/%d", row.PlayerID, row.Stamp), row.CreatedAt, p13UFMixedItemReceiptMetadata(&row)))
		}
	}
	for _, use := range uses {
		if use.Stamp > 0 && !matched[use.Key] {
			return nil, nil, p13UFMixedError()
		}
	}
	return receiptEvents, frozen, nil
}

func p13UFMixedInventoryChain(moves []p13UFMixedInventoryMove, actors map[int64]*db.P13UnionFightFieldPlayer) error {
	sort.Slice(moves, func(i, j int) bool {
		if moves[i].At != moves[j].At {
			return moves[i].At < moves[j].At
		}
		return moves[i].Key < moves[j].Key
	})
	amounts := map[string]int64{}
	seen := map[string]bool{}
	key := func(player int64, item string) string { return fmt.Sprintf("%d/%s", player, item) }
	for begin := 0; begin < len(moves); {
		end := begin + 1
		for end < len(moves) && moves[end].At == moves[begin].At {
			end++
		}
		pending := append([]p13UFMixedInventoryMove(nil), moves[begin:end]...)
		for len(pending) > 0 {
			chosen := -1
			for i, row := range pending {
				if !row.Inferred && amounts[key(row.Player, row.Item)] == row.Before {
					chosen = i
					break
				}
			}
			if chosen < 0 {
				for i, row := range pending {
					if row.Inferred && amounts[key(row.Player, row.Item)] > 0 {
						chosen = i
						break
					}
				}
			}
			if chosen < 0 {
				return p13UFMixedError()
			}
			row := pending[chosen]
			pending = append(pending[:chosen], pending[chosen+1:]...)
			if seen[row.Key] || actors[row.Player] == nil || row.Item == "" {
				return p13UFMixedError()
			}
			seen[row.Key] = true
			amount := row.After
			if row.Inferred {
				amount = amounts[key(row.Player, row.Item)] - 1
			}
			if amount < 0 || amount > 5 {
				return p13UFMixedError()
			}
			amounts[key(row.Player, row.Item)] = amount
		}
		begin = end
	}
	for id, actor := range actors {
		frame, err := p13UFFieldReadActor(actor)
		if err != nil {
			return err
		}
		items := map[string]bool{}
		slots := map[int32]bool{}
		for _, item := range frame.Items {
			if items[item.GetId()] || slots[item.GetIndex()] || item.GetIndex() < 0 || item.GetIndex() >= 5 || item.GetPlayerGuid() != id || item.GetLeftUseNum() < 0 || item.GetTotalUseNum() < item.GetLeftUseNum() {
				return p13UFMixedError()
			}
			items[item.GetId()] = true
			slots[item.GetIndex()] = true
			k := key(id, item.GetId())
			if int64(item.GetLeftUseNum()) != amounts[k] {
				return p13UFMixedError()
			}
			delete(amounts, k)
		}
	}
	for _, amount := range amounts {
		if amount != 0 {
			return p13UFMixedError()
		}
	}
	return nil
}

func p13UFMixedGateUses(tx *gorm.DB, match *db.P13UnionFightMatch, events []p13UFMixedEvent, guilds map[int64]string) ([]p13UFMixedItemUse, error) {
	var offers []db.P13UnionFightGateItemOffer
	if err := tx.Where("match_id=?", match.ID).Limit(3).Find(&offers).Error; err != nil {
		return nil, err
	}
	byHash := map[string]db.P13UnionFightGateItemOffer{}
	for _, row := range offers {
		if row.SourceHash != p13UFGateItemOfferHash(&row) {
			return nil, p13UFMixedError()
		}
		byHash[row.SourceHash] = row
	}
	var gates []db.P13UnionFightGateState
	if err := tx.Select("guid,camp").Where("match_id=?", match.ID).Limit(9).Find(&gates).Error; err != nil {
		return nil, err
	}
	camps := map[int64]int64{}
	for _, gate := range gates {
		camps[gate.GUID] = gate.Camp
	}
	uses := []p13UFMixedItemUse{}
	for _, event := range events {
		if event.Kind != "gate" {
			continue
		}
		var row db.P13UnionFightGateImpact
		if json.Unmarshal(event.Meta.Data, &row) != nil {
			return nil, p13UFMixedError()
		}
		if row.Kind != constUtil.P13UFGateImpactItem {
			continue
		}
		q, ok := byHash[row.OfferHash]
		if !ok || row.LogicalAt != row.AppliedAt || guilds[row.PlayerID] == "" || q.ItemID != row.ItemID || q.HPPercent != int64(math.Abs(float64(row.Percent))) {
			return nil, p13UFMixedError()
		}
		targetCamp, known := camps[row.GateGUID]
		playerCamp := int64(0)
		if guilds[row.PlayerID] == match.RightGuildID {
			playerCamp = 1
		}
		if !known || q.Enemy != (targetCamp != playerCamp) {
			return nil, p13UFMixedError()
		}
		uses = append(uses, p13UFMixedItemUse{Key: "gate:" + row.ID, ItemID: row.ItemID, PlayerID: row.PlayerID, Stamp: row.Stamp, At: row.AppliedAt, Before: int64(row.InventoryBefore), After: int64(row.InventoryAfter), ItemType: 0, Cooldown: q.CooldownMillis})
	}
	return uses, nil
}

func p13UFMixedCarEvents(tx *gorm.DB, match *db.P13UnionFightMatch, guilds map[int64]string) ([]p13UFMixedEvent, []p13UFMixedItemUse, *p13UFCarImpactLedgerData, error) {
	ledger, err := p13UFCarImpactLedger(tx, match)
	if err != nil {
		return nil, nil, nil, err
	}
	var offers []db.P13UnionFightCarItemOffer
	if err = tx.Where("match_id=?", match.ID).Limit(3).Find(&offers).Error; err != nil {
		return nil, nil, nil, err
	}
	byHash := map[string]db.P13UnionFightCarItemOffer{}
	for _, row := range offers {
		if row.SourceHash != p13UFCarItemOfferHash(&row) {
			return nil, nil, nil, p13UFMixedError()
		}
		byHash[row.SourceHash] = row
	}
	events := []p13UFMixedEvent{}
	uses := []p13UFMixedItemUse{}
	for _, row := range ledger.Impacts {
		before, after, err := p13UFCarImpactVerify(&row)
		if err != nil {
			return nil, nil, nil, err
		}
		q, ok := byHash[row.OfferHash]
		expectedCamp := int64(0)
		if guilds[row.PlayerID] == match.RightGuildID {
			expectedCamp = 1
		}
		if !ok || guilds[row.PlayerID] == "" || row.Camp != expectedCamp || q.ItemID != row.ItemID || q.HPPercent != row.HPPercent || row.LogicalAt != row.AppliedAt {
			return nil, nil, nil, p13UFMixedError()
		}
		meta := p13UFMixedMeta{Kind: "car-impact", Key: row.ID, CreatedAt: row.LogicalAt, EffectAt: row.LogicalAt, AppliedAt: row.AppliedAt, Data: p13UFMixedJSON(P13UnionFightCarImpactMetadata(&row))}
		events = append(events, p13UFMixedEvent{Kind: "car-impact", Key: row.ID, CreatedAt: row.LogicalAt, DueAt: row.LogicalAt, AppliedAt: row.AppliedAt, CarBefore: before, CarAfter: after, Meta: meta})
		uses = append(uses, p13UFMixedItemUse{Key: "car-impact:" + row.ID, ItemID: row.ItemID, PlayerID: row.PlayerID, Stamp: row.Stamp, At: row.AppliedAt, Before: row.InventoryBefore, After: row.InventoryAfter, ItemType: 0, Cooldown: q.CooldownMillis})
	}
	return events, uses, ledger, nil
}
func p13UFMixedCarFinal(ledger *p13UFCarImpactLedgerData, combat *p13UFCombatLedger) ([]p13UFMixedFrozenMeta, error) {
	seen := map[int64]bool{}
	frozen := []p13UFMixedFrozenMeta{}
	for _, car := range ledger.Cars {
		if seen[car.CarGUID] {
			return nil, p13UFMixedError()
		}
		seen[car.CarGUID] = true
		if last := combat.LastCars[car.CarGUID]; last != nil {
			if p13UFCarHPIdentity(last) != car.IdentityHash || last.GetHp() != car.CurrentHP {
				return nil, p13UFMixedError()
			}
		} else if car.CurrentHP != car.MaxHP {
			return nil, p13UFMixedError()
		}
		raw, _ := json.Marshal([]string{car.FactorySourceHash, car.CarHash, car.GenerationSourceHash})
		// First generation uses the historical empty generation seal for v2 compatibility.
		if car.Generation == 1 {
			raw, _ = json.Marshal([]string{car.FactorySourceHash, car.CarHash, ""})
		}
		frozen = append(frozen, p13UFMixedFrozenMeta{"car", strconv.FormatInt(car.CarGUID, 10), p6Digest(raw)})
	}
	for id := range combat.LastCars {
		if !seen[id] {
			return nil, p13UFMixedError()
		}
	}
	return frozen, nil
}

func p13UFMixedReceiptCount(tx *gorm.DB, match *db.P13UnionFightMatch, ids []int64) (int64, error) {
	total := int64(0)
	for start := 0; start < len(ids); start += 128 {
		var count int64
		if err := tx.Model(&db.P13GuildReceipt{}).Where("player_id IN ? AND operation=? AND created_at>=? AND created_at<? AND substr(request,1,?)=?", ids[start:min(start+128, len(ids))], constUtil.P13UnionFightItemUseOperation, match.StartAt, match.EndAt, len(match.ID)+1, []byte(match.ID+"\x00")).Count(&count).Error; err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func p13UFMixedCarOffersFrozen(tx *gorm.DB, match *db.P13UnionFightMatch) ([]p13UFMixedFrozenMeta, error) {
	out := []p13UFMixedFrozenMeta{}
	var items []db.P13UnionFightCarItemOffer
	if err := tx.Where("match_id=?", match.ID).Limit(3).Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) > 2 {
		return nil, p13UFMixedError()
	}
	for _, row := range items {
		if row.SourceHash != p13UFCarItemOfferHash(&row) {
			return nil, p13UFMixedError()
		}
		out = append(out, p13UFMixedFrozenMeta{"car-item-offer", row.ItemID, row.SourceHash})
	}
	var cycles []db.P13UnionFightFactoryCycleOffer
	if err := tx.Where("match_id=?", match.ID).Limit(5).Find(&cycles).Error; err != nil {
		return nil, err
	}
	if len(cycles) > 4 {
		return nil, p13UFMixedError()
	}
	for _, row := range cycles {
		if row.SourceHash != p13UFFactoryCycleOfferHash(&row) {
			return nil, p13UFMixedError()
		}
		out = append(out, p13UFMixedFrozenMeta{"car-cycle-offer", strconv.FormatInt(row.FactoryGUID, 10), row.SourceHash})
	}
	return out, nil
}

// A captured combat may reference an effect that has legitimately expired by
// final settlement. Reconstruct its active immutable source, not a new bonus.
func p13UFMixedEffectReferences(events []p13UFMixedEvent, actors []db.P13UnionFightFieldPlayer, guilds map[int64]string) error {
	type tagSource struct {
		Start, End int64
		Guild      string
	}
	known := map[string]tagSource{}
	actorIDs := map[int64]bool{}
	for _, actor := range actors {
		actorIDs[actor.PlayerID] = true
	}
	for _, event := range events {
		if event.Kind == "speed" {
			var row db.P13UnionFightSpeedItemEffect
			if json.Unmarshal(event.Meta.Data, &row) != nil || !actorIDs[row.CasterID] || !actorIDs[row.TargetID] {
				return p13UFMixedError()
			}
		}
		if event.Kind != "drum" {
			continue
		}
		var row db.P13UnionFightWarDrumEffect
		if json.Unmarshal(event.Meta.Data, &row) != nil || !actorIDs[row.CasterID] {
			return p13UFMixedError()
		}
		end := row.ExpiresAt
		if row.EndedAt > 0 {
			end = min(end, row.EndedAt)
		}
		active := row
		active.EndedAt = 0
		active.SourceHash = p13UFDrumEffectHash(&active)
		tag := constUtil.P13UnionFightWarDrumSource + ";effect=" + strconv.FormatInt(row.GUID, 10) + ";offer=" + row.OfferHash + ";effectsha=" + active.SourceHash
		known[tag] = tagSource{row.StartedAt, end, row.GuildID}
	}
	for _, event := range events {
		if event.Kind != "ordinary" && event.Kind != "guard" {
			continue
		}
		guildSet := map[string]bool{}
		for _, player := range event.Before {
			guildSet[guilds[player.GetGuid()]] = true
		}
		for hero, tags := range event.CombatApplied {
			for _, tag := range tags {
				if !strings.HasPrefix(tag, constUtil.P13UnionFightWarDrumSource+";effect=") {
					continue
				}
				source, ok := known[tag]
				owner := event.CombatOwners[hero]
				if !ok || owner <= 0 || guilds[owner] != source.Guild || event.CreatedAt < source.Start || event.CreatedAt >= source.End || !guildSet[source.Guild] {
					return p13UFMixedError()
				}
			}
		}
	}
	return nil
}
