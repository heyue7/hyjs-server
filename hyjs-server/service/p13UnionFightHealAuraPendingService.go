package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFAuraPendingError() error {
	return errors.New("回复法阵占用事件来源或终末不一致")
}
func p13UFAuraEffectRuleValid(rule string) bool {
	return rule == "" || rule == constUtil.P13UFHealAuraPendingRule
}
func p13UFAuraEffectIdentity(row *db.P13UnionFightHealAuraEffect) string {
	raw, _ := json.Marshal(struct {
		Match                    string
		GUID, Caster, Start, End int64
		Offer                    string
	}{row.MatchID, row.GUID, row.CasterID, row.StartedAt, row.ExpiresAt, row.OfferHash})
	return p6Digest(raw)
}
func p13UFAuraPendingSource(row *db.P13UnionFightHealAuraPending, quote *db.P13UnionFightHealAuraOffer, effect *db.P13UnionFightHealAuraEffect, consumed bool) error {
	if quote == nil || effect == nil || !p13UFHealAuraQuoteValid(quote) || quote.EffectRule != constUtil.P13UFHealAuraPendingRule || row.Rule != quote.EffectRule || row.MatchID != quote.MatchID || row.OfferHash != quote.SourceHash || row.HPPercent != quote.HPPercent || effect.MatchID != row.MatchID || effect.GUID != row.EffectGUID || effect.CasterID != row.CasterID || effect.OfferHash != quote.SourceHash || row.EffectHash != p13UFAuraEffectIdentity(effect) || row.TickAt < effect.StartedAt+quote.TickMillis || row.TickAt > effect.ExpiresAt || effect.ExpiresAt > effect.StartedAt+quote.DurationMillis || (row.TickAt-effect.StartedAt)%quote.TickMillis != 0 {
		return p13UFAuraPendingError()
	}
	if consumed && (effect.SourceHash != p13UFHealAuraEffectHash(effect) || row.TickAt >= effect.NextTickAt || effect.EndedAt > 0 && row.TickAt > effect.EndedAt) {
		return p13UFAuraPendingError()
	}
	dx, dy, dz := float64(row.CasterX-row.TargetX), float64(row.CasterY-row.TargetY), float64(row.CasterZ-row.TargetZ)
	if dx*dx+dy*dy+dz*dz > quote.Radius*quote.Radius {
		return p13UFAuraPendingError()
	}
	return nil
}

