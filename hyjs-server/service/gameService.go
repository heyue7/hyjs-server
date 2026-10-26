package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/elog"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"hyjs-server/utils"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// GameService 编排账号、主城和当前已恢复的副本协议。
type GameService struct {
	// RuntimeBase preserves live socket ownership across request-local table overlays.
	RuntimeBase  *GameService
	tableLookup  *gameTableLookup
	gameSchema64 atomic.Pointer[gameSchema64Snapshot]
	// ServerID 由启动配置注入；同一数据库中的本地ID归属该区服。
	ServerID int64
	Host     string
	HTTPPort int
	TCPPort  int
	Tables   map[string][]map[string]string
	// AdminCatalogueData 是启动独立加载、运行只读的原目录，不加入战斗Tables。
	AdminCatalogueData *model.AdminCatalogueData
	// ServerClockReady 表示全服时钟表及缓存已初始化。
	ServerClockReady bool
	// FieldResourcePolicyReady 表示普通野外首领奖励全局规则已持久化。
	FieldResourcePolicyReady bool
	// ServerClockState 为本服务后台任务提供并发安全的持久时钟快照。
	ServerClockState atomic.Pointer[model.ServerGameClockView]
	// adminFieldRuntimeReady 在野外管理表初始化成功后置位，正式高频路径跳过表存在检查。
	adminFieldRuntimeReady atomic.Bool
	// 已完成迁移的固定schema无需在每次属性读取时查询元数据。
	guildCoreSchemaReady     atomic.Bool
	guildPrayerSchemaReady   atomic.Bool
	guildTechSchemaReady     atomic.Bool
	adminActivitySchemaReady atomic.Bool
	// adminFieldRuntimeCache 只保存该基础服务当前区服与数据库连接的一份不可变覆盖表快照。
	adminFieldRuntimeCache atomic.Pointer[adminFieldRuntimeSnapshot]
	// 同一服务冷启动或发布后只构建一份覆盖表，避免并发请求重复分配整表。
	adminFieldRuntimeBuild sync.Mutex
	// P1Ready 仅在启动初始化全部成功后设置，运行中保持只读。
	P1Ready bool
	// P4Ready 在福利和商店规则初始化成功后启用存档事实钩子。
	P4Ready bool
	// P5Ready 在阵容与成长初始化完成后启用统一业务入口。
	P5Ready bool
	// P8GatherReady 在原采集配置和事务表初始化后启用单人资源节点。
	P8GatherReady bool
	// P8LifeReady 在生活技能、料理和生存事务表初始化后启用。
	P8LifeReady bool
	// P9PetsReady 在原精灵表和实例事务表初始化后启用。
	P9PetsReady bool
	// P9IntimacyReady is immutable after explicit compatibility rules are initialized.
	P9IntimacyReady bool
	// P10VoiceReady 仅在语音事务表初始化后路由；本地音频政策仍默认关闭。
	P10VoiceReady bool
	// P12WorldBossBattleReady 在本地首领结束回执表初始化后启用互斥检查和场景投影。
	P12WorldBossBattleReady bool
	// P12FieldPVPReady marks initialized tables; its operational policy stays off.
	P12FieldPVPReady       bool
	SharedFieldBoss55Ready bool
	FieldBossDeath73Ready  bool
	// P12WantedReady 在原表与领域事务表初始化后启用入口；发布政策独立默认关闭。
	P12WantedReady bool
	// P3AutoConfirmDemoPayments is immutable startup configuration for the local
	// demo provider. Ordinary clients cannot opt themselves into confirmation.
	P3AutoConfirmDemoPayments bool
	P3LocalWalletEnabled      bool
	LocalPlayToolsEnabled     bool
}

func gameReply(name string, fields any) ([]proto.Message, error) {
	m, err := model.GameMessage(name, fields)
	return []proto.Message{m}, err
}

// GameError 使用原客户端支持的数值错误码，避免客户端整数解析异常。
func GameError(code, text string) []proto.Message {
	return []proto.Message{&pb.ErrorMessage{Code: proto.String(code), Msg: proto.String(text)}}
}

