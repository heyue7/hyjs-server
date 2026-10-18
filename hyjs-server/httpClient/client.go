package httpClient

import "hyjs-server/utils"

// HttpClient provides two clients:
// - OutputCli: 外部服务调用
// - InnerCli: 内部服务调用（默认 ServerName 对应的 LB）
// ServerPools 用于维护服务地址池：ServerName -> []address
type HttpClient struct {
	OutputCli   *utils.HttpClient
	InnerCli    *utils.HttpClient
	ServerPools map[string][]string
	ServiceLBs  map[string]*utils.LB
	LBOptions   []utils.LBOption
}

// NewHttpClient - 创建 HttpClient
// 为每个 ServerName 创建独立 LB（使用同一策略），InnerCli 不绑定默认 LB
func NewHttpClient(serverPools map[string][]string, lbOptions ...utils.LBOption) *HttpClient {
	outputCli := utils.NewHttp()
	innerCli := utils.NewHttp()

	serviceLBs := make(map[string]*utils.LB, len(serverPools))
	for name, servers := range serverPools {
		if len(servers) == 0 {
			continue
		}
		serviceLBs[name] = utils.NewLB(servers, lbOptions...)
	}

	return &HttpClient{
		OutputCli:   outputCli,
		InnerCli:    innerCli,
		ServerPools: copyServerPools(serverPools),
		ServiceLBs:  serviceLBs,
		LBOptions:   lbOptions,
	}
}

// InnerFor - 获取指定 ServerName 的内部客户端（对应服务地址池的 LB）
func (hc *HttpClient) InnerFor(serverName string) *utils.HttpClient {
	if hc == nil {
		return utils.NewHttp()
	}
	if lb, ok := hc.ServiceLBs[serverName]; ok {
		return utils.NewHttp().WithLB(lb)
	}
	return utils.NewHttp()
}

func copyServerPools(pools map[string][]string) map[string][]string {
	if pools == nil {
		return nil
	}
	out := make(map[string][]string, len(pools))
	for k, v := range pools {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
