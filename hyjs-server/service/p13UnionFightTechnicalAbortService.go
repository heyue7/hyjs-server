package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Single-instance publication fence: readers enqueue complete committed frame
// groups; finalization holds the writer across its transaction and commit.
// Never acquire it while holding a guild/database mutation lock.
var p13UFFinalizationFence sync.RWMutex

func p13UFPublishBeforeAbort(ctx context.Context, scope string, publish func()) error {
	p13UFFinalizationFence.RLock()
	defer p13UFFinalizationFence.RUnlock()
	var row db.P13UnionFightSettlementResult
	err := db.GetIns().WithContext(ctx).Select("result_kind").First(&row, "match_id=?", scope).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && row.ResultKind == constUtil.P13UFTechnicalAbortKind {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	publish()
	return nil
}

func p13UFAbortRuleValid(rule *db.P13UnionFightSettlementPolicy) bool {
	return rule != nil && (rule.AbortRule == "" && rule.FinalizationGraceMillis == 0 || rule.AbortRule == constUtil.P13UFTechnicalAbortRule && rule.FinalizationGraceMillis == 5000 && rule.TerminalRule == constUtil.P13UFMixedTerminalRule)
}
func p13UFAbortEnabled(rule *db.P13UnionFightSettlementPolicy) bool {
	return p13UFAbortRuleValid(rule) && rule.AbortRule == constUtil.P13UFTechnicalAbortRule
}
func p13UFAbortDeadline(match *db.P13UnionFightMatch, rule *db.P13UnionFightSettlementPolicy) (int64, error) {
	if !p13UFAbortEnabled(rule) || match.EndAt <= match.StartAt || match.EndAt-match.StartAt > 1800000 || match.EndAt > math.MaxInt64-rule.FinalizationGraceMillis {
		return 0, errors.New(constUtil.P13UFMixedConflict)
	}
	return match.EndAt + rule.FinalizationGraceMillis, nil
}

// Independent freeze scalars preserve fail-closed finalization even if the
// reward rules bytes later become damaged. They are never backfilled on old rows.
func p13UFAbortFrozenRule(offer *db.P13UnionFightSettlementOffer, match *db.P13UnionFightMatch) (*db.P13UnionFightSettlementPolicy, error) {
	if offer.AbortRule == "" && offer.FinalizationGraceMillis == 0 {
		return nil, nil
	}
	if offer.MatchID != match.ID || offer.Source != constUtil.P13UnionFightSettlementSource || offer.RosterHash != match.SnapshotHash || offer.FrozenAt <= 0 || offer.FrozenAt >= match.StartAt || len(offer.RulesHash) != 64 || offer.AbortRule != constUtil.P13UFTechnicalAbortRule || offer.FinalizationGraceMillis != 5000 {
		return nil, errors.New(constUtil.P13UFMixedConflict)
	}
	rule := &db.P13UnionFightSettlementPolicy{AbortRule: offer.AbortRule, FinalizationGraceMillis: offer.FinalizationGraceMillis, TerminalRule: constUtil.P13UFMixedTerminalRule}
	if _, err := p13UFAbortDeadline(match, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// Called before locking an attempt. The current read is serialized with every
// admission/apply/finalizer by the existing guild gate, including cold retries.
func p13UFCombatCommitGate(tx *gorm.DB, scope string, now time.Time) (bool, error) {
	var gate db.P13GuildPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
		return false, err
	}
	var result db.P13UnionFightSettlementResult
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("match_id").First(&result, "match_id=?", scope).Error
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	var offer db.P13UnionFightSettlementOffer
	err = tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&offer, "match_id=?", scope).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var match db.P13UnionFightMatch
	if err = tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&match, "id=?", scope).Error; err != nil {
		return false, err
	}
	rule, err := p13UFAbortFrozenRule(&offer, &match)
	if err != nil {
		return false, err
	}
	if rule == nil || !p13UFAbortEnabled(rule) {
		return false, nil
	}
	deadline, err := p13UFAbortDeadline(&match, rule)
	return err == nil && now.UnixMilli() >= deadline, err
}

