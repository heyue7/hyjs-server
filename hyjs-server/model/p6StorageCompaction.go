package model

// P6StorageCompactionResult 汇总一次历史战斗输入压缩的实际处理量。
type P6StorageCompactionResult struct {
	Scanned        int64 `json:"scanned"`        // Scanned 为按主键顺序读取的记录数。
	Compacted      int64 `json:"compacted"`      // Compacted 为成功改写为 gzip 的记录数。
	SkippedInvalid int64 `json:"skippedInvalid"` // SkippedInvalid 为原JSON损坏、哈希不符或未知格式而保留的记录数。
	BytesBefore    int64 `json:"bytesBefore"`    // BytesBefore 为被压缩记录的原始输入字节总数。
	BytesAfter     int64 `json:"bytesAfter"`     // BytesAfter 为被压缩记录改写后的输入字节总数。
}

// ExpiredMatchCleanup reports expired publications with no accepted fight.
type ExpiredMatchCleanup struct {
	Table    string `json:"table"`
	Eligible int64  `json:"eligible"`
	Removed  int64  `json:"removed"`
}
