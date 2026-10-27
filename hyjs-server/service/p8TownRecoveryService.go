package service

import (
	"math"
	"strconv"
	"strings"
	"time"

	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p8TownScene(scene string) bool { return scene == "town" || scene == "town_main" }

// p8TownTransferRecovery joins the existing profile-locked transfer transaction.
// Only real elapsed time in town contributes; no tick or background task is used.
func (s *GameService) p8TownTransferRecovery(p *pb.Player, g *model.GameState, from, target string, now time.Time) error {
	if g.Field == nil || from == target || p8TownScene(from) && p8TownScene(target) {
		return nil
	}
	f := g.Field
	ms := now.UnixMilli()
	if ms <= 0 || f.TownRecoveryAt < 0 {
		return p8GatherError()
	}
	if p8TownScene(from) && f.TownRecoveryAt > 0 {
		if !p8SideValid(f.Side) {
			return p8GatherError()
		}
		if ms > f.TownRecoveryAt && f.Side.GetCurHp() > 0 && f.Side.GetCurHp() < 1 {
			pair := strings.Split(s.table("GameConfigure", "StaticName", "HpRecoveryPercent")["NameValue"], ",")
			if len(pair) != 2 {
				return p8GatherError()
			}
			seconds, err := strconv.ParseInt(pair[0], 10, 32)
			if err != nil || seconds < 1 || seconds > 3600 {
				return p8GatherError()
			}
			amount, err := strconv.ParseFloat(pair[1], 64)
			if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || amount > 1 {
				return p8GatherError()
			}
			steps := (ms - f.TownRecoveryAt) / (seconds * 1000)
			if steps > 0 {
				if err = s.p8Heal(p, g, min(1.0, float64(steps)*amount), false); err != nil {
					return err
				}
			}
		}
	}
	// Clear the baseline even for a backward clock or full/dead party. Re-entry
	// cannot reuse a settled interval, and this mechanism never revives the dead.
	f.TownRecoveryAt = 0
	if p8TownScene(target) && f.Side != nil && f.Side.GetCurHp() > 0 && f.Side.GetCurHp() < 1 {
		f.TownRecoveryAt = ms
	}
	return nil
}