type p13UFAbortDirectory struct {
	Columns []string
	Table   string
	Count   int64
	SHA     string
}
type p13UFAbortParticipant struct {
	PlayerID, Camp, JoinedAt int64
	GuildID, BattleHash      string
}
type p13UFAbortProof struct {
	Schema, Rule, MatchSHA, MatchIdentitySHA, OfferSHA, ObservedRosterSHA, ObservedRulesSHA, Reason string
	StartAt, Deadline, TerminalAt, ObservedAt                                                       int64
	Participants                                                                                    []p13UFAbortParticipant
	Directory                                                                                       []p13UFAbortDirectory
}
type p13UFAbortDomain struct {
	Model       any
	MatchColumn string
	Columns     []string
	Extra       string
	Label       string
	Args        []any
}

// Explicit finite domains. Slice protobuf/JSON payloads and string snapshots
// are excluded below; stored identities/hashes/statuses are never interpreted
// as a certified victory. Invalid bytes remain untouched in their source rows.
func p13UFAbortDomains(match *db.P13UnionFightMatch, ids []int64) []p13UFAbortDomain {
	models := []any{
		&db.P13UnionFightSettlementOffer{}, &db.P13UnionFightSettlementIntent{}, &db.P13UnionFightPlayerBattleAttempt{}, &db.P13UnionFightGuardBattleAttempt{},
		&db.P13UnionFightBattleAuditAttempt{}, &db.P13UnionFightGuardAuditAttempt{},
		&db.P13UnionFightPlayerBattleOffer{}, &db.P13UnionFightGuardAuditOffer{}, &db.P13UnionFightBattleAuditOffer{},
		&db.P13UnionFightTowerState{}, &db.P13UnionFightTowerHit{}, &db.P13UnionFightGuardState{},
		&db.P13UnionFightReviveReceipt{}, &db.P13UnionFightPlayerRecovery{}, &db.P13UnionFightGateImpact{}, &db.P13UnionFightGateState{},
		&db.P13UnionFightCarImpact{}, &db.P13UnionFightCarGeneration{}, &db.P13UnionFightFactoryState{}, &db.P13UnionFightFactoryCycleState{},
		&db.P13UnionFightFactoryCycleOffer{}, &db.P13UnionFightCarAttackState{}, &db.P13UnionFightCarAttackOffer{},
		&db.P13UnionFightShopPurchase{}, &db.P13UnionFightShopOffer{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightItemState{},
		&db.P13UnionFightGateItemOffer{}, &db.P13UnionFightCarItemOffer{}, &db.P13UnionFightSpeedItemOffer{}, &db.P13UnionFightSpeedItemEffect{},
		&db.P13UnionFightHealAuraOffer{}, &db.P13UnionFightHealAuraEffect{}, &db.P13UnionFightHealAuraPending{}, &db.P13UnionFightWarDrumOffer{}, &db.P13UnionFightWarDrumEffect{},
		&db.P13UnionFightGuildMissionFact{}, &db.P13UnionFightTransportScope{}, &db.P13UnionFightTransportStat{},
		&db.P13UnionFightResourceNode{}, &db.P13UnionFightResourceTask{}, &db.P13UnionFightCampResource{}, &db.P13UnionFightCarMotionState{}, &db.P13UnionFightCarMotionOffer{}, &db.P13UnionFightItemTargetCooldown{}, &db.P13UnionFightItemTeamUse{}, &db.P13UnionFightField{},
	}
	result := make([]p13UFAbortDomain, 0, len(models)+3)
	for _, m := range models {
		result = append(result, p13UFAbortDomain{Model: m, MatchColumn: "match_id"})
	}
	result = append(result, p13UFAbortDomain{Model: &db.P6BattleRecord{}, MatchColumn: "p13_match_id"})
	result = append(result, p13UFAbortDomain{Model: &db.P13UnionFightFieldPlayer{}, MatchColumn: "match_id", Columns: []string{"match_id", "player_id", "guild_id", "camp", "joined_at", "battle_hash", "frame_hash", "frame_state", "left_at", "hooked"}})
	// Only current-time unscoped UF records belong to this failed finalization;
	// unrelated player histories are never loaded or assigned invented authority.
	for start := 0; start < len(ids); start += 128 {
		batch := ids[start:min(start+128, len(ids))]
		result = append(result, p13UFAbortDomain{Model: &db.P6BattleRecord{}, MatchColumn: "p13_match_id", Label: fmt.Sprintf("/unscoped-current/%d", start/128), Extra: "player_id IN ? AND source IN ? AND created_at>=? AND created_at<=?", Args: []any{batch, p13P6MatchSources(), match.StartAt, match.EndAt}})
		result = append(result, p13UFAbortDomain{Model: &db.P13GuildReceipt{}, Label: fmt.Sprintf("/match-item-receipts/%d", start/128), Columns: []string{"player_id", "operation", "stamp", "request", "created_at"}, Extra: "player_id IN ? AND operation IN ? AND created_at>=? AND created_at<=? AND substr(request,1,?)=?", Args: []any{batch, []string{constUtil.P13UnionFightItemUseOperation, constUtil.P13UnionFightShopBuyOperation}, match.StartAt, match.EndAt, len(match.ID) + 1, []byte(match.ID + "\x00")}})
	}
	return result
}

func p13UFAbortScalar(value any) string {
	if value == nil {
		return "<NULL>"
	}
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}
	if b, ok := value.(bool); ok {
		if b {
			return "1"
		}
		return "0"
	}
	return fmt.Sprint(value)
}