// Dispatch 在已验证的协议请求上执行操作，只记录消息名和角色编号。
func (s *GameService) Dispatch(ctx context.Context, request *model.GamePacket, peer string) []proto.Message {
	if err := GuardP16UnavailableContent(request.Body); err != nil {
		return GameError(constUtil.ErrGameRequest, err.Error())
	}
	name := model.OpcodeNames[request.Opcode]
	started := time.Now()
	defer func() {
		if elapsed := time.Since(started); elapsed >= constUtil.GameSlowRequestThreshold {
			elog.Warn("slow game request", zap.String("message", name), zap.Int64("playerId", request.PlayerId), zap.Int64("elapsedMs", elapsed.Milliseconds()))
		}
	}()
	// Keep failures and slow requests observable without writing every movement tick.
	switch request.Body.(type) {
	case *pb.CSMove, *pb.CSMoveConfirm, *pb.CSPingTcp, *pb.CSPingAckTcp, *pb.CSSpeedCheck:
	default:
		elog.Info("game request", zap.String("message", name), zap.Int64("playerId", request.PlayerId))
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, request.Body)
	var (
		result []proto.Message
		err    error
	)
	for attempt := 0; attempt < constUtil.GameLocalMutationMaxAttempts; attempt++ {
		// 重建完整域调用及回包容器，避免旧闭包中的cost/get累加污染重试。
		result, err = s.dispatch(ctx, request, peer)
		var mysqlErr *mysqlDriver.MySQLError
		localOnly := false
		switch request.Body.(type) {
		case *pb.CSCombine, *pb.CSItemUse:
			localOnly = true
		}
		if !localOnly || !errors.As(err, &mysqlErr) || mysqlErr.Number != 1213 || attempt+1 == constUtil.GameLocalMutationMaxAttempts {
			break
		}
		timer := time.NewTimer(constUtil.GameLocalMutationRetryDelay * time.Duration(attempt+1))
		select {
		case <-ctx.Done():
			timer.Stop()
			return GameError(constUtil.ErrGameRequest, ctx.Err().Error())
		case <-timer.C:
		}
	}
	if err != nil {
		elog.Warn("game request failed", zap.String("message", name), zap.Error(err))
		return GameError(constUtil.ErrGameRequest, err.Error())
	}
	s.resumeRecruitGuide(ctx, result)
	failed := false
	for _, message := range result {
		if _, ok := message.(*pb.ErrorMessage); ok {
			failed = true
			break
		}
	}
	if !failed {
		// A committed war move already publishes its authoritative position to
		// the same field. Do not repeat the full presence transaction before
		// replying to this socket; the next queued gameplay request must proceed.
		if !p13UFMovePublished(request, result) {
			s.AfterP10SceneRequest(ctx, request.PlayerId, request.Body)
		}
		if current, ok := request.Body.(*pb.CSCurrentPosition); ok && current.GetShowArround() != 0 {
			peers, snapshotErr := s.p10CurrentScenePeers(ctx, request.PlayerId)
			if snapshotErr != nil {
				return GameError(constUtil.ErrGameRequest, snapshotErr.Error())
			}
			if len(peers) > 0 {
				// Keep the rebuilt self frame ahead of peers on the same TCP write.
				result = append(result, &pb.SCPlayerAppear{Player: peers})
			}
		}
		s.afterRedPointRequest(ctx, request.PlayerId, request.Body, result)
	}
	SanitizePlayerDisplay(result)
	return result
}

func p13UFMovePublished(request *model.GamePacket, messages []proto.Message) bool {
	if _, ok := request.Body.(*pb.CSMove); !ok {
		return false
	}
	for _, message := range messages {
		move, ok := message.(*pb.SCUFElementMove)
		if ok && len(move.Guid) == 1 && move.Guid[0] == request.PlayerId && len(move.Pos) == 1 && move.Pos[0] != nil {
			return true
		}
	}
	return false
}

