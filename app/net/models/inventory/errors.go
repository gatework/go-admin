// Package inventory defines safe failures shared by the inventory boundaries.
package inventory

// Error carries an API business status and a message safe to persist or display.
// It never wraps an upstream response, URL, credential or database error.
type Error struct {
	Code    int
	Message string
}

// Error returns the operation's diagnostic message.
func (e *Error) Error() string { return e.Message }

//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// NotConfiguredErr identifies not configured failures.
	NotConfiguredErr = &Error{503, "Netdisco 尚未配置，请联系管理员完成配置"}
	// UpstreamErr identifies upstream failures.
	UpstreamErr = &Error{502, "无法读取 Netdisco 台账，请稍后重试"}
	// AuthenticationErr identifies authentication failures.
	AuthenticationErr = &Error{502, "Netdisco 认证失败，请联系管理员检查连接配置"}
	// InvalidInventoryErr identifies invalid inventory failures.
	InvalidInventoryErr = &Error{502, "Netdisco 返回的台账数据无效，本次同步未写入"}
	// TimeoutErr identifies timeout failures.
	TimeoutErr = &Error{504, "同步超时或已取消，原台账保持不变"}
	// RunningErr identifies running failures.
	RunningErr = &Error{409, "已有台账同步正在执行，请稍后刷新同步状态"}
	// SyncScopeErr identifies sync scope failures.
	SyncScopeErr = &Error{403, "同步全部台账需要全量数据权限"}
	// NotFoundErr identifies not found failures.
	NotFoundErr = &Error{404, "设备不存在或无权查看"}
	// ConflictErr identifies conflict failures.
	ConflictErr = &Error{409, "台账已被更新，请重新加载后再保存"}
	// PersistenceErr identifies persistence failures.
	PersistenceErr = &Error{500, "台账保存失败，原资料保持不变，请稍后重试"}
	// InvalidInputErr identifies invalid input failures.
	InvalidInputErr = &Error{400, "台账参数无效，请检查输入内容"}
	// GroupNotFoundErr identifies group not found failures.
	GroupNotFoundErr = &Error{404, "设备分组不存在"}
	// GroupNotEmptyErr identifies group not empty failures.
	GroupNotEmptyErr = &Error{409, "分组包含子分组或设备，无法删除"}
	// GroupDuplicateErr identifies group duplicate failures.
	GroupDuplicateErr = &Error{409, "分组编码已存在"}
)
