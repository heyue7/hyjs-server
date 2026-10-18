package constUtil

import "errors"

// ErrP4WelfareNotOpen is an intentional absence, separate from invalid saved rules.
var ErrP4WelfareNotOpen = errors.New("福利实例未开放或领取已截止")
