package constUtil

import "errors"

// ErrP4CommerceNotOpen distinguishes a deliberately empty catalogue from damaged configuration.
var ErrP4CommerceNotOpen = errors.New("商店或兑换实例未开放")
