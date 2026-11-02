package service

import (
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p11PlunderSweepActors is created after the sorted profile locks, and lives
// only for that transaction attempt. An unlocked candidate is never retained.
type p11PlunderSweepActors struct {
	tx     *gorm.DB
	locked map[int64]bool
	loaded map[int64]*p11PlunderSweepActor
}

type p11PlunderSweepActor struct {
	row    db.Profile
	player *pb.Player
	game   *model.GameState
}

func newP11PlunderSweepActors(tx *gorm.DB, lockedIDs []int64) *p11PlunderSweepActors {
	actors := &p11PlunderSweepActors{tx: tx, locked: make(map[int64]bool, len(lockedIDs)), loaded: make(map[int64]*p11PlunderSweepActor, len(lockedIDs))}
	for _, id := range lockedIDs {
		actors.locked[id] = true
	}
	return actors
}

func (actors *p11PlunderSweepActors) require(tx *gorm.DB, ids ...int64) error {
	if actors == nil || actors.tx != tx {
		return errors.New(constUtil.P11PlunderSweepConflict)
	}
	for _, id := range ids {
		if !actors.locked[id] {
			return &errP11PlunderSweepParticipant{PlayerID: id}
		}
	}
	return nil
}

// prepareRepeat reproduces decodeProfileForUpdate's per-read recovery and its
// recovered consumption baseline. ProfileStorageBaseline remains the actual
// persisted image until saveProfile succeeds.
func (actor *p11PlunderSweepActor) prepareRepeat(tx *gorm.DB) error {
	actor.game.GuildPrayerProjection = nil
	actor.game.CopySweepCommitted = false
	actor.game.FieldCalendarDay = ""
	if ServerGameClockContextBound(tx.Statement.Context) && actor.row.Id == GameCalendarOwner(tx.Statement.Context) {
		actor.game.FieldCalendarDay = LocalPlayNow(tx.Statement.Context).In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02")
	}
	if _, err := settleResourceRecovery(actor.player, time.Now().UnixMilli()); err != nil {
		return err
	}
	player, err := json.Marshal(actor.player)
	if err != nil {
		return err
	}
	game, err := json.Marshal(actor.game)
	if err != nil {
		return err
	}
	playerBaseline, gameBaseline := string(player), string(game)
	actor.row.Player, actor.row.Game = &playerBaseline, &gameBaseline
	return nil
}

func (actors *p11PlunderSweepActors) load(id int64) (*p11PlunderSweepActor, error) {
	return actors.loadForServer(id, 0, false)
}

func (actors *p11PlunderSweepActors) loadForServer(id, serverID int64, checkServer bool) (*p11PlunderSweepActor, error) {
	if err := actors.require(actors.tx, id); err != nil {
		return nil, err
	}
	if actor := actors.loaded[id]; actor != nil {
		if checkServer && AdminProfileServerID(actor.row) != serverID {
			return nil, errors.New(constUtil.P11PlunderFightRefresh)
		}
		if err := actor.prepareRepeat(actors.tx); err != nil {
			return nil, err
		}
		return actor, nil
	}
	actor := &p11PlunderSweepActor{}
	if err := actors.tx.First(&actor.row, id).Error; err != nil {
		return nil, err
	}
	if checkServer && AdminProfileServerID(actor.row) != serverID {
		return nil, errors.New(constUtil.P11PlunderFightRefresh)
	}
	var err error
	actor.player, actor.game, err = decodeProfileForCalendarUpdate(actors.tx, &actor.row)
	if err != nil {
		return nil, err
	}
	actors.loaded[id] = actor
	return actor, nil
}

// candidate preserves the original MVCC reads for unlocked holders. Selection
// of one of them still causes a whole-attempt rollback before its lock is taken.
func (actors *p11PlunderSweepActors) candidate(id int64) (*p11PlunderSweepActor, error) {
	if actors.locked[id] {
		return actors.load(id)
	}
	actor := &p11PlunderSweepActor{}
	if err := actors.tx.First(&actor.row, id).Error; err != nil {
		return nil, err
	}
	var err error
	actor.player, actor.game, err = decodeProfileForCalendarUpdate(actors.tx, &actor.row)
	return actor, err
}

func (actors *p11PlunderSweepActors) save(actor *p11PlunderSweepActor) error {
	if actor == nil || actors.loaded[actor.row.Id] != actor {
		return errors.New(constUtil.P11PlunderSweepConflict)
	}
	if err := saveProfile(actors.tx, &actor.row, actor.player, actor.game); err != nil {
		return err
	}
	return actor.adoptSavedBaseline()
}

func (actor *p11PlunderSweepActor) adoptSavedBaseline() error {
	baseline := actor.game.ProfileStorageBaseline
	if baseline == nil {
		return errors.New(constUtil.P11PlunderSweepConflict)
	}
	actor.row.Player, actor.row.Game = &baseline.Player, &baseline.Game
	actor.row.GGOCredit, actor.row.GGOCreditMigrated = actor.game.GGOCredit, true
	if actor.game.GGOField != nil && p10SceneScope(actor.game) == constUtil.GGOFieldScene {
		_, total, err := ggoProtectedChips(actor.game.GGOField.Chips, 0)
		if err != nil {
			return err
		}
		actor.row.GGOChipTotal = &total
	} else {
		actor.row.GGOChipTotal = nil
	}
	return nil
}

// Projection has always used a separately decoded object. Copy only the
// mutable portions touched by prayer and formation preparation, keeping the
// authoritative actor untouched without decoding the whole profile again.
func (actors *p11PlunderSweepActors) projection(s *GameService, id int64) (*pb.SimplePlayerInfo, *pb.BattlePlayer, error) {
	actor := actors.loaded[id]
	if actor == nil || !actors.locked[id] {
		return nil, nil, errors.New(constUtil.P11PlunderSweepConflict)
	}
	player := proto.Clone(actor.player).(*pb.Player)
	game := p11PlunderSweepProjectionGame(actor.game)
	return s.p11ArenaProjectionState(&actor.row, player, &game, actors.tx)
}

func p11PlunderSweepProjectionGame(source *model.GameState) model.GameState {
	game := *source
	game.Formation = make([]*pb.FormationItem, len(source.Formation))
	for i, slot := range source.Formation {
		if slot != nil {
			game.Formation[i] = proto.Clone(slot).(*pb.FormationItem)
		}
	}
	if source.Field != nil {
		field := *source.Field
		if field.Side != nil {
			field.Side = proto.Clone(field.Side).(*pb.PlayerSide)
		}
		game.Field = &field
	}
	return game
}
