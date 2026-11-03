package service

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p10FamousRankLists struct {
	Power, Level []*pb.SimplePlayerInfo
}

type p10FamousRankKey struct {
	Database                             *sql.DB
	ServerID, ClockRevision, ClockOffset int64
	Day                                  string
}

type p10FamousRankSnapshot struct {
	Key       p10FamousRankKey
	ExpiresAt time.Time
	Lists     *p10FamousRankLists
}

type p10FamousRankFlight struct {
	Done  chan struct{}
	Lists *p10FamousRankLists
	Err   error
}

// The mutex protects publication and flight ownership only. All SQL, decoding
// and calculations run outside it; other keys do not wait for a refresh.
type p10FamousRankCache struct {
	mu       sync.Mutex
	snapshot *p10FamousRankSnapshot
	flights  map[p10FamousRankKey]*p10FamousRankFlight
}

func (s *GameService) p10FamousCachedRanks(ctx context.Context, serverID int64, now time.Time) (*p10FamousRankLists, error) {
	database, err := db.GetIns().DB()
	if err != nil {
		return nil, err
	}
	revision, _ := ctx.Value(constUtil.ServerGameClockRevisionKey).(int64)
	key := p10FamousRankKey{Database: database, ServerID: serverID, ClockRevision: revision, ClockOffset: int64(GameCalendarOffsetDays(ctx)), Day: p1Day(now)}
	cache := &s.p10ClientOwner().famousRankCache
	cache.mu.Lock()
	if snapshot := cache.snapshot; snapshot != nil && snapshot.Key == key && time.Now().Before(snapshot.ExpiresAt) {
		cache.mu.Unlock()
		return snapshot.Lists, nil
	}
	if flight := cache.flights[key]; flight != nil {
		cache.mu.Unlock()
		select {
		case <-flight.Done:
			return flight.Lists, flight.Err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	flight := &p10FamousRankFlight{Done: make(chan struct{})}
	if cache.flights == nil {
		cache.flights = make(map[p10FamousRankKey]*p10FamousRankFlight)
	}
	cache.flights[key] = flight
	cache.mu.Unlock()
	expires := time.Now().Add(constUtil.P10FamousCacheTTL)
	var lists *p10FamousRankLists
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		lists, e = s.p10WorshipRankLists(tx, serverID, now)
		return e
	})
	cache.mu.Lock()
	flight.Lists, flight.Err = lists, err
	if err == nil {
		cache.snapshot = &p10FamousRankSnapshot{Key: key, ExpiresAt: expires, Lists: lists}
	}
	delete(cache.flights, key)
	close(flight.Done)
	cache.mu.Unlock()
	return lists, err
}

// Public rankings may lag briefly. Personal daily limits and completed worship
// facts are read separately on every request and are never put in the cache.
func (s *GameService) p10FamousRead(ctx context.Context, id int64, kind int32, now time.Time) ([]proto.Message, error) {
	if kind < 0 || kind > 1 {
		return nil, errors.New("原名人榜类型无效")
	}
	var serverID int64
	var response pb.SCFamous
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var caller db.Profile
		if err := tx.Select("id", "server_id").First(&caller, id).Error; err != nil {
			return err
		}
		serverID = AdminProfileServerID(caller)
		var policy db.P10WorshipPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		daily, err := s.p10WorshipQuote(tx, id, p1Day(now), false)
		if err != nil {
			return err
		}
		ids, err := p10WorshipActions(tx, daily)
		if err != nil {
			return err
		}
		left := int32(daily.Limit - int64(len(ids)))
		if !policy.Enabled {
			left = 0
		}
		response.LeftAddGood = proto.Int32(left)
		response.GoodList = ids
		return nil
	})
	if err != nil {
		return nil, err
	}
	lists, err := s.p10FamousCachedRanks(ctx, serverID, now)
	if err != nil {
		return nil, err
	}
	rows := lists.Power
	if kind == 1 {
		rows = lists.Level
	}
	for _, row := range rows {
		response.PlayerList = append(response.PlayerList, proto.Clone(row).(*pb.SimplePlayerInfo))
	}
	return []proto.Message{&response}, nil
}
