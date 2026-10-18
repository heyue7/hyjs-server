package httpClient

import (
	"hyjs-server/utils"

	"github.com/gin-gonic/gin"
)

var httpCli *HttpClient

func Init(serverPools map[string][]string) {
	httpCli = NewHttpClient(serverPools, utils.LBRandom())
}

func HttpOutput() *utils.HttpClient {
	return httpCli.OutputCli
}

func HttpInner(serverName string) *utils.HttpClient {
	return httpCli.InnerFor(serverName)
}

func HttpUserInner(c *gin.Context, serverName string) *utils.HttpClient {
	return httpCli.InnerFor(serverName).WithLBKeyUserId(c)
}
