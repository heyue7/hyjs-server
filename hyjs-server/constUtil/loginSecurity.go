package constUtil

import "time"

const (
	LoginFailureWindow      = 10 * time.Minute
	LoginMaximumDelay       = 30 * time.Second
	LoginBurst              = 200
	LoginSourceRate         = 10.0
	LoginGlobalRate         = 20.0
	LoginSourceCapacity     = 8192
	LoginSourceLifetime     = 30 * time.Minute
	SecurityFailureLogRate  = 2.0
	SecurityFailureLogBurst = 100.0
)
