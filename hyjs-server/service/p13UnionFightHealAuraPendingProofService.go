package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFAuraPendingColumns() []string {
	return []string{"id", "match_id", "effect_guid", "player_id", "caster_id", "guild_id", "camp", "life_ordinal", "player_limit", "tick_at", "observed_at", "hp_percent", "rule", "offer_hash", "effect_hash", "battle_hash", "caster_x", "caster_y", "caster_z", "target_x", "target_y", "target_z", "occupancy_kind", "occupancy_id", "occupancy_input_hash", "occupancy_due_at", "capture_hash", "immutable_hash", "status", "applied_at", "cancel_reason", "cancel_left_at", "recovery_id", "final_hash", "source_hash"}
}

func p13UFMixedAuraPendings(tx *gorm.DB, match *db.P13UnionFightMatch, events []p13UFMixedEvent, actors []db.P13UnionFightFieldPlayer) ([]p13UFMixedEvent, error) {
	var offers []db.P13UnionFightHealAuraOffer
	if err := tx.Where("match_id=?", match.ID).Limit(2).Find(&offers).Error; err != nil {
		return nil, err
	}
	var quote *db.P13UnionFightHealAuraOffer
	for i := range offers {
		if !p13UFHealAuraQuoteValid(&offers[i]) {
			return nil, p13UFMixedError()
		}
		if offers[i].EffectRule == constUtil.P13UFHealAuraPendingRule {
			quote = &offers[i]
		}
	}
	actorMap := map[int64]*db.P13UnionFightFieldPlayer{}
	for i := range actors {
		actorMap[actors[i].PlayerID] = &actors[i]
	}
	occupancies := map[string]p13UFMixedEvent{}
	for _, event := range events {
		if event.Kind == "ordinary" || event.Kind == "guard" {
			occupancies[event.Kind+":"+event.Key] = event
		}
	}
	fullRows := []db.P13UnionFightHealAuraPending{}
	result := []p13UFMixedEvent{}
	cursor := ""
	limit := int64(0)
	for {
		var rows []db.P13UnionFightHealAuraPending
		if err := tx.Where("match_id=? AND id>?", match.ID, cursor).Order("id").Limit(128).Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) > 0 && limit == 0 {
			snapshot, err := p13UFFieldStoredProjection(tx, match.ID)
			if err != nil {
				return nil, err
			}
			limit = int64(snapshot.PlayerLimit)
		}
		for i := range rows {
			row := &rows[i]
			fullRows = append(fullRows, *row)
			cursor = row.ID
			if quote == nil || row.OfferHash != quote.SourceHash || row.PlayerLimit != limit || row.TickAt < match.StartAt || row.TickAt > match.EndAt || row.Status == constUtil.P13UFHealAuraPending || p13UFAuraPendingVerifyLoaded(row) != nil {
				return nil, p13UFMixedError()
			}
			actor := actorMap[row.PlayerID]
			caster := actorMap[row.CasterID]
			if actor == nil || caster == nil || !p13UFMixedActorRoster(match, actor) || !p13UFMixedActorRoster(match, caster) || caster.GuildID != row.GuildID || caster.Camp != row.Camp || actor.BattleHash != row.BattleHash || actor.GuildID != row.GuildID || actor.Camp != row.Camp {
				return nil, p13UFMixedError()
			}
			dx, dy, dz := float64(row.CasterX-row.TargetX), float64(row.CasterY-row.TargetY), float64(row.CasterZ-row.TargetZ)
			if dx*dx+dy*dy+dz*dz > quote.Radius*quote.Radius {
				return nil, p13UFMixedError()
			}
			if row.OccupancyID != "" {
				event, ok := occupancies[row.OccupancyKind+":"+row.OccupancyID]
				if !ok || event.Meta.Record == nil || event.Meta.Record.InputHash != row.OccupancyInputHash || event.DueAt != row.OccupancyDueAt || row.ObservedAt < event.CreatedAt || row.AppliedAt < event.AppliedAt {
					return nil, p13UFMixedError()
				}
				var capture pb.InstUFPlayer
				if proto.Unmarshal(row.Capture, &capture) != nil {
					return nil, p13UFMixedError()
				}
				found := false
				for _, before := range event.Before {
					if before.GetGuid() == row.PlayerID {
						found = true
						if before.GetCamp() != capture.GetCamp() || before.GetDeadNum() != capture.GetDeadNum() || before.GetKillNum() != capture.GetKillNum() || !proto.Equal(before.FightInfo, capture.FightInfo) {
							return nil, p13UFMixedError()
						}
					}
				}
				if !found {
					return nil, p13UFMixedError()
				}
			}
			if row.CancelReason == "left" && (row.CancelLeftAt <= 0 || row.CancelLeftAt > row.AppliedAt) {
				return nil, p13UFMixedError()
			}
			meta := p13UFMixedMeta{Kind: "aura-pending", Key: row.ID, CreatedAt: row.TickAt, EffectAt: row.TickAt, AppliedAt: row.AppliedAt, NoEffect: true, Data: p13UFMixedJSON(p13UFAuraPendingMetadata(row))}
			result = append(result, p13UFMixedEvent{Kind: meta.Kind, Key: row.ID, CreatedAt: row.TickAt, DueAt: row.TickAt, AppliedAt: row.AppliedAt, NoEffect: true, Meta: meta})
		}
		if len(rows) < 128 {
			break
		}
	}
	if err := p13UFMixedEnvironmentOrder(events, fullRows); err != nil {
		return nil, err
	}
	proof := p13UFMixedProof{}
	for _, event := range append(append([]p13UFMixedEvent(nil), events...), result...) {
		proof.Sources = append(proof.Sources, event.Meta)
	}
	if err := p13UFMixedAuraMetaBindings(&proof); err != nil {
		return nil, err
	}
	return result, nil
}

