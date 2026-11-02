package service

import (
	"context"
	"database/sql"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
)

type p10SceneRoom struct {
	scene string
	line  int32
}

type p10SceneLockKey struct {
	database *sql.DB
	kind     uint8
	playerID int64
	room     p10SceneRoom
}

type p10SceneLockEntry struct {
	mu   sync.RWMutex
	refs int
}

var p10SceneLockRegistry = struct {
	sync.Mutex
	entries map[p10SceneLockKey]*p10SceneLockEntry
}{entries: make(map[p10SceneLockKey]*p10SceneLockEntry)}

func p10SceneDatabase() *sql.DB {
	database, err := db.GetIns().DB()
	if err != nil {
		panic(err)
	}
	return database
}

// Reserve all entries before waiting, so a lock cannot be removed while any
// holder or waiter still references it. Callers acquire players before rooms.
func p10SceneAcquire(keys []p10SceneLockKey, read bool) func() {
	entries := make([]*p10SceneLockEntry, len(keys))
	p10SceneLockRegistry.Lock()
	for i, key := range keys {
		entry := p10SceneLockRegistry.entries[key]
		if entry == nil {
			entry = &p10SceneLockEntry{}
			p10SceneLockRegistry.entries[key] = entry
		}
		entry.refs++
		entries[i] = entry
	}
	p10SceneLockRegistry.Unlock()
	for _, entry := range entries {
		if read {
			entry.mu.RLock()
		} else {
			entry.mu.Lock()
		}
	}
	return func() {
		for i := len(entries) - 1; i >= 0; i-- {
			if read {
				entries[i].mu.RUnlock()
			} else {
				entries[i].mu.Unlock()
			}
		}
		p10SceneLockRegistry.Lock()
		for i, key := range keys {
			entries[i].refs--
			if entries[i].refs == 0 {
				delete(p10SceneLockRegistry.entries, key)
			}
		}
		p10SceneLockRegistry.Unlock()
	}
}

func p10ScenePlayers(ids []int64, read bool) func() {
	ids = append([]int64(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	database := p10SceneDatabase()
	keys := make([]p10SceneLockKey, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || len(keys) > 0 && keys[len(keys)-1].playerID == id {
			continue
		}
		keys = append(keys, p10SceneLockKey{database: database, kind: 1, playerID: id})
	}
	return p10SceneAcquire(keys, read)
}

func p10ScenePlayersLock(ids ...int64) func() { return p10ScenePlayers(ids, false) }
func p10ScenePlayersRead(ids ...int64) func() { return p10ScenePlayers(ids, true) }

func p10SceneRooms(rooms ...p10SceneRoom) []p10SceneRoom {
	rooms = append([]p10SceneRoom(nil), rooms...)
	sort.Slice(rooms, func(i, j int) bool {
		if rooms[i].scene != rooms[j].scene {
			return rooms[i].scene < rooms[j].scene
		}
		return rooms[i].line < rooms[j].line
	})
	unique := rooms[:0]
	for _, room := range rooms {
		if room.scene == "" || room.line <= 0 || len(unique) > 0 && unique[len(unique)-1] == room {
			continue
		}
		unique = append(unique, room)
	}
	return unique
}

func p10SceneRoomsLock(rooms ...p10SceneRoom) func() {
	rooms = p10SceneRooms(rooms...)
	database := p10SceneDatabase()
	keys := make([]p10SceneLockKey, len(rooms))
	for i, room := range rooms {
		keys[i] = p10SceneLockKey{database: database, kind: 2, room: room}
	}
	return p10SceneAcquire(keys, false)
}

type p10SceneMembershipRetry struct{ rooms []p10SceneRoom }

func (*p10SceneMembershipRetry) Error() string {
	return "scene membership changed; retry with room locks"
}

func p10SceneRequireRooms(held []p10SceneRoom, rooms ...p10SceneRoom) error {
	needed := p10SceneRooms(rooms...)
	if len(held) == len(needed) {
		equal := true
		for i := range held {
			if held[i] != needed[i] {
				equal = false
				break
			}
		}
		if equal {
			return nil
		}
	}
	return &p10SceneMembershipRetry{rooms: needed}
}

// Scene mutation entry points buffer pinned recipients until their room locks
// have been released. Outside those entry points delivery remains unchanged.
type p10SceneNoticeKey struct{}
type p10SceneNotice struct {
	send      func(context.Context)
	queueSafe bool
}
type p10SceneNotices struct{ pending []p10SceneNotice }

func p10SceneNoticeContext(ctx context.Context) (context.Context, *p10SceneNotices) {
	notices := &p10SceneNotices{}
	return context.WithValue(ctx, p10SceneNoticeKey{}, notices), notices
}

func p10SceneDeferNotice(ctx context.Context, send func(context.Context)) bool {
	if notices, ok := ctx.Value(p10SceneNoticeKey{}).(*p10SceneNotices); ok && notices != nil {
		notices.pending = append(notices.pending, p10SceneNotice{send: send, queueSafe: true})
		return true
	}
	return false
}

// Queue-backed sockets can be ordered before releasing the actor lease. Inline
// sinks are called only after release; preserve subsequent notice order there.
func p10SceneFinishNotices(ctx context.Context, notices *p10SceneNotices, unlockPlayer func()) {
	ctx = context.WithValue(ctx, p10SceneNoticeKey{}, nil)
	locked := true
	defer func() {
		if locked {
			unlockPlayer()
		}
	}()
	deferred := len(notices.pending)
	for i, notice := range notices.pending {
		if !notice.queueSafe {
			deferred = i
			break
		}
		notice.send(ctx)
	}
	unlockPlayer()
	locked = false
	for _, notice := range notices.pending[deferred:] {
		notice.send(ctx)
	}
}

func (s *GameService) p10SceneDeferPinned(ctx context.Context, id int64, sink *p10ClientSink, message proto.Message) bool {
	if sink == nil {
		return false
	}
	notices, ok := ctx.Value(p10SceneNoticeKey{}).(*p10SceneNotices)
	if !ok || notices == nil {
		return false
	}
	message = proto.Clone(message)
	notices.pending = append(notices.pending, p10SceneNotice{
		queueSafe: sink.pushQueue != nil,
		send: func(ctx context.Context) {
			s.publishP10PrivateBatchPinned(ctx, id, sink, []proto.Message{message})
		},
	})
	return true
}
