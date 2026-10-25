package service

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type p13UFCombatAttemptMeta struct {
	ID, MatchID, P6RecordID, Status, SourceHash, InputHash, TerminalHash, OfferHash            string
	PlayerID, TargetID, ClientStamp, CreatedAt, DueAt, SettledAt, DeadPlayerID, RewardPlayerID int64
}
type p13UFCombatRecordMeta struct {
	ID, P13MatchID, Source, SourceFight, Status, InputHash, OutcomeHash, ReplayHash, EngineVersion, ClientVersion, ConfigurationHash string
	PlayerID                                                                                                                         int64
	Authoritative                                                                                                                    bool
}
type p13UFCombatProofEvent struct {
	Attempt p13UFCombatAttemptMeta
	Record  p13UFCombatRecordMeta
}
type p13UFCombatProof struct {
	Players                                           []int64
	Schema, MatchSHA, LeftPreviewSHA, RightPreviewSHA string
	Events                                            []p13UFCombatProofEvent
}

func p13UFCombatAttemptMetadata(r *db.P13UnionFightPlayerBattleAttempt) p13UFCombatAttemptMeta {
	return p13UFCombatAttemptMeta{r.ID, r.MatchID, r.P6RecordID, r.Status, r.SourceHash, r.InputHash, r.TerminalHash, r.OfferHash, r.PlayerID, r.TargetID, r.ClientStamp, r.CreatedAt, r.DueAt, r.SettledAt, r.DeadPlayerID, r.RewardPlayerID}
}
func p13UFCombatRecordMetadata(r *db.P6BattleRecord) p13UFCombatRecordMeta {
	return p13UFCombatRecordMeta{r.ID, r.P13MatchID, r.Source, r.SourceFight, r.Status, r.InputHash, r.OutcomeHash, r.ReplayHash, r.EngineVersion, r.ClientVersion, r.ConfigurationHash, r.PlayerID, r.Authoritative}
}

// DeliveredAt is the only mutable terminal field. All business values, the
// frozen reward roster and the lightweight proof itself belong to this seal.
func p13UFCombatProofDigest(result *db.P13UnionFightSettlementResult) string {
	copy := *result
	copy.CombatProofHash = ""
	copy.DeliveredAt = 0
	// CombatProof is intentionally omitted from the model's public JSON. Bind it
	// explicitly so changing an event can never retain the same terminal seal.
	raw, _ := json.Marshal(struct {
		Result db.P13UnionFightSettlementResult
		Proof  []byte
		MVP    []byte
	}{copy, copy.CombatProof, copy.MVP})
	return p6Digest(raw)
}

func p13UFCombatProofVerify(tx *gorm.DB, match *db.P13UnionFightMatch, result *db.P13UnionFightSettlementResult) error {
	reject := func() error { return errors.New("公会战轻量终局证明或来源不一致") }
	combat := strings.HasPrefix(result.Source, constUtil.P13UnionFightCombatSettlementSource)
	if !combat && result.Reason == "" && result.LedgerHash == "" && result.LeftKills == 0 && result.RightKills == 0 && result.LeftReachedAt == 0 && result.RightReachedAt == 0 && len(result.CombatProof) == 0 && result.CombatProofHash == "" {
		return nil
	}
	if !combat || result.CombatProofHash == "" || p13UFCombatProofDigest(result) != result.CombatProofHash || result.MatchID != match.ID || result.EndAt != match.EndAt {
		return reject()
	}
	var proof p13UFCombatProof
	if json.Unmarshal(result.CombatProof, &proof) != nil || proof.Schema != constUtil.P13UFCombatProofSchema || proof.MatchSHA != match.SnapshotHash || p6Digest(match.Roster) != match.SnapshotHash || proof.LeftPreviewSHA != p6Digest(match.LeftPreview) || proof.RightPreviewSHA != p6Digest(match.RightPreview) || len(proof.Events) == 0 {
		return reject()
	}
	// Match/offer are lightweight identities; the original input tables and
	// terminal actors were completely checked before the proof was committed.
	var offer db.P13UnionFightPlayerBattleOffer
	if e := tx.Select("match_id,source_hash").First(&offer, "match_id=?", match.ID).Error; e != nil {
		return e
	}
	var count int64
	if e := tx.Model(&db.P13UnionFightPlayerBattleAttempt{}).Where("match_id=?", match.ID).Count(&count).Error; e != nil {
		return e
	}
	if count != int64(len(proof.Events)) {
		return reject()
	}
	if e := tx.Model(&db.P6BattleRecord{}).Where("p13_match_id=?", match.ID).Count(&count).Error; e != nil {
		return e
	}
	if count != int64(len(proof.Events)) {
		return reject()
	}
	seen := map[string]bool{}
	players := map[int64]bool{}
	rows := []db.P13UnionFightPlayerBattleAttempt{}
	for start := 0; start < len(proof.Events); start += 128 {
		end := min(start+128, len(proof.Events))
		ids := []string{}
		recordIDs := []string{}
		for _, event := range proof.Events[start:end] {
			if seen[event.Attempt.ID] || event.Attempt.MatchID != match.ID || event.Record.P13MatchID != match.ID || event.Attempt.P6RecordID != event.Record.ID || event.Attempt.Status != constUtil.P13UFPlayerBattleSettled || event.Record.Status != constUtil.P6BattleComputed || !event.Record.Authoritative || event.Attempt.OfferHash != offer.SourceHash {
				return reject()
			}
			seen[event.Attempt.ID] = true
			ids = append(ids, event.Attempt.ID)
			recordIDs = append(recordIDs, event.Record.ID)
		}
		var attempts []db.P13UnionFightPlayerBattleAttempt
		if e := tx.Select("id,match_id,p6_record_id,status,source_hash,input_hash,terminal_hash,offer_hash,player_id,target_id,client_stamp,created_at,due_at,settled_at,dead_player_id,reward_player_id").Where("id IN ?", ids).Find(&attempts).Error; e != nil {
			return e
		}
		var records []db.P6BattleRecord
		if e := tx.Select("id,p13_match_id,source,source_fight,status,input_hash,outcome_hash,replay_hash,engine_version,client_version,configuration_hash,player_id,authoritative").Where("id IN ?", recordIDs).Find(&records).Error; e != nil {
			return e
		}
		if len(attempts) != len(ids) || len(records) != len(recordIDs) {
			return reject()
		}
		a := map[string]p13UFCombatAttemptMeta{}
		r := map[string]p13UFCombatRecordMeta{}
		for i := range attempts {
			a[attempts[i].ID] = p13UFCombatAttemptMetadata(&attempts[i])
			players[attempts[i].PlayerID] = true
			players[attempts[i].TargetID] = true
			rows = append(rows, attempts[i])
		}
		for i := range records {
			r[records[i].ID] = p13UFCombatRecordMetadata(&records[i])
		}
		for _, event := range proof.Events[start:end] {
			if a[event.Attempt.ID] != event.Attempt || r[event.Record.ID] != event.Record {
				return reject()
			}
		}
	}
	ids := proof.Players
	rosterIDs := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || rosterIDs[id] {
			return reject()
		}
		rosterIDs[id] = true
	}
	for id := range players {
		if !rosterIDs[id] {
			return reject()
		}
	}
	blocked, e := p13UFCombatUnclosed(tx, match, rows, ids)
	if e != nil {
		return e
	}
	if blocked {
		return reject()
	}
	return nil
}
