package constUtil

const (
	// ProfileStorageRollbackBatchMax86 bounds the number of locked profiles in one transaction.
	ProfileStorageRollbackBatchMax86 = 50
	// ProfileStorageRollbackBatchBytesMax86 bounds raw game data loaded for a batch.
	ProfileStorageRollbackBatchBytesMax86 int64 = 64 * 1024 * 1024
)
