package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func ggoResumeHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func ggoResumeConflict() error {
	return ggoFieldError(constUtil.GGOFieldRequestConflict, constUtil.GGOBattleResumeConflict)
}

// ggoResumeFrozenInput verifies the persisted compressed bytes before decoding.
// Older uncompressed inputs use their original JSON digest without rewriting them.
func ggoResumeFrozenInput(fight *model.FieldFight) (*model.FrozenBattleInput, string, error) {
	if fight == nil {
		return nil, "", ggoResumeConflict()
	}
	input, err := fieldBossFrozenInput(fight)
	if err != nil || input == nil {
		return nil, "", ggoResumeConflict()
	}
	hash := fight.BossInputHash
	if fight.BossInput != nil {
		raw, marshalErr := json.Marshal(fight.BossInput)
		if marshalErr != nil {
			return nil, "", ggoResumeConflict()
		}
		computed := p6Digest(raw)
		if hash != "" && hash != computed {
			return nil, "", ggoResumeConflict()
		}
		hash = computed
	}
	if !ggoResumeHex(hash, 64) {
		return nil, "", ggoResumeConflict()
	}
	return input, hash, nil
}

// ggoResumeSamePublicInput compares exported protocol fields while leaving
// unknown wire fields untouched in the original stored start response.
func ggoResumeSamePublicInput(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && bytes.Equal(left, right)
}

func ggoResumeReceipt(tx *gorm.DB, playerID int64, req model.GGOFieldBattleResumeRequest) (*model.GGOFieldBattleResumeResponse, bool, error) {
	var (
		row       db.GGOFieldReceipt
		operation = constUtil.GGOFieldOpMonsterResult
		kind      = "monster-result"
	)
	if req.BattleType == constUtil.GGOBattleResumeBoss {
		operation, kind = constUtil.GGOFieldBossFightOperation, "boss-fight"
	}
	err := tx.Where("player_id=? AND request_id=? AND operation=?", playerID, ggoSystemReceiptID(kind, req.ExpectedFightID), operation).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if row.BusinessKey == nil || *row.BusinessKey != req.ExpectedFightID || !ggoResumeHex(row.RequestHash, 64) {
		return nil, true, ggoResumeConflict()
	}
	out := &model.GGOFieldBattleResumeResponse{Status: constUtil.GGOBattleResumeSettled, BattleType: req.BattleType, FightID: req.ExpectedFightID, InputHash: req.ExpectedInputHash}
	if req.BattleType == constUtil.GGOBattleResumeMonster {
		var receipt model.GGOFieldMonsterReceipt
		if json.Unmarshal(row.Response, &receipt) != nil || receipt.Status != constUtil.GGOBattleResumeSettled ||
			receipt.FightID != req.ExpectedFightID || receipt.InputHash != req.ExpectedInputHash ||
			receipt.EndRequestHash != row.RequestHash || len(receipt.NativeEnd) == 0 {
			return nil, true, ggoResumeConflict()
		}
		var native pb.SCSidePveFightEnd
		if proto.Unmarshal(receipt.NativeEnd, &native) != nil || native.GetCode() != 1 {
			return nil, true, ggoResumeConflict()
		}
		out.NativeEnd, out.EndRequestHash = receipt.NativeEnd, receipt.EndRequestHash
	} else {
		var receipt model.GGOFieldBossBattleReceipt
		if json.Unmarshal(row.Response, &receipt) != nil || receipt.State != constUtil.GGOBattleResumeSettled ||
			receipt.FightID != req.ExpectedFightID || receipt.InputHash != req.ExpectedInputHash || receipt.EventID == "" ||
			receipt.EndRequestHash != row.RequestHash || len(receipt.NativeEnd) == 0 {
			return nil, true, ggoResumeConflict()
		}
		var native pb.SCSidePveBossFightEnd
		if proto.Unmarshal(receipt.NativeEnd, &native) != nil || native.GetCode() != 1 {
			return nil, true, ggoResumeConflict()
		}
		out.NativeEnd, out.EndRequestHash, out.EventID = receipt.NativeEnd, receipt.EndRequestHash, receipt.EventID
	}
	return out, true, nil
}

