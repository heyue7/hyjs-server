package handler

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/elog"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"hyjs-server/routes"
	"hyjs-server/service"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

var gameService *service.GameService

func init() {
	routes.RegisterPostRouter("/sao/", GameHandler)
	routes.RegisterPostRouter("/sao", GameHandler)
}

// ConfigureGame 在开放监听端口前注入已加载配置表的服务。
func ConfigureGame(s *service.GameService) { gameService = s }

// ValidateGameRequest 在 HTTP 和 TCP 的公共入口检查游戏请求字段。
func ValidateGameRequest(p *model.GamePacket) error {
	switch b := p.Body.(type) {
	case *pb.CSRefreshSideHp:
		if b.GetOldFormation() == "" || b.GetNewFormation() == "" || len(b.GetOldFormation()) > 128 || len(b.GetNewFormation()) > 128 || strings.IndexFunc(b.GetOldFormation(), unicode.IsControl) >= 0 || strings.IndexFunc(b.GetNewFormation(), unicode.IsControl) >= 0 {
			return errors.New("阵型血量刷新参数无效")
		}
	case *pb.CSGetAchievementReward:
		if p.Timestamp <= 0 || b.GetId() == "" || len(b.GetId()) > 128 || strings.TrimSpace(b.GetId()) != b.GetId() || strings.IndexFunc(b.GetId(), unicode.IsControl) >= 0 {
			return errors.New("成就领取参数无效")
		}

	case *pb.CSBigClientReward:
		if b.GetType() != constUtil.BigClientRewardType {
			return errors.New(constUtil.BigClientRewardInvalidType)
		}
	case *pb.CSQuestReward:
		if p.Timestamp < 0 || b.GetQuestId() == "" || len(b.GetQuestId()) > 64 || strings.TrimSpace(b.GetQuestId()) != b.GetQuestId() || strings.IndexFunc(b.GetQuestId(), unicode.IsControl) >= 0 {
			return errors.New("剧情任务领奖参数无效")
		}
	case *pb.CSRushRewardPreview:
		if p.Timestamp < 0 {
			return errors.New("Invalid Trial section preview timestamp")
		}
	case *pb.CSRushGetSectionReward:
		if p.Timestamp <= 0 {
			return errors.New("Invalid Trial section claim timestamp")
		}
	case *pb.CSRushMazeTrader:
		if p.Timestamp < 0 {
			return errors.New("Invalid maze trader query timestamp")
		}
	case *pb.CSRushMazeTraderBuy:
		if p.Timestamp <= 0 {
			return errors.New("Invalid maze trader purchase timestamp")
		}
	case *pb.CSCreateTeam:
		if p.Timestamp < 0 {
			return errors.New("队伍时间戳无效")
		}
	case *pb.CSQueryTeam:
		if len(b.GetTeamId()) > 64 || strings.TrimSpace(b.GetTeamId()) != b.GetTeamId() {
			return errors.New("队伍查询编号无效")
		}
	case *pb.CSJoinTeam:
		if p.Timestamp < 0 || b.GetTeamId() == "" || len(b.GetTeamId()) > 64 || strings.TrimSpace(b.GetTeamId()) != b.GetTeamId() {
			return errors.New("加入队伍编号无效")
		}
	case *pb.CSExitTeam:
		if p.Timestamp < 0 || b.GetTeamId() == "" || len(b.GetTeamId()) > 64 || strings.TrimSpace(b.GetTeamId()) != b.GetTeamId() {
			return errors.New("退出队伍编号无效")
		}
	case *pb.CSDisbandTeam:
		if p.Timestamp < 0 || b.GetTeamId() == "" || len(b.GetTeamId()) > 64 || strings.TrimSpace(b.GetTeamId()) != b.GetTeamId() {
			return errors.New("解散队伍编号无效")
		}
	case *pb.CSChangeTeamLeader:
		if p.Timestamp < 0 || b.GetTeamId() == "" || len(b.GetTeamId()) > 64 || strings.TrimSpace(b.GetTeamId()) != b.GetTeamId() || b.GetToPlayerId() < 1 || b.GetToPlayerId() == p.PlayerId {
			return errors.New("队伍转让目标无效")
		}
	case *pb.CSTeamKick:
		if p.Timestamp < 0 || b.GetTreamId() == "" || len(b.GetTreamId()) > 64 || strings.TrimSpace(b.GetTreamId()) != b.GetTreamId() || b.GetToPlayerId() < 1 || b.GetToPlayerId() == p.PlayerId {
			return errors.New("队伍踢人目标无效")
		}
	case *pb.CSCheckTeam:
		if b.GetToPlayerId() < 1 || b.GetToServerid() != p.ZoneId {
			return errors.New("本地队伍查询对象无效")
		}
	case *pb.CSTeamCreateAndInvite:
		if p.Timestamp < 0 || len(b.GetInvitePlayerIdList()) < 1 || len(b.GetInvitePlayerIdList()) > int(constUtil.P13TeamMaxSize)-1 || len(b.GetInvitePlayerIdList()) != len(b.GetInviteServerIdList()) {
			return errors.New("原组队邀请列表不配对")
		}
		seen := map[int64]bool{}
		for i, id := range b.GetInvitePlayerIdList() {
			if id < 1 || id == p.PlayerId || seen[id] || b.GetInviteServerIdList()[i] != p.ZoneId {
				return errors.New("原组队邀请对象无效")
			}
			seen[id] = true
		}
	case *pb.CSTeamInvite:
		if p.Timestamp < 0 || b.GetTeamId() == "" || len(b.GetTeamId()) > 64 || strings.TrimSpace(b.GetTeamId()) != b.GetTeamId() || len(b.GetInvitePlayerIdList()) < 1 || len(b.GetInvitePlayerIdList()) > int(constUtil.P13TeamMaxSize)-1 || len(b.GetInvitePlayerIdList()) != len(b.GetInviteServerIdList()) {
			return errors.New("原队伍邀请列表不配对")
		}
		seen := map[int64]bool{}
		for i, id := range b.GetInvitePlayerIdList() {
			if id < 1 || id == p.PlayerId || seen[id] || b.GetInviteServerIdList()[i] != p.ZoneId {
				return errors.New("原队伍邀请对象无效")
			}
			seen[id] = true
		}
	case *pb.CSUnionCreate:
		budget := 0
		for _, unit := range utf16.Encode([]rune(b.GetName())) {
			if unit <= 256 {
				budget++
			} else {
				budget += 2
			}
		}
		if p.Timestamp <= 0 || !utf8.ValidString(b.GetName()) || strings.TrimSpace(b.GetName()) == "" || budget > 12 {
			return errors.New("原公会名称或请求时间戳无效")
		}
	case *pb.CSUnionSearch:
		budget := 0
		for _, unit := range utf16.Encode([]rune(b.GetName())) {
			if unit <= 256 {
				budget++
			} else {
				budget += 2
			}
		}
		if b.GetPage() < 1 || b.GetPage() > 100000 || !utf8.ValidString(b.GetName()) || budget > 12 {
			return errors.New("公会搜索参数无效")
		}
	case *pb.CSUnionApply:
		if p.Timestamp <= 0 || b.GetUnionId() == "" || len(b.GetUnionId()) > 64 || strings.TrimSpace(b.GetUnionId()) != b.GetUnionId() {
			return errors.New("公会申请参数无效")
		}
	case *pb.CSUnionUnApply:
		if p.Timestamp <= 0 || b.GetUnionId() == "" || len(b.GetUnionId()) > 64 || strings.TrimSpace(b.GetUnionId()) != b.GetUnionId() {
			return errors.New("公会撤回参数无效")
		}
	case *pb.CSUnionAgreeApply:
		if p.Timestamp <= 0 || b.GetPlayerId() < 1 || b.GetPlayerId() == p.PlayerId {
			return errors.New("公会审批对象无效")
		}
	case *pb.CSUnionRefuseApply:
		if p.Timestamp <= 0 || b.GetPlayerId() < 1 || b.GetPlayerId() == p.PlayerId {
			return errors.New("公会拒绝对象无效")
		}
	case *pb.CSPromotionViceChairman:
		if p.Timestamp <= 0 || b.GetPlayerId() < 1 || b.GetPlayerId() == p.PlayerId {
			return errors.New("公会任职对象无效")
		}
	case *pb.CSDismissViceChairman:
		if p.Timestamp <= 0 || b.GetPlayerId() < 1 || b.GetPlayerId() == p.PlayerId {
			return errors.New("公会免职对象无效")
		}
	case *pb.CSRemoveUnionPlayer:
		if p.Timestamp <= 0 || b.GetPlayerId() < 1 || b.GetPlayerId() == p.PlayerId {
			return errors.New("公会移除对象无效")
		}
	case *pb.CSTransferChairman:
		if p.Timestamp <= 0 || b.GetPlayerId() < 1 || b.GetPlayerId() == p.PlayerId {
			return errors.New("公会转让对象无效")
		}
	case *pb.CSUnionNoticeChange:
		budget := 0
		for _, unit := range utf16.Encode([]rune(b.GetContent())) {
			if unit <= 256 {
				budget++
			} else {
				budget += 2
			}
		}
		if p.Timestamp <= 0 || !utf8.ValidString(b.GetContent()) || budget > 140 {
			return errors.New("公会公告参数无效")
		}
	case *pb.CSUnionDeclarationChange:
		budget := 0
		for _, unit := range utf16.Encode([]rune(b.GetContent())) {
			if unit <= 256 {
				budget++
			} else {
				budget += 2
			}
		}
		if p.Timestamp <= 0 || !utf8.ValidString(b.GetContent()) || budget > 80 {
			return errors.New("公会宣言参数无效")
		}
	case *pb.CSChangeUnionFlag:
		if p.Timestamp <= 0 || b.GetUnionFlag() < 0 {
			return errors.New("公会旗帜参数无效")
		}
	case *pb.CSUnionDonate:
		if p.Timestamp <= 0 || (b.GetDonateType() != "coin" && b.GetDonateType() != "badge") {
			return errors.New("原捐献类型或请求时间戳无效")
		}
	case *pb.CSUnionOpenDonateBox:
		if p.Timestamp <= 0 || len(b.GetBoxId()) != 64 {
			return errors.New("公会捐献宝箱参数无效")
		}
		for _, char := range b.GetBoxId() {
			if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
				return errors.New("公会捐献宝箱参数无效")
			}
		}
	case *pb.CSUnionOpenBuilder:
		if b.Type == nil || (b.GetType() != "HeroStatue" && b.GetType() != "GuardTree") || p.Timestamp <= 0 {
			return errors.New("原公会建设类型或请求时间戳无效")
		}
	case *pb.CSKFGuessList:
		if p.Timestamp <= 0 {
			return errors.New("原冠军竞猜列表需正HTTP时间戳")
		}
	case *pb.CSKFGuess:
		// Original81040 omits fight/item fields; owner identity is verified by
		// the domain against the authenticated local profile before any debit.
		if p.Timestamp <= 0 || b.GetOwnerPlayer() == nil || b.GetGuessPlayer() == nil || b.GetGuessType() < 1 || b.GetGuessType() > 2 || b.GetOwnerPlayer().GetPlayerId() <= 0 || b.GetGuessPlayer().GetPlayerId() <= 0 || b.GetOwnerPlayer().GetServerId() <= 0 || b.GetOwnerPlayer().GetServerId() > 32 || b.GetGuessPlayer().GetServerId() <= 0 || b.GetGuessPlayer().GetServerId() > 32 {
			return errors.New("原竞猜身份或请求时间戳无效")
		}
		if b.GetGuessType() == 1 && (b.GetBattleType() < 0 || b.GetBattleType() > 1) {
			return errors.New("原冠军竞猜当前或回顾类型无效")
		}
		if b.GetGuessType() == 2 && b.GetBattleType() != 0 && b.GetBattleType() != 7 && b.GetBattleType() != 9 && b.GetBattleType() != 11 && b.GetBattleType() != 13 && b.GetBattleType() != 15 && b.GetBattleType() != 17 {
			return errors.New("原普通竞猜备战阶段类型无效")
		}
		for _, text := range []string{b.GetOwnerPlayer().GetPlayerName(), b.GetGuessPlayer().GetPlayerName(), b.GetFrightId(), b.GetItemId()} {
			if !utf8.ValidString(text) || strings.ContainsRune(text, 0) || len(text) > 128 {
				return errors.New("原竞猜文字字段无效")
			}
		}
	case *pb.CSKFMatchFrightList:
		// Original81046 uses HTTP and group A-D; an omitted group is the
		// separate original FinalFour query, whose results stay domain-gated.
		group := strings.TrimSpace(b.GetGroupId())
		if p.Timestamp <= 0 || (group != "" && group != "A" && group != "B" && group != "C" && group != "D") || b.GetReplayFlag() < 0 || b.GetReplayFlag() > 1 {
			return errors.New("原跨服决赛分组或回顾参数无效")
		}
		if b.GroupId != nil {
			b.GroupId = proto.String(group)
		}
	case *pb.CSKFRankInfo:
		// 原晋级榜使用type2/subType0，先校验查询边界再核对赛季资格。
		if b.GetType() == 2 && (p.Timestamp <= 0 || b.GetSubType() != 0 || b.GetPeakMatchIndex() < 0) {
			return errors.New("跨服晋级榜类型或赛季参数无效")
		}
	case *pb.CSKuafuMatchPlayer:
		// 原随机匹配由HTTP81038提交，时间戳标识同一场请求的幂等重试。
		if p.Timestamp <= 0 {
			return errors.New("跨服晋级匹配时间戳无效")
		}
	case *pb.CSKFEnterGame, *pb.CSKFEnterFrightGame:
		if p.Timestamp <= 0 {
			return errors.New("跨服报名或阵容提交时间戳无效")
		}
	case *pb.CSKFTimeInfo:
		if b.GetPeakMatchIndex() < 0 {
			return errors.New("跨服赛季编号无效")
		}
	case *pb.CSKFReplay:
		war := strings.TrimSpace(b.GetWarId())
		if b.GetType() < 1 || b.GetType() > 3 || war == "" || len(war) > 128 || !utf8.ValidString(war) || strings.ContainsRune(war, '\x00') {
			return errors.New("跨服录像类型或场次无效")
		}
		if b.GetType() == 3 {
			decoded, err := hex.DecodeString(war)
			if p.Timestamp <= 0 || err != nil || len(decoded) != 32 {
				return errors.New("决赛录像场次或时间戳无效")
			}
		}
		b.WarId = proto.String(war)
	case *pb.CSKFReplayInfo:
		// 先收敛原战报查询枚举和分页，再交给赛季服务核对资格来源。
		if b.GetType() < 1 || b.GetType() > 3 || b.GetType() == 1 && (b.GetSubType() < 1 || b.GetSubType() > 2) || b.GetPageIndex() < 0 || b.GetReplayFlag() < 0 || b.GetReplayFlag() > 1 {
			return errors.New("跨服战报类型或分页参数无效")
		}
		regID := strings.TrimSpace(b.GetRegId())
		if !utf8.ValidString(regID) || len(regID) > 128 || strings.ContainsRune(regID, '\x00') {
			return errors.New("跨服战报报名编号无效")
		}
		if b.RegId != nil {
			b.RegId = proto.String(regID)
		}
		if b.GetType() == 3 {
			if p.Timestamp <= 0 {
				return errors.New("决赛录像查询时间戳无效")
			}
			switch b.GetSubType() {
			case 8, 10, 12, 14, 16, 18:
			default:
				return errors.New("决赛录像轮次无效")
			}
			parts := strings.Split(regID, "_")
			if len(parts) != 3 {
				return errors.New("决赛报名联合身份无效")
			}
			for index, value := range parts {
				limit := 64
				if index < 2 {
					limit = 32
				}
				parsed, err := strconv.ParseInt(value, 10, limit)
				if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
					return errors.New("决赛报名联合身份无效")
				}
			}
		}
	case *pb.CSKFPlayerInfo:
		if b.PlayerId == nil || b.ServerId == nil || b.GetPlayerId() < 1 || b.GetServerId() < 1 || b.GetPeakMatchIndex() < 0 || b.GetReplayFlag() < 0 || b.GetType() < 0 {
			return errors.New("跨服联合角色与查询枚举无效")
		}
	case *pb.CSFamous:
		if b.GetType() != 0 && b.GetType() != 1 {
			return errors.New("原排行榜类型无效")
		}
	case *pb.CSGood:
		if b.GetPlayerId() <= 0 || p.Timestamp <= 0 {
			return errors.New("排行榜膜拜目标或请求时间戳无效")
		}
	case *pb.CSUFRankInfo:
		weekly := b.GetRankType() == 1 && b.GetSubType() >= 1 && b.GetSubType() <= 4
		monthly := b.GetRankType() == 2 && b.GetSubType() == 0
		if (!weekly && !monthly) || p.Timestamp <= 0 {
			return errors.New("Invalid original guild rank type, group or HTTP timestamp")
		}
	case *pb.CSUFAutoApplySet:
		if b.IsAuto == nil || (b.GetIsAuto() != 0 && b.GetIsAuto() != 1) || p.Timestamp <= 0 {
			return errors.New("公会战自动宣战参数无效")
		}
	case *pb.CSUFSetAutoFight:
		if b.IsAuto == nil || (b.GetIsAuto() != 0 && b.GetIsAuto() != 1) || p.Timestamp <= 0 {
			return errors.New("公会战自动战斗参数无效")
		}
	case *pb.CSUFAdjustFirstOn:
		if b.GetOptPlayerId() < 1 || (b.GetOptType() != 0 && b.GetOptType() != 1) || p.Timestamp <= 0 {
			return errors.New("公会战首发调整对象或类型无效")
		}
	case *pb.CSUFOnHook:
		if b.GetOnHook() != 0 && b.GetOnHook() != 1 {
			return errors.New("公会战挂机状态无效")
		}
	case *pb.CSUFFightRecordReplay:
		if b.GetFightId() <= 0 {
			return errors.New("公会战录像编号无效")
		}
	case *pb.CSUFPlayerAskFight:
		if b.GetTargetGuid() <= 0 || b.GetTargetGuid() == p.PlayerId || p.Timestamp < 0 {
			return errors.New("公会战目标角色或请求时间戳无效")
		}
	case *pb.CSUFPlayerCollect:
		if b.ResGuid == nil || b.GetResGuid() <= 0 {
			return errors.New("公会战采集资源编号无效")
		}
	case *pb.CSUFPlayerUpResource:
		if p.Timestamp < 0 || (b.GetNpcId() != "2" && b.GetNpcId() != "5") {
			return errors.New("公会战回收NPC或请求时间戳无效")
		}
	case *pb.CSUFShopList:
		if b.GetNpcId() != "" && b.GetNpcId() != "3" && b.GetNpcId() != "6" {
			return errors.New("公会战商店NPC无效")
		}
	case *pb.CSUFPlayerBuyItem:
		if p.Timestamp < 0 || (b.GetNpcId() != "3" && b.GetNpcId() != "6") || b.GetShopItemId() == "" || strings.TrimSpace(b.GetShopItemId()) != b.GetShopItemId() || len(b.GetShopItemId()) > 256 {
			return errors.New("公会战商店商品、NPC或请求时间戳无效")
		}
	case *pb.CSUFPlayerUseItem:
		if p.Timestamp < 0 || b.GetItemId() == "" || len(b.GetItemId()) > 64 || strings.TrimSpace(b.GetItemId()) != b.GetItemId() || !utf8.ValidString(b.GetItemId()) || strings.ContainsRune(b.GetItemId(), '\x00') {
			return errors.New("公会战使用道具编号或请求时间戳无效")
		}
	case *pb.CSUFOpenFactoryCost:
		if b.GetFactoryId() <= 0 {
			return errors.New("公会战工厂编号无效")
		}
	case *pb.CSUFFieldCariotFactoryOpen:
		if b.GetFactoryGuid() <= 0 || (b.GetNpcId() != "1" && b.GetNpcId() != "4") || p.Timestamp < 0 {
			return errors.New("公会战工厂、NPC或请求时间戳无效")
		}
	case *pb.CSUFBeginTransfer:
		validID := false
		for index := 1; index <= 18; index++ {
			if b.GetTransferId() == fmt.Sprintf("transfer_%d", index) {
				validID = true
				break
			}
		}
		if !validID || b.GetTarget() == nil || p.Timestamp < 0 {
			return errors.New("公会战传送编号、目标或请求时间戳无效")
		}
		for _, coordinate := range []float64{float64(b.GetTarget().GetX()), float64(b.GetTarget().GetY()), float64(b.GetTarget().GetZ())} {
			if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
				return errors.New("公会战传送目标无效")
			}
		}
	case *pb.CSUFEndTransfer:
		if b.GetTarget() == nil || p.Timestamp < 0 {
			return errors.New("公会战传送目标或请求时间戳无效")
		}
		for _, coordinate := range []float64{float64(b.GetTarget().GetX()), float64(b.GetTarget().GetY()), float64(b.GetTarget().GetZ())} {
			if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
				return errors.New("公会战传送目标无效")
			}
		}
	case *pb.CSUFLeaderFocus:
		if b.GetPos() == nil {
			return errors.New("公会战集结位置缺失")
		}
		for _, coordinate := range []float64{float64(b.GetPos().GetX()), float64(b.GetPos().GetY()), float64(b.GetPos().GetZ())} {
			if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
				return errors.New("公会战集结位置无效")
			}
		}
	case *pb.CSUFRefreshElement:
		if b.GetGuid() <= 0 {
			return errors.New("公会战场景元素编号无效")
		}
	case *pb.CSUFRefreshElementArray:
		if len(b.GetGuid()) > 100 {
			return errors.New("公会战场景元素数量无效")
		}
		seen := map[int64]bool{}
		for _, guid := range b.GetGuid() {
			if guid <= 0 || seen[guid] {
				return errors.New("公会战场景元素编号需为正数且唯一")
			}
			seen[guid] = true
		}
	case *pb.CSUFSetFightItems:
		if b.GetFightIndex() < 0 || b.GetFightIndex() > 2 || len(b.GetItemIds()) > 3 || p.Timestamp <= 0 {
			return errors.New("公会战场次或携带道具数量无效")
		}
		seen := map[string]bool{}
		for _, item := range b.GetItemIds() {
			if item == "" || len(item) > 64 || strings.TrimSpace(item) != item || seen[item] {
				return errors.New("公会战携带道具需为唯一原类型")
			}
			seen[item] = true
		}
	case *pb.CSQuitUnion, *pb.CSDissolveUnion, *pb.CSUnionUpgrade, *pb.CSGetUnionStamina,
		*pb.CSUFUnionApply, *pb.CSUFPlayerApply, *pb.CSUFUpdateBattlePlayer:
		if p.Timestamp <= 0 {
			return errors.New("公会变更时间戳无效")
		}
	case *pb.CSEnterBackgroud:
		if b.GetExit() != 0 && b.GetExit() != 1 {
			return errors.New("Invalid application pause state")
		}
	case *pb.CSUserLogin:
		uid := b.GetPlatformUid()
		password := b.GetAuthCode()
		if uid == "" || uid != strings.TrimSpace(uid) || utf8.RuneCountInString(uid) > 64 {
			return errors.New("账号需要 1-64 个字符，首尾不能有空格")
		}
		if n := utf8.RuneCountInString(password); n < 6 || n > 128 {
			return errors.New("密码需要 6-128 个字符；首次登录自动创建本地账号")
		}
	case *pb.CSChangePlayerName:
		// 原C# char[]按U+0100及以下1、其它UTF16单元2计长，保留原输入身份。
		name := b.GetPlayerName()
		width := 0
		for _, unit := range utf16.Encode([]rune(name)) {
			if unit <= 256 {
				width++
			} else {
				width += 2
			}
		}
		if p.Timestamp <= 0 || !utf8.ValidString(name) || name == "" || strings.TrimSpace(name) != name || width > 12 || strings.IndexFunc(name, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 {
			return errors.New(constUtil.PlayerRenameInvalid)
		}
	case *pb.CSPlayerCreate:
		name := strings.TrimSpace(b.GetPlayerName())
		b.PlayerName = &name
		if n := utf8.RuneCountInString(name); n < 1 || n > 20 {
			return errors.New("角色名称需要 1-20 个字符")
		}
		hero := b.GetHeroId()
		if hero == "" {
			hero = "hero_043"
			b.HeroId = &hero
		}
		switch hero {
		case "hero_001", "hero_043", "hero_044", "hero_045", "hero_046", "hero_047", "hero_048", "hero_049", "hero_050":
		default:
			return errors.New("不支持的初始角色")
		}
	case *pb.CSMailList:
		switch b.GetMailType() {
		case "", "allMail", "sysMail", "fightMail", "contactMail", "unionMail":
		default:
			return errors.New("邮箱类型无效")
		}
	case *pb.CSDeleteMail:
		if b.GetType() < 0 || b.GetType() > 3 || b.GetCreateTime() <= 0 {
			return errors.New("邮件标识无效")
		}
	case *pb.CSAddRewardsToPlayer:
		if len(b.GetRewardIds()) < 1 || len(b.GetRewardIds()) > constUtil.P1RewardMaximumBatch {
			return errors.New("领奖数量无效")
		}
		seen := map[int64]bool{}
		for _, id := range b.GetRewardIds() {
			if id <= 0 || seen[id] {
				return errors.New("领奖ID无效或重复")
			}
			seen[id] = true
		}
	case *pb.CSActivityInfo:
		if b.GetActivityId() == "" || len(b.GetActivityId()) > 64 || strings.TrimSpace(b.GetActivityId()) != b.GetActivityId() {
			return errors.New("活动ID无效")
		}
	case *pb.CSActivityReward:
		if b.GetActivityId() == "" || len(b.GetActivityId()) > 64 || strings.TrimSpace(b.GetActivityId()) != b.GetActivityId() || b.GetIndex() <= 0 || (len(b.GetGetResources()) < 1 && b.GetActivityId() != constUtil.P3NativeFirstActivityID) || len(b.GetGetResources()) > constUtil.P1RewardMaximumBatch {
			return errors.New("活动领奖参数无效")
		}
		for _, resource := range b.GetGetResources() {
			if resource == nil || resource.GetId() == "" || len(resource.GetId()) > 64 || resource.GetNum() <= 0 {
				return errors.New("活动奖励资源参数无效")
			}
		}
	case *pb.CSMove:
		if b.GetPointNew() == nil {
			return errors.New("Missing scene position")
		}
		for _, v := range []float64{float64(b.GetPointNew().GetX()), float64(b.GetPointNew().GetY()), float64(b.GetPointNew().GetZ())} {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 10000 {
				return errors.New("Invalid scene position")
			}
		}
	case *pb.CSPetFragSell:
		if len(b.GetSells()) < 1 || len(b.GetSells()) > 100 || p.Timestamp <= 0 {
			return errors.New("Invalid pet fragment sale")
		}
		for _, cost := range b.GetSells() {
			if cost.GetId() == "" || len(cost.GetId()) > 64 || cost.GetGuid() != "" || cost.GetNum() <= 0 {
				return errors.New("Invalid pet fragment material")
			}
		}
	case *pb.CSPetEat:
		if b.GetGuid() == "" || len(b.GetGuid()) > 64 || len(b.GetCost()) < 1 || len(b.GetCost()) > 20 || p.Timestamp <= 0 {
			return errors.New("Invalid pet food")
		}
		for _, cost := range b.GetCost() {
			if cost.GetId() == "" || len(cost.GetId()) > 64 || cost.GetGuid() != "" || cost.GetNum() <= 0 || cost.GetNum() > 1000 {
				return errors.New("Invalid pet food material")
			}
		}
	case *pb.CSPetHudongShow:
		if b.GetGuid() == "" || len(b.GetGuid()) > 64 {
			return errors.New("Invalid pet interaction query")
		}
	case *pb.CSPetSell:
		if len(b.GetGuids()) < 1 || len(b.GetGuids()) > 100 || p.Timestamp <= 0 {
			return errors.New("Invalid pet sale")
		}
		for _, guid := range b.GetGuids() {
			if guid == "" || len(guid) > 64 || strings.TrimSpace(guid) != guid {
				return errors.New("Invalid pet sale GUID")
			}
		}
	case *pb.CSPetReset:
		if b.GetGuid() == "" || len(b.GetGuid()) > 64 || strings.TrimSpace(b.GetGuid()) != b.GetGuid() || p.Timestamp <= 0 {
			return errors.New("原精灵重置实例或时间戳无效")
		}
	case *pb.CSPetEquipStrengthOne:
		if b.GetPetGuid() == "" || len(b.GetPetGuid()) > 64 || b.GetEquipId() == "" || len(b.GetEquipId()) > 64 || (b.GetNum() != 1 && b.GetNum() != 5) || p.Timestamp <= 0 {
			return errors.New("Invalid pet equipment strength")
		}
	case *pb.CSPetEquipStrengthAll:
		if b.GetPetGuid() == "" || len(b.GetPetGuid()) > 64 || p.Timestamp <= 0 {
			return errors.New("精灵一键强化实例或请求时间戳无效")
		}
	case *pb.CSPetEquipAdvance:
		if b.GetPetGuid() == "" || len(b.GetPetGuid()) > 64 || b.GetEquipId() == "" || len(b.GetEquipId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid pet equipment advance")
		}
	case *pb.CSPetEquipAwake:
		if b.GetPetGuid() == "" || len(b.GetPetGuid()) > 64 || b.GetEquipId() == "" || len(b.GetEquipId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid pet equipment awake")
		}
	case *pb.CSGetPetEquipInfo:
		if len(b.GetPetGuid()) > 100 {
			return errors.New("Invalid pet equipment query")
		}
		for _, guid := range b.GetPetGuid() {
			if guid == "" || len(guid) > 64 || strings.TrimSpace(guid) != guid {
				return errors.New("Invalid pet equipment GUID")
			}
		}
	case *pb.CSPetLevelup:
		if b.GetGuid() == "" || len(b.GetGuid()) > 64 || len(b.GetCost()) < 1 || len(b.GetCost()) > 20 || p.Timestamp <= 0 {
			return errors.New("Invalid pet level up")
		}
		for _, cost := range b.GetCost() {
			if cost.GetId() == "" || len(cost.GetId()) > 64 || cost.GetGuid() != "" || cost.GetNum() <= 0 {
				return errors.New("Invalid pet experience material")
			}
		}
	case *pb.CSPetBreached:
		if b.GetGuid() == "" || len(b.GetGuid()) > 64 || len(b.GetCost()) != 0 || p.Timestamp <= 0 {
			return errors.New("Invalid pet breach")
		}
	case *pb.CSPetFirstRecruit:
		if p.Timestamp <= 0 {
			return errors.New("Invalid first pet recruitment")
		}
	case *pb.CSSideSetShortCut:
		if b.GetPosition() < 1 || b.GetPosition() > 2 || len(b.GetId()) > 64 || strings.TrimSpace(b.GetId()) != b.GetId() || p.Timestamp <= 0 {
			return errors.New("Invalid field shortcut")
		}
	case *pb.CSHpRecover:
		cost := b.GetCostItem()
		if cost.GetId() == "" || len(cost.GetId()) > 64 || cost.GetNum() != 1 || cost.GetGuid() != "" || p.Timestamp < 0 {
			return errors.New("Invalid field recovery")
		}
	case *pb.CSResurrection, *pb.CSTransfer2Safe:
		if p.Timestamp < 0 {
			return errors.New("Invalid field revival")
		}
	case *pb.CSFieldResourceStartGather:
		if b.GetPosId() == "" || len(b.GetPosId()) > 64 || strings.TrimSpace(b.GetPosId()) != b.GetPosId() || p.Timestamp < 0 {
			return errors.New("Invalid field resource position")
		}
	case *pb.CSSideResourceCheck:
		if b.GetPosId() == "" || len(b.GetPosId()) > 64 || strings.TrimSpace(b.GetPosId()) != b.GetPosId() {
			return errors.New("Invalid field resource check")
		}
	case *pb.CSGatherCost:
		if b.GetPosId() == "" || len(b.GetPosId()) > 64 || strings.TrimSpace(b.GetPosId()) != b.GetPosId() {
			return errors.New("Invalid gathering cost position")
		}
	case *pb.CSGuideFish, *pb.CSGuideMine:
		if p.Timestamp <= 0 {
			return errors.New("Missing original life guide HTTP timestamp")
		}
	case *pb.CSFieldGetResource:
		if p.Timestamp < 0 {
			return errors.New("Invalid gathering receipt")
		}
	case *pb.CSFormationOn:
		if b.GetId() == "" || b.GetGuid() == "" || b.GetFormatPos() < 1 || b.GetFormatPos() > 5 {
			return errors.New("Invalid formation position or resource")
		}
	case *pb.CSFormationOff:
		if b.GetId() == "" || b.GetGuid() == "" || b.GetFormatPos() < 1 || b.GetFormatPos() > 5 {
			return errors.New("Invalid formation position or resource")
		}
	case *pb.CSFormationExChange:
		if b.GetFromPos() < 1 || b.GetFromPos() > 5 || b.GetToPos() < 1 || b.GetToPos() > 5 || b.GetFromPos() == b.GetToPos() || p.Timestamp <= 0 {
			return errors.New("Invalid formation exchange")
		}
	case *pb.CSFormationChange:
		if b.GetFomationMapStr() == "" || len(b.GetFomationMapStr()) > 128 {
			return errors.New("Invalid formation layout")
		}
	case *pb.CSGetTaticalFormation:
		if b.GetId() == "" || len(b.GetId()) > 32 {
			return errors.New("Invalid tactical formation")
		}
	case *pb.CSTaticalFormationChange:
		if b.GetId() == "" || b.GetFormation() == "" || len(b.GetFormation()) > 128 {
			return errors.New("Invalid tactical layout")
		}
	case *pb.CSUseTaticalFormation:
		if b.GetId() == "" || len(b.GetId()) > 32 {
			return errors.New("Invalid tactical formation")
		}
	case *pb.CSTaticalFormationUnLock:
		if b.GetTacticalId() == "" || len(b.GetTacticalId()) > 32 || p.Timestamp <= 0 {
			return errors.New("Invalid tactical formation")
		}
	case *pb.CSTaticalFormationLevelUp:
		if b.GetTacticalId() == "" || len(b.GetTacticalId()) > 32 || p.Timestamp <= 0 {
			return errors.New("Invalid tactical upgrade")
		}
	case *pb.CSItemUse:
		item := b.GetCostItem()
		if item == nil || item.GetId() == "" || item.GetGuid() != "" || item.GetNum() <= 0 || item.GetNum() > 100 || b.GetGetItemIndex() < 0 || b.GetGetItemIndex() > 10000 || p.Timestamp <= 0 {
			return errors.New("Invalid item use")
		}
	case *pb.CSEquipRefine:
		if b.GetGuid() == "" || len(b.GetCost()) == 0 || len(b.GetCost()) > 4 || p.Timestamp <= 0 {
			return errors.New("Invalid equipment refine request")
		}
		for _, cost := range b.GetCost() {
			if cost.GetId() == "" || cost.GetGuid() != "" || cost.GetNum() <= 0 {
				return errors.New("Invalid refine material")
			}
		}
	case *pb.CSEquipStar:
		if b.GetGuid() == "" || p.Timestamp <= 0 {
			return errors.New("Invalid equipment star request")
		}
	case *pb.CSEquipStrengthen:
		if b.GetGuid() == "" || b.GetTimes() < 1 || b.GetTimes() > 5 || len(b.GetCost()) != 1 {
			return errors.New("装备强化需指定实例、1至5次及单项费用")
		}
		cost := b.GetCost()[0]
		if cost.GetId() != "coin" || cost.GetGuid() != "" || cost.GetNum() <= 0 {
			return errors.New("Invalid equipment cost")
		}
	case *pb.CSTransferScene:
		if !constUtil.P8OriginalFieldScene(b.GetTarget()) && b.GetTarget() != "town" && b.GetTarget() != "town_main" && b.GetTarget() != constUtil.P13GuildRoomTarget && b.GetTarget() != constUtil.P13UnionFightFieldTarget {
			return errors.New("Unsupported field scene")
		}
		if point := b.GetPoint(); point != nil {
			for _, v := range []float64{float64(point.GetX()), float64(point.GetY()), float64(point.GetZ())} {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return errors.New("Invalid transfer point")
				}
			}
		}
	case *pb.CSGetRebels, *pb.CSGetRebelRewardInfo:
		if p.Timestamp < 0 {
			return errors.New("Invalid original Wanted query timestamp")
		}
	case *pb.CSShareRebel:
		if b.GetRebelId() == "" || len(b.GetRebelId()) > 64 || strings.TrimSpace(b.GetRebelId()) != b.GetRebelId() || p.Timestamp < 0 {
			return errors.New("Invalid original Wanted instance")
		}
	case *pb.CSRebelFightStart:
		if b.GetRebelId() == "" || len(b.GetRebelId()) > 64 || strings.TrimSpace(b.GetRebelId()) != b.GetRebelId() || (b.GetType() != 1 && b.GetType() != 2) || p.Timestamp < 0 {
			return errors.New("Invalid original Wanted attack")
		}
	case *pb.CSRebelFightEnd:
		if b.GetFightId() == "" || len(b.GetFightId()) > 64 || strings.TrimSpace(b.GetFightId()) != b.GetFightId() || (b.GetResult() != 0 && b.GetResult() != 1) || b.GetOptRecord() == nil || b.GetBattlePlayer() != nil || p.Timestamp < 0 {
			return errors.New("Invalid original Wanted finish")
		}
	case *pb.CSGetRebelReward:
		if (b.GetRebelRewardId() != 1 && b.GetRebelRewardId() != 2) || p.Timestamp < 0 {
			return errors.New("Invalid original Wanted daily reward")
		}
	case *pb.CSSidePvpFightResult:
		// Original TCP has no stamp; omitted serverId0 means the current deployment.
		if b.GetPlayerId() <= 0 || b.GetPlayerId() == p.PlayerId || b.GetServerId() < 0 || b.GetFightType() < 0 || b.GetFightType() > 1 || p.Timestamp < 0 {
			return errors.New("Invalid original field-PK identity/type")
		}
	case *pb.CSPlayerRedeem:
		if b.GetType() != 0 && b.GetType() != 1 || p.Timestamp < 0 {
			return errors.New("Invalid original prison redemption type")
		}
	case *pb.CSSideGetPvpMV:
		if b.GetFightId() == "" || len(b.GetFightId()) > 64 {
			return errors.New("Invalid original field-PK replay identity")
		}
	case *pb.CSSideMonsterCheck:
		if b.GetUuid() == "" {
			return errors.New("Missing monster UUID")
		}
	case *pb.CSSidePveFightStart:
		if b.GetMonsterId() == "" {
			return errors.New("Missing field monster")
		}
	case *pb.CSSidePveFightEnd:
		// 原 protobuf-net 省略零值：败场 result=0、死亡 HP=0 和终末总血量缺省均合法。
		// 仍校验消息容器、非零身份、枚举和有限数值；归属/阵型/HP 上限由冻结战斗核对。
		if b.GetFightId() == "" || (b.GetResult() != 0 && b.GetResult() != 1) || b.GetBattlePlayer() == nil || b.GetBattlePlayer().GetInst() == nil || len(b.GetBattlePlayer().GetFormation()) == 0 || b.GetOptRecord() == nil {
			return errors.New("Invalid field battle result")
		}
		record := b.GetOptRecord()
		if record.GetFinalRoundNum() < 0 || record.GetFinalRoundNum() > 40 {
			return errors.New("Missing field battle record")
		}
		values := []float64{record.GetAttackerTotalLeftHp(), record.GetDefenderTotalLeftHp(), b.GetBattlePlayer().GetInst().GetAnger()}
		for _, hero := range b.GetBattlePlayer().GetFormation() {
			if hero == nil || hero.GetGuid() == "" || hero.GetId() == "" || hero.Pose == nil || hero.GetInst() == nil {
				return errors.New("Invalid field battle actor")
			}
			values = append(values, hero.GetInst().GetCurHp())
		}
		for _, v := range values {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
				return errors.New("Invalid field battle value")
			}
		}
	case *pb.CSPlayBigTurn:
		if (b.GetCount() != 1 && b.GetCount() != 10) || p.Timestamp <= 0 {
			return errors.New("Invalid turntable draw")
		}
	case *pb.CSWarriorRecruitGet:
		if len(b.GetCostType()) > 3 {
			return errors.New("Invalid recruit types")
		}
		seen := map[int32]bool{}
		for _, kind := range b.GetCostType() {
			if (kind < 1 || kind > 3) || seen[kind] {
				return errors.New("Invalid recruit type")
			}
			seen[kind] = true
		}
	case *pb.CSWarriorRecruit:
		if (b.GetCostType() < 1 || b.GetCostType() > 3) || (b.GetNumType() != 1 && b.GetNumType() != 2) || (b.GetFree() != 0 && b.GetFree() != 1) || (b.GetFree() == 1 && b.GetNumType() != 1) || p.Timestamp <= 0 {
			return errors.New("Invalid recruit request")
		}
	case *pb.CSWarriorRecruitGetPreview:
		if b.GetCostType() < 1 || b.GetCostType() > 3 {
			return errors.New("Invalid recruit preview")
		}
	case *pb.CSWarriorRecruitLuckyReward:
		if b.GetReward() == "" || len(b.GetReward()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid recruit lucky reward")
		}
	case *pb.CSWarriorFragGet:
		if len(b.GetId()) > 100 {
			return errors.New("Invalid warrior fragments query")
		}
		for _, key := range b.GetId() {
			if !strings.HasPrefix(key, "frag_hero_") || len(key) > 64 {
				return errors.New("Invalid warrior fragment id")
			}
		}
	case *pb.CSWarriorFragSell:
		if len(b.GetSell()) < 1 || len(b.GetSell()) > 100 || p.Timestamp <= 0 {
			return errors.New("Invalid warrior fragments sale")
		}
		for _, resource := range b.GetSell() {
			if resource == nil || !strings.HasPrefix(resource.GetId(), "frag_hero_") || len(resource.GetId()) > 64 || resource.GetGuid() != "" || resource.GetNum() < 1 {
				return errors.New("Invalid warrior fragment cost")
			}
		}
	case *pb.CSActivityShopInfo:
		if b.GetActivityId() == "" || len(b.GetActivityId()) > 64 {
			return errors.New("Invalid activity shop")
		}
	case *pb.CSGetActivityShopReward:
		if b.GetActivityId() == "" || len(b.GetActivityId()) > 64 || b.GetIndex() < 1 || len(b.GetCostResources()) > 100 || len(b.GetGetResources()) > 100 || p.Timestamp <= 0 {
			return errors.New("Invalid activity shop purchase")
		}
		for _, list := range [][]*pb.BaseResource{b.GetCostResources(), b.GetGetResources()} {
			for _, resource := range list {
				if resource == nil || resource.GetId() == "" || len(resource.GetId()) > 64 || resource.GetGuid() != "" || resource.GetNum() < 1 {
					return errors.New("Invalid activity shop resource")
				}
			}
		}
	case *pb.CSShopBuy:
		if (b.GetShopType() != 1 && b.GetShopType() != 2 && b.GetShopType() != 4 && b.GetShopType() != 5 && b.GetShopType() != 7 && b.GetShopType() != 8) || b.GetShopItemId() == "" || len(b.GetShopItemId()) > 256 || b.GetBuyCount() < 1 || b.GetBuyCount() > 100 || ((b.GetShopType() == 2 || b.GetShopType() == 8) && b.GetBuyCount() != 1) || p.Timestamp <= 0 {
			return errors.New("Invalid shop purchase")
		}
	case *pb.CSShopProductList:
		if b.GetShopType() != 1 && b.GetShopType() != 2 && b.GetShopType() != 4 && b.GetShopType() != 5 && b.GetShopType() != 7 && b.GetShopType() != 8 {
			return errors.New("Invalid shop catalogue")
		}
	case *pb.CSRefreshShopQuery:
		if b.GetShopType() != 3 {
			return errors.New("Invalid memory catalogue")
		}
	case *pb.CSRefreshShopRefresh:
		if b.GetShopType() != 3 || p.Timestamp <= 0 {
			return errors.New("Invalid memory refresh")
		}
	case *pb.CSRefreshShopBuy:
		if b.GetShopType() != 3 || b.GetShopItemId() == "" || len(b.GetShopItemId()) > 256 || p.Timestamp <= 0 {
			return errors.New("Invalid memory purchase")
		}
	case *pb.CSGetLoginReward:
		if len(b.GetItemId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid independent login claim")
		}
	case *pb.CSReceiveLimitMonthSign, *pb.CSStaminaGet:
		if p.Timestamp <= 0 {
			return errors.New("Missing welfare claim timestamp")
		}
	case *pb.CSFortuneCatReward:
		if (b.GetType() != 0 && b.GetType() != 1) || p.Timestamp <= 0 {
			return errors.New("Invalid fortune cat stage")
		}
	case *pb.CSConsumeReturnReward:
		if b.GetActivityId() == "" || len(b.GetActivityId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid daily consumption rebate")
		}
	case *pb.CSMysticDiscount:
		if b.GetActivityId() == "" || len(b.GetActivityId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid mystic discount")
		}
	case *pb.CSBuyActivityShop:
		if b.GetActivityId() == "" || len(b.GetActivityId()) > 64 || b.GetIndex() < 1 || b.GetNum() < 1 || b.GetNum() > 100 || len(b.GetCostResources()) < 1 || len(b.GetCostResources()) > 100 || len(b.GetGetResources()) < 1 || len(b.GetGetResources()) > 100 || p.Timestamp <= 0 {
			return errors.New("Invalid activity shop batch")
		}
		for _, list := range [][]*pb.BaseResource{b.GetCostResources(), b.GetGetResources()} {
			for _, r := range list {
				if r == nil || r.GetId() == "" || len(r.GetId()) > 64 || r.GetGuid() != "" || r.GetNum() <= 0 {
					return errors.New("Invalid activity batch resource")
				}
			}
		}
	case *pb.CSRushFightStart:
		if (b.GetRushType() != 1 && b.GetRushType() != 2) || b.GetTaskId() == "" || len(b.GetTaskId()) > 64 || strings.TrimSpace(b.GetTaskId()) != b.GetTaskId() || (b.GetRushType() == 1 && (b.GetStar() < 1 || b.GetStar() > 3)) || (b.GetRushType() == 2 && b.GetStar() != 0) || p.Timestamp <= 0 {
			return errors.New("Invalid Rush start")
		}
	case *pb.CSRushFightEnd:
		// 原RushBattleOver只发送fightId/result/optRecord，battlePlayer保持可选。
		if b.GetFightId() == "" || len(b.GetFightId()) > 64 || strings.TrimSpace(b.GetFightId()) != b.GetFightId() || (b.GetResult() != 0 && b.GetResult() != 1) || b.GetOptRecord() == nil || p.Timestamp <= 0 {
			return errors.New("Invalid Rush end")
		}
	case *pb.CSRushEliteBossBuyTime, *pb.CSRushResetChallengeCount:
		if p.Timestamp <= 0 {
			return errors.New("Missing Rush mutation timestamp")
		}
	case *pb.CSAcceptMission:
		if b.GetMissionSerial() == "" || len(b.GetMissionSerial()) > 64 || strings.TrimSpace(b.GetMissionSerial()) != b.GetMissionSerial() || len(b.GetHeroGuids()) < 1 || len(b.GetHeroGuids()) > 3 || p.Timestamp <= 0 {
			return errors.New("Invalid expedition assignment")
		}
		for _, guid := range b.GetHeroGuids() {
			if guid == "" || len(guid) > 64 || strings.TrimSpace(guid) != guid {
				return errors.New("Invalid expedition hero")
			}
		}
	case *pb.CSRefreshMissionList:
		if p.Timestamp <= 0 {
			return errors.New("Missing expedition refresh timestamp")
		}
	case *pb.CSGetFinalReward:
		if b.GetMissionSerial() == "" || len(b.GetMissionSerial()) > 64 || strings.TrimSpace(b.GetMissionSerial()) != b.GetMissionSerial() || p.Timestamp <= 0 {
			return errors.New("Invalid expedition reward")
		}
	case *pb.CSExpeditionMissionDetail:
		if b.GetMissionSerial() == "" || len(b.GetMissionSerial()) > 64 || strings.TrimSpace(b.GetMissionSerial()) != b.GetMissionSerial() {
			return errors.New("Invalid expedition detail")
		}
	case *pb.CSGetExpeditionIdleHeros:
		if b.GetMissionSerial() == "" || len(b.GetMissionSerial()) > 64 || strings.TrimSpace(b.GetMissionSerial()) != b.GetMissionSerial() {
			return errors.New("Invalid expedition available heroes")
		}
	case *pb.CSEnterAwakeCopy:
		if b.GetCopyId() == "" || len(b.GetCopyId()) > 64 || strings.TrimSpace(b.GetCopyId()) != b.GetCopyId() || len(b.GetItems()) > 20 || p.Timestamp <= 0 {
			return errors.New("Invalid awake copy entry")
		}
		for _, item := range b.GetItems() {
			if item == nil || item.GetId() == "" || len(item.GetId()) > 64 || item.GetNum() < 1 || item.GetNum() > 3 {
				return errors.New("Invalid awake carried item")
			}
		}
	case *pb.CSApplyFriend, *pb.CSAgreeApply, *pb.CSDenyApply, *pb.CSDeleteFriend, *pb.CSSendToBlackList, *pb.CSDeleteFromBlackList:
		target := b.(interface{ GetFriendId() int64 }).GetFriendId()
		if target <= 0 || target == p.PlayerId || p.Timestamp <= 0 {
			return errors.New("Invalid social target or timestamp")
		}
		if apply, ok := b.(*pb.CSApplyFriend); ok && len(apply.GetWord()) > 160 {
			return errors.New("Invalid friend application text")
		}
	case *pb.CSSearchPlayer:
		if strings.TrimSpace(b.GetIdOrName()) == "" || len(b.GetIdOrName()) > 160 {
			return errors.New("Invalid player search")
		}
	case *pb.CSHelpFriendEvent:
		if b.GetFriendId() <= 0 || b.GetFriendId() == p.PlayerId || p.Timestamp <= 0 {
			return errors.New("Invalid expedition friend or receipt timestamp")
		}
		for _, serial := range []string{b.GetMissionSerial(), b.GetEventSerial()} {
			if serial == "" || len(serial) > 128 || strings.TrimSpace(serial) != serial || !utf8.ValidString(serial) || strings.ContainsRune(serial, '\x00') {
				return errors.New("Invalid expedition mission or event serial")
			}
		}
	case *pb.CSSendMassageToFriend:
		// 原LeaveWordPage以HTTP24105发送1～40个UTF16单位；服务端保留真实重试时间戳。
		if p.Timestamp <= 0 || b.GetPlayerId() <= 0 || b.GetPlayerId() == p.PlayerId || !utf8.ValidString(b.GetContent()) || strings.TrimSpace(b.GetContent()) == "" || len(utf16.Encode([]rune(b.GetContent()))) > 40 {
			return errors.New("Invalid original friend leave-word target, text or timestamp")
		}
		for _, ch := range b.GetContent() {
			if ch < 32 && ch != '\n' {
				return errors.New("Invalid original friend leave-word control character")
			}
		}
	case *pb.CSChatTcp:
		if b.GetIsAudio() != 0 {
			kind := b.GetChatType()
			name := b.GetDownLoadUrl()
			if b.GetIsAudio() != 2 || (kind != 1 && kind != 2 && kind != 3 && kind != 5 && kind != 6) || p.Timestamp < 0 ||
				!utf8.ValidString(b.GetContent()) || strings.ContainsRune(b.GetContent(), '\x00') || len(utf16.Encode([]rune(b.GetContent()))) > 40 ||
				len(name) != 46 || !strings.HasPrefix(name, constUtil.P10VoicePrefix) || !strings.HasSuffix(name, ".wav") ||
				strings.Trim(name[10:42], "0123456789abcdef") != "" || (b.GetFileName() != "" && b.GetFileName() != name) ||
				b.GetAudioTime() < 0 || b.GetAudioTime() > 30 || b.GetFileSize() < 44 || int64(b.GetFileSize()) > constUtil.P10VoiceMaxBytes ||
				(kind == 3 && (b.GetPlayerId() <= 0 || b.GetPlayerId() == p.PlayerId)) || (kind != 3 && b.GetPlayerId() < 0) {
				return errors.New("Invalid original local audio frame")
			}
			break
		}
		// 原TCP头不带时间戳；服务端以原20秒间隔内同内容处理重传。
		if b.GetChatType() == 1 || b.GetChatType() == 5 || b.GetChatType() == constUtil.P13GuildChatType || b.GetChatType() == 6 {
			if b.GetPlayerId() < 0 || len(b.GetContent()) > 160 {
				return errors.New("Invalid public chat text")
			}
		} else if b.GetPlayerId() <= 0 || b.GetPlayerId() == p.PlayerId || len(b.GetContent()) > 160 {
			return errors.New("Invalid private chat target or text")
		}
	case *pb.CSAudioChatCheck:
		kind := b.GetChatType()
		if (kind != 1 && kind != 2 && kind != 3 && kind != 5 && kind != 6) || p.Timestamp < 0 {
			return errors.New("Invalid original audio channel")
		}
	case *pb.CSUFTalk:
		if b.GetTalkType() != 0 || strings.TrimSpace(b.GetContent()) == "" || len(b.GetContent()) > 512 || !utf8.ValidString(b.GetContent()) || strings.ContainsRune(b.GetContent(), '\x00') {
			return errors.New("Invalid original guild battle command")
		}
	case *pb.CSGiveEnergy, *pb.CSGetEnergy:
		target := b.(interface{ GetFriendId() int64 }).GetFriendId()
		if target <= 0 || target == p.PlayerId || p.Timestamp <= 0 {
			return errors.New("Invalid friend energy target or timestamp")
		}
	case *pb.CSBatchSendFriendEnergy, *pb.CSBatchGetFriendEnergy:
		if p.Timestamp <= 0 {
			return errors.New("Missing friend energy timestamp")
		}
	case *pb.CSUnionHeroStatueUpgrade, *pb.CSUnionGuardTreeUpgrade:
		kind := strings.TrimSpace(b.(interface{ GetType() string }).GetType())
		switch request := b.(type) {
		case *pb.CSUnionHeroStatueUpgrade:
			request.Type = &kind
		case *pb.CSUnionGuardTreeUpgrade:
			request.Type = &kind
		}
		switch kind {
		case "peratt", "perhp", "perpdef", "permdef", "cri", "anticri", "hit", "dodge":
		default:
			return errors.New("Invalid original guild tech type")
		}
		if p.Timestamp <= 0 {
			return errors.New("Missing original guild tech HTTP timestamp")
		}
	case *pb.CSFindPlayerInfo:
		if b.GetPlayerId() <= 0 || (b.GetServerId() != 0 && b.GetServerId() != p.ZoneId) {
			return errors.New("Invalid local profile target")
		}
	case *pb.CSPvpFight, *pb.CSPvpFightResult, *pb.CSFriendFight, *pb.CSPvpFightGuide, *pb.CSPvpFightResultGuide, *pb.CSLimitPvpFightResult:
		switch b.(type) {
		case *pb.CSFriendFight, *pb.CSPvpFightResult, *pb.CSLimitPvpFightResult:
			if p.Timestamp <= 0 {
				return errors.New("Missing original competitive fight HTTP timestamp")
			}
		}
		target := b.(interface {
			GetPlayerId() int64
			GetServerId() int32
		})
		if target.GetPlayerId() <= 0 || target.GetPlayerId() == p.PlayerId || (target.GetServerId() != 0 && target.GetServerId() != 1) {
			return errors.New("Invalid original local arena target")
		}
	case *pb.CSWorldBossPropPurchase:
		if (b.GetPropId() != "wordboss_1" && b.GetPropId() != "wordboss_2" && b.GetPropId() != "wordboss_3" && b.GetPropId() != "wordboss_4") || p.Timestamp <= 0 {
			return errors.New("Invalid original world boss prop or timestamp")
		}
	case *pb.CSWorldBossFight:
		if b.GetBossId() == "" || len(b.GetBossId()) > 64 || p.Timestamp < 0 {
			return errors.New("Invalid original world boss instance or timestamp")
		}
	case *pb.CSGrab, *pb.CSGiveUpGrab:
		index := b.(interface{ GetIndex() int32 }).GetIndex()
		if index < 1 || index > 18 || p.Timestamp <= 0 {
			return errors.New("Invalid original grab index or timestamp")
		}
	case *pb.CSBuyGrab:
		if b.GetCount() < 1 || b.GetCount() > 3 || p.Timestamp <= 0 {
			return errors.New("Invalid original grab purchase count or timestamp")
		}
	case *pb.CSGetTimeReward:
		if b.GetTime() < 1 || b.GetTime() > 3 || p.Timestamp <= 0 {
			return errors.New("Invalid original grab reward hours or timestamp")
		}
	case *pb.CSGetRandReward:
		if p.Timestamp <= 0 {
			return errors.New("Invalid original grab random reward timestamp")
		}
	case *pb.CSSweepTreasureFight:
		if p.Timestamp <= 0 || !strings.HasPrefix(b.GetFragId(), "frag_trea_") || len(b.GetFragId()) > 64 || (b.GetSweepNumber() != 1 && b.GetSweepNumber() != 5) || b.GetAutoAddEnergy() < 0 || b.GetAutoAddEnergy() > 1 {
			return errors.New("Invalid original plunder sweep fragment/count/medicine/timestamp")
		}
	case *pb.CSTreasureBattleList:
		if !strings.HasPrefix(b.GetFragTreasId(), "frag_trea_") || len(b.GetFragTreasId()) > 64 {
			return errors.New("Invalid original plunder fragment")
		}
	case *pb.CSFightTreasureStartResult:
		if p.Timestamp <= 0 || b.GetFightPlayerId() <= 0 || !strings.HasPrefix(b.GetTargetFlagId(), "frag_trea_") || len(b.GetTargetFlagId()) > 64 {
			return errors.New("Invalid original plunder target or timestamp")
		}
	case *pb.CSUseAvoidPlunder:
		if (b.GetUsedItem() != "item_014" && b.GetUsedItem() != "item_015") || p.Timestamp <= 0 {
			return errors.New("Invalid original avoid plunder item or timestamp")
		}
	case *pb.CSGetLimitPvpRewardInfo:
		key, err := strconv.ParseInt(b.GetItemId(), 10, 32)
		if err != nil || key < 1 || key > 11 || b.GetItemId() != strconv.FormatInt(key, 10) || p.Timestamp <= 0 {
			return errors.New("Invalid original limit arena score reward key or timestamp")
		}
	case *pb.CSBuyLimitPvpTimes:
		if b.GetNum() < 1 || b.GetNum() > 10 || p.Timestamp <= 0 {
			return errors.New("Invalid original limit arena purchase count or timestamp")
		}
	case *pb.CSPvpFightEnd, *pb.CSPvpFightEndGuide:
		claim := b.(interface {
			GetFightId() string
			GetWinPlayerId() int64
		})
		if claim.GetFightId() == "" || len(claim.GetFightId()) > 64 || claim.GetWinPlayerId() < 0 {
			return errors.New("Invalid original arena end")
		}
	case *pb.CSThrowLuckyBags:
		if b.GetNum() < 1 || b.GetNum() > 99 || len(b.GetPosArray()) != int(b.GetNum()) {
			return errors.New("Invalid original lucky bag number or positions")
		}
		for _, point := range b.GetPosArray() {
			if point == nil || math.IsNaN(float64(point.GetX())) || math.IsInf(float64(point.GetX()), 0) || math.IsNaN(float64(point.GetY())) || math.IsInf(float64(point.GetY()), 0) || math.IsNaN(float64(point.GetZ())) || math.IsInf(float64(point.GetZ()), 0) {
				return errors.New("Invalid original lucky bag position")
			}
		}
	case *pb.CSSnatchOneLuckyBag:
		if b.GetGuid() == "" || len(b.GetGuid()) > 64 {
			return errors.New("Invalid original lucky bag guid")
		}
	case *pb.CSTransferLine:
		// 原 SendSocket 28字节头没有HTTP时间戳，TCP请求的此字段自然为0。
		if b.GetLine() < 1 || b.GetLine() > 32 || p.Timestamp < 0 {
			return errors.New("Invalid local scene line")
		}
	case *pb.CSMovePlayerInfo:
		if len(b.GetPlayerIds()) > 1000 {
			return errors.New("Too many scene player ids")
		}
		for _, id := range b.GetPlayerIds() {
			if id <= 0 {
				return errors.New("Invalid scene player id")
			}
		}
	case *pb.CSGetPaymentInfo:
		if b.GetChannelId() == "" || len(b.GetChannelId()) > 64 || strings.TrimSpace(b.GetChannelId()) != b.GetChannelId() || b.GetShopId() != "common" {
			return errors.New("Invalid payment catalogue")
		}
	case *pb.CSGenerateOrder:
		if p.Timestamp <= 0 || b.GetPaymentId() == "" || len(b.GetPaymentId()) > 128 || len(b.GetChannelId()) < 1 || len(b.GetChannelId()) > 64 || b.GetShopId() != "common" || b.GetMoneyType() != "RMB" || math.IsNaN(b.GetMoney()) || math.IsInf(b.GetMoney(), 0) || b.GetMoney() <= 0 || b.GetMoney() > 100000 || len(b.GetDeviceId()) > 256 || len(b.GetCustomStr()) > 1024 || len(b.GetAppVersion()) > 64 || len(b.GetClientIp()) > 128 {
			return errors.New("Invalid local payment order")
		}
	case *pb.CSCheckOrder:
		if !strings.HasPrefix(b.GetOrderId(), "p3-") || len(b.GetOrderId()) > 64 || b.GetPaymentId() == "" || len(b.GetPaymentId()) > 128 {
			return errors.New("Invalid local order query")
		}
	case *pb.CSFailOrder:
		if !strings.HasPrefix(b.GetOrderId(), "p3-") || len(b.GetOrderId()) > 64 || b.GetPaymentId() == "" || len(b.GetPaymentId()) > 128 {
			return errors.New("Invalid local order cancellation")
		}
	case *pb.CSClaimCard:
		if b.GetCardType() != "month" && b.GetCardType() != "permanent" {
			return errors.New("Invalid membership card")
		}
	case *pb.CSGetVipWeekBoonReward:
		if b.GetIndex() != 1 || len(b.GetCostResources()) < 1 || len(b.GetCostResources()) > 8 || len(b.GetGetResources()) < 1 || len(b.GetGetResources()) > 8 {
			return errors.New("Invalid VIP weekly purchase")
		}
		for _, list := range [][]*pb.BaseResource{b.GetCostResources(), b.GetGetResources()} {
			for _, r := range list {
				if r == nil || r.GetId() == "" || len(r.GetId()) > 64 || r.GetNum() <= 0 || r.GetGuid() != "" {
					return errors.New("Invalid VIP resource")
				}
			}
		}
	case *pb.CSContinuedRechargeInfo:
		if b.GetActivityId() == "" || len(b.GetActivityId()) > 64 {
			return errors.New("Invalid continued payment instance")
		}
	case *pb.CSContinuedRechargeReward:
		if b.GetActivityId() == "" || len(b.GetActivityId()) > 64 || (b.GetType() != 0 && b.GetType() != 1) || (b.GetType() == 0 && (b.GetIndex() < 1 || b.GetIndex() > 3)) || (b.GetType() == 1 && b.GetIndex() != -1) || len(b.GetGetResources()) < 1 || len(b.GetGetResources()) > 100 {
			return errors.New("Invalid continued payment reward")
		}
		for _, r := range b.GetGetResources() {
			if r == nil || r.GetId() == "" || len(r.GetId()) > 64 || r.GetNum() <= 0 || r.GetGuid() != "" {
				return errors.New("Invalid continued payment resource")
			}
		}
	case *pb.CSOpenLuxuryEquipment:
		if b.GetChapter() < 1 || b.GetChapter() > 100 || b.GetMission() < 1 || b.GetMission() > 1000 {
			return errors.New("Invalid luxury offer trigger")
		}
	case *pb.CSConvertGiftCode:
		if b.GetGiftCode() == "" || len(b.GetGiftCode()) > 128 || strings.TrimSpace(b.GetGiftCode()) != b.GetGiftCode() || strings.ContainsAny(b.GetGiftCode(), "\r\n\x00") {
			return errors.New("Invalid gift code")
		}
	case *pb.CSWarriorTraining:
		if b.GetGuid() == "" || len(b.GetGuid()) > 128 || b.GetCostType() < 1 || b.GetCostType() > 3 || b.GetNumType() < 1 || b.GetNumType() > 3 || p.Timestamp <= 0 {
			return errors.New("Invalid warrior training")
		}
	case *pb.CSWarriorTrainingReplace:
		if b.GetGuid() == "" || len(b.GetGuid()) > 128 || (b.GetResult() != 0 && b.GetResult() != 1) || p.Timestamp <= 0 {
			return errors.New("Invalid training confirmation")
		}
	case *pb.CSWarriorBreached:
		if b.GetGuid() == "" || len(b.GetGuid()) > 128 || (b.GetType() != 1 && b.GetType() != 2) || p.Timestamp <= 0 {
			return errors.New("Invalid warrior breach")
		}
	case *pb.CSCombine:
		if b.GetTargetId() == "" || len(b.GetTargetId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid combine target")
		}
	case *pb.CSTreasureRefine:
		if b.GetTargetId() == "" || len(b.GetTargetId()) > 128 || p.Timestamp <= 0 {
			return errors.New("Invalid treasure refine")
		}
	case *pb.CSTreasureStrengthen:
		if b.GetTargetId() == "" || len(b.GetTargetId()) > 128 || len(b.GetGuids()) < 1 || len(b.GetGuids()) > 100 {
			return errors.New("Invalid treasure strengthen")
		}
		for _, guid := range b.GetGuids() {
			if guid == "" || len(guid) > 128 {
				return errors.New("Invalid treasure material")
			}
		}
	case *pb.CSWarriorTalent:
		if b.GetGuid() == "" || b.GetTalent() == "" || len(b.GetCost()) > 1024 {
			return errors.New("Invalid talent request")
		}
		for _, resource := range b.GetCost() {
			if resource.GetId() == "" || resource.GetNum() < 0 || (resource.GetGuid() == "" && resource.GetNum() == 0) {
				return errors.New("Invalid talent cost")
			}
		}
	case *pb.CSWarriorStrengthen:
		// 原选择页会提交整批材料；实际数量与归属由角色存档校验，传输沿用MaxPacket边界。
		if b.GetGuid() == "" || len(b.GetCost()) < 1 {
			return errors.New("Invalid strengthen request")
		}
		for _, resource := range b.GetCost() {
			if resource.GetId() == "" || resource.GetGuid() == "" || (resource.GetNum() != 0 && resource.GetNum() != 1) {
				return errors.New("Invalid strengthen material")
			}
		}
	case *pb.CSFightPlayback:
		if b.GetFightId() == "" || len(b.GetFightId()) > 128 || (b.GetServerId() != 0 && b.GetServerId() != 1) {
			return errors.New("Invalid battle replay")
		}
	case *pb.CSCopyFightEnd:
		if b.GetFightId() == "" || b.GetBattlePlayer() == nil || (b.GetResult() != 0 && b.GetResult() != 1) {
			return errors.New("Invalid fight result")
		}
	case *pb.CSCopySweep:
		if b.GetChapterId() == "" || len(b.GetChapterId()) > 32 || b.GetMissionId() == "" || len(b.GetMissionId()) > 32 || p.Timestamp <= 0 {
			return errors.New("Invalid copy sweep")
		}
	case *pb.CSGetTaskReward:
		if b.GetTaskId() == "" || len(b.GetTaskId()) > 128 {
			return errors.New("Invalid daily task")
		}
	case *pb.CSUpRoleSkillRefineLevel:
		if b.GetRoleGuid() == "" || len(b.GetRoleGuid()) > 128 || b.GetType() < 1 || b.GetType() > 5 || p.Timestamp <= 0 {
			return errors.New("Invalid skill specialty upgrade")
		}
	case *pb.CSStrengthRoleSkillRefineLevel:
		if b.GetRoleGuid() == "" || len(b.GetRoleGuid()) > 128 || b.GetType() < 1 || b.GetType() > 5 || p.Timestamp <= 0 {
			return errors.New("Invalid skill specialty strength")
		}
	case *pb.CSDailyCopyFightStart:
		if b.GetChapterId() == "" || len(b.GetChapterId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid daily copy chapter")
		}
	case *pb.CSDailyCopyFightEnd:
		record := b.GetOptRecord()
		if b.GetChapterId() == "" || len(b.GetChapterId()) > 64 || b.GetFightId() == "" || len(b.GetFightId()) > 128 || (b.GetIsWin() != 0 && b.GetIsWin() != 1) || record == nil || p.Timestamp <= 0 {
			return errors.New("Invalid daily copy result")
		}
		// 原protobuf-net省略值为零的HP；保留默认零语义及正回合边界。
		if record.GetFinalRoundNum() < 1 || record.GetFinalRoundNum() > 40 {
			return errors.New("Invalid daily copy record")
		}
		for _, hp := range []float64{record.GetAttackerTotalLeftHp(), record.GetDefenderTotalLeftHp()} {
			if math.IsNaN(hp) || math.IsInf(hp, 0) || hp < 0 {
				return errors.New("Invalid daily copy remaining HP")
			}
		}
	case *pb.CSFishCooking:
		fish := b.GetFish()
		if fish == nil || fish.GetId() == "" || len(fish.GetId()) > 64 || fish.GetNum() < 1 || fish.GetNum() > 1000 || fish.GetGuid() != "" || p.Timestamp <= 0 {
			return errors.New("Invalid cooking request")
		}
	case *pb.CSFishUsing:
		item := b.GetCostItem()
		if item == nil || item.GetId() == "" || len(item.GetId()) > 64 || item.GetNum() < 1 || item.GetNum() > 1000 || item.GetGuid() != "" || len(b.GetHeroGuid()) > 128 || p.Timestamp <= 0 {
			return errors.New("Invalid cooking use")
		}
	case *pb.CSGetTaskBoxReward:
		if b.GetBoxId() == "" || len(b.GetBoxId()) > 128 {
			return errors.New("Invalid daily task box")
		}
	case *pb.CSMineRefine:
		if b.GetMineId() == "" || len(b.GetMineId()) > 64 || b.GetNum() < 5 || b.GetNum() > constUtil.GemMaxBatch || b.GetNum()%5 != 0 || p.Timestamp <= 0 {
			return errors.New("Invalid mine refinement")
		}
	case *pb.CSGemMake:
		if b.GetGemId() == "" || len(b.GetGemId()) > 64 || b.GetNum() < 1 || b.GetNum() > constUtil.GemMaxBatch || p.Timestamp <= 0 {
			return errors.New("Invalid gem making")
		}
	case *pb.CSGemMount:
		if b.GetGuid() == "" || len(b.GetGuid()) > 128 || b.GetGemId() == "" || len(b.GetGemId()) > 64 || b.GetPos() < 0 || b.GetPos() > 3 || p.Timestamp <= 0 {
			return errors.New("Invalid gem mount")
		}
	case *pb.CSGemUnmount:
		if b.GetGuid() == "" || len(b.GetGuid()) > 128 || b.GetPos() < 0 || b.GetPos() > 3 || p.Timestamp <= 0 {
			return errors.New("Invalid gem unmount")
		}
	case *pb.CSGemMountOrChange:
		if b.GetGuid() == "" || len(b.GetGuid()) > 128 || b.GetGemId() == "" || len(b.GetGemId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid gem change")
		}
	case *pb.CSGemCombineByThree:
		if b.GetGemId() == "" || len(b.GetGemId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid gem combine")
		}
	case *pb.CSGemCombineBySix:
		if b.GetGemId() == "" || len(b.GetGemId()) > 64 || p.Timestamp <= 0 {
			return errors.New("Invalid gem combine")
		}
	case *pb.CSGemCombineByTwo:
		if len(b.GetGemId()) != 3 || p.Timestamp <= 0 {
			return errors.New("Invalid gem conversion")
		}
		for _, key := range b.GetGemId() {
			if key == "" || len(key) > 64 {
				return errors.New("Invalid gem conversion resource")
			}
		}
	case *pb.CSCopyGetStarAward:
		if b.GetChapterId() == "" || len(b.GetChapterId()) > 32 || b.GetStarIndex() < 0 || b.GetStarIndex() > 2 {
			return errors.New("Invalid chapter reward")
		}
	}
	return nil
}

