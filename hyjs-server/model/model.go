package model

import (
	"fmt"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type BizCode struct {
	Code int64       `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func NewBizCode() *BizCode {
	return &BizCode{
		Code: 0,
		Msg:  "success",
	}
}

func NewErrBizCode(msg string) *BizCode {
	return &BizCode{
		Code: 1,
		Msg:  msg,
	}
}

func NewErrParamsError() *BizCode {
	return &BizCode{
		Code: constUtil.ErrParamsError,
		Msg:  "参数错误",
	}
}

func NewErrSystemError() *BizCode {
	return &BizCode{
		Code: constUtil.ErrSystemError,
		Msg:  "系统错误",
	}
}

func NewErrBizCodeMsg(code int64, msg string) *BizCode {
	return &BizCode{
		Code: code,
		Msg:  msg,
	}
}

func NewBizCodeData(data interface{}) *BizCode {
	return &BizCode{
		Code: 0,
		Msg:  "success",
		Data: data,
	}
}

func NewLangSystemErrBizCode() *BizCode {
	return &BizCode{
		Code: constUtil.ErrSystemError,
		Msg:  "System error",
	}
}

func (c BizCode) Error() string {
	if c.Msg != "" {
		return c.Msg
	}
	return fmt.Sprintf("%d", c.Code)
}

type Page = db.PageInfo
