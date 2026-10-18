package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestResourceRecoveryAuditBoundaryWithoutConsumption 跨越恢复边界的成功事务仍保留该周期奖励。
func TestResourceRecoveryAuditBoundaryWithoutConsumption(t *testing.T) {
	s, id, _ := gemFixture(t, false)
	ctx := context.Background()
	player, _ := gemProfile(t, id)
	player.Stamina = proto.Int32(99)
	// 直接准备隔离档案，避免 saveProfile 先对这份测试时间执行消费归一化。
	player.StaminaLastRefreshTime = proto.Int64(time.Now().UnixMilli() - constUtil.StaminaRecoveryIntervalMS + 500)
	raw, err := json.Marshal(player)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.Profile{}).Where("id = ?", id).Update("player", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error {
		if p.GetStamina() != 99 {
			t.Fatal("fixture crossed the recovery boundary before the transaction started")
		}
		// 模拟跨过一个周期边界的业务计算，期间不消费体力。
		time.Sleep(700 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	current, _, err := s.resourceProfile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if current.GetStamina() != 100 {
		t.Fatalf("a transaction without resource consumption discarded a due recovery tick: stamina=%d", current.GetStamina())
	}
}
