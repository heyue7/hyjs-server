package service

import (
	"hyjs-server/constUtil"
	"hyjs-server/utils/contentfilter"
)

// validateChatContent 在回执读取、持久化和推送之前校验，拒绝内容不占用发送间隔。
func validateChatContent(text string) error {
	if contentfilter.Contains(text) {
		return constUtil.ErrChatSensitive
	}
	return nil
}

// validateNameContent 复用于账号、昵称和角色名，不改写用户输入或检查密码。
func validateNameContent(text string) error {
	if contentfilter.Contains(text) {
		return constUtil.ErrNameSensitive
	}
	return nil
}