// Keyset scans are limited to 128 metadata rows per SQL, with no joins and no
// P6 input/outcome/operates payload. Composite PK cursors retain exact order.
func p13UFAbortDirectoryRead(tx *gorm.DB, match *db.P13UnionFightMatch, ids []int64, frozen []p13UFAbortDirectory) ([]p13UFAbortDirectory, error) {
	result := []p13UFAbortDirectory{}
	domains := p13UFAbortDomains(match, ids)
	wanted := map[string]p13UFAbortDirectory{}
	for _, row := range frozen {
		if _, exists := wanted[row.Table]; exists {
			return nil, errors.New("公会战中止目录域重复")
		}
		wanted[row.Table] = row
	}
	consumed := 0
	for _, domain := range domains {
		statement := &gorm.Statement{DB: tx}
		if err := statement.Parse(domain.Model); err != nil {
			return nil, err
		}
		schema := statement.Schema
		columns := append([]string(nil), domain.Columns...)
		if len(columns) == 0 {
			for _, field := range schema.Fields {
				if field.DBName != "" && field.FieldType.Kind() != reflect.Slice && field.Name != "Snapshot" {
					columns = append(columns, field.DBName)
				}
			}
		}
		name := schema.Table + domain.Label
		expected, recorded := wanted[name]
		if frozen != nil && recorded {
			consumed++
			allowed := map[string]bool{}
			for _, c := range columns {
				allowed[c] = true
			}
			columns = append([]string(nil), expected.Columns...)
			if len(columns) == 0 {
				return nil, errors.New("公会战中止目录缺列")
			}
			seen := map[string]bool{}
			for _, c := range columns {
				if !allowed[c] || seen[c] {
					return nil, errors.New("公会战中止目录列不一致")
				}
				seen[c] = true
			}
		}
		sort.Strings(columns)
		keys := []string{}
		for _, field := range schema.PrimaryFields {
			keys = append(keys, field.DBName)
		}
		if len(keys) == 0 {
			return nil, errors.New("公会战中止目录缺主键")
		}
		digest := sha256.New()
		count := int64(0)
		var cursor []any
		for {
			q := tx.Model(domain.Model).Select(columns).Clauses(clause.Locking{Strength: "SHARE"})
			scope := match.ID
			if domain.Extra != "" {
				scope = ""
			}
			if domain.MatchColumn != "" {
				q = q.Where(domain.MatchColumn+"=?", scope)
			}
			if domain.Extra != "" {
				q = q.Where(domain.Extra, domain.Args...)
			}
			for _, key := range keys {
				q = q.Order(key)
			}
			if cursor != nil {
				if len(keys) == 1 {
					q = q.Where(keys[0]+">?", cursor[0])
				} else {
					q = q.Where("("+strings.Join(keys, ",")+") > ("+strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")+")", cursor...)
				}
			}
			rows, err := q.Limit(128).Rows()
			if err != nil {
				return nil, err
			}
			n := 0
			for rows.Next() {
				values := make([]any, len(columns))
				ptrs := make([]any, len(columns))
				for i := range values {
					ptrs[i] = &values[i]
				}
				if err = rows.Scan(ptrs...); err != nil {
					rows.Close()
					return nil, err
				}
				scalar := map[string]string{}
				rawKeys := map[string]any{}
				for i, name := range columns {
					scalar[name] = p13UFAbortScalar(values[i])
					rawKeys[name] = values[i]
				}
				raw, _ := json.Marshal(scalar)
				digest.Write(raw)
				digest.Write([]byte{10})
				cursor = make([]any, len(keys))
				for i, key := range keys {
					cursor[i] = rawKeys[key]
				}
				n++
				count++
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
			if n < 128 {
				break
			}
		}
		// A later empty domain may be added without rewriting old abort
		// certificates. Any actual row in an unsealed new domain is rejected.
		if frozen != nil && !recorded {
			if count != 0 {
				return nil, errors.New("公会战中止后出现新领域来源")
			}
			continue
		}
		result = append(result, p13UFAbortDirectory{Table: name, Columns: columns, Count: count, SHA: hex.EncodeToString(digest.Sum(nil))})
	}
	if frozen != nil && consumed != len(frozen) {
		return nil, errors.New("公会战中止目录缺领域")
	}
	return result, nil
}

func p13UFAbortParticipants(tx *gorm.DB, match *db.P13UnionFightMatch) ([]p13UFAbortParticipant, error) {
	// These identities only authorize reading an abort notice; they grant no
	// participation or economic award. Damaged normal roster bytes stay evidence.
	result := []p13UFAbortParticipant{}
	cursor := int64(0)
	for {
		var rows []db.P13UnionFightFieldPlayer
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("player_id,guild_id,camp,joined_at,battle_hash").Where("match_id=? AND player_id>?", match.ID, cursor).Order("player_id").Limit(128).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			cursor = row.PlayerID
			if row.JoinedAt <= 0 || row.JoinedAt >= match.EndAt || row.Camp < 0 || row.Camp > 1 || row.GuildID != []string{match.LeftGuildID, match.RightGuildID}[row.Camp] {
				continue
			}
			result = append(result, p13UFAbortParticipant{row.PlayerID, row.Camp, row.JoinedAt, row.GuildID, row.BattleHash})
		}
		if len(rows) < 128 {
			break
		}
	}
	return result, nil
}

