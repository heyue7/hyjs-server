package service

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// A refusal seals an original non-authoritative computation, never an effect.
// It lives outside Sources so the authority gate for combat remains unchanged.
type p13UFAuditRefusalMeta struct {
	Kind     string
	Key      string
	NoEffect bool
	Attempt  json.RawMessage
	Record   p13UFMixedRecordMeta
}

func p13UFAuditFactorEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func p13UFAuditRequestHash(match string, player, target int64) string {
	return p6Digest([]byte(match + "\x00" + strconv.FormatInt(player, 10) + "\x00" + strconv.FormatInt(target, 10)))
}

// Metadata is read in keyset batches of 128; full P6 payloads in batches of four.
// Consumer verification uses the previously sealed hashes without loading P6 PB.
func p13UFAuditRefusalsRead(tx *gorm.DB, match *db.P13UnionFightMatch, full bool) ([]p13UFAuditRefusalMeta, error) {
	result := []p13UFAuditRefusalMeta{}
	var common db.P13UnionFightBattleAuditOffer
	commonLoaded := false
	guards := map[int64]*db.P13UnionFightGuardAuditOffer{}
	roster := p13UFMatchRoster{}
	if json.Unmarshal(match.Roster, &roster) != nil || p6Digest(match.Roster) != match.SnapshotHash {
		return nil, p13UFMixedError()
	}
	owners := map[int64]string{}
	for camp, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		guild := match.LeftGuildID
		if camp == 1 {
			guild = match.RightGuildID
		}
		for _, f := range side {
			if f.PlayerID <= 0 || owners[f.PlayerID] != "" || f.GuildID != guild {
				return nil, p13UFMixedError()
			}
			owners[f.PlayerID] = guild
		}
	}
	for _, kind := range []string{"battle-refusal", "guard-refusal"} {
		cursor := int64(0)
		for {
			var battle []db.P13UnionFightBattleAuditAttempt
			var guard []db.P13UnionFightGuardAuditAttempt
			if kind == "battle-refusal" {
				if err := tx.Where("match_id=? AND guid>?", match.ID, cursor).Order("guid").Limit(128).Find(&battle).Error; err != nil {
					return nil, err
				}
			} else {
				if err := tx.Where("match_id=? AND guid>?", match.ID, cursor).Order("guid").Limit(128).Find(&guard).Error; err != nil {
					return nil, err
				}
			}
			count := len(battle) + len(guard)
			if count == 0 {
				break
			}
			if !commonLoaded {
				if err := tx.First(&common, "match_id=?", match.ID).Error; err != nil {
					return nil, err
				}
				if !p13UFBattleAuditQuoteValid(&common) || common.MatchID != match.ID {
					return nil, p13UFMixedError()
				}
				commonLoaded = true
			}
			if kind == "guard-refusal" && len(guards) == 0 {
				var quotes []db.P13UnionFightGuardAuditOffer
				if err := tx.Where("match_id=?", match.ID).Limit(11).Find(&quotes).Error; err != nil {
					return nil, err
				}
				if len(quotes) > 10 {
					return nil, p13UFMixedError()
				}
				for i := range quotes {
					v := &quotes[i]
					if v.MatchID != match.ID || v.BattleOfferHash != common.SourceHash || v.SourceHash != p13UFGuardOfferHash(v) || v.MonsterHash != p6Digest(v.Monster) {
						return nil, p13UFMixedError()
					}
					guards[v.GuardGUID] = v
				}
			}
			width := 128
			if full {
				width = 4
			}
			for start := 0; start < count; start += width {
				end := min(start+width, count)
				ids := []string{}
				for i := start; i < end; i++ {
					if kind == "battle-refusal" {
						ids = append(ids, battle[i].P6RecordID)
					} else {
						ids = append(ids, guard[i].P6RecordID)
					}
				}
				var records []db.P6BattleRecord
				q := tx
				if !full {
					q = q.Select("id,p13_match_id,source,source_fight,status,input_hash,outcome_hash,replay_hash,engine_version,client_version,configuration_hash,player_id,authoritative,client_result,server_result,created_at,finished_at")
				}
				if err := q.Where("id IN ?", ids).Find(&records).Error; err != nil {
					return nil, err
				}
				if len(records) != len(ids) {
					return nil, p13UFMixedError()
				}
				byID := map[string]*db.P6BattleRecord{}
				for i := range records {
					byID[records[i].ID] = &records[i]
				}
				for i := start; i < end; i++ {
					var guid, player, target, stamp, created int64
					var recordID, sourceHash, expectedHash, request, offerHash, refusal, inputHash, outcomeHash, replayHash, source string
					var raw []byte
					if kind == "battle-refusal" {
						a := &battle[i]
						guid, player, target, stamp, created = a.GUID, a.PlayerID, a.TargetID, a.Stamp, a.CreatedAt
						recordID, sourceHash, expectedHash, request, offerHash, refusal, inputHash, outcomeHash, replayHash = a.P6RecordID, a.SourceHash, p13UFBattleAuditAttemptHash(a), a.RequestHash, a.OfferHash, a.Refusal, a.InputHash, a.OutcomeHash, a.ReplayHash
						source = constUtil.P13UnionFightBattleAuditP6Source
						raw = p13UFMixedJSON(a)
						if offerHash != common.SourceHash || owners[player] == "" || owners[target] == "" || owners[player] == owners[target] {
							return nil, p13UFMixedError()
						}
					} else {
						a := &guard[i]
						guid, player, target, stamp, created = a.GUID, a.PlayerID, a.GuardGUID, a.Stamp, a.CreatedAt
						recordID, sourceHash, expectedHash, request, offerHash, refusal, inputHash, outcomeHash, replayHash = a.P6RecordID, a.SourceHash, p13UFGuardAttemptHash(a), a.RequestHash, a.OfferHash, a.Refusal, a.InputHash, a.OutcomeHash, a.ReplayHash
						source = constUtil.P13UnionFightGuardAuditP6Source
						raw = p13UFMixedJSON(a)
						quote := guards[target]
						if quote == nil || quote.SourceHash != offerHash || owners[player] == "" {
							return nil, p13UFMixedError()
						}
					}
					r := byID[recordID]
					guidKind := "battle-audit"
					if kind == "guard-refusal" {
						guidKind = "guard-audit"
					}
					if guid != p13UFFieldGUID(match.ID, guidKind, strconv.FormatInt(player, 10)+"/"+strconv.FormatInt(stamp, 10)) {
						return nil, p13UFMixedError()
					}
					if guid <= 0 || stamp <= 0 || created < match.StartAt || created >= match.EndAt || sourceHash != expectedHash || request != p13UFAuditRequestHash(match.ID, player, target) || refusal != constUtil.P13UnionFightBattleAuditRefusal || r == nil || r.ID != recordID || r.P13MatchID != match.ID || r.PlayerID != player || r.Source != source || r.SourceFight != strconv.FormatInt(guid, 10) || r.Status != constUtil.P6BattleComputed || r.Authoritative || r.InputHash != inputHash || r.OutcomeHash != outcomeHash || r.ReplayHash != replayHash || r.ClientResult != r.ServerResult {
						return nil, p13UFMixedError()
					}
					if full {
						input, e := decodeP6Record(r)
						if e != nil {
							return nil, e
						}
						seedKey := r.SourceFight + "/" + common.SourceHash + "/"
						if kind == "guard-refusal" {
							seedKey += guards[target].SourceHash + "/"
						}
						seedKey += strconv.FormatInt(common.SeedSalt, 10)
						seedRaw, _ := hex.DecodeString(p6Digest([]byte(seedKey)))
						if input.Seed != int64(binary.BigEndian.Uint32(seedRaw[:4])&0x7fffffff) || input.MaxRound != common.MaxRounds || !p13UFAuditFactorEqual(input.AttackerDamageFactor, common.AttackerDamageFactor) || !p13UFAuditFactorEqual(input.DefenderDamageFactor, common.DefenderDamageFactor) || r.ClientVersion != match.ClientVersion || r.EngineVersion != match.EngineVersion {
							return nil, p13UFMixedError()
						}
						var err error
						if kind == "battle-refusal" {
							err = p13UFBattleAuditVerifyLoaded(&battle[i], r)
						} else {
							err = p13UFGuardAuditVerifyLoaded(&guard[i], r)
						}
						if err != nil {
							return nil, err
						}
					}
					result = append(result, p13UFAuditRefusalMeta{Kind: kind, Key: fmt.Sprintf("%d", guid), NoEffect: true, Attempt: raw, Record: p13UFMixedRecordMetadata(r)})
					cursor = guid
				}
			}
			if count < 128 {
				break
			}
		}
	}
	return result, nil
}

func p13UFAuditRefusalsVerify(tx *gorm.DB, match *db.P13UnionFightMatch, want []p13UFAuditRefusalMeta) error {
	got, err := p13UFAuditRefusalsRead(tx, match, false)
	if err != nil {
		return err
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	// nil and empty retain the legacy no-audit proof contract.
	if len(got) == 0 && len(want) == 0 {
		return nil
	}
	if !bytes.Equal(a, b) {
		return p13UFMixedError()
	}
	return nil
}