// GameHandler 校验原始二进制入参并返回兼容客户端的消息帧。
// @Summary 游戏协议入口
// @Description 68 字节大端包头后接原游戏 Protobuf，响应可连续携带多条消息。
// @Tags game
// @Accept octet-stream
// @Produce octet-stream
// @Param packet body string true "原客户端二进制请求帧"
// @Success 200 {string} binary "游戏响应帧"
// @Router /sao/ [post]
func GameHandler(c *gin.Context) {
	if c.Request.ContentLength < 68 || c.Request.ContentLength > constUtil.MaxPacket || len(c.Request.TransferEncoding) > 0 {
		writeGameError(c, http.StatusBadRequest, constUtil.ErrGamePacket, "Invalid Content-Length")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, constUtil.MaxPacket))
	if err != nil {
		writeGameError(c, http.StatusBadRequest, constUtil.ErrGamePacket, "Invalid packet body")
		return
	}
	p, err := model.ParseGamePacket(raw, false)
	if err != nil {
		writeGameError(c, http.StatusBadRequest, constUtil.ErrGamePacket, err.Error())
		return
	}
	if err = ValidateGameRequest(p); err != nil {
		writeGameError(c, http.StatusOK, constUtil.ErrGameRequest, err.Error())
		return
	}
	// 原TCP没有时间戳字段；HTTP资源与购买使用原时间戳绑定持久回执。
	switch p.Body.(type) {
	case *pb.CSHpRecover, *pb.CSResurrection, *pb.CSTransfer2Safe, *pb.CSFieldResourceStartGather, *pb.CSFieldGetResource, *pb.CSWorldBossFight, *pb.CSQuestReward:
		// 原SendSocket为28字节无时间戳头；这些HTTP入口仍须绑定原回执时间戳。
		if p.Timestamp <= 0 {
			writeGameError(c, http.StatusOK, constUtil.ErrGameRequest, "HTTP请求缺少有效时间戳")
			return
		}
	case *pb.CSUFPlayerUpResource, *pb.CSUFPlayerBuyItem, *pb.CSUFFieldCariotFactoryOpen:
		if p.Timestamp <= 0 {
			writeGameError(c, http.StatusOK, constUtil.ErrGameRequest, "公会战HTTP资源、购买或工厂时间戳无效")
			return
		}
	}
	responses := gameService.Dispatch(c.Request.Context(), p, c.ClientIP())
	payload, err := model.EncodeGameResponses(responses)
	if err != nil {
		elog.Error("encode game response", zap.Error(err))
		writeGameError(c, http.StatusInternalServerError, constUtil.ErrGameRequest, "Response encoding failed")
		return
	}
	c.Data(http.StatusOK, "application/octet-stream", payload)
}