func p13UFAuraPendingID(row *db.P13UnionFightHealAuraPending) string {
	return p6Digest([]byte(fmt.Sprintf("%s/%d/%d/%d/aura-tick", row.MatchID, row.EffectGUID, row.TickAt, row.PlayerID)))
}
func p13UFAuraPendingHash(row *db.P13UnionFightHealAuraPending) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFAuraPendingImmutable(row *db.P13UnionFightHealAuraPending) string {
	copy := *row
	copy.ImmutableHash = ""
	copy.SourceHash = ""
	copy.Status = ""
	copy.AppliedAt = 0
	copy.CancelReason = ""
	copy.CancelLeftAt = 0
	copy.RecoveryID = ""
	copy.Final = nil
	copy.FinalHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// This is the payload-free directory representation used by sealed results.
func p13UFAuraPendingMetadata(row *db.P13UnionFightHealAuraPending) db.P13UnionFightHealAuraPending {
	copy := *row
	copy.Capture = nil
	copy.Final = nil
	return copy
}
func p13UFAuraPendingVerifyMeta(row *db.P13UnionFightHealAuraPending) error {
	if row == nil || row.ID != p13UFAuraPendingID(row) || len(row.MatchID) != 64 || row.PlayerID <= 0 || row.CasterID <= 0 || row.EffectGUID <= 0 || row.GuildID == "" || row.Camp < 0 || row.Camp > 1 || row.LifeOrdinal < 0 || row.PlayerLimit < 1 || row.PlayerLimit > 200 || row.Rule != constUtil.P13UFHealAuraPendingRule || row.HPPercent != 2 || row.TickAt <= 0 || row.ObservedAt < row.TickAt || len(row.OfferHash) != 64 || len(row.EffectHash) != 64 || len(row.BattleHash) != 64 || len(row.CaptureHash) != 64 || row.ImmutableHash != p13UFAuraPendingImmutable(row) || row.SourceHash != p13UFAuraPendingHash(row) {
		return p13UFAuraPendingError()
	}
	for _, value := range []float32{row.CasterX, row.CasterY, row.CasterZ, row.TargetX, row.TargetY, row.TargetZ} {
		if !combatFinite(float64(value)) {
			return p13UFAuraPendingError()
		}
	}
	if row.OccupancyID != "" {
		if (row.OccupancyKind != "ordinary" && row.OccupancyKind != "guard") || len(row.OccupancyID) != 64 || len(row.OccupancyInputHash) != 64 || row.OccupancyDueAt <= 0 {
			return p13UFAuraPendingError()
		}
	} else if row.OccupancyKind != "" || row.OccupancyInputHash != "" || row.OccupancyDueAt != 0 {
		return p13UFAuraPendingError()
	}
	if row.CancelReason == "left" {
		if row.CancelLeftAt <= 0 || row.CancelLeftAt > row.AppliedAt {
			return p13UFAuraPendingError()
		}
	} else if row.CancelLeftAt != 0 {
		return p13UFAuraPendingError()
	}
	switch row.Status {
	case constUtil.P13UFHealAuraPending:
		if row.AppliedAt != 0 || row.CancelReason != "" || row.RecoveryID != "" || row.FinalHash != "" || row.CancelLeftAt != 0 {
			return p13UFAuraPendingError()
		}
	case constUtil.P13UFHealAuraApplied:
		if row.AppliedAt < row.ObservedAt || row.AppliedAt < row.OccupancyDueAt || row.CancelReason != "" || row.CancelLeftAt != 0 || len(row.FinalHash) != 64 || row.RecoveryID != "" && len(row.RecoveryID) != 64 {
			return p13UFAuraPendingError()
		}
	case constUtil.P13UFHealAuraCanceled:
		if row.AppliedAt < row.ObservedAt || len(row.FinalHash) != 64 || row.RecoveryID != "" || (row.CancelReason != "dead" && row.CancelReason != "life-changed" && row.CancelReason != "left" && row.CancelReason != "occupancy-changed") {
			return p13UFAuraPendingError()
		}
	default:
		return p13UFAuraPendingError()
	}
	return nil
}
func p13UFAuraPendingVerifyLoaded(row *db.P13UnionFightHealAuraPending) error {
	if err := p13UFAuraPendingVerifyMeta(row); err != nil {
		return err
	}
	var capture pb.InstUFPlayer
	if row.CaptureHash != p6Digest(row.Capture) || proto.Unmarshal(row.Capture, &capture) != nil || capture.GetGuid() != row.PlayerID || int64(capture.GetCamp()) != row.Camp || int64(capture.GetDeadNum()) != row.LifeOrdinal || !p13UFResourceLiving(&capture) {
		return p13UFAuraPendingError()
	}
	if row.OccupancyID != "" {
		if capture.GetState() != 1 {
			return p13UFAuraPendingError()
		}
	} else if capture.GetState() != 0 && capture.GetState() != 2 && capture.GetState() != 5 {
		return p13UFAuraPendingError()
	}
	if row.Status == constUtil.P13UFHealAuraPending {
		if len(row.Final) != 0 {
			return p13UFAuraPendingError()
		}
		return nil
	}
	var final pb.InstUFPlayer
	if row.FinalHash != p6Digest(row.Final) || proto.Unmarshal(row.Final, &final) != nil || final.GetGuid() != row.PlayerID || int64(final.GetCamp()) != row.Camp {
		return p13UFAuraPendingError()
	}
	if row.Status == constUtil.P13UFHealAuraApplied {
		if int64(final.GetDeadNum()) != row.LifeOrdinal || !p13UFResourceLiving(&final) || (final.GetState() != 0 && final.GetState() != 2 && final.GetState() != 5) {
			return p13UFAuraPendingError()
		}
		if row.RecoveryID == "" {
			for i, hp := range final.FightInfo.HpPos {
				if hp > 0 && hp != final.FightInfo.MaxHpPos[i] {
					return p13UFAuraPendingError()
				}
			}
		}
	} else if row.CancelReason == "life-changed" && int64(final.GetDeadNum()) == row.LifeOrdinal || row.CancelReason == "dead" && final.GetState() != 3 && p13UFResourceLiving(&final) {
		return p13UFAuraPendingError()
	}
	return nil
}

// The shared guild lock serializes reserve admission with tick consumption and
// occupancy drain. Reserve every potential target conservatively before cost.
func p13UFAuraReserve(tx *gorm.DB, scope string, quote *db.P13UnionFightHealAuraOffer, limit int64) error {
	if quote.EffectRule != constUtil.P13UFHealAuraPendingRule {
		return nil
	}
	if limit < 1 || limit > 200 {
		return p13UFAuraPendingError()
	}
	var maximum struct{ Count int64 }
	if err := tx.Model(&db.P13UnionFightHealAuraPending{}).Select("COUNT(*) AS count").Where("match_id=? AND status=?", scope, constUtil.P13UFHealAuraPending).Group("player_id").Order("count DESC").Limit(1).Scan(&maximum).Error; err != nil {
		return err
	}
	future := int64(0)
	cursor := int64(0)
	for {
		var rows []db.P13UnionFightHealAuraEffect
		if err := tx.Select("match_id,guid,caster_id,started_at,expires_at,next_tick_at,ended_at,offer_hash,source_hash").Where("match_id=? AND ended_at=0 AND next_tick_at<=expires_at AND guid>?", scope, cursor).Order("guid").Limit(128).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			cursor = row.GUID
			if row.SourceHash != p13UFHealAuraEffectHash(&row) || row.OfferHash != quote.SourceHash || row.NextTickAt < row.StartedAt+1000 || row.ExpiresAt > row.StartedAt+5000 {
				return p13UFAuraPendingError()
			}
			future += (row.ExpiresAt-row.NextTickAt)/1000 + 1
			if maximum.Count+future+5 > 5*limit {
				return errors.New(constUtil.P13UFHealAuraBackpressure)
			}
		}
		if len(rows) < 128 {
			break
		}
	}
	if maximum.Count+future+5 > 5*limit {
		return errors.New(constUtil.P13UFHealAuraBackpressure)
	}
	return nil
}

