package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 有限已消费余额隔离个人日期恢复规则；不据此声明自然资源来源通过。
func TestLocalPlayRecoveryNewDayOnceRewindColdAndOverCap(t *testing.T) {
	f, id, owner := localClockFixture(t)
	localClockEnable(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx := localClockBind(t, f, owner)
	consume := func(stamina, energy int32) {
		t.Helper()
		if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error {
			p.Stamina = proto.Int32(stamina)
			p.Energy = proto.Int32(energy)
			p.StaminaLastRefreshTime = proto.Int64(time.Now().UnixMilli())
			p.EnergyLastRefreshTime = proto.Int64(time.Now().UnixMilli())
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	change := func(action, key string) {
		t.Helper()
		if _, err := f.s.LocalPlayTimeChange(ctx, id, action, key); err != nil {
			t.Fatal(err)
		}
		ctx = localClockBind(t, f, owner)
	}
	check := func(stamina, energy int32) {
		t.Helper()
		p, _ := f.profile(t)
		if p.GetStamina() != stamina || p.GetEnergy() != energy {
			t.Fatal("personal recovery balance", p.GetStamina(), p.GetEnergy(), stamina, energy)
		}
		if p.GetStaminaLastRefreshTime() > time.Now().UnixMilli() || p.GetEnergyLastRefreshTime() > time.Now().UnixMilli() {
			t.Fatal("personal date left future cursors in the ordinary resource clock")
		}
	}
	consume(40, 10)
	before, game := f.profile(t)
	change("nextDay", "recovery-first-day")
	check(100, 30)
	after, actualGame := f.profile(t)
	if before.GetCoin() != after.GetCoin() || before.GetDiamond() != after.GetDiamond() || len(game.Items) != len(actualGame.Items) || len(game.Warriors) != len(actualGame.Warriors) {
		t.Fatal("date recovery changed unrelated currency or inventory")
	}
	consume(90, 29)
	// 八个已提交请求重试保持原日期回包，不能在消费后再次恢复。
	var workers sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := f.s.LocalPlayTimeChange(ctx, id, "nextDay", "recovery-first-day")
			errs <- err
		}()
	}
	workers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	check(90, 29)
	change("previousDay", "recovery-back")
	check(90, 29)
	change("nextDay", "recovery-revisit")
	check(90, 29)
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	if _, err := f.s.LocalPlayTimeChange(ctx, id, "nextDay", "recovery-first-day"); err != nil {
		t.Fatal(err)
	}
	check(90, 29)
	change("nextDay", "recovery-second-day")
	check(100, 30)
	consume(150, 40)
	change("nextDay", "recovery-overcap")
	check(150, 40)
	change("reset", "recovery-reset")
	check(150, 40)
	// 已有正日期的老存档以当前日期与历史回执为高水位，不补发旧日期恢复。
	consume(80, 25)
	change("nextDay", "recovery-old-day-one")
	check(80, 25)
	change("nextDay", "recovery-old-day-two")
	check(80, 25)
	change("nextDay", "recovery-old-day-three")
	check(80, 25)
	change("nextDay", "recovery-new-day-four")
	check(100, 30)
}
