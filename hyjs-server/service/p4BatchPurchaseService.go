package service

import (
	"hyjs-server/constUtil"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// p4BatchPurchasePrices publishes exact totals so old rounded unit prices never determine batch cost.
// Original clients ignore field 100. New clients deserialize it as repeated BaseResource, count-indexed from one.
func (s *GameService) p4BatchPurchasePrices(item proto.Message, product p4Product, state *p4CommerceProgress, ordinal, maximum int32) error {
	var raw []byte
	for count := int32(1); count <= min(maximum, constUtil.PurchaseBatchMaximum); count++ {
		price, err := s.p4Price(product, state, ordinal, count)
		if err != nil {
			if count == 1 {
				return err
			}
			break // A larger total cannot fit the original int32 resource contract.
		}
		payload, err := proto.Marshal(price)
		if err != nil {
			return err
		}
		raw = protowire.AppendTag(raw, constUtil.PurchaseBatchCountField, protowire.BytesType)
		raw = protowire.AppendBytes(raw, payload)
	}
	item.ProtoReflect().SetUnknown(raw)
	return nil
}
