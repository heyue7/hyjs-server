package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p7RushCertificateRuntime struct {
	key       []byte
	validator string
}

var p7RushCertificateDatabases sync.Map

func p7RushCertificateSettings() (*p7RushCertificateRuntime, error) {
	if config.AppConfigIns == nil || len(config.AppConfigIns.Security.RushHistoryCertificateKey) < 32 {
		return nil, nil
	}
	derive := hmac.New(sha256.New, []byte(config.AppConfigIns.Security.RushHistoryCertificateKey))
	_, _ = derive.Write([]byte(constUtil.P7RushHistoryCertificateDomain))
	return &p7RushCertificateRuntime{key: derive.Sum(nil), validator: p6Digest([]byte(constUtil.P7RushHistoryValidationContract + ":" + constUtil.P6EngineVersion))}, nil
}

// InitializeP7RushHistoryCertificates initializes durable settlement evidence.
// An absent/short key disables it while all original verification remains available.
func InitializeP7RushHistoryCertificates(ctx context.Context) error {
	settings, err := p7RushCertificateSettings()
	if err != nil || settings == nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P7RushHistoryCertificate{}); err != nil {
		return err
	}
	pool, err := tx.DB()
	if err != nil {
		return err
	}
	p7RushCertificateDatabases.Store(pool, true)
	return nil
}

func p7RushCertificateRuntimeFor(tx *gorm.DB) *p7RushCertificateRuntime {
	settings, err := p7RushCertificateSettings()
	if err != nil || settings == nil {
		return nil
	}
	pool, err := tx.DB()
	if err != nil {
		return nil
	}
	if _, ok := p7RushCertificateDatabases.Load(pool); !ok {
		return nil
	}
	return settings
}

func (r *p7RushCertificateRuntime) sign(c db.P7RushHistoryCertificate) string {
	fields := []string{constUtil.P7RushHistoryCertificateDomain, strconv.FormatInt(c.Format, 10), strconv.FormatInt(c.ServerID, 10), strconv.FormatInt(c.PlayerID, 10), c.FightID, c.Validator, c.Fingerprint, c.OutcomeHash, strconv.FormatInt(int64(c.Winner), 10), strconv.FormatInt(c.VerifiedAt, 10)}
	body, _ := json.Marshal(fields)
	mac := hmac.New(sha256.New, r.key)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func p7RushCertificateFingerprint(fight *db.P7RushFight, manual *db.P7RushManualCalculation, record *db.P6BattleRecord, end *pb.CSRushFightEnd) (string, error) {
	return p7RushDurableFingerprint(fight, manual, record, end, nil, nil)
}

func p7RushDurableFingerprint(fight *db.P7RushFight, manual any, record *db.P6BattleRecord, end *pb.CSRushFightEnd, manualBlobs, recordBlobs map[string]p7RushBlobDigest) (string, error) {
	a, err := p7RushStoredRowFingerprint(fight)
	if err != nil {
		return "", err
	}
	b, err := p7RushDurableRowFingerprint(manual, manualBlobs)
	if err != nil {
		return "", err
	}
	c, err := p7RushDurableRowFingerprint(record, recordBlobs)
	if err != nil {
		return "", err
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(end)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal([]string{a, p6Digest(raw), b, c})
	if err != nil {
		return "", err
	}
	return p6Digest(body), nil
}

// accepted keeps raw-row callers strict; projected rows use a separately named
// boundary after their complete actual-byte canonical fingerprint is checked.
func (r *p7RushCertificateRuntime) accepted(c db.P7RushHistoryCertificate, serverID, playerID int64, fightID, fingerprint string, record *db.P6BattleRecord) *model.CombatOutcome {
	if p6Digest(record.Outcome) != c.OutcomeHash {
		return nil
	}
	return r.acceptedProjection(c, serverID, playerID, fightID, fingerprint, record)
}

func (r *p7RushCertificateRuntime) acceptedProjection(c db.P7RushHistoryCertificate, serverID, playerID int64, fightID, fingerprint string, record *db.P6BattleRecord) *model.CombatOutcome {
	if c.Format != constUtil.P7RushHistoryCertificateFormat || c.Validator != r.validator || c.ServerID != serverID || c.PlayerID != playerID || c.FightID != fightID || c.Fingerprint != fingerprint || len(fingerprint) != 64 || c.VerifiedAt <= 0 || c.OutcomeHash != record.OutcomeHash {
		return nil
	}
	actual, err := hex.DecodeString(c.MAC)
	if err != nil {
		return nil
	}
	expected, _ := hex.DecodeString(r.sign(c))
	if !hmac.Equal(actual, expected) {
		return nil
	}
	if record.PlayerID != playerID || record.SourceFight != fightID || record.Source != constUtil.P7RushReplaySource || record.Status != constUtil.P6BattleComputed || !record.Authoritative {
		return nil
	}
	if c.Winner < 0 || c.Winner > 1 {
		return nil
	}
	return &model.CombatOutcome{Authoritative: true, Winner: c.Winner}
}

func (q *p7EliteQueryProjection) loadCertificates() {
	if q.serverID < 1 {
		return
	}
	q.certificateRuntime = p7RushCertificateRuntimeFor(q.tx)
	if q.certificateRuntime == nil {
		return
	}
	q.certificates = map[string]db.P7RushHistoryCertificate{}
	ids := make([]string, 0, len(q.manuals))
	for id := range q.manuals {
		ids = append(ids, id)
	}
	for id := range q.scoped {
		ids = append(ids, id)
	}
	for from := 0; from < len(ids); from += 128 {
		to := min(from+128, len(ids))
		var rows []db.P7RushHistoryCertificate
		if err := q.tx.Where("server_id=? AND player_id=? AND fight_id IN ?", q.serverID, q.state.PlayerID, ids[from:to]).Find(&rows).Error; err != nil {
			q.certificateRuntime = nil
			q.certificates = nil
			return
		}
		for _, row := range rows {
			q.certificates[row.FightID] = row
		}
	}
}

// Only a successful original strict verifier may publish durable evidence in this transaction.
func (q *p7EliteQueryProjection) certify(fight *db.P7RushFight, record *db.P6BattleRecord, fingerprint string, outcome *model.CombatOutcome) error {
	r := q.certificateRuntime
	if r == nil || q.serverID < 1 || len(fingerprint) != 64 || outcome == nil || !outcome.Authoritative {
		return nil
	}
	var actual model.CombatOutcome
	encoded, encodeErr := json.Marshal(outcome)
	if record.PlayerID != q.state.PlayerID || record.SourceFight != fight.ID || record.Source != constUtil.P7RushReplaySource || record.Status != constUtil.P6BattleComputed || !record.Authoritative || p6Digest(record.Outcome) != record.OutcomeHash || json.Unmarshal(record.Outcome, &actual) != nil || !actual.Authoritative || encodeErr != nil || !bytes.Equal(encoded, record.Outcome) {
		return errors.New(constUtil.P7ScopedRushConflict)
	}
	if old, ok := q.certificates[fight.ID]; ok && r.accepted(old, q.serverID, q.state.PlayerID, fight.ID, fingerprint, record) != nil {
		return nil
	}
	c := db.P7RushHistoryCertificate{ServerID: q.serverID, PlayerID: q.state.PlayerID, FightID: fight.ID, Format: constUtil.P7RushHistoryCertificateFormat, Validator: r.validator, Fingerprint: fingerprint, OutcomeHash: record.OutcomeHash, Winner: outcome.Winner, VerifiedAt: time.Now().UnixMilli()}
	c.MAC = r.sign(c)
	err := q.tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "server_id"}, {Name: "player_id"}, {Name: "fight_id"}}, DoUpdates: clause.AssignmentColumns([]string{"format", "validator", "fingerprint", "outcome_hash", "winner", "verified_at", "mac"})}).Create(&c).Error
	if err == nil {
		q.certificates[fight.ID] = c
	}
	return err
}