func p13UFAbortIDs(participants []p13UFAbortParticipant) []int64 {
	ids := []int64{}
	for _, p := range participants {
		ids = append(ids, p.PlayerID)
	}
	return ids
}

func p13UFTechnicalAbortSeal(tx *gorm.DB, match *db.P13UnionFightMatch, offer *db.P13UnionFightSettlementOffer, rule *db.P13UnionFightSettlementPolicy, now time.Time) (*db.P13UnionFightSettlementResult, error) {
	deadline, err := p13UFAbortDeadline(match, rule)
	if err != nil {
		return nil, err
	}
	if now.UnixMilli() < deadline {
		return nil, nil
	}
	var saved db.P13UnionFightSettlementResult
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&saved, "match_id=?", match.ID).Error; err == nil {
		return &saved, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	participants, err := p13UFAbortParticipants(tx, match)
	if err != nil {
		return nil, err
	}
	directory, err := p13UFAbortDirectoryRead(tx, match, p13UFAbortDirectoryIDs(match, participants), nil)
	if err != nil {
		return nil, err
	}
	terminal := match.EndAt
	// A corrupt base cannot supply a winner or an earlier trusted clock. The
	// original scheduled end remains the conservative business cutoff.
	if at, e := p13UFMixedTerminalAt(tx, match.ID); e == nil && at >= match.StartAt && at < terminal {
		terminal = at
	}
	proof := p13UFAbortProof{Schema: constUtil.P13UFTechnicalAbortSchema, Rule: rule.AbortRule, MatchSHA: match.SnapshotHash, MatchIdentitySHA: p13UFAbortMatchIdentity(match), OfferSHA: offer.RulesHash, ObservedRosterSHA: p6Digest(match.Roster), ObservedRulesSHA: p6Digest(offer.Rules), Reason: "finalization-deadline", StartAt: match.StartAt, Deadline: deadline, TerminalAt: terminal, ObservedAt: now.UnixMilli(), Participants: participants, Directory: directory}
	raw, err := json.Marshal(proof)
	if err != nil {
		return nil, err
	}
	saved = db.P13UnionFightSettlementResult{MatchID: match.ID, ResultKind: constUtil.P13UFTechnicalAbortKind, TerminalAt: terminal, EndAt: match.EndAt, SettledAt: now.UnixMilli(), AwardAt: 0, DeliveredAt: now.UnixMilli(), OfferHash: offer.RulesHash, Awards: []byte("[]"), Reason: proof.Reason, CombatProof: raw, LedgerHash: p6Digest(raw), Source: constUtil.P13UFTechnicalAbortSource}
	saved.CombatProofHash = p13UFCombatProofDigest(&saved)
	if err = tx.Create(&saved).Error; err != nil {
		return nil, err
	}
	return &saved, nil
}

