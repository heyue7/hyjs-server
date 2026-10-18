package handler

import (
	"net/http"
	"strings"

	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"

	"github.com/gin-gonic/gin"
)

func init() {
	urlPrefix := "/api"
	routes.RegisterPostRouter(urlPrefix+"/hello", wrapper.Wrapper(HelloHandler))
}

// HelloHandler 处理函数
// Handler 只做参数校验 和 分配Service
func HelloHandler(c *gin.Context) error {
	// 业务逻辑
	var req model.ExampleListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return model.NewErrParamsError()
	}

	req.FirstName = strings.TrimSpace(req.FirstName)

	if req.FirstName == "Claude" {
		// 自定义错误消息
		return model.NewErrBizCodeMsg(constUtil.ExampleError, "Claude 已被禁用")
	}

	resp, err := service.ExampleService(c, req)
	if err != nil {
		return err
	}

	c.JSON(http.StatusOK, model.NewBizCodeData(resp))

	return nil
}
