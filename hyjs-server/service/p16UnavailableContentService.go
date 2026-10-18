package service

import (
	"errors"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

// GuardP16UnavailableContent 在真实业务事务和转场前阻止已明确缺项的窄入口。
// 试炼/精英继续由其实际目录Enabled和原冻结回执控制，避免覆盖已有可用前段及恢复。
func GuardP16UnavailableContent(request proto.Message) error {
	switch message := request.(type) {
	case *pb.CSTransferScene:
		switch message.GetTarget() {
		case "field_003", "field_004", "field_005", "field_006":
			return errors.New(constUtil.P16UnavailableFieldTip)
		}
	case *pb.CSUFStrengthBuildingInfo, *pb.CSUFStrengthBuilding:
		// 80065/80067只属于公会战建筑升级，不限制既有公会科技消费者。
		return errors.New(constUtil.P16UnavailableWarBuildingTip)
	}
	return nil
}
