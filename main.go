// Command motor runs the administration backend and its maintenance commands.
package main

import (
	"github.com/gatework/motor/cmd"
)

//go:generate swag init --parseDependency --parseDepth=6 --instanceName admin --exclude example,test -o ./docs/admin

// @title motor API
// @version 2.0.0
// @description 基于Gin + Vue + Element UI的前后端分离权限管理系统的接口文档
// @description 添加qq群: 521386980 进入技术交流群 请先star，谢谢！
// @license.name MIT
// @license.url https://github.com/gatework/motor/blob/master/LICENSE.md

// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
func main() {
	cmd.Execute()
}
