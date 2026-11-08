// Package network defines safe failures shared by the inventory boundaries.
package network

// Error carries an API business status and a message safe to persist or display.
// It never wraps an upstream response, URL, credential or database error.
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

var (
	ErrNotConfigured    = &Error{503, "Netdisco 尚未配置，请联系管理员完成配置"}
	ErrUpstream         = &Error{502, "无法读取 Netdisco 台账，请稍后重试"}
	ErrAuthentication   = &Error{502, "Netdisco 认证失败，请联系管理员检查连接配置"}
	ErrInvalidInventory = &Error{502, "Netdisco 返回的台账数据无效，本次同步未写入"}
	ErrTimeout          = &Error{504, "同步超时或已取消，原台账保持不变"}
	ErrRunning          = &Error{409, "已有台账同步正在执行，请稍后刷新同步状态"}
	ErrSyncScope        = &Error{403, "同步全部台账需要全量数据权限"}
	ErrNotFound         = &Error{404, "设备不存在或无权查看"}
	ErrConflict         = &Error{409, "台账已被更新，请重新加载后再保存"}
	ErrPersistence      = &Error{500, "台账保存失败，原资料保持不变，请稍后重试"}
	ErrInvalidInput     = &Error{400, "台账参数无效，请检查输入内容"}
	ErrGroupNotFound    = &Error{404, "设备分组不存在"}
	ErrGroupNotEmpty    = &Error{409, "分组包含子分组或设备，无法删除"}
	ErrGroupDuplicate   = &Error{409, "分组编码已存在"}
)
