package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ProfileStorageRollback86Options bounds one run. A nil After starts with the
// first stored ID, including historical negative IDs.
type ProfileStorageRollback86Options struct {
	After     *int64
	BatchSize int
	MaxRows   int64
	Apply     bool
}

// ProfileStorageRollback86Row contains only the identity and digests required
// to audit one stored game. Null games have no byte digest.
type ProfileStorageRollback86Row struct {
	ID           int64  `json:"id"`
	GameNull     bool   `json:"gameNull"`
	Changed      bool   `json:"changed"`
	BeforeSHA256 string `json:"beforeSHA256,omitempty"`
	AfterSHA256  string `json:"afterSHA256,omitempty"`
}

// ProfileStorageRollback86Batch is emitted only after its transaction commits.
type ProfileStorageRollback86Batch struct {
	After     *int64                        `json:"after"`
	NextAfter *int64                        `json:"nextAfter"`
	Scanned   int                           `json:"scanned"`
	Expanded  int                           `json:"expanded"`
	Rows      []ProfileStorageRollback86Row `json:"rows"`
}

// ProfileStorageRollback86Summary reports committed batches and whether the
// scan reached the end. A bounded run can stop before proving exhaustion.
type ProfileStorageRollback86Summary struct {
	Apply     bool   `json:"apply"`
	Scanned   int64  `json:"scanned"`
	Expanded  int64  `json:"expanded"`
	Batches   int64  `json:"batches"`
	NextAfter *int64 `json:"nextAfter"`
	Exhausted bool   `json:"exhausted"`
}

func profileStorageRollback86ID(id int64) *int64 {
	return &id
}

func profileStorageRollback86Digest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

type profileStorageRollback86Metadata struct {
	ID        int64
	GameBytes sql.NullInt64
}

type profileStorageRollback86Error struct {
	message string
}

func (e *profileStorageRollback86Error) Error() string {
	return e.message
}

func profileStorageRollback86Errorf(format string, args ...any) error {
	return &profileStorageRollback86Error{message: fmt.Sprintf(format, args...)}
}

// profileStorageRollback86Page keeps an ordered prefix inside the memory bound.
func profileStorageRollback86Page(metadata []profileStorageRollback86Metadata, byteLimit int64) ([]int64, error) {
	ids := make([]int64, 0, len(metadata))
	var totalBytes int64
	for _, item := range metadata {
		size := item.GameBytes.Int64
		if !item.GameBytes.Valid {
			size = 0
		}
		if size < 0 {
			return nil, profileStorageRollback86Errorf("profile %d has an invalid game size", item.ID)
		}
		if size > byteLimit || totalBytes > byteLimit-size {
			if len(ids) == 0 {
				return nil, profileStorageRollback86Errorf("profile %d exceeds the rollback batch byte limit", item.ID)
			}
			break
		}
		ids = append(ids, item.ID)
		totalBytes += size
	}
	return ids, nil
}

// profileStorageRollback86UniqueFields refuses duplicate top-level keys before
// the raw-map codec can collapse them, so maintenance never loses such input.
func profileStorageRollback86UniqueFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if first != nil {
		if first != json.Delim('{') {
			return errors.New("game must be an object or null")
		}
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid game key")
			}
			if _, ok = seen[key]; ok {
				return errors.New("duplicate game key")
			}
			seen[key] = struct{}{}
			var value json.RawMessage
			if err = decoder.Decode(&value); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("invalid game object")
		}
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing game data")
	}
	return nil
}

