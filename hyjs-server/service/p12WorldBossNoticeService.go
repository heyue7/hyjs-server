package service

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

// p12WorldBossKillNotice只由共享HP首次归零的权威Finish事务构造。
// 名称来自已认证玩家快照，去掉NGUI标签括号与控制符，避免名字改变全服提示的格式。
func p12WorldBossKillNotice(playerName string) *pb.SCPublicMessage {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		switch r {
		case '[':
			return '【'
		case ']':
			return '】'
		}
		return r
	}, playerName)
	return &pb.SCPublicMessage{Pubmsg: &pb.PublicMessage{
		Type:       proto.Int32(constUtil.P12WorldBossKillNoticeType),
		ScrollType: proto.Int32(constUtil.P12WorldBossKillNoticeScrollType),
		Msg:        proto.String(fmt.Sprintf(constUtil.P12WorldBossKillNoticeFormat, name)),
	}}
}

// publishP12WorldBossKill必须在结算提交成功后调用，包含最后一击玩家及本服全部当前会话。
// publishP10Private复用实际TCP发送器并逐条重新核验session；切号的旧socket不会收到消息。
// 持久KillerID是首次死亡标记，重试和冷恢复均不重发。TCP无确认，进程在提交后发送前退出可能漏发，
// 保持至多一次即时提示，避免冷启动重播历史击杀。
func (s *GameService) publishP12WorldBossKill(ctx context.Context, message *pb.SCPublicMessage) {
	if message == nil {
		return
	}
	p10ClientSinks.Range(func(key, _ any) bool {
		client := key.(p10ClientKey)
		if client.service == s {
			s.publishP10Private(ctx, client.playerID, message)
		}
		return true
	})
}
