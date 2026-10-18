package service

import (
	"context"
	"hyjs-server/constUtil"
	"sync"

	"google.golang.org/protobuf/proto"
)

type p10ClientKey struct {
	service  *GameService
	playerID int64
}
type p10ClientSink struct {
	session []byte
	send    func(proto.Message) error
}

var p10ClientSinks sync.Map

// p10ClientOwner keeps connection identity stable while resource tables vary per request.
func (s *GameService) p10ClientOwner() *GameService {
	if s.RuntimeBase != nil {
		return s.RuntimeBase
	}
	return s
}

// RegisterP10Client 必须在原CSSocketLogin成功后调用；旧连接注销不会删除新会话。
func (s *GameService) RegisterP10Client(ctx context.Context, id int64, session []byte, send func(proto.Message) error) func() {
	key := p10ClientKey{s.p10ClientOwner(), id}
	sink := &p10ClientSink{session: append([]byte(nil), session...), send: send}
	p10ClientSinks.Store(key, sink)
	_ = s.SyncP10Scene(ctx, id, false)
	return func() {
		if p10ClientSinks.CompareAndDelete(key, sink) {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constUtil.P10SceneCleanupTimeout)
			defer cancel()
			s.DisconnectP10Scene(cleanupCtx, id, sink.session)
		}
	}
}
func (s *GameService) publishP10Private(ctx context.Context, id int64, message proto.Message) {
	key := p10ClientKey{s.p10ClientOwner(), id}
	value, ok := p10ClientSinks.Load(key)
	if !ok {
		return
	}
	sink := value.(*p10ClientSink)
	// 每次推送重新授权，重登后的旧socket没有收取后续私聊的权限。
	if _, err := AuthorizeAccount(ctx, id, sink.session); err != nil {
		if p10ClientSinks.CompareAndDelete(key, sink) {
			go func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constUtil.P10SceneCleanupTimeout)
				defer cancel()
				s.DisconnectP10Scene(cleanupCtx, id, sink.session)
			}()
		}
		return
	}
	if err := sink.send(proto.Clone(message)); err != nil {
		if p10ClientSinks.CompareAndDelete(key, sink) {
			go func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constUtil.P10SceneCleanupTimeout)
				defer cancel()
				s.DisconnectP10Scene(cleanupCtx, id, sink.session)
			}()
		}
	}
}