// Lightweight first/downstream closure binds every recovery and every canceled
// or successful pending fact without reading P6 or any actor protobuf payload.
func p13UFMixedAuraMetaBindings(proof *p13UFMixedProof) error {
	rows := map[string]db.P13UnionFightHealAuraPending{}
	recoveries := map[string]db.P13UnionFightPlayerRecovery{}
	effects := map[int64]db.P13UnionFightHealAuraEffect{}
	occupancies := map[string]p13UFMixedMeta{}
	for _, source := range proof.Sources {
		switch source.Kind {
		case "aura-pending":
			var row db.P13UnionFightHealAuraPending
			if json.Unmarshal(source.Data, &row) != nil || p13UFAuraPendingVerifyMeta(&row) != nil || row.Status == constUtil.P13UFHealAuraPending || source.Key != row.ID || source.CreatedAt != row.TickAt || source.EffectAt != row.TickAt || source.AppliedAt != row.AppliedAt || !source.NoEffect {
				return p13UFMixedError()
			}
			if _, exists := rows[row.ID]; exists {
				return p13UFMixedError()
			}
			rows[row.ID] = row
		case "recovery":
			var row db.P13UnionFightPlayerRecovery
			if json.Unmarshal(source.Data, &row) != nil {
				return p13UFMixedError()
			}
			recoveries[row.ID] = row
		case "aura":
			var row db.P13UnionFightHealAuraEffect
			if json.Unmarshal(source.Data, &row) != nil {
				return p13UFMixedError()
			}
			effects[row.GUID] = row
		case "ordinary", "guard":
			occupancies[source.Kind+":"+source.Key] = source
		}
	}
	perEffect := map[int64]int64{}
	perTick := map[string]int64{}
	bound := map[string]bool{}
	for _, row := range rows {
		effect, ok := effects[row.EffectGUID]
		if !ok || row.MatchID != effect.MatchID || row.CasterID != effect.CasterID || row.OfferHash != effect.OfferHash || row.EffectHash != p13UFAuraEffectIdentity(&effect) || row.TickAt < effect.StartedAt+1000 || row.TickAt > effect.ExpiresAt || (row.TickAt-effect.StartedAt)%1000 != 0 || row.TickAt >= effect.NextTickAt || effect.EndedAt > 0 && row.TickAt > effect.EndedAt {
			return p13UFMixedError()
		}
		perEffect[row.EffectGUID]++
		key := strconv.FormatInt(row.EffectGUID, 10) + "/" + strconv.FormatInt(row.TickAt, 10)
		perTick[key]++
		if perEffect[row.EffectGUID] > 5*row.PlayerLimit || perTick[key] > row.PlayerLimit {
			return p13UFMixedError()
		}
		if row.OccupancyID != "" {
			source, ok := occupancies[row.OccupancyKind+":"+row.OccupancyID]
			if !ok || source.Record == nil || source.Record.InputHash != row.OccupancyInputHash || source.EffectAt != row.OccupancyDueAt || row.ObservedAt < source.CreatedAt || row.AppliedAt < source.AppliedAt {
				return p13UFMixedError()
			}
			own := false
			if source.Kind == "ordinary" {
				var meta p13UFCombatAttemptMeta
				if json.Unmarshal(source.Data, &meta) != nil {
					return p13UFMixedError()
				}
				own = row.PlayerID == meta.PlayerID || row.PlayerID == meta.TargetID
			} else {
				var meta p13UFMixedGuardMeta
				if json.Unmarshal(source.Data, &meta) != nil {
					return p13UFMixedError()
				}
				own = row.PlayerID == meta.PlayerID
			}
			if !own {
				return p13UFMixedError()
			}
		}
		if row.RecoveryID != "" {
			recovery, ok := recoveries[row.RecoveryID]
			if !ok || bound[row.RecoveryID] || recovery.PendingID != row.ID || recovery.MatchID != row.MatchID || recovery.PlayerID != row.PlayerID || recovery.CasterID != row.CasterID || recovery.EffectGUID != row.EffectGUID || recovery.OfferHash != row.OfferHash || recovery.HPPercent != 2 || recovery.Kind != constUtil.P13UFRecoveryAura || recovery.LogicalAt != row.TickAt || recovery.AppliedAt != row.AppliedAt || recovery.AfterHash != row.FinalHash || recovery.DeadOrdinal != row.LifeOrdinal || row.Status != constUtil.P13UFHealAuraApplied {
				return p13UFMixedError()
			}
			bound[row.RecoveryID] = true
		}
	}
	for id, recovery := range recoveries {
		if recovery.PendingID != "" && !bound[id] {
			return p13UFMixedError()
		}
	}
	return nil
}

