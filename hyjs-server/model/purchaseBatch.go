package model

import (
	"errors"
	"hyjs-server/constUtil"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// GamePurchaseCount reads the optional quantity extension without changing original APK fields.
// Older clients omit field 100 and continue buying one unit.
func GamePurchaseCount(message proto.Message) (int32, error) {
	if message == nil {
		return 0, errors.New("购买请求为空")
	}
	var (
		data  = message.ProtoReflect().GetUnknown()
		count = int32(1)
		seen  bool
	)
	for len(data) > 0 {
		number, kind, n := protowire.ConsumeTag(data)
		if n < 0 {
			return 0, errors.New("购买数量格式无效")
		}
		data = data[n:]
		if number == constUtil.PurchaseBatchCountField {
			if seen || kind != protowire.VarintType {
				return 0, errors.New("购买数量字段重复或类型无效")
			}
			value, size := protowire.ConsumeVarint(data)
			if size < 0 || value < 1 || value > constUtil.PurchaseBatchMaximum {
				return 0, errors.New("购买数量须为1至100")
			}
			count, seen, data = int32(value), true, data[size:]
			continue
		}
		size := protowire.ConsumeFieldValue(number, kind, data)
		if size < 0 {
			return 0, errors.New("购买扩展字段无效")
		}
		data = data[size:]
	}
	return count, nil
}
