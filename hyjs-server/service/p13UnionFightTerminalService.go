package service

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Terminal events stop new actions as soon as a certified base falls. The
// original frozen Match.EndAt remains unchanged for proofs and cold recovery.
func p13UFFieldTerminalAction(tx *gorm.DB, scope string, now time.Time) error {
	at, err := p13UFMixedTerminalAt(tx, scope)
	if err != nil {
		return err
	}
	if at > 0 && now.UnixMilli() >= at {
		return errors.New("本场公会战已结束")
	}
	barrier, err := p13UFBaseBarrierAt(tx, scope, now)
	if err != nil {
		return err
	}
	if barrier > 0 {
		return errors.New("基地战斗正在结算，请稍后重试")
	}
	return nil
}