func p13UFTechnicalAbortVerify(tx *gorm.DB, match *db.P13UnionFightMatch, result *db.P13UnionFightSettlementResult) error {
	reject := func() error { return errors.New("公会战技术中止证明或来源目录不一致") }
	if result.ResultKind != constUtil.P13UFTechnicalAbortKind || result.Source != constUtil.P13UFTechnicalAbortSource || result.MatchID != match.ID || result.EndAt != match.EndAt || result.WinnerGuildID != "" || result.LoserGuildID != "" || result.LeftKills != 0 || result.RightKills != 0 || result.LeftPower != 0 || result.RightPower != 0 || result.WinTaskPrestige != 0 || result.LoseTaskPrestige != 0 || result.LeftReachedAt != 0 || result.RightReachedAt != 0 || len(result.MVP) != 0 || string(result.Awards) != "[]" || result.AwardAt != 0 || result.CombatProofHash != p13UFCombatProofDigest(result) || result.LedgerHash != p6Digest(result.CombatProof) {
		return reject()
	}
	var proof p13UFAbortProof
	if json.Unmarshal(result.CombatProof, &proof) != nil || proof.Schema != constUtil.P13UFTechnicalAbortSchema || proof.MatchSHA != match.SnapshotHash || proof.MatchIdentitySHA != p13UFAbortMatchIdentity(match) || proof.OfferSHA != result.OfferHash || proof.Reason != result.Reason || proof.Reason != "finalization-deadline" || proof.StartAt != match.StartAt || proof.TerminalAt != result.TerminalAt || proof.TerminalAt < match.StartAt || proof.TerminalAt > match.EndAt || proof.ObservedAt != result.SettledAt {
		return reject()
	}
	var offer db.P13UnionFightSettlementOffer
	if err := tx.First(&offer, "match_id=?", match.ID).Error; err != nil {
		return err
	}
	rule, err := p13UFAbortFrozenRule(&offer, match)
	if err != nil {
		return err
	}
	if rule == nil {
		return reject()
	}
	deadline, err := p13UFAbortDeadline(match, rule)
	if err != nil || proof.Deadline != deadline || proof.ObservedAt < deadline || proof.Rule != rule.AbortRule || offer.RulesHash != proof.OfferSHA || proof.ObservedRosterSHA != p6Digest(match.Roster) || proof.ObservedRulesSHA != p6Digest(offer.Rules) {
		return reject()
	}
	participants, err := p13UFAbortParticipants(tx, match)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(participants, proof.Participants) {
		return reject()
	}
	directory, err := p13UFAbortDirectoryRead(tx, match, p13UFAbortDirectoryIDs(match, participants), proof.Directory)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(directory, proof.Directory) {
		return reject()
	}
	return nil
}

