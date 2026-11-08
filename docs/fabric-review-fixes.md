# Fabric review fixes

2026-10-26；本轮只使用本地 SQLite、解析器及假传输测试，没有连接或修改真实设备。

## 修复范围

- 总览纳入全部 Leaf 的网段矩阵：缺失、冲突、差异产生故障 finding；证据不足产生 unknown。SVI 每个未绑定 ACL 方向单列 unknown finding，要求确认业务意图。观察并集及内容一致均不代表正确意图。
- 终端按采集时间查找配置：匹配 Fabric、设备、来源、成员及规则版本，限制配置时间窗口并只读取最新一条匹配记录；后续仅刷新配置不会丢失较早终端的 VRF。
- ACL 预览同时校验 VLAN/VNI、非空相同 VRF 和 tenant 路由声明。执行前重新由标准设备配置派生计划，并校验规范计划 hash；目标读回预检再次校验 VRF 上下文。仅有相同 ACL 名称及有序内容不足以允许跨 VRF 挂载。
- 自动归属只接受 leaf_switch、spine_switch、border_switch。角色变化也触发重新归属；人工指定或人工未匹配保留，unknown 角色允许人工覆盖。
- device_busy 采集失败写入各请求维度的失败 observation，保留历史完整证据，同时阻止旧证据继续成为当前健康和变更授权依据。
- IP 的 Local MAC-IP 记录与同设备 MAC 表关联端口；要求当前证据、相同来源/成员/规则、兼容 VRF/VNI/VLAN。过期、失败、其他来源或远端 VTEP 不用于猜测物理端口。
- 单个 vPC next-hop 标签保留未知分类。双归属要求两端当前终端、互相匹配的 peer 配置、健康且匹配 domain 的当前 vPC 运行证据及相同业务/成员；本地冲突 attachment 不形成双归属。
- 历史工单和采集的详情、列表、幂等采集请求回执以及分页计数检查记录中的源/目标设备权限，防止设备迁出 Fabric 后经当前完整范围读取旧敏感记录。
- 受限数据范围的历史列表按降序 ID 游标扫描，每批最多 100 条；只保留当前页结果，过滤后准确计数。可见库存 ID 只查一次，不逐条查询设备权限。DataScopeAll 或关闭 DataScope 时直接数据库计数及分页，不读取整段序列化 targets 历史。

## 验证

回归文件：`app/fabric/service/review_regression_test.go`。最初 8 条审查复现均先在原实现得到预期失败，再修复通过；额外覆盖伪造跨 VRF 计划、库存角色变化、历史列表/采集权限、8 类不可关联 MAC 证据、vPC 部分及冲突证据。

初次审查修复集成时已验证：

- `go test ./app/fabric/... ./app/delivery/... -count=1` 通过（Fabric router/service，Delivery apis/models/service；包含本轮回归和集成中的候选方案测试）。
- `go test -race ./app/fabric/service ./app/delivery/service -count=1` 通过（5.453s / 3.982s）。
- `go vet ./app/fabric/... ./app/delivery/...` 通过。

相同 IP/VRF 下不同 MAC 或不同端口仍分别计为候选位置；仅有相同设备 ID 不会隐藏歧义。当前双端 vPC 证据的完整/部分/失败状态另有端到端分类回归。

根任务另行运行完整仓库 gates；前端权限依赖及终端歧义修复见 UI 作者的 `docs/FABRIC_VALIDATION.md`。没有提交、推送、真实设备操作或远端发布。

根任务最终复核：在并行连接功能完成后，`go test -race -cover -p 2 ./...` 全仓重跑通过（退出码 0）。后文记录的中间 RED 状态不代表最终 gate 结果；完整交付和本地环境记录见 `network-fabric.md`。


## 历史列表容量收尾

`app/fabric/service/history_test.go` 在工单与采集、Self 与 All 四个组合各建立 225 条历史记录，混合隐藏目标，校验第 7 页、准确可见计数及真实反序列化批量大小。原实现每次读取 225 条，测试先失败；修复后受限范围每次不超过 100 条，All 范围只读取请求的 17 条。

本次收尾后，`go vet ./app/fabric/... ./app/delivery/...` 通过。并行集成自动连接功能的新测试已加入，完整 affected suite 和 race 的本次重跑出现该功能的预期 RED：Fabric `TestFabricEffectiveConnectionSupportsAutomaticNXOSAndRejectsChangedDriver`；Delivery API `TestOptionsInventoryGroupsAreScopedAndRetainUnclassifiedDevices`；Delivery service 的 `TestAutomaticConnectionUsesInventoryWithoutWritingMetadata`、`TestConnectionInfersOnlySupportedUnambiguousProfiles`、`TestManualProfileAndRestoreAutomaticRetainCAS`、`TestAutomaticTargetsPersistGroupingAndHistoricalProfile`、`TestDeviceListUsesEffectiveConnections`、`TestSubmissionRejectsChangedProfileOrRoleFromSelection`、`TestExecutionRechecksAutomaticProfileBeforeEveryWrite`。失败原因为自动连接尚返回 `connection_missing`；该功能属于本代理树以外的并发工作，本轮未修改，保留其代码及测试。根协调者汇总最终集成验证。本次历史分页与原有权限回归未失败。

本轮历史列表专项最终命令及结果：

- `go test ./app/fabric/service -run 'TestHistoricalPages|TestReview' -count=1`：通过，0.618s。
- `go test -race ./app/fabric/service -run 'TestHistoricalPages|TestReview' -count=1`：通过，3.836s。
- `go vet ./app/fabric/... ./app/delivery/...`：通过。