type p13UFAuraOccupancy struct {
	Kind, ID, InputHash string
	DueAt               int64
}

func p13UFAuraOccupancies(tx *gorm.DB, scope string, ids []int64) (map[int64]p13UFAuraOccupancy, error) {
	result := map[int64]p13UFAuraOccupancy{}
	for start := 0; start < len(ids); start += 128 {
		batch := ids[start:min(start+128, len(ids))]
		var ordinary []db.P13UnionFightPlayerBattleAttempt
		if err := tx.Select("id,player_id,target_id,input_hash,due_at").Where("match_id=? AND status=? AND (player_id IN ? OR target_id IN ?)", scope, constUtil.P13UFPlayerBattlePending, batch, batch).Limit(128).Find(&ordinary).Error; err != nil {
			return nil, err
		}
		wanted := map[int64]bool{}
		for _, id := range batch {
			wanted[id] = true
		}
		for _, row := range ordinary {
			for _, pid := range []int64{row.PlayerID, row.TargetID} {
				if !wanted[pid] {
					continue
				}
				if _, exists := result[pid]; exists {
					return nil, p13UFAuraPendingError()
				}
				result[pid] = p13UFAuraOccupancy{"ordinary", row.ID, row.InputHash, row.DueAt}
			}
		}
		var guards []db.P13UnionFightGuardBattleAttempt
		if err := tx.Select("id,player_id,input_hash,due_at").Where("match_id=? AND status=? AND player_id IN ?", scope, constUtil.P13UFGuardPending, batch).Limit(128).Find(&guards).Error; err != nil {
			return nil, err
		}
		for _, row := range guards {
			if _, exists := result[row.PlayerID]; exists {
				return nil, p13UFAuraPendingError()
			}
			result[row.PlayerID] = p13UFAuraOccupancy{"guard", row.ID, row.InputHash, row.DueAt}
		}
	}
	return result, nil
}

