package model

import pb "hyjs-server/model/protocol"

// CopySweepReceipt 持久化一次原始扫荡包，重试复用奖励增量且不重复扣费。
type CopySweepReceipt struct {
	Timestamp int64           `json:"timestamp"`
	Request   *pb.CSCopySweep `json:"request"`
	Response  *pb.SCCopySweep `json:"response"`
}
