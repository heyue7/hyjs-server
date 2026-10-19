package service

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// PlayerDisplayName 清理旧导入内容的展示前缀，原ID、来源和冻结配置保持不变。
func PlayerDisplayName(value string) string {
	if !strings.HasPrefix(value, "Re") {
		return value
	}
	rest := strings.TrimLeftFunc(value[2:], func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("·:：-_", r)
	})
	r, _ := utf8.DecodeRuneInString(rest)
	if unicode.Is(unicode.Han, r) {
		return rest
	}
	return value
}

// SanitizePlayerDisplay 在原协议回包边界处理展示文本，兼容已经发布的旧名字。
// 仅处理文字字段，不遍历原始JSON、不改活动ID和资源ID。
func SanitizePlayerDisplay(messages []proto.Message) {
	var visit func(protoreflect.Message)
	visit = func(message protoreflect.Message) {
		message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
			if field.IsMap() {
				return true
			}
			if field.Kind() == protoreflect.MessageKind {
				if field.IsList() {
					list := value.List()
					for i := 0; i < list.Len(); i++ {
						visit(list.Get(i).Message())
					}
				} else {
					visit(value.Message())
				}
			} else if field.Kind() == protoreflect.StringKind && !field.IsList() {
				switch string(message.Descriptor().Name()) + "." + string(field.Name()) {
				case "ActivityTab.name", "ActivityItem.message", "SCActivityInfo.information", "SCContinuedRechargeInfo.information", "RewardItemInfo.rewardTitle", "SCBenefitGiftInfo.giftTitle", "SCKaiGongActivityList.name", "SCKaiGongActivityList.content", "SCActivityList.information":
					clean := playerDisplayText(value.String())
					if clean != value.String() {
						message.Set(field, protoreflect.ValueOfString(clean))
					}
				}
			}
			return true
		})
	}
	for _, message := range messages {
		if message != nil {
			visit(message.ProtoReflect())
		}
	}
}

// playerDisplayText 仅供上方系统文案白名单处理嵌入的旧活动名。
func playerDisplayText(value string) string {
	runes := []rune(value)
	out := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		if i+2 < len(runes) && runes[i] == 'R' && runes[i+1] == 'e' && unicode.Is(unicode.Han, runes[i+2]) {
			i++
			continue
		}
		out = append(out, runes[i])
	}
	return string(out)
}
