package middleware

// URLInfo defines the urlinfo contract.
type URLInfo struct {
	URL    string
	Method string
}

// CasbinExclude casbin 排除的路由列表
var CasbinExclude = []URLInfo{
	{URL: "/api/v1/dict/type-option-select", Method: "GET"},
	{URL: "/api/v1/dict-data/option-select", Method: "GET"},
	{URL: "/api/v1/deptTree", Method: "GET"},
	{URL: "/api/v1/getCaptcha", Method: "GET"},
	{URL: "/api/v1/getinfo", Method: "GET"},
	{URL: "/api/v1/menuTreeselect", Method: "GET"},
	{URL: "/api/v1/menurole", Method: "GET"},
	{URL: "/api/v1/menuids", Method: "GET"},
	{URL: "/api/v1/roleMenuTreeselect/:roleId", Method: "GET"},
	{URL: "/api/v1/roleDeptTreeselect/:roleId", Method: "GET"},
	{URL: "/api/v1/configKey/:configKey", Method: "GET"},
	{URL: "/api/v1/app-config", Method: "GET"},
	{URL: "/api/v1/user/profile", Method: "GET"},
	{URL: "/info", Method: "GET"},
	{URL: "/api/v1/login", Method: "POST"},
	{URL: "/api/v1/logout", Method: "POST"},
	{URL: "/api/v1/user/avatar", Method: "POST"},
	{URL: "/api/v1/user/pwd", Method: "PUT"},
	{URL: "/api/v1/health", Method: "GET"},
	{URL: "/api/v1/ready", Method: "GET"},
	{URL: "/", Method: "GET"},
	{URL: "/api/v1/server-monitor", Method: "GET"},
	{URL: "/api/v1/user/pwd/set", Method: "PUT"},
	{URL: "/api/v1/sys-user", Method: "PUT"},
}
