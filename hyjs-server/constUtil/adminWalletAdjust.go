package constUtil

import "fmt"

const (
	AdminWalletAdjustCredit       = "credit"
	AdminWalletAdjustDebit        = "debit"
	AdminWalletAdjustSource       = "admin_adjust"
	AdminWalletAdjustKeyPrefix    = "admin-adjust:"
	AdminWalletAdjustCreditLabel  = "后台发放："
	AdminWalletAdjustDebitLabel   = "后台扣除："
	AdminWalletAdjustAuditAction  = "wallet.adjust"
	AdminWalletAdjustConflictText = "余额已更新，请刷新后重试"
	AdminWalletAdjustInsufficient = "超频点数余额不足，无法扣除"
)

var (
	ErrAdminWalletAdjustRevision = fmt.Errorf("%w: %s", ErrAdminConfigConflict, AdminWalletAdjustConflictText)
	ErrAdminWalletAdjustReplay   = fmt.Errorf("%w: 调整请求编号已用于其他内容，请刷新后重试", ErrAdminConfigConflict)
	ErrAdminWalletAdjustUnlinked = fmt.Errorf("%w: 角色尚未关联全局账号，无法调整超频点数", ErrAdminInput)
	ErrAdminWalletAdjustOverflow = fmt.Errorf("%w: 超频点数余额或版本已达上限，无法调整", ErrAdminInput)
)
