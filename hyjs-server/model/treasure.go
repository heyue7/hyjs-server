package model

import pb "hyjs-server/model/protocol"

// TreasureRefineReceipt 保存原请求时间与已结算响应，避免同包重复扣费。
type TreasureRefineReceipt struct {
	Timestamp int64                `json:"timestamp"`
	Response  *pb.SCTreasureRefine `json:"response"`
}

// TreasureCombineReceipt 保存合成请求及响应，原包重试复用实例。
type TreasureCombineReceipt struct {
	Timestamp int64         `json:"timestamp"`
	TargetID  string        `json:"target_id"`
	Response  *pb.SCCombine `json:"response"`
}

// TreasureBoxReceipt 将一次开箱请求绑定到原增量响应。
type TreasureBoxReceipt struct {
	Timestamp int64         `json:"timestamp"`
	Request   *pb.CSItemUse `json:"request"`
	Response  *pb.SCItemUse `json:"response"`
}