// RunProfileStorageRollback86 expands the original raw roster under bounded
// transactions. Apply serializes with maintenance changes and each selected
// profile; preview runs read-only transactions. It does not initialize schema.
// The caller verifies the target database, backup and stopped application.
func RunProfileStorageRollback86(ctx context.Context, database *gorm.DB, options ProfileStorageRollback86Options, emit func(ProfileStorageRollback86Batch) error) (ProfileStorageRollback86Summary, error) {
	summary := ProfileStorageRollback86Summary{Apply: options.Apply, NextAfter: options.After}
	if database == nil || options.BatchSize < 1 || options.BatchSize > constUtil.ProfileStorageRollbackBatchMax86 || options.MaxRows < 1 {
		return summary, errors.New("invalid profile rollback bounds")
	}
	lengthExpression := "OCTET_LENGTH(game)"
	switch database.Dialector.Name() {
	case "mysql":
	case "sqlite":
		lengthExpression = "length(CAST(game AS BLOB))"
	default:
		return summary, errors.New("unsupported profile rollback database")
	}
	for summary.Scanned < options.MaxRows {
		limit := options.BatchSize
		if left := options.MaxRows - summary.Scanned; left < int64(limit) {
			limit = int(left)
		}
		batch := ProfileStorageRollback86Batch{After: summary.NextAfter, Rows: []ProfileStorageRollback86Row{}}
		endOfRange := false
		txOptions := &sql.TxOptions{ReadOnly: !options.Apply}
		err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Maintenance first follows the same lock order as session binding.
			var policy db.ClientUpdatePolicy
			policyQuery := tx.Model(&db.ClientUpdatePolicy{}).Select("id", "maintenance")
			if options.Apply {
				policyQuery = policyQuery.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := policyQuery.Where("id = ?", 1).Take(&policy).Error; err != nil || !policy.Maintenance {
				return profileStorageRollback86Errorf("maintenance policy unavailable or disabled")
			}
			// Lock only a bounded candidate page and inspect lengths before loading
			// game JSON; a second set query avoids per-profile reads.
			var metadata []profileStorageRollback86Metadata
			query := tx.Model(&db.Profile{}).Select("id, " + lengthExpression + " AS game_bytes").Order("id ASC").Limit(limit)
			if options.Apply {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if batch.After != nil {
				query = query.Where("id > ?", *batch.After)
			}
			if err := query.Find(&metadata).Error; err != nil {
				return profileStorageRollback86Errorf("cannot read profile batch")
			}
			ids, err := profileStorageRollback86Page(metadata, constUtil.ProfileStorageRollbackBatchBytesMax86)
			if err != nil {
				return err
			}
			endOfRange = len(ids) == len(metadata) && len(metadata) < limit
			if len(ids) == 0 {
				return nil
			}
			var profiles []db.Profile
			values := tx.Model(&db.Profile{}).Select("id", "game").Where("id IN ?", ids).Order("id ASC")
			if options.Apply {
				values = values.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := values.Find(&profiles).Error; err != nil || len(profiles) != len(ids) {
				return profileStorageRollback86Errorf("cannot read the selected profile batch")
			}
			for index, profile := range profiles {
				if profile.Id != ids[index] || (profile.Game == nil) != !metadata[index].GameBytes.Valid ||
					profile.Game != nil && int64(len(*profile.Game)) != metadata[index].GameBytes.Int64 {
					return profileStorageRollback86Errorf("profile batch changed while reading")
				}
				row := ProfileStorageRollback86Row{ID: profile.Id, GameNull: profile.Game == nil}
				if profile.Game != nil {
					original := []byte(*profile.Game)
					if err := profileStorageRollback86UniqueFields(original); err != nil {
						return profileStorageRollback86Errorf("profile %d has invalid or duplicate game fields", profile.Id)
					}
					expanded, err := model.ExpandProfileGame86(original)
					if err != nil {
						return profileStorageRollback86Errorf("profile %d has an invalid game or roster", profile.Id)
					}
					row.BeforeSHA256 = profileStorageRollback86Digest(original)
					row.AfterSHA256 = profileStorageRollback86Digest(expanded)
					row.Changed = !bytes.Equal(original, expanded)
					if row.Changed {
						batch.Expanded++
						if options.Apply {
							// UpdateColumn avoids Save hooks and modifies only game under its row lock.
							update := tx.Model(&db.Profile{}).Where("id = ?", profile.Id).UpdateColumn("game", string(expanded))
							if update.Error != nil || update.RowsAffected != 1 {
								return profileStorageRollback86Errorf("profile %d update failed", profile.Id)
							}
						}
					}
				}
				batch.Rows = append(batch.Rows, row)
				batch.NextAfter = profileStorageRollback86ID(profile.Id)
			}
			batch.Scanned = len(batch.Rows)
			return nil
		}, txOptions)
		if err != nil {
			var safe *profileStorageRollback86Error
			if errors.As(err, &safe) {
				return summary, safe
			}
			return summary, errors.New("profile rollback transaction failed")
		}
		if batch.Scanned == 0 {
			summary.Exhausted = true
			break
		}
		summary.Scanned += int64(batch.Scanned)
		summary.Expanded += int64(batch.Expanded)
		summary.Batches++
		summary.NextAfter = batch.NextAfter
		if emit != nil {
			if err := emit(batch); err != nil {
				return summary, errors.New("cannot write committed batch report")
			}
		}
		if endOfRange {
			summary.Exhausted = true
			break
		}
	}
	return summary, nil
}
