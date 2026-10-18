package model

// P10VoiceConfigRequest 声明本地实际音频Config请求头；不接受外部SDK凭据。
type P10VoiceConfigRequest struct {
	// PlayerID 当前本地区服角色，正整数。
	PlayerID int64 `header:"X-Hyjs-Player-ID" binding:"required,gt=0"`
	// SessionKey 当前16字节原会话的标准Base64，仅用于鉴权。
	SessionKey string `header:"X-Hyjs-Session-Key" binding:"required,max=64"`
}

// P10VoiceUploadRequest 声明本地实际音频Upload请求头；不接受外部SDK凭据。
type P10VoiceUploadRequest struct {
	// PlayerID 当前本地区服角色，正整数。
	PlayerID int64 `header:"X-Hyjs-Player-ID" binding:"required,gt=0"`
	// SessionKey 当前16字节原会话的标准Base64，仅用于鉴权。
	SessionKey string `header:"X-Hyjs-Session-Key" binding:"required,max=64"`
	// ChatType 1=世界，2=公会，3=私聊，5=野外，6=公会战。
	ChatType int32 `header:"X-Hyjs-Chat-Type" binding:"required"`
	// RequestID 非保密ASCII请求键，真实上传按同键幂等。
	RequestID string `header:"X-Hyjs-Request-ID" binding:"required,max=64"`
	// VoiceScope 录音开始时原配置返回的当前频道范围。
	VoiceScope string `header:"X-Hyjs-Voice-Scope" binding:"required,max=160"`
}

// P10VoiceDownloadRequest 声明本地实际音频Download请求头；不接受外部SDK凭据。
type P10VoiceDownloadRequest struct {
	// PlayerID 当前本地区服角色，正整数。
	PlayerID int64 `header:"X-Hyjs-Player-ID" binding:"required,gt=0"`
	// SessionKey 当前16字节原会话的标准Base64，仅用于鉴权。
	SessionKey string `header:"X-Hyjs-Session-Key" binding:"required,max=64"`
	// FileName 已持久存储的随机opaque音频名，仅允许hyjsvoice_加32位小写hex及.wav。
	FileName string `header:"X-Hyjs-File-Name" binding:"required,len=46"`
}

// P10VoiceDeleteRequest 声明本地实际音频Delete请求头；不接受外部SDK凭据。
type P10VoiceDeleteRequest struct {
	// PlayerID 当前本地区服角色，正整数。
	PlayerID int64 `header:"X-Hyjs-Player-ID" binding:"required,gt=0"`
	// SessionKey 当前16字节原会话的标准Base64，仅用于鉴权。
	SessionKey string `header:"X-Hyjs-Session-Key" binding:"required,max=64"`
	// FileName 已持久存储的随机opaque音频名，仅允许hyjsvoice_加32位小写hex及.wav。
	FileName string `header:"X-Hyjs-File-Name" binding:"required,len=46"`
}
