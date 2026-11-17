# 设备台账后端

路由前缀 `/api/v1/network-device`，遵循宿主 JWT、Casbin、DataScope 和响应信封。`cmd/api/net.go` 通过 `app/net/router` 统一注册路由，数据库结构与菜单/API 权限由宿主迁移注册。

- `GET /` 列表（`absent=true` 仅列出最近一次成功同步未见到的设备：已从 Netdisco 消失或不再被发现；同步从不删除设备），`GET /:id` 详情，`PUT /:id` 人工资料编辑。
- `GET /options` 有权查看的厂商、型号与分组；`GET /export` 导出当前筛选下全部有权查看的记录。
- `GET /sync-status` 配置与最近同步状态；`POST /sync` 将一次全量同步排队，无请求体，立即返回 `status: "queued"` 的同步状态。
- 失败以真实 HTTP 状态与稳定错误码（`inventory.*`）返回。

## 同步

同步由后台 `SyncWorker` 执行（每个租户库一个，随宿主启动）：`POST /sync` 校验权限与配置后把状态行置为 `queued` 并记录请求人；工作进程以令牌和租期认领（`queued → running`），抓取 Netdisco 后在一个事务内写入全部设备，并记录 `imported`、`updated`、`deviceCount`。同一时刻只允许一次排队或运行中的同步，否则返回 409 `inventory.sync_running`。单次同步上限 5 分钟；租期过期仍为 `running` 的同步在下次查询状态时标记为失败（`inventory.sync_interrupted`）。前端轮询 `GET /sync-status` 获取结果，失败原因写在 `message`（错误码）。

## 配置

仅从服务进程环境读取 `NETDISCO_URL`、`NETDISCO_USERNAME`、`NETDISCO_PASSWORD`；已有 API key 时可使用 `NETDISCO_API_KEY`，跳过登录。配置不从 HTTP 请求读取，不返回客户端。URL 不接受内嵌凭据、query 或 fragment；仅允许 HTTP/HTTPS，TLS 校验保持开启。

复用 `github.com/orange-cloudfoundry/go-netdisco v0.2.2` 的认证传输与查询参数编码。适配层自行管理 context 和一次性认证，拒绝所有重定向，不调用 SDK 的自动重试逻辑。唯一来源请求为 `POST /login` 与 `GET /api/v1/search/device?q=%&seeallcolumns=true`。未向 Netdisco 下发设备操作。

## 数据边界

`models.Device` 对应 `sys_network_device`；`models.SyncState` 对应 `sys_network_device_sync_state`。

`SourceFacts` 是明确的来源字段白名单。IP 经标准库解析、规范化，并作为唯一键；缺少序列号仍保留。非法 IP、重复 IP、非数组/损坏响应或超大响应使整个同步失败。来源数组最多读取 32 MiB，认证响应最多 64 KiB；原始响应、SNMP 字段和日志不持久化。

来源日期带时区时转为 UTC ISO 字符串。来源日期没有时区时仅转为不带时区的 ISO 字符串，保留未知时区事实；非法日期为空字符串。

自动分组：每个分组可设 `marker`（不区分大小写，最长 32 字符）。设备名称（无名称时用序列号）包含某分组的 marker 时，自动归入按 `position` 排序的第一个匹配分组；都不匹配时归入编码为 `site.other` 的分组。停用的分组不再接收新成员，但保留现有成员；删除分组即删除其 marker。标准区域的 marker 由迁移写入，此后在分组管理中维护，无需发版。

分组、资产状态、类型、角色、负责人、联系资料、标签与备注由人工维护。同步 upsert 仅更新来源列与同步时间，保留人工资料与初始 `create_by`；不删除未返回的设备。新设备为 `pending`，首次创建人是同步用户。编辑与同步均递增 version，编辑通过 SQL 条件更新避免覆盖新版本。

查询、详情、编辑、选项与导出统一复用宿主 DataScope。开启数据权限时，全局同步仅允许全量范围或管理员；负责人文本不参与授权。CSV 对每个单元格处理公式前缀与控制字符，导出不受分页影响。

## 同步生命周期

同步总预算 25 秒，来源访问预算 20 秒。持久化租约有效 45 秒，获取与终结均使用条件更新；UUID token 防止过期执行覆盖新执行。来源校验完成后，批量 upsert 与成功状态在同一事务提交。失败保留旧快照与最近成功信息，取消后以独立的最多 3 秒清理预算更新失败状态。清理失败或进程崩溃时，后续状态读取/同步会回收过期租约。

## Zabbix 与专线流量

Zabbix 的设备/监控项查询、history/trend 读取与截断判断、速率单位换算、
缺测与峰值处理统一调用 `github.com/gatework/go-zabbix` 的 `Monitoring`
和 `ReadTraffic`。本模块保留环境配置、HTTPS 策略、请求预算、业务错误映射、
DataScope、专线 A/Z 端绑定和原有 HTTP JSON 格式；SDK 使用秒，DTO 转为毫秒。

接口绑定使用 host ID 与入/出方向 item ID；Zabbix 的 hostinterface ID 不是物理端口。
告警原生 PNG 由 SDK 的 `Frontend` 使用独立前端 Cookie 会话获取；通知渠道、
权限检查、事件去重与发送记录由业务层负责。详见 SDK 的 `docs/monitoring.md`。

监控能力依赖 `github.com/gatework/go-zabbix v0.3.0`，通过 `go.mod` 固定发布版本。
使用 `GOWORK=off` 验证正式依赖，不在仓库加入本地 `replace`。

## 验证

在后端模块目录执行：

```sh
go test -race -count=1 ./app/net/...
go test -race -tags integration -count=1 ./app/net/...
```

PostgreSQL 测试要求显式设置 `NETWORK_TEST_DSN`。测试创建独立临时 schema，结束后删除，不修改应用表；未设置时仅跳过该 PostgreSQL 测试。provider 测试使用 `httptest`，不连接真实 Netdisco。运行全仓门禁使用 `go test -race -cover ./...` 与 `make checksilent`。
