package model

// P10VoiceConfigResponse 返回真实本地适配状态与原录音上限。
type P10VoiceConfigResponse struct {
	// Enabled false=未开放，true=已明确开放本地音频适配。
	Enabled bool `json:"enabled"`
	// Adapter 0=关闭，1=本地PCM16单声道16k WAV；原GCloud外部认证未恢复。
	Adapter int64 `json:"adapter"`
	// CanRecord false=当前原等级/VIP不允许发送，true=可录音后按具体频道校验。
	CanRecord bool `json:"canRecord"`
	// Scope 当前请求频道实际范围，录音开始后上传必须携带同值以拒绝中途换会/换线。
	Scope string `json:"scope"`
	// MaxSeconds 原最大录音秒数30。
	MaxSeconds int64 `json:"maxSeconds"`
	// RecordingGapSeconds 原录音操作间隔秒数1。
	RecordingGapSeconds int64 `json:"recordingGapSeconds"`
	// MaxBytes 本地合法PCM音频最大字节数。
	MaxBytes int64 `json:"maxBytes"`
	// MinDurationMillis 提前具名本地最低音频时长。
	MinDurationMillis int64 `json:"minDurationMillis"`
}

// P10VoiceUploadResponse 仅在真实WAV字节与SHA已持久保存后返回。
type P10VoiceUploadResponse struct {
	// FileName 实际存储对象的随机opaque名称，不能作为未鉴权公共URL。
	FileName string `json:"fileName"`
	// AudioTime 原协议秒数，按真实录音时长向下取整。
	AudioTime int64 `json:"audioTime"`
	// DurationMillis 实际WAV时长。
	DurationMillis int64 `json:"durationMillis"`
	// FileSize 实际WAV完整字节数。
	FileSize int64 `json:"fileSize"`
	// SHA256 实际音频完整性摘要。
	SHA256 string `json:"sha256"`
	// ExpiresAt 当前对象固定有效期限，Unix毫秒。
	ExpiresAt int64 `json:"expiresAt"`
}

// P10VoiceDeleteResponse 表示当前会话有权删除的对象已持久清字节并失效。
type P10VoiceDeleteResponse struct {
	// Deleted true=对象现已失效，不返回假成功的不存在名称。
	Deleted bool `json:"deleted"`
}
