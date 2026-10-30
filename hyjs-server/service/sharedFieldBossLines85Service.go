package service

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// migrateSharedFieldBossLines85 keeps each old generation on line one. Startup
// changes the existing key before any independent line can accept a fight.
func migrateSharedFieldBossLines85(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&db.SharedFieldBoss55{}) {
		return tx.AutoMigrate(&db.SharedFieldBoss55{})
	}
	switch tx.Dialector.Name() {
	case "mysql":
		var columns []string
		if err := tx.Raw("SELECT COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='shared_field_boss55' AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX").Scan(&columns).Error; err != nil {
			return err
		}
		key := strings.Join(columns, ",")
		switch key {
		case "server_id,scene,pos_id":
			if tx.Migrator().HasColumn(&db.SharedFieldBoss55{}, "line") {
				if err := tx.Exec("ALTER TABLE shared_field_boss55 DROP PRIMARY KEY, ADD PRIMARY KEY (server_id,scene,line,pos_id)").Error; err != nil {
					return err
				}
			} else if err := tx.Exec("ALTER TABLE shared_field_boss55 ADD COLUMN line BIGINT NOT NULL DEFAULT 1, DROP PRIMARY KEY, ADD PRIMARY KEY (server_id,scene,line,pos_id)").Error; err != nil {
				return err
			}
		case "server_id,scene,line,pos_id":
		default:
			return errors.New("shared boss primary key does not match the migration source")
		}
	case "sqlite":
		var columns []struct {
			Name string
			PK   int
		}
		if err := tx.Raw("PRAGMA table_info(shared_field_boss55)").Scan(&columns).Error; err != nil {
			return err
		}
		lineKey := false
		for _, column := range columns {
			if column.Name == "line" && column.PK > 0 {
				lineKey = true
			}
		}
		if !lineKey {
			// SQLite cannot extend a primary key in place. The startup transaction keeps
			// the old row payloads intact and commits the replacement table atomically.
			if err := tx.AutoMigrate(&db.SharedFieldBoss55{}); err != nil {
				return err
			}
			if err := tx.Transaction(func(t *gorm.DB) error {
				if err := t.Exec("ALTER TABLE shared_field_boss55 RENAME TO shared_field_boss55_legacy85").Error; err != nil {
					return err
				}
				if err := t.Migrator().CreateTable(&db.SharedFieldBoss55{}); err != nil {
					return err
				}
				if err := t.Exec("INSERT INTO shared_field_boss55 (server_id,scene,line,pos_id,uuid,payload,hp,death_rules73,ggo_event,respawn_at,killer_id) SELECT server_id,scene,line,pos_id,uuid,payload,hp,death_rules73,ggo_event,respawn_at,killer_id FROM shared_field_boss55_legacy85").Error; err != nil {
					return err
				}
				return t.Exec("DROP TABLE shared_field_boss55_legacy85").Error
			}); err != nil {
				return err
			}
		}
	default:
		return errors.New("unsupported shared boss migration database")
	}
	return tx.AutoMigrate(&db.SharedFieldBoss55{})
}

// fieldBossCurrentLine85 uses the persisted server-selected line, including
// disconnect recovery. Client parameters cannot choose another spawn namespace.
func (s *GameService) fieldBossCurrentLine85(tx *gorm.DB, id int64) (int64, error) {
	if !gameSchemaHasTable(tx, &db.P10ScenePresence{}) {
		return 1, nil
	}
	var presence db.P10ScenePresence
	err := tx.Select("line").Where("player_id=?", id).First(&presence).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	if presence.Line < 1 {
		return 1, nil
	}
	return int64(presence.Line), nil
}

// fieldBossHandshakeScene85 recognizes only the original grass/snow field
// response. Desert bosses use their own per-line snapshot projection.
func fieldBossHandshakeScene85(request *model.GamePacket, messages []proto.Message) string {
	switch request.Body.(type) {
	case *pb.CSCurrentPosition, *pb.CSLoginScene:
	default:
		return ""
	}
	for _, message := range messages {
		var self *pb.MovePlayer
		switch value := message.(type) {
		case *pb.SCCurrentPosition:
			self = value.GetSelf()
		case *pb.SCLoginScene:
			self = value.GetSelf()
		}
		if self != nil && self.GetInfo().GetPlayerId() == request.PlayerId {
			scene := self.GetPosition().GetScene()
			if scene == "field_001" || scene == "field_002" {
				return scene
			}
		}
	}
	return ""
}