func p13UFAuraQueue(tx *gorm.DB, effect *db.P13UnionFightHealAuraEffect, quote *db.P13UnionFightHealAuraOffer, caster, target *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer, occupancy p13UFAuraOccupancy, limit int64, now time.Time) (*db.P13UnionFightHealAuraPending, error) {
	row := &db.P13UnionFightHealAuraPending{MatchID: effect.MatchID, EffectGUID: effect.GUID, PlayerID: target.PlayerID, CasterID: effect.CasterID, GuildID: target.GuildID, Camp: target.Camp, LifeOrdinal: int64(frame.GetDeadNum()), PlayerLimit: limit, TickAt: effect.NextTickAt, ObservedAt: now.UnixMilli(), HPPercent: quote.HPPercent, Rule: quote.EffectRule, OfferHash: quote.SourceHash, EffectHash: p13UFAuraEffectIdentity(effect), BattleHash: target.BattleHash, CasterX: caster.X, CasterY: caster.Y, CasterZ: caster.Z, TargetX: target.X, TargetY: target.Y, TargetZ: target.Z, Status: constUtil.P13UFHealAuraPending}
	if frame.GetState() == 1 {
		if occupancy.ID == "" {
			return nil, p13UFAuraPendingError()
		}
		row.OccupancyKind, row.OccupancyID, row.OccupancyInputHash, row.OccupancyDueAt = occupancy.Kind, occupancy.ID, occupancy.InputHash, occupancy.DueAt
	}
	if err := p13UFAuraPendingSource(row, quote, effect, false); err != nil {
		return nil, err
	}
	row.ID = p13UFAuraPendingID(row)
	var err error
	row.Capture, err = proto.MarshalOptions{Deterministic: true}.Marshal(frame)
	if err != nil {
		return nil, err
	}
	row.CaptureHash = p6Digest(row.Capture)
	row.ImmutableHash = p13UFAuraPendingImmutable(row)
	row.SourceHash = p13UFAuraPendingHash(row)
	if err = p13UFAuraPendingVerifyLoaded(row); err != nil {
		return nil, err
	}
	if err = tx.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func p13UFAuraApply(tx *gorm.DB, row *db.P13UnionFightHealAuraPending, actor *db.P13UnionFightFieldPlayer, current *pb.InstUFPlayer, occupancy p13UFAuraOccupancy, now time.Time) (*pb.InstUFPlayer, error) {
	if err := p13UFAuraPendingVerifyLoaded(row); err != nil {
		return nil, err
	}
	if row.Status != constUtil.P13UFHealAuraPending {
		return nil, p13UFAuraPendingError()
	}
	if actor.MatchID != row.MatchID || actor.PlayerID != row.PlayerID || actor.BattleHash != row.BattleHash || actor.Camp != row.Camp {
		return nil, p13UFAuraPendingError()
	}
	old := row.SourceHash
	reason := ""
	switch {
	case actor.LeftAt > 0:
		reason = "left"
		row.CancelLeftAt = actor.LeftAt
	case int64(current.GetDeadNum()) != row.LifeOrdinal:
		reason = "life-changed"
	case current.GetState() == 3 || !p13UFResourceLiving(current):
		reason = "dead"
	case row.OccupancyID != "" && (row.OccupancyKind != occupancy.Kind || row.OccupancyID != occupancy.ID || row.OccupancyInputHash != occupancy.InputHash || row.OccupancyDueAt != occupancy.DueAt):
		reason = "occupancy-changed"
	}
	after := proto.Clone(current).(*pb.InstUFPlayer)
	row.AppliedAt = now.UnixMilli()
	if reason != "" {
		row.Status, row.CancelReason = constUtil.P13UFHealAuraCanceled, reason
	} else {
		evidence := db.P13UnionFightPlayerRecovery{PendingID: row.ID, MatchID: row.MatchID, PlayerID: row.PlayerID, Kind: constUtil.P13UFRecoveryAura, ItemID: "ufitem_013", CasterID: row.CasterID, EffectGUID: row.EffectGUID, OfferHash: row.OfferHash, HPPercent: row.HPPercent, LogicalAt: row.TickAt, AppliedAt: now.UnixMilli()}
		if err := p13UFRecoveryLife(after, &evidence); err != nil {
			return nil, err
		}
		if !proto.Equal(current, after) {
			row.RecoveryID = p13UFPlayerRecoveryID(&evidence)
			if err := p13UFPlayerRecoveryRecordOpen(tx, current, after, evidence); err != nil {
				return nil, err
			}
		}
		row.Status = constUtil.P13UFHealAuraApplied
	}
	var err error
	row.Final, err = proto.MarshalOptions{Deterministic: true}.Marshal(after)
	if err != nil {
		return nil, err
	}
	row.FinalHash = p6Digest(row.Final)
	row.SourceHash = p13UFAuraPendingHash(row)
	if err = p13UFAuraPendingVerifyLoaded(row); err != nil {
		return nil, err
	}
	result := tx.Model(&db.P13UnionFightHealAuraPending{}).Where("id=? AND status=? AND source_hash=?", row.ID, constUtil.P13UFHealAuraPending, old).Select("*").Updates(row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, p13UFAuraPendingError()
	}
	return after, nil
}

func p13UFAuraTickFrames(row *db.P13UnionFightHealAuraPending, before, after *pb.InstUFPlayer, actor *db.P13UnionFightFieldPlayer) []proto.Message {
	if proto.Equal(before, after) {
		return nil
	}
	delta := float64(0)
	for i, hp := range after.FightInfo.HpPos {
		delta += hp - before.FightInfo.HpPos[i]
	}
	return []proto.Message{&pb.SCUFElementTickExport{TargetGuid: []int64{actor.PlayerID}, SourceGuid: []int64{row.CasterID}, HpDelta: []float64{delta}}, &pb.SCUFPlayer{Player: []*pb.InstUFPlayer{after}, Local: []*pb.PointNew{{X: proto.Float32(actor.X), Y: proto.Float32(actor.Y), Z: proto.Float32(actor.Z)}}}}
}

// One directory read for each domain; event application below performs only
// bounded writes and pure conversion, never one source SELECT per event.
func p13UFEnvironmentDrain(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, scene *pb.InstUFPlayer, kind, id, inputHash string, dueAt int64, now time.Time) (*pb.InstUFPlayer, []proto.Message, error) {
	if tx == nil || actor == nil || scene == nil || len(id) != 64 || len(inputHash) != 64 || dueAt <= 0 || now.UnixMilli() < dueAt || (kind != "ordinary" && kind != "guard") {
		return nil, nil, p13UFAuraPendingError()
	}
	var err error
	limit := int64(200)
	var towers []db.P13UnionFightTowerHit
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=? AND target_kind='player' AND target_guid=? AND occupancy_kind=? AND occupancy_id=? AND status=?", actor.MatchID, actor.PlayerID, kind, id, constUtil.P13UnionFightTowerHitPending).Order("hit_at,tower_guid,sequence").Limit(7).Find(&towers).Error; err != nil {
		return nil, nil, err
	}
	if len(towers) > 6 {
		return nil, nil, p13UFAuraPendingError()
	}
	towerIDs := []int64{}
	seenTowers := map[int64]bool{}
	for _, row := range towers {
		if seenTowers[row.TowerGUID] {
			return nil, nil, p13UFAuraPendingError()
		}
		seenTowers[row.TowerGUID] = true
		towerIDs = append(towerIDs, row.TowerGUID)
	}
	directory := map[int64]*db.P13UnionFightTowerState{}
	if len(towerIDs) > 0 {
		var rows []db.P13UnionFightTowerState
		if err = tx.Where("match_id=? AND tower_guid IN ?", actor.MatchID, towerIDs).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		for i := range rows {
			directory[rows[i].TowerGUID] = &rows[i]
		}
	}
	auras := []db.P13UnionFightHealAuraPending{}
	cursor := ""
	for {
		var rows []db.P13UnionFightHealAuraPending
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=? AND player_id=? AND status=? AND occupancy_kind=? AND occupancy_id=? AND id>?", actor.MatchID, actor.PlayerID, constUtil.P13UFHealAuraPending, kind, id, cursor).Order("id").Limit(128).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			cursor = row.ID
			auras = append(auras, row)
			if int64(len(auras)) > 5*limit {
				return nil, nil, p13UFAuraPendingError()
			}
		}
		if len(rows) < 128 {
			break
		}
	}
	if len(auras) > 0 {
		snapshot, readErr := p13UFFieldStoredProjection(tx, actor.MatchID)
		if readErr != nil {
			return nil, nil, readErr
		}
		limit = int64(snapshot.PlayerLimit)
		if limit < 1 || limit > 200 || int64(len(auras)) > 5*limit {
			return nil, nil, p13UFAuraPendingError()
		}
	}
	if len(auras) > 0 {
		var quote db.P13UnionFightHealAuraOffer
		if err = tx.First(&quote, "match_id=? AND item_id=?", actor.MatchID, "ufitem_013").Error; err != nil {
			return nil, nil, err
		}
		effectIDs := []int64{}
		seen := map[int64]bool{}
		for _, row := range auras {
			if !seen[row.EffectGUID] {
				seen[row.EffectGUID] = true
				effectIDs = append(effectIDs, row.EffectGUID)
			}
		}
		effects := map[int64]*db.P13UnionFightHealAuraEffect{}
		for start := 0; start < len(effectIDs); start += 128 {
			var rows []db.P13UnionFightHealAuraEffect
			if err = tx.Where("match_id=? AND guid IN ?", actor.MatchID, effectIDs[start:min(start+128, len(effectIDs))]).Find(&rows).Error; err != nil {
				return nil, nil, err
			}
			for i := range rows {
				effects[rows[i].GUID] = &rows[i]
			}
		}
		for i := range auras {
			if err = p13UFAuraPendingSource(&auras[i], &quote, effects[auras[i].EffectGUID], true); err != nil {
				return nil, nil, err
			}
		}
	}
	type entry struct {
		At    int64
		Kind  int
		Key   string
		Index int
	}
	entries := []entry{}
	for i, row := range towers {
		entries = append(entries, entry{row.HitAt, 0, fmt.Sprintf("%020d/%020d", row.TowerGUID, row.Sequence), i})
	}
	for i, row := range auras {
		entries = append(entries, entry{row.TickAt, 1, row.ID, i})
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		return p13UFEnvironmentEventLess(a.At, a.Kind, a.Key, b.At, b.Kind, b.Key)
	})
	var party pb.BattlePlayer
	if actor.BattleHash != p6Digest(actor.Battle) || proto.Unmarshal(actor.Battle, &party) != nil || party.GetPlayerId() != actor.PlayerID {
		return nil, nil, p13UFAuraPendingError()
	}
	current := proto.Clone(scene).(*pb.InstUFPlayer)
	frames := []proto.Message{}
	for _, entry := range entries {
		if entry.Kind == 0 {
			hit := towers[entry.Index]
			var messages []proto.Message
			current, messages, err = p13UFTowerDrainRows(tx, actor, current, kind, id, inputHash, dueAt, now, []db.P13UnionFightTowerHit{hit}, directory, &party)
			if err != nil {
				return nil, nil, err
			}
			frames = append(frames, messages...)
		} else {
			row := &auras[entry.Index]
			before := current
			current, err = p13UFAuraApply(tx, row, actor, current, p13UFAuraOccupancy{kind, id, inputHash, dueAt}, now)
			if err != nil {
				return nil, nil, err
			}
			frames = append(frames, p13UFAuraTickFrames(row, before, current, actor)...)
		}
	}
	return current, frames, nil
}