// WarmP7RushHistory validates bounded existing settled sources without running a
// directory query, calendar recovery, resource changes, rewards or progress writes.
func WarmP7RushHistory(ctx context.Context, id int64) error {
	if id < 1 {
		return fmt.Errorf("invalid trial history owner")
	}
	if p7RushCertificateRuntimeFor(db.GetIns()) == nil {
		return constUtil.ErrP7RushHistoryCertificateUnavailable
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		var state db.P7RushState
		if err := tx.First(&state, "player_id=?", id).Error; err != nil {
			return err
		}
		realm := constUtil.GameDefaultServerID
		if config.AppConfigIns != nil && config.AppConfigIns.Game.ServerID > 0 {
			realm = config.AppConfigIns.Game.ServerID
		}
		if game, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok {
			realm = game.OriginalServerID()
		}
		if AdminProfileServerID(profile) != realm {
			return fmt.Errorf("trial history owner belongs to another realm")
		}
		var ids []db.P7RushFight
		if err := tx.Select("id").Where("player_id=? AND kind=? AND status=? AND diagnostic=?", id, constUtil.P7RushTrial, constUtil.P7RushSettled, false).Limit(constUtil.P7RushHistoryWarmLimit + 1).Find(&ids).Error; err != nil {
			return err
		}
		if len(ids) > constUtil.P7RushHistoryWarmLimit {
			return constUtil.ErrP7RushHistoryWarmLimit
		}
		q := &p7EliteQueryProjection{tx: tx, state: &state, serverID: realm, reads: map[string]p7EliteQueryRead{}, parsed: map[string]*p7EliteQueryParsed{}}
		if err := q.load(); err != nil {
			return err
		}
		for i := range q.fights {
			fight := &q.fights[i]
			manual, manualOK := q.manuals[fight.ID]
			_, scopedOK := q.scoped[fight.ID]
			if (!manualOK || manual.Status != constUtil.P7RushManualAccepted) && !scopedOK {
				continue
			}
			var end pb.CSRushFightEnd
			if proto.Unmarshal(fight.EndRequest, &end) != nil || end.GetFightId() != fight.ID {
				return errors.New(constUtil.P7ScopedRushConflict)
			}
			outcome, scoped, err := q.read(fight, &end)
			if err != nil {
				return err
			}
			if !scoped || outcome == nil || !outcome.Authoritative {
				return errors.New(constUtil.P7ScopedRushConflict)
			}

		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}