// Persist only the projection's four fields. This keeps unknown save fields,
// compressed roster bytes, position, resources and frozen fights intact.
func fieldBossSaveProjection85(tx *gorm.DB, row *db.Profile, field *model.FieldState) error {
	var root, savedField map[string]json.RawMessage
	if row.Game == nil {
		return errors.New("野外存档不存在")
	}
	if err := json.Unmarshal([]byte(*row.Game), &root); err != nil {
		return err
	}
	if err := json.Unmarshal(root["field"], &savedField); err != nil {
		return err
	}
	if savedField == nil {
		return errors.New("野外存档不存在")
	}
	values := map[string]any{"boss_line85": field.BossLine85, "respawn_at": field.RespawnAt}
	if len(field.Bosses) > 0 {
		values["bosses"] = field.Bosses
	} else {
		delete(savedField, "bosses")
	}
	if len(field.BossHP) > 0 {
		values["boss_hp"] = field.BossHP
	} else {
		delete(savedField, "boss_hp")
	}
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		savedField[key] = raw
	}
	fieldRaw, err := json.Marshal(savedField)
	if err != nil {
		return err
	}
	root["field"] = fieldRaw
	raw, err := json.Marshal(root)
	if err != nil {
		return err
	}
	if string(raw) == *row.Game {
		return nil
	}
	return tx.Model(&db.Profile{}).Where("id=?", row.Id).Update("game", string(raw)).Error
}

// The scene lock preserves the authoritative presence through the profile and
// shared-Boss row locks. Only a changed line refreshes and writes the save.
func (s *GameService) fieldBossAfterHandshake85(ctx context.Context, request *model.GamePacket, scene string, messages []proto.Message) ([]proto.Message, error) {
	runtime, err := s.fieldRuntimeForRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	runtime, err = runtime.guildRuntimeForRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	fieldConfig := runtime.fieldSceneRow(scene)
	if fieldConfig == nil || fieldConfig["adminOrdinaryBossEnabled"] == "false" {
		return messages, nil
	}
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	var projection []proto.Message
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		config, err := p10SceneConfig(tx)
		if err != nil || config == nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, request.PlayerId).Error; err != nil {
			return err
		}
		if err := validateProfileSession(&row, request.Session); err != nil {
			return err
		}
		if row.AccountID > 0 {
			account, err := AccountForProfile(tx, &row)
			if err != nil {
				return err
			}
			if err := validateProfileAccount(&row, account); err != nil {
				return err
			}
		}
		if AdminProfileServerID(row) != s.OriginalServerID() || row.Game == nil {
			return errors.New("当前区服或野外存档已变更")
		}
		// Decode only the scene-owned fields; the cold roster is never expanded.
		var state struct {
			Position        *pb.Position      `json:"position"`
			PositionLoading bool              `json:"position_loading"`
			Run             *model.CopyRun    `json:"run"`
			Field           *model.FieldState `json:"field"`
		}
		if err := json.Unmarshal([]byte(*row.Game), &state); err != nil {
			return err
		}
		game := &model.GameState{Position: state.Position, PositionLoading: state.PositionLoading, Run: state.Run, Field: state.Field}
		if game.Position.GetScene() != scene || game.Field == nil || game.Field.Scene != scene {
			return errors.New("场景已变更，请重试")
		}
		if p10SceneScope(game) != scene {
			// Loading or an accepted fight has no allocated scene. Leave its
			// persisted battle projection intact until a later position handshake.
			projection = make([]proto.Message, 0)
			return nil
		}
		value, ok := p10LiveClient(p10ClientKey{s.p10ClientOwner(), request.PlayerId})
		if !ok {
			projection = make([]proto.Message, 0)
			return nil
		}
		if subtle.ConstantTimeCompare(value.(*p10ClientSink).session, request.Session) != 1 {
			return errors.New("场景会话已变更，请重新登录")
		}
		config, err = s.adminFieldSceneConfig(tx, scene, config)
		if err != nil {
			return err
		}
		var presence db.P10ScenePresence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&presence, "player_id=?", request.PlayerId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				projection = make([]proto.Message, 0)
				return nil
			}
			return err
		}
		if presence.Scene != scene || presence.Line < 1 || presence.Line > config.LineCount || presence.SessionHash != p6Digest(request.Session) {
			projection = make([]proto.Message, 0)
			return nil
		}
		if game.Field.BossLine85 != int64(presence.Line) {
			if err := runtime.refreshSharedFieldBosses55(tx, request.PlayerId, game); err != nil {
				return err
			}
			if err := fieldBossSaveProjection85(tx, &row, game.Field); err != nil {
				return err
			}
		}
		projection = append(projection, runtime.fieldBossProjection(game)...)
		projection = append(projection, fieldBossCountdownProjection(game.Field, time.Now())...)
		current, ok := p10LiveClient(p10ClientKey{s.p10ClientOwner(), request.PlayerId})
		if !ok || current != value {
			return errors.New("场景会话已断开，请重试")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil || projection == nil {
		return messages, err
	}
	result := make([]proto.Message, 0, len(messages))
	inserted := false
	for _, message := range messages {
		switch message.(type) {
		case *pb.SCSideBoss, *pb.SCSideBossCountdown:
			if !inserted {
				result = append(result, projection...)
				inserted = true
			}
		default:
			result = append(result, message)
		}
	}
	if !inserted {
		return nil, errors.New("野外首领投影缺失")
	}
	return result, nil
}
