package inventory

//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// CircuitNotFoundErr identifies circuit not found failures.
	CircuitNotFoundErr = &Error{404, "专线不存在或无权查看"}
	// CircuitDuplicateErr identifies circuit duplicate failures.
	CircuitDuplicateErr = &Error{422, "线路编号已存在，请使用其他编号"}
	// CircuitLifecycleErr identifies circuit lifecycle failures.
	CircuitLifecycleErr = &Error{400, "已终止专线不能恢复状态，只有规划中专线允许删除"}
	// CircuitScopeErr identifies circuit scope failures.
	CircuitScopeErr = &Error{403, "缺少专线数据权限"}
	// CircuitDeviceErr identifies circuit device failures.
	CircuitDeviceErr = &Error{400, "端点设备不存在或无权查看"}
	// CircuitBindingErr identifies circuit binding failures.
	CircuitBindingErr = &Error{400, "监控绑定无效，请重新选择主机及其启用的速率监控项"}
	// CircuitExportLimitErr identifies circuit export limit failures.
	CircuitExportLimitErr = &Error{400, "导出结果超过 10000 条，请缩小筛选范围"}
	// ZabbixNotConfiguredErr identifies zabbix not configured failures.
	ZabbixNotConfiguredErr = &Error{503, "Zabbix 尚未配置，请联系管理员完成配置"}
	// ZabbixUpstreamErr identifies zabbix upstream failures.
	ZabbixUpstreamErr = &Error{502, "无法读取 Zabbix 监控数据，请稍后重试"}
	// ZabbixTimeoutErr identifies zabbix timeout failures.
	ZabbixTimeoutErr = &Error{504, "Zabbix 查询超时或已取消，请稍后重试"}
)
