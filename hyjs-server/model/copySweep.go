package model

import pb "hyjs-server/model/protocol"

// CopySweepReceipt 保存一次原始扫荡包和对应奖励增量。
// GameState 的旧字段保留最大时间戳；独立 witness 保存最近一次。
type CopySweepReceipt struct {
	Timestamp int64           `json:"timestamp"`
	Request   *pb.CSCopySweep `json:"request"`
	Response  *pb.SCCopySweep `json:"response"`
}
