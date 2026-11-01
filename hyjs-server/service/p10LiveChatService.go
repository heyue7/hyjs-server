package service

import (
	"context"
	"hyjs-server/constUtil"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"hyjs-server/elog"

	"google.golang.org/protobuf/proto"
)

type p10ClientKey struct {
	service  *GameService
	playerID int64
}
type p10ClientSink struct {
	session        []byte
	send           func(proto.Message) error
	close          func()
	tradeReady     atomic.Bool
	disconnectedAt atomic.Int64
	sceneMutation  sync.Mutex
	pushQueue      chan p10PushBatch
	pushDone       chan struct{}
	pushStop       sync.Once
	// 角色场景锁保护的短期公会展示缓存，随连接释放。
	sceneGuildName  string
	sceneGuildUntil time.Time
}

type p10PushBatch struct {
	ctx      context.Context
	messages []proto.Message
}

var p10ClientSinks sync.Map

// Registration and retirement serialize old cleanup before a new sink becomes
// visible. Broadcasts can hold the scene lock, so they only mark/close a failed
// sink and schedule retirement; they never wait for this mutex.
var p10ClientLifecycle sync.Mutex

type p10LifecycleEntry struct {
	mutex sync.Mutex
	users int
}

var p10ClientLifecycleEntries = make(map[p10ClientKey]*p10LifecycleEntry)

func p10LockClientLifecycle(key p10ClientKey) func() {
	p10ClientLifecycle.Lock()
	entry := p10ClientLifecycleEntries[key]
	if entry == nil {
		entry = &p10LifecycleEntry{}
		p10ClientLifecycleEntries[key] = entry
	}
	entry.users++
	p10ClientLifecycle.Unlock()
	entry.mutex.Lock()
	return func() {
		entry.mutex.Unlock()
		p10ClientLifecycle.Lock()
		entry.users--
		if entry.users == 0 {
			delete(p10ClientLifecycleEntries, key)
		}
		p10ClientLifecycle.Unlock()
	}
}

func p10LiveClient(key p10ClientKey) (any, bool) {
	value, ok := p10ClientSinks.Load(key)
	if !ok || value.(*p10ClientSink).disconnectedAt.Load() != 0 {
		return nil, false
	}
	return value, true
}

func p10MarkDisconnected(sink *p10ClientSink, closeSocket bool) {
	if sink.disconnectedAt.CompareAndSwap(0, time.Now().UnixMilli()) {
		if sink.pushDone != nil {
			sink.pushStop.Do(func() { close(sink.pushDone) })
		}
		if closeSocket && sink.close != nil {
			sink.close()
		}
	}
}

// The lifecycle mutex is held here; the dead sink stays available for retry if
// cleanup fails and is filtered from live-client lookups in the meantime.
func (s *GameService) p10CleanupClient(ctx context.Context, key p10ClientKey, sink *p10ClientSink) error {
	current, ok := p10ClientSinks.Load(key)
	if !ok || current != sink {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constUtil.P10SceneCleanupTimeout)
	defer cancel()
	if err := s.clientUpdateDisconnectedAt(cleanupCtx, key.playerID, sink.session, sink.disconnectedAt.Load()); err != nil {
		return err
	}
	if err := s.p10DisconnectScene(cleanupCtx, key.playerID, sink.session, sink.disconnectedAt.Load()); err != nil {
		return err
	}
	p10ClientSinks.CompareAndDelete(key, sink)
	return nil
}

func (s *GameService) p10RetireFailedPush(ctx context.Context, id int64, sink *p10ClientSink) {
	p10MarkDisconnected(sink, true)
	go func() {
		release := p10LockClientLifecycle(p10ClientKey{s.p10ClientOwner(), id})
		defer release()
		if err := s.p10CleanupClient(ctx, p10ClientKey{s.p10ClientOwner(), id}, sink); err != nil {
			elog.Warn("p10 failed push cleanup failed", zap.Int64("player_id", id), zap.Error(err))
		}
	}()
}

// p10ClientOwner keeps connection identity stable while resource tables vary per request.
func (s *GameService) p10ClientOwner() *GameService {
	if s.RuntimeBase != nil {
		return s.RuntimeBase
	}
	return s
}

