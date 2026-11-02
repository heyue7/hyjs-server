package service

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func copyRunToken(run *model.CopyRun) (string, error) {
	raw, err := json.Marshal(run)
	if err != nil {
		return "", err
	}
	return p6Digest(raw), nil
}

func copyRunStatus(run *model.CopyRun) (*model.CopyRunRecoveryResponse, error) {
	out := &model.CopyRunRecoveryResponse{}
	if run == nil || run.Complete {
		return out, nil
	}
	token, err := copyRunToken(run)
	if err != nil {
		return nil, err
	}
	out.Active, out.ChapterID, out.MissionID, out.RunToken = true, run.Chapter, run.Mission, token
	return out, nil
}

// copyRunAuthorizedProfile uses the shared session gates, while projecting
// only the columns needed for the requested operation and clock binding.
func (s *GameService) copyRunAuthorizedProfile(ctx context.Context, auth model.GGOFieldAuth, session []byte, status bool) (*db.Profile, error) {
	if s == nil || auth.PlayerID < 1 || len(session) != 16 || auth.SessionHex != hex.EncodeToString(session) {
		return nil, model.NewErrParamsError()
	}
	columns := []string{"id", "server_id", "session", "account_id", "account_auth_revision"}
	if status {
		columns = append(columns, "game")
	} else {
		// The legacy local-play clock checks the account name and whether a
		// character exists before binding its isolated test clock.
		columns = append(columns, "uid", "player")
	}
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).Select(columns).First(&row, auth.PlayerID).Error; err != nil {
		return nil, err
	}
	if err := authorizeProfileSession(ctx, &row, session); err != nil {
		return nil, ggoClientAuthError(err)
	}
	if AdminProfileServerID(row) != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	return &row, nil
}

// CopyRunStatus reads only the stored Run. Shared session authorization may
// maintain its existing client-update metadata, but this query never saves the
// profile or applies resource, calendar, scene, or battle recovery.
func (s *GameService) CopyRunStatus(ctx context.Context, auth model.GGOFieldAuth, session []byte) (*model.CopyRunRecoveryResponse, error) {
	row, err := s.copyRunAuthorizedProfile(ctx, auth, session, true)
	if err != nil {
		return nil, err
	}
	if row.Game == nil {
		return nil, errors.New("请先创建角色")
	}
	var saved struct {
		Run *model.CopyRun `json:"run"`
	}
	if err := json.Unmarshal([]byte(*row.Game), &saved); err != nil {
		return nil, err
	}
	return copyRunStatus(saved.Run)
}

// copyLeaveLocked is shared by the native leave request and explicit recovery.
// Its caller owns the profile lock and saves the resulting GameState.
func (s *GameService) copyLeaveLocked(ctx context.Context, tx *gorm.DB, id int64, game *model.GameState) error {
	if game.Run != nil && game.Run.Fight != nil && !game.Run.Fight.Settled {
		if err := s.CancelOriginalCopyReplay(ctx, tx, id, game.Run.Fight.Start); err != nil {
			return err
		}
	}
	game.Run = nil
	return nil
}

// CopyRunLeave clears the observed Run only when its complete persisted state
// still has the token confirmed by this session's player.
func (s *GameService) CopyRunLeave(ctx context.Context, auth model.GGOFieldAuth, session []byte, token string) (*model.CopyRunRecoveryResponse, error) {
	if !ggoResumeHex(token, 64) {
		return nil, model.NewErrParamsError()
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, &pb.CSCopyLeave{})
	owner, err := s.copyRunAuthorizedProfile(ctx, auth, session, false)
	if err != nil {
		return nil, err
	}
	ctx, err = s.BindLocalPlayContext(ctx, owner)
	if err != nil {
		return nil, err
	}
	out := &model.CopyRunRecoveryResponse{}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, auth.PlayerID).Error; err != nil {
			return err
		}
		if err := validateProfileSession(&row, session); err != nil {
			return err
		}
		if AdminProfileServerID(row) != s.OriginalServerID() {
			return constUtil.ErrAdminPermission
		}
		if row.AccountID > 0 {
			account, err := AccountForProfile(tx, &row)
			if err != nil {
				return validateProfileAccount(&row, nil)
			}
			if err := validateProfileAccount(&row, account); err != nil {
				return err
			}
		}
		persisted := row
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if game.Run == nil {
			return nil
		}
		current, err := copyRunToken(game.Run)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(current), []byte(token)) != 1 {
			return model.NewErrBizCode("副本进度已变化，请重新查询后再确认退出")
		}
		if err := s.copyLeaveLocked(ctx, tx, row.Id, game); err != nil {
			return err
		}
		return saveProfileAgainst(tx, &row, player, game, &persisted)
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	return out, nil
}