// Replay only the small already-certified scene transitions, in the same
// explicit order used by the drain. This never recalculates a native battle.
func p13UFMixedEnvironmentOrder(events []p13UFMixedEvent, rows []db.P13UnionFightHealAuraPending) error {
	type entry struct {
		At    int64
		Kind  int
		Key   string
		Row   *db.P13UnionFightHealAuraPending
		Event *p13UFMixedEvent
	}
	groups := map[string][]entry{}
	starts := map[string]*pb.InstUFPlayer{}
	recoveries := map[string]*p13UFMixedEvent{}
	for i := range events {
		event := &events[i]
		if event.Kind == "ordinary" || event.Kind == "guard" {
			for _, after := range event.After {
				starts[event.Kind+":"+event.Key+":"+strconv.FormatInt(after.GetGuid(), 10)] = after
			}
		}
		if event.Kind == "recovery" {
			recoveries[event.Key] = event
		}
		if event.Kind == "tower" && !event.NoEffect {
			var meta P13UnionFightTowerHitMeta
			if json.Unmarshal(event.Meta.Data, &meta) != nil {
				return p13UFMixedError()
			}
			if meta.OccupancyID != "" && meta.TargetKind == "player" {
				key := meta.OccupancyKind + ":" + meta.OccupancyID + ":" + strconv.FormatInt(meta.TargetGUID, 10)
				groups[key] = append(groups[key], entry{At: meta.HitAt, Kind: 0, Key: fmt.Sprintf("%020d/%020d", meta.TowerGUID, meta.Sequence), Event: event})
			}
		}
	}
	hasNew := map[string]bool{}
	for i := range rows {
		row := &rows[i]
		if row.OccupancyID == "" {
			continue
		}
		key := row.OccupancyKind + ":" + row.OccupancyID + ":" + strconv.FormatInt(row.PlayerID, 10)
		hasNew[key] = true
		groups[key] = append(groups[key], entry{At: row.TickAt, Kind: 1, Key: row.ID, Row: row})
	}
	for key, list := range groups {
		if !hasNew[key] {
			continue
		} // Old tower-only evidence keeps its original path.
		current := starts[key]
		if current == nil {
			return p13UFMixedError()
		}
		sort.Slice(list, func(i, j int) bool {
			a, b := list[i], list[j]
			return p13UFEnvironmentEventLess(a.At, a.Kind, a.Key, b.At, b.Kind, b.Key)
		})
		for _, item := range list {
			if item.Kind == 0 {
				event := item.Event
				if len(event.Before) != 1 || len(event.After) != 1 || !p13UFMixedPlayerEqual(current, event.Before[0], false) {
					return p13UFMixedError()
				}
				current = event.After[0]
				continue
			}
			row := item.Row
			if row.RecoveryID != "" {
				recovery := recoveries[row.RecoveryID]
				if recovery == nil || len(recovery.Before) != 1 || len(recovery.After) != 1 || !p13UFMixedPlayerEqual(current, recovery.Before[0], false) {
					return p13UFMixedError()
				}
				current = recovery.After[0]
			}
			var final pb.InstUFPlayer
			if proto.Unmarshal(row.Final, &final) != nil || !p13UFMixedPlayerEqual(current, &final, false) {
				return p13UFMixedError()
			}
		}
	}
	return nil
}
