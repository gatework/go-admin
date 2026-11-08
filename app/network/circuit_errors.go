package network

var (
	ErrCircuitNotFound     = &Error{404, "专线不存在或无权查看"}
	ErrCircuitDuplicate    = &Error{422, "线路编号已存在，请使用其他编号"}
	ErrCircuitLifecycle    = &Error{400, "已终止专线不能恢复状态，只有规划中专线允许删除"}
	ErrCircuitScope        = &Error{403, "缺少专线数据权限"}
	ErrCircuitDevice       = &Error{400, "端点设备不存在或无权查看"}
	ErrCircuitBinding      = &Error{400, "监控绑定无效，请重新选择主机及其启用的速率监控项"}
	ErrCircuitExportLimit  = &Error{400, "导出结果超过 10000 条，请缩小筛选范围"}
	ErrZabbixNotConfigured = &Error{503, "Zabbix 尚未配置，请联系管理员完成配置"}
	ErrZabbixUpstream      = &Error{502, "无法读取 Zabbix 监控数据，请稍后重试"}
	ErrZabbixTimeout       = &Error{504, "Zabbix 查询超时或已取消，请稍后重试"}
)
