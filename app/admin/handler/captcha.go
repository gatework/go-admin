package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/common/logincaptcha"
)

// Captcha handles HTTP requests for its administration resource.
type Captcha struct {
	*api.API
}

// Generate 获取验证码
// @Summary 获取验证码
// @Description 获取验证码
// @Tags 登陆
// @Success 200 {object} response.Response{data=string,id=string,msg=string} "{"code": 200, "data": [...]}"
// @Router /api/v1/captcha [get]
func (e Captcha) Generate(c *gin.Context) {
	a, err := api.New(c, api.Dependencies{})
	if err != nil {
		e.Error(500, err, "服务初始化失败！")
		return
	}
	e.API = a
	// The answer is deliberately discarded rather than logged. It used to be
	// written at info level, which put a currently valid captcha answer in the
	// application log - anyone able to read the log could bypass the check the
	// captcha exists to enforce.
	id, b64s, err := logincaptcha.Generate()
	if err != nil {
		e.Logger.Errorf("Generate captcha error, %s", err.Error())
		e.Error(500, err, "验证码获取失败")
		return
	}
	e.Custom(gin.H{
		"code": 200,
		"data": b64s,
		"id":   id,
		"msg":  "success",
	})
}