func p13UFAuraFrame(actor *db.P13UnionFightFieldPlayer, hashes map[int64]string) (*pb.InstUFPlayer, error) {
	if hashes[actor.PlayerID] == "" || hashes[actor.PlayerID] != actor.BattleHash || actor.FrameHash != p6Digest(actor.Frame) || !p13UFMapProjectionKnown(actor.MapProjection) || actor.MapProjection == constUtil.P13UnionFightMapProjectionReFlat && actor.Y != 0 {
		return nil, p13UFAuraPendingError()
	}
	frame := &pb.InstUFPlayer{}
	if proto.Unmarshal(actor.Frame, frame) != nil || frame.GetGuid() != actor.PlayerID || int64(frame.GetCamp()) != actor.Camp {
		return nil, p13UFAuraPendingError()
	}
	return frame, nil
}

func (s *GameService) p13UFHealAuraPendingTick(tx *gorm.DB, effect *db.P13UnionFightHealAuraEffect, quote *db.P13UnionFightHealAuraOffer, match *db.P13UnionFightMatch, enabled bool, end int64, now time.Time) ([]proto.Message, error) {
	if !p13UFHealAuraQuoteValid(quote) || quote.EffectRule != constUtil.P13UFHealAuraPendingRule {
		return nil, p13UFAuraPendingError()
	}
	if at, err := p13UFMixedTerminalAt(tx, match.ID); err != nil {
		return nil, err
	} else if at > 0 && at < end {
		end = at
	}
	hashes, err := p13UFMixedRosterBattleHashes(match)
	if err != nil {
		return nil, err
	}
	snapshot, err := p13UFFieldStoredProjection(tx, match.ID)
	if err != nil {
		return nil, err
	}
	limit := int64(snapshot.PlayerLimit)
	if limit < 1 || limit > 200 {
		return nil, p13UFAuraPendingError()
	}
	var caster db.P13UnionFightFieldPlayer
	actorColumns := "a.match_id,a.player_id,a.guild_id,a.camp,a.battle_hash,a.frame,a.frame_hash,a.frame_state,a.joined_at,a.left_at,a.hooked,a.session_hash,a.x,a.y,a.z,a.map_projection"
	if err = tx.Select("match_id,player_id,guild_id,camp,battle_hash,frame,frame_hash,frame_state,joined_at,left_at,hooked,session_hash,x,y,z,map_projection").First(&caster, "match_id=? AND player_id=?", match.ID, effect.CasterID).Error; err != nil {
		return nil, err
	}
	casterFrame, err := p13UFAuraFrame(&caster, hashes)
	if err != nil {
		return nil, err
	}
	targets := []p13UFFieldLiveRow{}
	cursor := int64(0)
	scene := "json_extract(p.game,'$.position.scene')"
	if tx.Dialector.Name() == "mysql" {
		scene = "JSON_UNQUOTE(" + scene + ")"
	}
	for {
		var batch []p13UFFieldLiveRow
		if err = tx.Table("p13_union_fight_field_players AS a").Select(actorColumns+",m.guild_id AS current_guild_id,p.session AS profile_session,"+scene+" AS actual_scene,COALESCE(json_extract(p.game,'$.position_loading'),0) AS position_loading").Joins("JOIN profiles AS p ON p.id=a.player_id").Joins("JOIN p13_guild_members AS m ON m.player_id=a.player_id").Where("a.match_id=? AND a.camp=? AND a.left_at=0 AND a.player_id>?", match.ID, caster.Camp, cursor).Order("a.player_id").Limit(128).Scan(&batch).Error; err != nil {
			return nil, err
		}
		for _, row := range batch {
			cursor = row.PlayerID
			targets = append(targets, row)
			if int64(len(targets)) > limit {
				return nil, p13UFAuraPendingError()
			}
		}
		if len(batch) < 128 {
			break
		}
	}
	liveOK := func(live *p13UFFieldLiveRow) bool {
		return live.CurrentGuildID == live.GuildID && live.GuildID == caster.GuildID && live.ActualScene == constUtil.P13UnionFightFieldTarget && !live.PositionLoading && live.SessionHash == p6Digest(live.ProfileSession)
	}
	casterLive := false
	ids := []int64{}
	for i := range targets {
		row := &targets[i]
		ids = append(ids, row.PlayerID)
		if row.PlayerID == caster.PlayerID {
			casterLive = liveOK(row)
		}
	}
	cancel := !enabled || effect.NextTickAt > end || caster.LeftAt != 0 || caster.Hooked || !casterLive || !p13UFResourceLiving(casterFrame) || (casterFrame.GetState() != 0 && casterFrame.GetState() != 1 && casterFrame.GetState() != 2 && casterFrame.GetState() != 5)
	frames := []proto.Message{}
	if cancel {
		effect.EndedAt = min(now.UnixMilli(), effect.ExpiresAt, end)
	} else {
		occupancies, readErr := p13UFAuraOccupancies(tx, match.ID, ids)
		if readErr != nil {
			return nil, readErr
		}
		counts := map[int64]int64{}
		groupCursor := int64(0)
		for {
			var grouped []struct{ PlayerID, Count int64 }
			if err = tx.Model(&db.P13UnionFightHealAuraPending{}).Select("player_id,COUNT(*) AS count").Where("match_id=? AND status=? AND player_id>?", match.ID, constUtil.P13UFHealAuraPending, groupCursor).Group("player_id").Order("player_id").Limit(128).Scan(&grouped).Error; err != nil {
				return nil, err
			}
			for _, row := range grouped {
				groupCursor = row.PlayerID
				if row.Count > 5*limit {
					return nil, p13UFAuraPendingError()
				}
				counts[row.PlayerID] = row.Count
				if int64(len(counts)) > limit {
					return nil, p13UFAuraPendingError()
				}
			}
			if len(grouped) < 128 {
				break
			}
		}
		var effectCount int64
		if err = tx.Model(&db.P13UnionFightHealAuraPending{}).Where("match_id=? AND effect_guid=?", match.ID, effect.GUID).Count(&effectCount).Error; err != nil {
			return nil, err
		}
		for effect.NextTickAt <= now.UnixMilli() && effect.NextTickAt <= effect.ExpiresAt && effect.NextTickAt <= end {
			for i := range targets {
				live := &targets[i]
				actor := &live.P13UnionFightFieldPlayer
				if !liveOK(live) || actor.Hooked || p13UFResourceDistance(actor, caster.X, caster.Y, caster.Z) > quote.Radius {
					continue
				}
				frame, readErr := p13UFAuraFrame(actor, hashes)
				if readErr != nil {
					return nil, readErr
				}
				if !p13UFResourceLiving(frame) || (frame.GetState() != 0 && frame.GetState() != 1 && frame.GetState() != 2 && frame.GetState() != 5) {
					continue
				}
				if frame.GetState() == 1 && counts[actor.PlayerID] >= 5*limit {
					return nil, errors.New(constUtil.P13UFHealAuraBackpressure)
				}
				row, queueErr := p13UFAuraQueue(tx, effect, quote, &caster, actor, frame, occupancies[actor.PlayerID], limit, now)
				if queueErr != nil {
					return nil, queueErr
				}
				effectCount++
				if effectCount > 5*limit {
					return nil, p13UFAuraPendingError()
				}
				if frame.GetState() == 1 {
					counts[actor.PlayerID]++
					continue
				}
				after, applyErr := p13UFAuraApply(tx, row, actor, frame, p13UFAuraOccupancy{}, now)
				if applyErr != nil {
					return nil, applyErr
				}
				if !proto.Equal(frame, after) {
					if err = p13UFFieldSaveActor(tx, actor, after); err != nil {
						return nil, err
					}
					frames = append(frames, p13UFAuraTickFrames(row, frame, after, actor)...)
				}
			}
			effect.NextTickAt += quote.TickMillis
		}
		if now.UnixMilli() >= min(effect.ExpiresAt, end) {
			effect.EndedAt = min(effect.ExpiresAt, end)
		}
	}
	effect.SourceHash = p13UFHealAuraEffectHash(effect)
	if err = tx.Save(effect).Error; err != nil {
		return nil, err
	}
	if effect.EndedAt > 0 {
		// Reload the small current frame after self-healing; never overwrite its
		// HP/state or collection clock with the pre-tick emitter snapshot.
		var row db.P13UnionFightFieldPlayer
		if err = tx.Select("match_id,player_id,guild_id,camp,battle_hash,frame,frame_hash,frame_state,joined_at,left_at,hooked,session_hash,x,y,z,map_projection").First(&row, "match_id=? AND player_id=?", match.ID, effect.CasterID).Error; err != nil {
			return nil, err
		}
		frame, readErr := p13UFAuraFrame(&row, hashes)
		if readErr != nil {
			return nil, readErr
		}
		changed, projectErr := p13UFHealAuraProject(tx, match.ID, effect.CasterID, frame, now)
		if projectErr != nil {
			return nil, projectErr
		}
		if changed {
			if err = p13UFFieldSaveActor(tx, &row, frame); err != nil {
				return nil, err
			}
		}
		frames = append(frames, &pb.SCUFElementBuffOff{BuffGuid: []int64{effect.GUID}})
	}
	return frames, nil
}

func p13UFEnvironmentEventLess(atA int64, kindA int, keyA string, atB int64, kindB int, keyB string) bool {
	if atA != atB {
		return atA < atB
	}
	if kindA != kindB {
		return kindA < kindB
	}
	return keyA < keyB
}
