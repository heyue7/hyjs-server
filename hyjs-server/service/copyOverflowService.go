package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// copyOverflowState 仅在主线胜利事务内保存福利计算所需的暂存角色计数。
// 这些标记不会写进角色库存、战斗回包或客户端显示。
type copyOverflowState struct {
	playerID    int64
	fightID     string
	basePending []*pb.Warrior
}

func beginCopyOverflow(tx *gorm.DB, playerID int64, fightID string) *gorm.DB {
	state := &copyOverflowState{playerID: playerID, fightID: fightID}
	return tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.CopyOverflowContextKey, state))
}

// copyOverflowBenefitBase 将暂存角色按原数量计入福利倍率，真实结算Gain仍只包含已入包资源。
func copyOverflowBenefitBase(tx *gorm.DB, base *pb.ResultObject) *pb.ResultObject {
	state, _ := tx.Statement.Context.Value(constUtil.CopyOverflowContextKey).(*copyOverflowState)
	if state == nil || len(state.basePending) == 0 {
		return base
	}
	result := proto.Clone(base).(*pb.ResultObject)
	for _, pending := range state.basePending {
		result.Wars = append(result.Wars, proto.Clone(pending).(*pb.Warrior))
	}
	return result
}

// grantCopyBattleReward 沿原授奖入口保留容量校验，只有主线胜利事务的超额角色转原领奖中心。
// 奖励内容已经按原冻结配置抽定；相同战斗和阶段只能建立一份永久保留附件。
func (s *GameService) grantCopyBattleReward(tx *gorm.DB, player *pb.Player, game *model.GameState, content, stage string, grant func(string) (*pb.ResultObject, error)) (*pb.ResultObject, error) {
	state, _ := tx.Statement.Context.Value(constUtil.CopyOverflowContextKey).(*copyOverflowState)
	if !s.P1Ready || state == nil {
		return grant(content)
	}
	if state.playerID != player.GetPlayerId() || state.fightID == "" || stage == "" || game.Run == nil || game.Run.Fight == nil || game.Run.Fight.Start.GetFightId() != state.fightID {
		return nil, errors.New(constUtil.CopyOverflowStateError)
	}
	supported, err := s.openServerRewardSupported(content)
	if err != nil {
		return nil, err
	}
	if !supported {
		return nil, errors.New(constUtil.CopyOverflowStateError)
	}
	var (
		fit      []string
		overflow []string
		pending  []*pb.Warrior
		slots    = max(int64(0), int64(player.GetWarriorBagSize())-int64(len(game.Warriors)))
	)
	hash := sha256.Sum256([]byte(state.fightID + ":" + stage))
	digest := hex.EncodeToString(hash[:])
	for _, part := range strings.Split(content, ";") {
		pair := strings.Split(part, ",")
		if !strings.HasPrefix(pair[0], "hero_") {
			fit = append(fit, part)
			continue
		}
		amount, err := p1PositiveAmount(pair[1])
		if err != nil {
			return nil, err
		}
		inBag := min(slots, amount)
		if inBag > 0 {
			fit = append(fit, fmt.Sprintf("%s,%d", pair[0], inBag))
			slots -= inBag
		}
		if amount > inBag {
			overflow = append(overflow, fmt.Sprintf("%s,%d", pair[0], amount-inBag))
			for i := int64(0); i < amount-inBag; i++ {
				pending = append(pending, &pb.Warrior{Id: proto.String(pair[0]), Guid: proto.String(fmt.Sprintf("copy-pending-%s-%d", digest, len(pending)))})
			}
		}
	}
	gain := &pb.ResultObject{}
	if len(fit) > 0 {
		gain, err = grant(strings.Join(fit, ";"))
		if err != nil {
			return nil, err
		}
	}
	if len(overflow) > 0 {
		rewards := strings.Join(overflow, ";")
		saved, err := s.EnqueueRewardNotice(tx, player.GetPlayerId(), "copy-overflow:"+digest, constUtil.CopyOverflowTitle, constUtil.CopyOverflowBody, rewards, 0)
		if err != nil {
			return nil, err
		}
		if saved.Rewards != rewards {
			return nil, errors.New(constUtil.CopyOverflowStateError)
		}
		if stage != "bonus" {
			state.basePending = append(state.basePending, pending...)
		}
	}
	return gain, nil
}