func (s *GameService) dispatch(ctx context.Context, r *model.GamePacket, peer string) ([]proto.Message, error) {
	// Internal original protocol consumers share the same trusted deployment identity as Dispatch.
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	switch b := r.Body.(type) {
	case *pb.CSCheckVersion:
		return s.ClientCheckVersionReply(ctx, b.GetAppVersion(), b.GetResVersoin())
	case *pb.CSGetServerList:
		serverID := s.OriginalServerID()
		realm, err := s.runtimeGameServer(ctx)
		if err != nil {
			return nil, err
		}
		var (
			row    db.Profile
			recent []any
		)
		err = db.GetIns().WithContext(ctx).Where("uid = ?", b.GetPlatformUid()).First(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if err == nil && row.Player != nil && AdminProfileServerID(row) == serverID {
			p, _, err := decodeProfile(&row)
			if err != nil {
				return nil, err
			}
			recent = append(recent, map[string]any{"serverId": serverID, "platformUid": b.GetPlatformUid(), "playerId": row.Id, "playerName": p.GetName(), "level": p.GetLevel(), "state": "recommend", "globalServerKey": fmt.Sprintf("local-%d", serverID)})
		}
		state := "recommend"
		display := "1"
		if !realm.Enabled {
			state = "maintenance"
			display = "0"
		}
		return gameReply("SCGetServerList", map[string]any{"serverList": []any{map[string]any{"serverId": serverID, "name": realm.Name, "serverIp": fmt.Sprintf("%s:%d", s.Host, s.HTTPPort), "state": state, "globalServerKey": fmt.Sprintf("local-%d", serverID), "display": display}}, "playerServerList": recent})
	case *pb.CSUserLogin:
		if int64(r.ZoneId) != s.OriginalServerID() {
			return nil, errors.New("请求区服与当前部署不一致")
		}
		realm, err := s.runtimeGameServer(ctx)
		if err != nil {
			return nil, err
		}
		if !realm.Enabled {
			return nil, errors.New("当前区服维护中")
		}
		if err := s.CheckClientLogin(ctx, b.GetAppVersion()); err != nil {
			return nil, err
		}
		username := b.GetPlatformUid()
		event := model.SecurityEventInput{Actor: username, SourceIP: peer, Action: "game.login", Target: "account", Outcome: "failure", ReasonCode: "invalid_credentials"}
		if !LoginIngressAllowed(peer, time.Now()) {
			event.Outcome = "throttled"
			event.ReasonCode = "ingress_limit"
			ObserveSecurityEvent(ctx, event)
			return nil, errors.New("登录请求较多，请稍后重试")
		}
		if delay, e := GameLoginRetryAfter(ctx, username, time.Now()); e != nil {
			return nil, e
		} else if delay > 0 {
			event.Outcome = "throttled"
			event.ReasonCode = "account_retry"
			ObserveSecurityEvent(ctx, event)
			return nil, fmt.Errorf("登录尝试较多，请%d秒后重试", max(1, int64(delay.Seconds())+1))
		}
		row, err := LoginAccount(ctx, username, b.GetAuthCode())
		if err != nil {
			if strings.HasPrefix(err.Error(), "账号或密码错误") {
				if strings.Contains(err.Error(), "请先注册") {
					if e := utils.VerifyUnavailablePassword(ctx, b.GetAuthCode()); e != nil {
						return nil, e
					}
				}
				delay, e := RecordGameLoginFailure(ctx, username, time.Now())
				if e != nil {
					return nil, e
				}
				ObserveSecurityEvent(ctx, event)
				if delay > 0 {
					return nil, fmt.Errorf("登录尝试较多，请%d秒后重试", max(1, int64(delay.Seconds())))
				}
				return nil, errors.New("账号或密码错误")
			}
			event.ReasonCode = "service_unavailable"
			ObserveSecurityEvent(ctx, event)
			return nil, err
		}
		if e := ClearGameLoginFailures(ctx, username); e != nil {
			elog.Warn("login retry state reset unavailable")
		}
		// 在发会话前重新核对权威策略并绑定版本。
		if err := s.BindClientLogin(ctx, row, b.GetAppVersion()); err != nil {
			return nil, err
		}
		event.Outcome = "success"
		event.ReasonCode = "authenticated"
		ObserveSecurityEvent(ctx, event)
		ctx, err = s.BindLocalPlayContext(ctx, row)
		if err != nil {
			return nil, err
		}
		return gameReply("SCUserLogin", map[string]any{"sessionKey": row.Session, "playerId": row.Id, "serverTimeNow": LocalPlayNow(ctx).UnixMilli(), "isFirstCreated": row.Player == nil, "ip": s.Host, "port": s.TCPPort, "clientTime": b.GetClientTime(), "clientIp": peer})
	}
	session := r.Session
	switch b := r.Body.(type) {
	case *pb.CSSocketLogin:
		session = b.GetSessionKey()
	case *pb.CSRefreshGatewayIPAndPort:
		session = b.GetSessionKey()
	}
	row, err := AuthorizeAccount(ctx, r.PlayerId, session)
	if err != nil {
		return GameError(constUtil.ErrGameSession, "本地会话无效，请重新登录"), nil
	}
	if int64(r.ZoneId) != s.OriginalServerID() || AdminProfileServerID(*row) != s.OriginalServerID() {
		return nil, errors.New("请求区服与当前部署不一致")
	}
	if _, ok := r.Body.(*pb.CSMoveConfirm); ok {
		// Original SCMovePush ends with this empty one-way acknowledgement.
		// It must not start another scene handshake or mutate player state.
		return nil, nil
	}
	ctx, err = s.BindLocalPlayContext(ctx, row)
	if err != nil {
		return nil, err
	}
	// Transport and frozen war admission retain authentication/clock binding;
	// personal activity sources are prepared by their next gameplay consumer.
	prepareActivities := true
	switch r.Body.(type) {
	case *pb.CSMove, *pb.CSPingTcp, *pb.CSPingAckTcp, *pb.CSSpeedCheck, *pb.CSEnterBackgroud, *pb.CSUFPlayerAskFight:
		prepareActivities = false
	case *pb.CSSocketLogin, *pb.CSCurrentPosition, *pb.CSLoginScene, *pb.CSUFInit:
		inside, locationErr := p13UFProfileInField(row)
		if locationErr != nil {
			return nil, locationErr
		}
		prepareActivities = !inside
	}
	if row.Player != nil && prepareActivities {
		if err = s.PrepareAdminCharacterActivitySchedules(ctx, row.Id); err != nil {
			return nil, err
		}
	}
	runtime, err := s.fieldRuntimeForRequest(ctx, r)
	if err != nil {
		return nil, err
	}
	s, err = runtime.guildRuntimeForRequest(ctx, r)
	if err != nil {
		return nil, err
	}
	ctx = bindFieldRuntime(ctx, s)
	switch b := r.Body.(type) {
	case *pb.CSMove:
		// Reuse the authenticated snapshot to route war movement before the
		// generic field fast path. Its transaction rechecks the actual room and
		// updates the frozen actor and profile together under guild/profile locks.
		if row.Game != nil {
			var location struct {
				Position struct {
					Scene string `json:"scene"`
				} `json:"position"`
			}
			if err := json.Unmarshal([]byte(*row.Game), &location); err != nil {
				return nil, err
			}
			if location.Position.Scene == constUtil.P13UnionFightFieldTarget {
				return s.p13UFFieldMove(ctx, row.Id, b, p13UFNow(ctx))
			}
		}
		// The movement transaction already recovers resources under its role lock.
		return nil, s.fieldMove(ctx, row.Id, b)
	case *pb.CSRefreshSideHp:
		return s.formationSideHPRefresh(ctx, row.Id, b)
	case *pb.CSGenerateName:
		return gameReply("SCGenerateName", map[string]any{"playerName": fmt.Sprintf("归来的剑士%d", row.Id)})
	case *pb.CSPlayerCreate:
		row, err = CreatePlayer(ctx, row.Id, b.GetPlayerName(), b.GetHeroId())
		if err != nil {
			return nil, err
		}
		p, _, err := decodeProfile(row)
		if err != nil {
			return nil, err
		}
		return gameReply("SCPlayerCreate", map[string]any{"player": p, "createTime": row.CreatedAt})
	case *pb.CSSocketLogin:
		if err := s.CheckClientReconnect(ctx, row); err != nil {
			return nil, err
		}
		return gameReply("SCSocketLogin", map[string]any{"result": 100})
	case *pb.CSRefreshGatewayIPAndPort:
		return gameReply("SCRefreshGatewayIPAndPort", map[string]any{"ip": s.Host, "port": s.TCPPort})
	case *pb.CSPingTcp:
		reply, err := gameReply("SCPingTcp", map[string]any{"clientTime": b.GetClientTime(), "serverTime": LocalPlayNow(ctx).UnixMilli()})
		if err != nil {
			return nil, err
		}
		if row.Player == nil || row.Game == nil {
			return reply, nil
		}
		// A war heartbeat has no ordinary-field work. Its existing scene sync
		// still verifies the frozen actor, live membership and closed-world exit.
		if inside, locationErr := p13UFProfileInField(row); locationErr != nil {
			return nil, locationErr
		} else if inside {
			return reply, nil
		}
		newGain, err := s.CollectFieldBossPendingRewards(ctx, row.Id)
		if err != nil {
			return nil, err
		}
		if newGain != nil {
			reply = append(reply, &pb.SCAddRewardsToPlayer{Result: newGain})
		}
		push, err := s.fieldHeartbeat(ctx, row.Id)
		if err != nil {
			return nil, err
		}
		return append(reply, push...), nil
	case *pb.CSRefreshMovePlayerInfo:
		if reply, handled, err := s.DispatchP10Scene(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
		if s.P9PetsReady {
			return s.P9OwnMovePlayerInfo(ctx, row.Id)
		}
		return nil, nil
	case *pb.CSPingAckTcp, *pb.CSSpeedCheck:
		return nil, nil
	case *pb.CSEnterBackgroud:
		// 原 NetTickCtrl 在暂停和恢复时发送单向通知；保留会话、战斗和存档，随后由原重连流程续接。
		return nil, nil
	}
	// Preserve PK's result/side/player order before generic profile recovery.
	// The actual packet session is rechecked under ordered profile locks inside PK.
	if s.P12FieldPVPReady {
		if reply, handled, err := s.DispatchP12FieldPVP(ctx, row.Id, r.Session, r.Body, r.Timestamp); handled {
			return reply, err
		}
	}
	// Frozen war transport and admission own their locked profile checks. Avoid
	// a separate personal-resource transaction before the same domain operation.
	switch r.Body.(type) {
	case *pb.CSCurrentPosition, *pb.CSLoginScene, *pb.CSUFInit, *pb.CSUFPlayerAskFight:
		inside, locationErr := p13UFProfileInField(row)
		if locationErr != nil {
			return nil, locationErr
		}
		if inside {
			if reply, handled, warErr := s.DispatchP13UnionFight(ctx, row.Id, r.Body, r.Timestamp); handled {
				return reply, warErr
			}
		}
	}
	player, game, err := s.resourceProfile(ctx, row.Id)
	if err != nil {
		return nil, err
	}
	// 原请求进入新域后仍使用同一会话和事务边界，已有类型继续原分支。
	if reply, handled, err := s.HandlePayment(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP13Team(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	// 使用实际收到的会话字节复核录音权限，阻止首次鉴权后的并发换会话穿透。
	if s.P10VoiceReady {
		if reply, handled, err := s.DispatchP10Voice(ctx, row.Id, session, r.Body, r.Timestamp); handled {
			return reply, err
		}
	}
	if reply, handled, err := s.DispatchP13GuildTech(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP13GuildPrayer(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP13Guild(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP14Cross(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP12WorldBoss(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if s.P12WantedReady {
		if reply, handled, err := s.DispatchP12Wanted(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
	}
	if reply, handled, err := s.DispatchP11GrabRandom(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP11Arena(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP10Lucky(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP10Scene(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if s.P9PetsReady {
		if reply, handled, err := s.DispatchP9PetFragments(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
		if reply, handled, err := s.DispatchP9PetIntimacy(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
		if reply, handled, err := s.DispatchP9PetRecycle(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
		if reply, handled, err := s.DispatchP9PetEquip(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
		if reply, handled, err := s.DispatchP9PetFollow(ctx, row.Id, r.Body); handled {
			return reply, err
		}
		if reply, handled, err := s.DispatchP9PetNext(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
		if reply, handled, err := s.DispatchP9Pets(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
	}
	if s.P8LifeReady {
		if reply, handled, err := s.DispatchP8Life(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
	}
	if s.P8GatherReady {
		if reply, handled, err := s.DispatchP8Gather(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
	}
	if s.P5Ready {
		if reply, handled, err := s.DispatchP5Next(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
		if reply, handled, err := s.DispatchP5(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
	}
	if reply, handled, err := s.DispatchP7(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP10ExpeditionHelp(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP7Expedition(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP7Awake(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP10FriendEnergy(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP10Profile(ctx, row.Id, r.Body); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP10Channel(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP10Social(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if s.P4Ready {
		if reply, handled, err := s.DispatchP4(ctx, row.Id, r.Body, r.Timestamp); handled {
			return reply, err
		}
	}
	if reply, handled, err := s.DispatchP6(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	if reply, handled, err := s.DispatchP2(ctx, row.Id, r.Body, r.Timestamp); handled {
		return reply, err
	}
	switch b := r.Body.(type) {
	case *pb.CSPlayer:
		return []proto.Message{&pb.SCPlayer{Player: player}}, nil
	case *pb.CSMailList, *pb.CSCheckNewMail, *pb.CSDeleteMail:
		return s.Mailbox(ctx, row.Id, b)
	case *pb.CSQueryUserReward, *pb.CSAddRewardsToPlayer:
		// 原领奖中心查询在独立事务中补齐已结束首领的离线奖，再读取原待领列表。
		if _, query := b.(*pb.CSQueryUserReward); query {
			if err := s.SyncP12WorldBossRewards(ctx, row.Id); err != nil {
				return nil, err
			}
		}
		return s.RewardCenter(ctx, row.Id, b)
	case *pb.CSTaskList, *pb.CSGetTaskReward, *pb.CSGetTaskBoxReward:
		return s.dailyTasks(ctx, row.Id, b)
	case *pb.CSCookingLevel:
		level, exp, err := s.GemCraftLevel(ctx, row.Id)
		if err != nil {
			return nil, err
		}
		cookingLevel, cookingExp, err := s.CookingCraftLevel(ctx, row.Id)
		if err != nil {
			return nil, err
		}
		return []proto.Message{&pb.SCCookingLevel{Level: proto.Int32(cookingLevel), Exp: proto.Int32(cookingExp), GemLevel: proto.Int32(level), GemExp: proto.Int32(exp)}}, nil
	case *pb.CSFishCooking:
		return s.fishCooking(ctx, row.Id, b, r.Timestamp)
	case *pb.CSFishUsing:
		return s.fishUsing(ctx, row.Id, b, r.Timestamp)
	case *pb.CSCookingEffectList:
		return s.cookingEffectList(ctx, row.Id)
	case *pb.CSDailyCopyState, *pb.CSDailyCopyFightStart, *pb.CSDailyCopyFightEnd:
		return s.dailyCopy(ctx, row.Id, b, r.Timestamp)
	case *pb.CSMineRefine:
		return s.gemRefine(ctx, row.Id, b, r.Timestamp)
	case *pb.CSGemMake:
		return s.gemMake(ctx, row.Id, b, r.Timestamp)
	case *pb.CSGemCombineByThree:
		return s.gemCombineThree(ctx, row.Id, b, r.Timestamp)
	case *pb.CSGemCombineByTwo:
		return s.gemCombineTwo(ctx, row.Id, b, r.Timestamp)
	case *pb.CSGemCombineBySix:
		return s.gemCombineSix(ctx, row.Id, b, r.Timestamp)
	case *pb.CSGemMount:
		return s.gemMount(ctx, row.Id, b, r.Timestamp)
	case *pb.CSGemUnmount:
		return s.gemUnmount(ctx, row.Id, b, r.Timestamp)
	case *pb.CSGemMountOrChange:
		return s.gemMountOrChange(ctx, row.Id, b, r.Timestamp)
	case *pb.CSUpRoleSkillRefineLevel, *pb.CSStrengthRoleSkillRefineLevel:
		return s.skillSpecialtyUpgrade(ctx, row.Id, b, r.Timestamp)
	case *pb.CSPlayerLogin:
		if err := s.CollectFieldBossPending(ctx, row.Id); err != nil {
			return nil, err
		}
		if err := s.RecordSproutLogin(ctx, row.Id); err != nil {
			return nil, err
		}
		if err := s.SyncP12WorldBossRewards(ctx, row.Id); err != nil {
			return nil, err
		}
		if err := s.RecordActivityLogin(ctx, row.Id); err != nil {
			return nil, err
		}
		if err := s.RecordP1Login(ctx, row.Id); err != nil {
			return nil, err
		}
		if err := s.RecordP13GuildLogin(ctx, row.Id); err != nil {
			return nil, err
		}
		if err := s.RecordP4Login(ctx, row.Id); err != nil {
			return nil, err
		}
		// Original LoginPlayerLogin always fetches guild properties before
		// InitPlayerData. A dirty cached login Player would send a duplicate
		// request during ResetPlayer and silently drop CSFriendList on the
		// original single-flight HTTP transport. The durable flag is only
		// acknowledged by successful CSGetUnionAddBaseProp below.
		var fresh db.Profile
		if err := db.GetIns().WithContext(ctx).First(&fresh, row.Id).Error; err != nil {
			return nil, err
		}
		freshPlayer, _, err := decodeProfile(&fresh)
		if err != nil {
			return nil, err
		}
		loginPlayer := proto.Clone(freshPlayer).(*pb.Player)
		loginPlayer.NeedRefreshUnionProp = proto.Int32(0)
		return gameReply("SCPlayerLogin", map[string]any{"player": loginPlayer, "playerCreateTime": row.CreatedAt})
	case *pb.CSWarriorRecruitGuide:
		return s.recruitGuide(ctx, row.Id)
	case *pb.CSWarriorAwake:
		return s.warriorAwake(ctx, row.Id, b, r.Timestamp)
	case *pb.CSAwakeFuse:
		return s.warriorAwakeFuse(ctx, row.Id, b, r.Timestamp)
	case *pb.CSWarriorTraining:
		return s.warriorTraining(ctx, row.Id, b, r.Timestamp)
	case *pb.CSWarriorTrainingReplace:
		return s.warriorTrainingReplace(ctx, row.Id, b, r.Timestamp)
	case *pb.CSWarriorBreached:
		return s.warriorBreached(ctx, row.Id, b, r.Timestamp)
	case *pb.CSWarriorTalent:
		return s.p5WarriorTalent(ctx, row.Id, b, r.Timestamp)
	case *pb.CSWarriorStrengthen:
		return s.warriorStrengthen(ctx, row.Id, b)
	case *pb.CSFormationOn:
		if strings.HasPrefix(b.GetId(), "trea_") {
			return s.treasureFormationOn(ctx, row.Id, b)
		}
		return s.formationOn(ctx, row.Id, b)
	case *pb.CSFormationOff:
		if strings.HasPrefix(b.GetId(), "trea_") {
			return s.treasureFormationOff(ctx, row.Id, b)
		}
		return s.formationOff(ctx, row.Id, b)
	case *pb.CSFormationExChange:
		return s.formationExchange(ctx, row.Id, b, r.Timestamp)
	case *pb.CSFormationChange:
		return s.formationChange(ctx, row.Id, b)
	case *pb.CSGetTaticalFormation:
		return s.tacticalFormationGet(ctx, row.Id, b)
	case *pb.CSTaticalFormationList:
		return s.tacticalFormationList(ctx, row.Id)
	case *pb.CSTaticalFormationChange:
		return s.tacticalFormationChange(ctx, row.Id, b)
	case *pb.CSUseTaticalFormation:
		return s.tacticalFormationUse(ctx, row.Id, b)
	case *pb.CSTaticalFormationUnLock:
		return s.tacticalFormationUnlock(ctx, row.Id, b, r.Timestamp)
	case *pb.CSTaticalFormationLevelUp:
		return s.tacticalFormationLevelUp(ctx, row.Id, b, r.Timestamp)
	case *pb.CSItemUse:
		return s.itemUse(ctx, row.Id, b, r.Timestamp)
	case *pb.CSCombine:
		return s.treasureCombine(ctx, row.Id, b, r.Timestamp)
	case *pb.CSTreasureStrengthen:
		return s.treasureStrengthen(ctx, row.Id, b)
	case *pb.CSTreasureRefine:
		return s.treasureRefine(ctx, row.Id, b, r.Timestamp)
	case *pb.CSFragmentList:
		return s.treasureFragmentList(ctx, row.Id)
	case *pb.CSRewardMonthSignList, *pb.CSReceiveMonthSign, *pb.CSReceiveLuxurySign:
		return s.monthSign(ctx, row.Id, b, r.Timestamp)
	case *pb.CSOpenServerActivity, *pb.CSNewOpenSevenDayByTitle, *pb.CSGetOpenServerReward:
		return s.openServerActivity(ctx, row.Id, b, r.Timestamp)
	case *pb.CSOpenServerFundInfo, *pb.CSBuyOpenServerFund, *pb.CSOpenServerFundReward:
		return s.openServerFund(ctx, row.Id, b)
	case *pb.CSEquipmentTreasureList:
		return s.equipmentList(ctx, row.Id)
	case *pb.CSEquipDecompose:
		return s.equipmentDecompose(ctx, row.Id, b, r.Timestamp)
	case *pb.CSEquipForge:
		return s.equipmentForge(ctx, row.Id, b, r.Timestamp)
	case *pb.CSEquipRefine:
		return s.equipmentRefine(ctx, row.Id, b, r.Timestamp)
	case *pb.CSEquipStar:
		return s.equipmentStar(ctx, row.Id, b, r.Timestamp)
	case *pb.CSEquipStrengthen:
		return s.equipmentStrengthenStamped(ctx, row.Id, b, r.Timestamp)
	case *pb.CSActivityInfo, *pb.CSActivityReward:
		var activityID string
		switch request := b.(type) {
		case *pb.CSActivityInfo:
			activityID = request.GetActivityId()
		case *pb.CSActivityReward:
			activityID = request.GetActivityId()
		}
		if s.IsP1Activity(activityID) {
			return s.P1Activity(ctx, row.Id, b)
		}
		return s.loginActivity(ctx, row.Id, b)
	case *pb.CSActivityList:
		return s.turntableActivityList(ctx, row.Id)
	case *pb.CSBigTurnTableInfo:
		return s.bigTurnInfo(ctx, row.Id)
	case *pb.CSPlayBigTurn:
		return s.playBigTurn(ctx, row.Id, b, r.Timestamp)
	case *pb.CSBigTurnRank:
		return s.bigTurnRank(ctx, row.Id)
	case *pb.CSBigTurnRecord:
		return s.bigTurnRecord(ctx, row.Id)
	case *pb.CSBigTurnDesc:
		return s.bigTurnDesc(ctx, row.Id)
	case *pb.CSWarriorRecruitGet:
		return s.recruitInfo(ctx, row.Id, b)
	case *pb.CSWarriorRecruit:
		return s.warriorRecruit(ctx, row.Id, b, r.Timestamp)
	case *pb.CSWarriorGet:
		if err := s.skillSpecialtyPrepare(ctx, row.Id); err != nil {
			return nil, err
		}
		_, game, err = s.resourceProfile(ctx, row.Id)
		if err != nil {
			return nil, err
		}
		var warriors []*pb.Warrior
		for _, guid := range b.GetGuid() {
			for _, w := range game.Warriors {
				if w.GetGuid() == guid {
					warriors = append(warriors, w)
					break
				}
			}
		}
		return gameReply("SCWarriorGet", map[string]any{"warrior": warriors})
	case *pb.CSWarriorList:
		if err := s.skillSpecialtyPrepare(ctx, row.Id); err != nil {
			return nil, err
		}
		_, game, err = s.resourceProfile(ctx, row.Id)
		if err != nil {
			return nil, err
		}
		warriors := make([]*pb.Warrior, 0, len(game.Warriors))
		for _, owned := range game.Warriors {
			encoded, err := s.P5WarriorProtocol(owned)
			if err != nil {
				return nil, err
			}
			warriors = append(warriors, encoded)
		}
		return gameReply("SCWarriorList", map[string]any{"warriorList": warriors})
	case *pb.CSFormationList:
		return s.formationList(ctx, row.Id)
	case *pb.CSItemList:
		// 碎片由SCFragmentList单独初始化，避免原客户端重登时叠加两次。
		var items []*pb.SimpleItem
		for _, item := range game.Items {
			if !strings.HasPrefix(item.GetId(), "frag_") {
				items = append(items, item)
			}
		}
		return gameReply("SCItemList", map[string]any{"itemList": items})
	case *pb.CSQuestList:
		return gameReply("SCQuestList", map[string]any{"questList": game.Quests})
	case *pb.CSKVInfo:
		var specialty db.SkillSpecialtyRule
		if err := db.GetIns().WithContext(ctx).Take(&specialty, "id = ?", constUtil.SkillSpecialtyRuleID).Error; err != nil {
			return nil, err
		}
		specialtySwitch := "off"
		if specialty.Enabled {
			specialtySwitch = "on"
		}
		petEquipSwitch := "off"
		if s.P9PetsReady {
			petEquipSwitch = "on"
		}
		entries := make([]*pb.KeyValueEntry, 0, len(game.KV)+5)
		for _, entry := range game.KV {
			if entry.GetKey() != "taticalSwitch" && entry.GetKey() != "skillSpecialtySwitch" && entry.GetKey() != "petEquipSwitch" && (!s.P10VoiceReady || (entry.GetKey() != "audiosystem" && entry.GetKey() != "audioautoget")) {
				entries = append(entries, entry)
			}
		}
		entries = append(entries, &pb.KeyValueEntry{Key: proto.String("taticalSwitch"), Value: proto.String("on")})
		entries = append(entries, &pb.KeyValueEntry{Key: proto.String("skillSpecialtySwitch"), Value: proto.String(specialtySwitch)})
		entries = append(entries, &pb.KeyValueEntry{Key: proto.String("petEquipSwitch"), Value: proto.String(petEquipSwitch)})
		if s.P10VoiceReady {
			voiceEntries, err := s.P10VoiceKVInfo(ctx, row.Id, session)
			if err != nil {
				return nil, err
			}
			entries = append(entries, voiceEntries...)
		}
		return gameReply("SCKVInfo", map[string]any{"entry": entries})
	case *pb.CSCopyPassedInfo:
		return s.copyPassedInfo(ctx, row.Id)
	case *pb.CSCopyCurrentInfo:
		return s.copyCurrentInfo(ctx, row.Id)
	case *pb.CSGetUnionAddBaseProp:
		return gameReply("SCGetUnionAddBaseProp", map[string]any{"unionAddBaseProp": map[string]any{}, "unionPrayProp": map[string]any{}})
	case *pb.CSShopProductList:
		return s.shopProducts(ctx, row.Id, b)
	case *pb.CSShopBuy:
		return s.shopBuy(ctx, row.Id, b, r.Timestamp)
	case *pb.CSBigClientReward:
		return s.bigClientReward(ctx, row.Id)
	case *pb.CSTransferScene:
		return s.fieldTransfer(ctx, row.Id, b)
	case *pb.CSLoginScene:
		return s.fieldScene(ctx, row.Id, true)
	case *pb.CSCurrentPosition:
		return s.fieldScene(ctx, row.Id, false)
	case *pb.CSSideMonsterCheck:
		return s.fieldMonsterCheck(ctx, row.Id, b)
	case *pb.CSSideBossCheck:
		return s.fieldBossCheck(ctx, row.Id, b)
	case *pb.CSSidePveBossFightStart:
		return s.fieldBossFightStart(ctx, row.Id, b)
	case *pb.CSSidePveBossFightEnd:
		return s.fieldBossFightEnd(ctx, row.Id, b)
	case *pb.CSSidePveFightStart:
		return s.fieldFightStart(ctx, row.Id, b)
	case *pb.CSSidePveFightEnd:
		return s.fieldFightEnd(ctx, row.Id, b)
	case *pb.CSCopyEnter:
		return s.copyEnter(ctx, row.Id, b)
	case *pb.CSCopySweep:
		return s.copySweep(ctx, row.Id, b, r.Timestamp)
	case *pb.CSCopyGetStarAward:
		return s.chapterStarReward(ctx, row.Id, b)
	case *pb.CSCopyRunMapInfo:
		if game.Run == nil || game.Run.Chapter != b.GetChapterId() || game.Run.Mission != b.GetMissionId() {
			return nil, errors.New("No active mission")
		}
		party, err := s.copyContinuityParty(player, game)
		if err != nil {
			return nil, err
		}
		return gameReply("SCCopyRunMapInfo", map[string]any{"passedTaskIdList": game.Run.Passed, "battlePlayer": party})
	case *pb.CSCopyFightStart:
		return s.copyFightStart(ctx, row.Id, b)
	case *pb.CSCopyFightEnd:
		return s.copyFightEnd(ctx, row.Id, b)
	case *pb.CSCopyLeave:
		if err = updateProfileWithTx(ctx, row.Id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
			if g.Run != nil && g.Run.Fight != nil && !g.Run.Fight.Settled {
				if err := s.CancelOriginalCopyReplay(ctx, tx, row.Id, g.Run.Fight.Start); err != nil {
					return err
				}
			}
			g.Run = nil
			return nil
		}); err != nil {
			return nil, err
		}
		return gameReply("SCCopyLeave", nil)
	case *pb.CSQuestReward:
		return s.storyQuestReward(ctx, row.Id, b.GetQuestId(), r.Timestamp)
	case *pb.CSGemAndCookingList:
		return []proto.Message{&pb.SCGemAndCookingList{GemList: game.Gems, CookingList: game.Cooking}}, nil
	case *pb.CSOpenStateCheck:
		states, err := s.openServerOpenStates(ctx, row.Id)
		if err != nil {
			return nil, err
		}
		return []proto.Message{&pb.SCOpenStateCheck{States: states}}, nil
	case *pb.CSKFOpenFrightServerInfo:
		// 主城图标会用 currentTime 重新校准全局时钟；关闭阶段仍须返回真实 Unix 毫秒。
		return []proto.Message{&pb.SCKFOpenFrightServerInfo{TypeStage: proto.Int32(0), CurrentTime: proto.Int64(time.Now().UnixMilli())}}, nil
	case *pb.CSChangePlayerName:
		return s.changePlayerName(ctx, row.Id, b, r.Timestamp)
	case *pb.CSChangeNameTimes:
		return s.changeNameTimes(ctx, row.Id)
	case *pb.CSNavigationItem:
		return s.navigationItems(ctx, row.Id)
	case *pb.CSBenefitGiftList, *pb.CSBenefitGiftInfo, *pb.CSBenefitGiftReward:
		return s.benefitGiftAt(ctx, row.Id, b, r.Timestamp, LocalPlayNow(ctx))
	case *pb.CSNoticeUrl:
		return gameReply("SCNoticeUrl", map[string]any{"noticeUrl": fmt.Sprintf("http://%s:%d/notices", s.Host, s.HTTPPort)})
	case *pb.CSTitleList, *pb.CSPetList, *pb.CSFriendList, *pb.CSGetUnionInfo, *pb.CSRedPoint, *pb.CSFriendBlackList, *pb.CSKFLogin, *pb.CSChatCache, *pb.CSCurrentLuckyBagsInfo, *pb.CSLoginRewardInfo, *pb.CSUFInfoPreView, *pb.CSGetRebels:
		return gameReply("SC"+model.OpcodeNames[r.Opcode][2:], nil)
	}
	return GameError(constUtil.ErrGameUnsupported, "尚未实现："+model.OpcodeNames[r.Opcode]), nil
}