func writeGameError(c *gin.Context, status int, code, message string) {
	raw, _ := model.EncodeGameResponses(service.GameError(code, message))
	c.Data(status, "application/octet-stream", raw)
}

// ServeGameTCP 绑定连接内的会话，并在停服时关闭所有连接等待处理协程退出。
func ServeGameTCP(ctx context.Context, listener net.Listener, s *service.GameService) error {
	var (
		mu          sync.Mutex
		wg          sync.WaitGroup
		connections = map[net.Conn]bool{}
	)
	closeConnections := func() {
		listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for c := range connections {
			c.Close()
		}
	}
	stop := context.AfterFunc(ctx, closeConnections)
	defer func() {
		// Accept 失败也要解除读取阻塞，再等待连接协程结束并向主进程报告错误。
		stop()
		closeConnections()
		wg.Wait()
	}()
	for {
		c, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		mu.Lock()
		if ctx.Err() != nil {
			mu.Unlock()
			c.Close()
			return nil
		}
		connections[c] = true
		mu.Unlock()
		wg.Add(1)
		go func(ctx context.Context, c net.Conn) {
			defer wg.Done()
			defer c.Close()
			defer func() { mu.Lock(); delete(connections, c); mu.Unlock() }()
			serveGameConnection(ctx, c, s)
		}(ctx, c)
	}
}

