package wrapper

import (
	"errors"
	"fmt"
	"hyjs-server/elog"
	"hyjs-server/model"
	"net/http"
	"reflect"
	"runtime"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Wrapper - fn returns model.BizCode or other error
func Wrapper(fn func(c *gin.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := fn(c); err != nil {
			var bizCode = new(model.BizCode)
			if errors.As(err, &bizCode) {
				c.JSON(http.StatusOK, bizCode)
				return
			}
			elog.Error(fmt.Sprintf("%s error.", runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()), zap.Error(err))
			c.JSON(http.StatusOK, model.NewErrBizCode("系统错误"))
			return
		}
	}
}

// I18nWrapper - fn returns i18n model.BizCode or other error
func I18nWrapper(fn func(c *gin.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := fn(c); err != nil {
			var bizCode = new(model.BizCode)
			if errors.As(err, &bizCode) {
				c.JSON(http.StatusOK, bizCode)
				return
			}
			elog.Error(fmt.Sprintf("%s error.", runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()), zap.Error(err))
			c.JSON(http.StatusOK, model.NewLangSystemErrBizCode())
			return
		}
	}
}
