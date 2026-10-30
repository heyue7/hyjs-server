package service

import (
	"crypto/rand"
	"errors"
	"io"
	"math/big"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// breachProbabilityUnits parses the original percentage without losing any of
// the integer dice precision previously supported by the growth operation.
func breachProbabilityUnits(value string) (int64, error) {
	p, ok := new(big.Rat).SetString(value)
	if !ok || p.Sign() < 0 {
		return 0, errors.New(constUtil.WarriorGrowthConfig)
	}
	p.Mul(p, big.NewRat(10000, 1))
	if !p.IsInt() || !p.Num().IsInt64() {
		return 0, errors.New(constUtil.WarriorGrowthConfig)
	}
	units := p.Num().Int64()
	if units > constUtil.WarriorBreachPityScale {
		return 0, errors.New(constUtil.WarriorGrowthConfig)
	}
	return units, nil
}

// advanceWarriorBreachPity runs inside the locked profile transaction. A
// uniform initial phase avoids synchronized hits while carrying the exact
// remaining probability across levels, days, restarts and batch requests.
func advanceWarriorBreachPity(g *model.GameState, guid string, probability int64, random io.Reader) (bool, error) {
	if guid == "" || probability < 0 || probability > constUtil.WarriorBreachPityScale {
		return false, errors.New(constUtil.WarriorGrowthConfig)
	}
	phase, initialized := g.WarriorBreachPity[guid]
	if initialized && (phase == nil || phase.Phase < 0 || phase.Phase >= constUtil.WarriorBreachPityScale) {
		return false, errors.New(constUtil.WarriorGrowthInvalid)
	}
	if probability == 0 {
		return false, nil
	}
	if !initialized {
		draw, err := rand.Int(random, big.NewInt(constUtil.WarriorBreachPityScale))
		if err != nil {
			return false, err
		}
		phase = &model.WarriorBreachPity{Phase: draw.Int64()}
		if g.WarriorBreachPity == nil {
			g.WarriorBreachPity = make(map[string]*model.WarriorBreachPity)
		}
		g.WarriorBreachPity[guid] = phase
	}
	phase.Phase += probability
	if phase.Phase >= constUtil.WarriorBreachPityScale {
		phase.Phase -= constUtil.WarriorBreachPityScale
		return true, nil
	}
	return false, nil
}

// retireWarriorBreachPity is called only inside the character's owning profile
// mutation, so a failed reset, refund or consumption also rolls this back.
func retireWarriorBreachPity(g *model.GameState, guid string) {
	delete(g.WarriorBreachPity, guid)
}