func ggoResumeActive(game *model.GameState, playerID int64, req model.GGOFieldBattleResumeRequest, now int64) (*model.GGOFieldBattleResumeResponse, error) {
	if game.Field == nil || game.Field.Scene != constUtil.GGOFieldScene || game.Field.Fight == nil || game.Field.Fight.Settled || game.GGOField == nil {
		return nil, ggoResumeConflict()
	}
	fight := game.Field.Fight
	input, hash, err := ggoResumeFrozenInput(fight)
	if err != nil || hash != req.ExpectedInputHash || int64(input.BattleType) != req.BattleType || input.Attacker == nil ||
		input.Attacker.GetPlayerId() != playerID || input.Monster == nil {
		return nil, ggoResumeConflict()
	}
	out := &model.GGOFieldBattleResumeResponse{Status: constUtil.GGOBattleResumeActive, BattleType: req.BattleType, FightID: req.ExpectedFightID, InputHash: hash}
	var native proto.Message
	if req.BattleType == constUtil.GGOBattleResumeMonster {
		pending, start := game.GGOField.MonsterFight, fight.Start
		if pending == nil || pending.FightID != req.ExpectedFightID || pending.MonsterUUID == "" ||
			pending.MonsterUUID != fight.MonsterUUID || pending.PointID == "" || pending.Generation < 1 || pending.Line < 1 ||
			(pending.ReservedUntilMS > 0 && now >= pending.ReservedUntilMS) || fight.GGOBoss != nil || fight.BossStart != nil ||
			start == nil || start.GetFightId() != req.ExpectedFightID || start.GetCode() != 1 ||
			start.GetRandSeed() != input.Seed || start.GetMaxround() != input.MaxRound || start.GetBattlePlayer() == nil ||
			len(start.BattleMonsterList) != 1 || !ggoResumeSamePublicInput(start.GetBattlePlayer(), input.Attacker) ||
			!ggoResumeSamePublicInput(start.BattleMonsterList[0], input.Monster) {
			return nil, ggoResumeConflict()
		}
		out.MonsterUUID, native = pending.MonsterUUID, start
	} else {
		frozen, start := fight.GGOBoss, fight.BossStart
		if frozen == nil || frozen.EventID == "" || frozen.EventID != fight.BossUUID || frozen.InputHash != hash ||
			frozen.Boss == nil || frozen.Boss.GetUuid() != frozen.EventID || game.GGOField.MonsterFight != nil ||
			start == nil || start.GetFightId() != req.ExpectedFightID || start.GetCode() != 1 ||
			(fight.Start != nil && fight.Start.GetFightId() != req.ExpectedFightID) ||
			start.GetRandSeed() != input.Seed || start.GetMaxround() != input.MaxRound || start.GetBattlePlayer() == nil ||
			len(start.BattleMonsterList) != 1 || !ggoResumeSamePublicInput(start.GetBattlePlayer(), input.Attacker) ||
			!ggoResumeSamePublicInput(start.BattleMonsterList[0], input.Monster) {
			return nil, ggoResumeConflict()
		}
		out.EventID, native = frozen.EventID, start
	}
	out.NativeStart, err = proto.Marshal(native)
	if err != nil || len(out.NativeStart) == 0 {
		return nil, ggoResumeConflict()
	}
	return out, nil
}

// GGOFieldBattleResume atomically reads a single accepted start or immutable
// result under the same profile lock as native settlement. It never starts,
// refreshes, expires or awards a fight, including after a publication change.
func (s *GameService) GGOFieldBattleResume(ctx context.Context, req model.GGOFieldBattleResumeRequest, session []byte) (*model.GGOFieldBattleResumeResponse, error) {
	if req.PlayerID < 1 || len(session) != 16 || req.SessionHex != hex.EncodeToString(session) ||
		(req.BattleType != constUtil.GGOBattleResumeMonster && req.BattleType != constUtil.GGOBattleResumeBoss) ||
		!ggoResumeHex(req.ExpectedFightID, 24) || !ggoResumeHex(req.ExpectedInputHash, 64) {
		return nil, model.NewErrParamsError()
	}
	profile, err := AuthorizeAccount(ctx, req.PlayerID, session)
	if err != nil {
		return nil, ggoClientAuthError(err)
	}
	if AdminProfileServerID(*profile) != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	ctx, err = s.BindLocalPlayContext(ctx, profile)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil {
		return nil, err
	}
	runtime := s.withRuntimeTables(snapshot.tables)
	runtime.GGOFieldConfig, runtime.GGOFieldEconomy, runtime.GGOFieldRevision = snapshot.ggo, snapshot.ggoEconomy, snapshot.ggoRevision
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, runtime)
	var out *model.GGOFieldBattleResumeResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, req.PlayerID).Error; err != nil {
			return err
		}
		if AdminProfileServerID(row) != s.OriginalServerID() || subtle.ConstantTimeCompare(row.Session, session) != 1 {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "会话已失效")
		}
		if err := ggoCheckClientBuildLocked(tx, row.Id, session, runtime.GGOFieldEconomy); err != nil {
			return err
		}
		// A resume is observational: do not run calendar recovery or persist a profile.
		_, game, err := decodeProfile(&row)
		if err != nil {
			return err
		}
		if game.Position == nil || game.Position.GetScene() != constUtil.GGOFieldScene {
			return ggoResumeConflict()
		}
		settled, found, err := ggoResumeReceipt(tx, row.Id, req)
		if err != nil {
			return err
		}
		if found {
			out = settled
			return nil
		}
		out, err = ggoResumeActive(game, row.Id, req, time.Now().UnixMilli())
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
