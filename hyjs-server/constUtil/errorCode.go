package constUtil

// 7001 - 7100 为系统固定错误码
const (
	ErrParamsError int64 = 7001
	ErrSystemError int64 = 7002
)

// 从 7101 开始是业务错误码，业务用的多分配2位数(如7101-7199)，业务用得少就分配1位数(如7101-7109)
const (
	ExampleError int64 = 7101
)
