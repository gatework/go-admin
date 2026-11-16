// Command motor runs the administration backend and its maintenance commands.
package main

import (
	"github.com/gatework/motor/cmd"
)

//go:generate swag init --parseDependency --parseDepth=6 --instanceName admin --exclude example,test -o ./docs/admin

// @title motor API
// @version 2.4.1
// @description motor 网络运维平台后端接口。运行时版本以 buildinfo.Version 为准。
// @license.name MIT
// @license.url https://github.com/gatework/motor/blob/master/LICENSE.md

// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
func main() {
	cmd.Execute()
}
