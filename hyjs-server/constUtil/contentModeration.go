package constUtil

import "errors"

var (
	ErrChatSensitive = errors.New("内容含有不允许的词语，请修改后再发送")
	ErrNameSensitive = errors.New("名称含有不允许的词语，请重新输入")
)
