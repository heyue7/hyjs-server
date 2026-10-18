package service

import (
	"errors"
	"math"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// prepareBagCapacity 对旧容量只放大一次，保留原VIP/已购增量与全部库存实例。
// 读取仅修改解码对象，成功业务在原角色行锁事务内和版本标记一起保存。
func prepareBagCapacity(player *pb.Player, game *model.GameState) error {
	if player == nil || game == nil || player.GetWarriorBagSize() < 0 || player.GetEquipBagSize() < 0 || player.GetPetBagSize() < 0 {
		return errors.New(constUtil.BagCapacityStateError)
	}
	if game.BagCapacityScale == constUtil.BagCapacityScale && game.WarriorBagCapacityScale == 0 {
		return nil
	}
	var (
		warriorScale = constUtil.BagCapacityScale
		equipScale   = constUtil.BagCapacityScale
		petScale     = constUtil.BagCapacityScale
	)
	switch {
	case game.BagCapacityScale == 0 && game.WarriorBagCapacityScale == 0:
	case game.BagCapacityScale == 0 && game.WarriorBagCapacityScale == 5:
		// 仅内部未发布候选曾扩大角色五倍，准确补到十倍，其余两类仍从原容量扩大。
		warriorScale /= 5
	case game.BagCapacityScale == 5 && game.WarriorBagCapacityScale == 0:
		warriorScale /= 5
		equipScale /= 5
		petScale /= 5
	default:
		return errors.New(constUtil.BagCapacityStateError)
	}
	// 原协议容量使用int32；超大旧容量饱和到协议上限，避免溢出或缩减已有栏位。
	scale := func(value, factor int32) *int32 {
		return proto.Int32(int32(min(int64(value)*int64(factor), int64(math.MaxInt32))))
	}
	player.WarriorBagSize = scale(player.GetWarriorBagSize(), warriorScale)
	player.EquipBagSize = scale(player.GetEquipBagSize(), equipScale)
	player.PetBagSize = scale(player.GetPetBagSize(), petScale)
	game.BagCapacityScale = constUtil.BagCapacityScale
	game.WarriorBagCapacityScale = 0
	return nil
}