func serveGameConnection(ctx context.Context, c net.Conn, s *service.GameService) {
	var (
		playerId int64
		session  []byte
	)
	var writeLock sync.Mutex
	writeMessages := func(messages []proto.Message) error {
		payload, err := model.EncodeGameResponses(messages)
		if err != nil {
			return err
		}
		writeLock.Lock()
		defer writeLock.Unlock()
		n, err := c.Write(payload)
		if err == nil && n != len(payload) {
			return io.ErrShortWrite
		}
		return err
	}
	var unregister func()
	defer func() {
		if unregister != nil {
			unregister()
		}
	}()
	// 仅记录连接终止原因与协议编号，避免把账号凭据或原始帧写入日志。
	var lastOpcode int32
	closed := func(reason string, err error) {
		fields := []zap.Field{zap.String("reason", reason), zap.Int64("player_id", playerId), zap.Int32("opcode", lastOpcode)}
		if err != nil {
			fields = append(fields, zap.Error(err))
		}
		elog.Info("game tcp closed", fields...)
	}
	peer, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	for {
		c.SetDeadline(time.Now().Add(constUtil.PacketTimeout))
		prefix := make([]byte, 4)
		if _, err := io.ReadFull(c, prefix); err != nil {
			closed("read-prefix", err)
			return
		}
		size := binary.BigEndian.Uint32(prefix)
		if size < 24 || size > constUtil.MaxPacket-4 {
			closed("packet-size", nil)
			return
		}
		raw := make([]byte, int(size)+4)
		copy(raw, prefix)
		if _, err := io.ReadFull(c, raw[4:]); err != nil {
			closed("read-body", err)
			return
		}
		p, err := model.ParseGamePacket(raw, true)
		if err != nil {
			closed("packet-parse", err)
			return
		}
		lastOpcode = p.Opcode
		if err = ValidateGameRequest(p); err != nil {
			closed("request-validation", err)
			return
		}
		// 每条消息仍重新校验数据库中的会话，账号重登会使旧 TCP 连接失效。
		if p.PlayerId == playerId {
			p.Session = session
		}
		messages := s.Dispatch(ctx, p, peer)
		registerClient := false
		if b, ok := p.Body.(*pb.CSSocketLogin); ok && len(messages) > 0 {
			if reply, ok := messages[0].(*pb.SCSocketLogin); ok && reply.GetResult() == 100 {
				playerId = p.PlayerId
				session = append([]byte(nil), b.GetSessionKey()...)
				if unregister != nil {
					unregister()
				}
				registerClient = true
			}
		}
		if err = writeMessages(messages); err != nil {
			closed("response-write", err)
			return
		}
		// 原登录响应先完整写入，再允许私聊推送进入同一连接。
		if registerClient {
			unregister = s.RegisterP10Client(ctx, playerId, session, func(message proto.Message) error { return writeMessages([]proto.Message{message}) }, func() { _ = c.Close() })
		}
	}
}
