package constUtil

import "errors"

const AdminKingRecruitMode = "king-recruit"
const KingRecruitNativeOpenState = "heroRecuritKing"
const AdminKingRecruitPrefix = "admin_king_recruit_"

var ErrKingRecruitActivityClosed = errors.New("王者招募活动尚未开放")
