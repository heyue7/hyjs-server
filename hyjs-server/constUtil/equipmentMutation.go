package constUtil

const (
	EquipmentDecomposeOpcode        int32 = 21009
	EquipmentForgeOpcode            int32 = 20215
	RecycleDecomposeMaxBatch              = 100
	EquipmentDecomposeMaxBatch            = RecycleDecomposeMaxBatch
	EquipmentRefundBaseOnly               = "base_only"
	EquipmentRefundRejectProgressed       = "reject_progressed"
)

// EquipmentOrdinaryStrengthenOpcode isolates ordinary quote receipts.
const EquipmentOrdinaryStrengthenOpcode int32 = 21001