func (s *GameService) p13UFSealTechnicalAbort(tx *gorm.DB, match *db.P13UnionFightMatch, offer *db.P13UnionFightSettlementOffer, rule *db.P13UnionFightSettlementPolicy, now time.Time) (*db.P13UnionFightSettlementResult, error) {
	return p13UFTechnicalAbortSeal(tx, match, offer, rule, now)
}

func p13UFAbortMatchIdentity(match *db.P13UnionFightMatch) string {
	raw, _ := json.Marshal(match)
	return p6Digest(raw)
}

// Filter terminal and expired-new-rule scopes before loading attempt payloads.
// CommitGate repeats the decision under the shared mutation lock.
func p13UFCombatOpenCandidates(tx *gorm.DB, table string, now time.Time) *gorm.DB {
	return tx.Where("NOT EXISTS (SELECT 1 FROM p13_union_fight_settlement_results AS terminal WHERE terminal.match_id="+table+".match_id)").Where("NOT EXISTS (SELECT 1 FROM p13_union_fight_settlement_offers AS o JOIN p13_union_fight_matches AS m ON m.id=o.match_id WHERE o.match_id="+table+".match_id AND o.abort_rule=? AND m.end_at<=?)", constUtil.P13UFTechnicalAbortRule, now.UnixMilli()-5000)
}

// This terminal notice is a private original-protocol response, with no
// settlement rewards, battle HP or fabricated winner/loser frame.
func (s *GameService) p13UFTechnicalAbortPublish(ctx context.Context, result *db.P13UnionFightSettlementResult) {
	var proof p13UFAbortProof
	if result == nil || result.ResultKind != constUtil.P13UFTechnicalAbortKind || json.Unmarshal(result.CombatProof, &proof) != nil {
		return
	}
	for _, participant := range proof.Participants {
		s.publishP10Private(ctx, participant.PlayerID, &pb.SCUnionFightUpdateState{UnionFightId: proto.String(result.MatchID), State: proto.Int32(3), LeftTime: proto.Int64(0), TotalTime: proto.Int64(proof.TerminalAt - proof.StartAt)})
		s.publishP10Private(ctx, participant.PlayerID, &pb.SCUFMessage{Msg: proto.String(constUtil.P13UFTechnicalAbortMessage)})
	}
}

func p13UFAbortDirectoryIDs(match *db.P13UnionFightMatch, participants []p13UFAbortParticipant) []int64 {
	seen := map[int64]bool{}
	for _, p := range participants {
		seen[p.PlayerID] = true
	}
	var roster p13UFMatchRoster
	if p6Digest(match.Roster) == match.SnapshotHash && json.Unmarshal(match.Roster, &roster) == nil {
		for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
			for _, f := range side {
				if f.PlayerID > 0 {
					seen[f.PlayerID] = true
				}
			}
		}
	}
	ids := []int64{}
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
