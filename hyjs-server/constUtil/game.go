package constUtil

import "time"

const (
	MaxPacket          = 4 * 1024 * 1024
	SigningSuffix      = "sao2016@wyht"
	PacketTimeout      = 30 * time.Second
	ErrGameUnsupported = "90001"
	ErrGameSession     = "90002"
	ErrGameRequest     = "90003"
	ErrGamePacket      = "90004"
)