// RegisterP10Client 必须在原CSSocketLogin成功后调用；旧连接注销不会删除新会话。
func (s *GameService) RegisterP10Client(ctx context.Context, id int64, session []byte, send func(proto.Message) error, closeConnection ...func()) func() {
	key := p10ClientKey{s.p10ClientOwner(), id}
	sink := &p10ClientSink{session: append([]byte(nil), session...), send: send}
	if len(closeConnection) > 0 {
		sink.close = closeConnection[0]
	}
	// Socket consumers provide a close callback, allowing bounded asynchronous
	// delivery and retirement on backpressure. Inline consumers stay synchronous.
	if sink.close != nil {
		sink.pushQueue = make(chan p10PushBatch, constUtil.P10PushQueueDepth)
		sink.pushDone = make(chan struct{})
	}
	release := p10LockClientLifecycle(key)
	if old, ok := p10ClientSinks.Load(key); ok {
		previous := old.(*p10ClientSink)
		p10MarkDisconnected(previous, true)
		if err := s.p10CleanupClient(ctx, key, previous); err != nil {
			release()
			elog.Warn("p10 old connection cleanup failed", zap.Int64("player_id", id), zap.Error(err))
			if sink.close != nil {
				sink.close()
			}
			return func() {}
		}
	}
	if err := s.ClientUpdateConnected(ctx, id, session); err != nil {
		release()
		elog.Warn("p10 connection status update failed", zap.Int64("player_id", id), zap.Error(err))
		if sink.close != nil {
			sink.close()
		}
		return func() {}
	}
	p10ClientSinks.Store(key, sink)
	release()
	if sink.pushQueue != nil {
		go s.p10DeliverQueuedPushes(id, sink)
	}
	// A newly connected socket cannot inherit an uncompleted trade from a
	// disconnected/replaced socket, even if its account session is unchanged.
	if s.ggoExtractionEnabled() {
		if err := s.ggoTradeCancelSession(ctx, id, session); err != nil {
			elog.Warn("ggo trade reconnect invalidation failed", zap.Error(err))
		} else {
			sink.tradeReady.Store(true)
		}
	} else {
		sink.tradeReady.Store(true)
	}
	if err := s.SyncP10Scene(ctx, id, false); err == nil {
		s.ggoCancelOfflineTimer(id)
	}
	return func() {
		p10MarkDisconnected(sink, false)
		release := p10LockClientLifecycle(key)
		defer release()
		if err := s.p10CleanupClient(ctx, key, sink); err != nil {
			elog.Warn("p10 connection cleanup failed", zap.Int64("player_id", id), zap.Error(err))
		}
	}
}
func (s *GameService) publishP10Private(ctx context.Context, id int64, message proto.Message) {
	s.publishP10PrivateBatch(ctx, id, []proto.Message{message})
}

// Each connected socket owns its ordered queue; slow sockets cannot stall other players.
func (s *GameService) publishP10PrivateBatch(ctx context.Context, id int64, messages []proto.Message) {
	if len(messages) == 0 {
		return
	}
	key := p10ClientKey{s.p10ClientOwner(), id}
	value, ok := p10LiveClient(key)
	if !ok {
		return
	}
	sink := value.(*p10ClientSink)
	s.publishP10PrivateBatchPinned(ctx, id, sink, messages)
}

// A directory caller pins its authenticated socket so a replacement connection
// cannot inherit frames selected for an earlier scene/session.
func (s *GameService) publishP10PrivateBatchPinned(ctx context.Context, id int64, sink *p10ClientSink, messages []proto.Message) {
	if sink == nil || len(messages) == 0 {
		return
	}
	key := p10ClientKey{s.p10ClientOwner(), id}
	current, ok := p10LiveClient(key)
	if !ok || current != sink {
		return
	}
	if sink.pushQueue != nil {
		batch := p10PushBatch{ctx: context.WithoutCancel(ctx), messages: make([]proto.Message, len(messages))}
		for i, message := range messages {
			batch.messages[i] = proto.Clone(message)
		}
		select {
		case sink.pushQueue <- batch:
		case <-sink.pushDone:
		default:
			s.p10RetireFailedPush(ctx, id, sink)
		}
		return
	}
	s.p10DeliverPrivateBatch(ctx, id, sink, messages)
}

func (s *GameService) p10DeliverQueuedPushes(id int64, sink *p10ClientSink) {
	for {
		select {
		case <-sink.pushDone:
			return
		case batch := <-sink.pushQueue:
			ctx, cancel := context.WithTimeout(batch.ctx, constUtil.P10SceneCleanupTimeout)
			s.p10DeliverPrivateBatch(ctx, id, sink, batch.messages)
			cancel()
		}
	}
}

func (s *GameService) p10DeliverPrivateBatch(ctx context.Context, id int64, sink *p10ClientSink, messages []proto.Message) {
	key := p10ClientKey{s.p10ClientOwner(), id}
	current, ok := p10LiveClient(key)
	if !ok || current != sink {
		return
	}
	// 每次推送重新授权，重登后的旧socket没有收取后续私聊的权限。
	if err := authorizeP10Push(ctx, id, sink.session); err != nil {
		s.p10RetireFailedPush(ctx, id, sink)
		return
	}
	current, ok = p10LiveClient(key)
	if !ok || current != sink {
		return
	}
	for _, message := range messages {
		if err := sink.send(proto.Clone(message)); err != nil {
			s.p10RetireFailedPush(ctx, id, sink)
			return
		}
	}
}
