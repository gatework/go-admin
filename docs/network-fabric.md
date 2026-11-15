# Fabric 管理与精准变更

实现日期：2026-10-26。前端入口：**网络管理 → Fabric 管理**，本地开发地址 `http://localhost:9527/#/network/fabric`。

## 使用流程

1. 归属规则将库存中的 Leaf、Spine、Border 关联到区域。初始区域为 SZX9、SZX10、SZX8、SZX11、SHA、CKG；规则修改先预览，冲突和未匹配设备保留，支持人工覆盖。
2. 详情按概览、设备与 vPC、网段与 ACL、终端定位、变更记录组织。概览按检查项、VRF/VNI 和 ACL 方向聚合问题，优先展示明确故障；问题数量是组数，原始发现数单列。全部 Leaf 参与业务网段矩阵；Spine 排除，Border 单列。最新采集失败时仍能查看历史完整证据，但不能据此宣称当前健康。
3. 从问题进入全域对照，展示受影响、正常、未知和不适用设备。四类数量可点击筛选，并保留全部可见设备总数。可继续进入设备核查，也可先从设备列表的问题入口进入，再返回 Fabric 同类问题。业务筛选、设备筛选、当前问题和设备定位保存在 URL，支持刷新、后退和返回；跨 Fabric 清理旧上下文。配置版本及原始证据作为次级信息展开。
4. 从具体问题选择增量变更，自动带入 VNI、操作类型、ACL 方向和受影响设备。用户明确选择标准设备；前端显示其 VRF、VLAN、网段、网关及对应方向的 ACL 名称。变更页保留返回该问题全域对照的入口。
5. 后台逐台核查，只预选确实缺失且符合条件的设备。已一致设备排除；冲突、缺少已有 ACL、策略内容不同、业务上下文不同或当前证据不可用的设备显示原因并阻止加入。可取消勾选以进一步缩小范围，刷新不会恢复已取消的选择。
6. 生成不可编辑的命令预览后再执行。执行前重新验证身份权限、成员归属、参考配置、目标配置及设备租约；逐台执行并读回验证。失败或结果不确定时停止后续设备，不能自动重试未知写入。

初版支持 Cisco NX-OS，受控增量限于补充 VLAN/VNI 映射、补充 SVI 网关地址，以及绑定设备上已经存在且内容匹配的 ACL。未知意图、全体均未绑定 ACL 或已有配置冲突不会自动决定正确策略。自动采集默认关闭，开启后运行维度默认 5 分钟、配置默认 30 分钟。

## 接口与权限

模块按 `app/net` 的 handler、router、service、models 分层组织，前端位于 `../go-admin-ui/src/views/network/fabric`。接口前缀 `/api/v1/network-fabrics`，沿用 JWT、Casbin 和设备 DataScope。只有完整可见 Fabric 范围可预览、采集或变更；历史任务另外验证其冻结目标的当前可见范围。

`POST /:id/change-candidates` 归入 `network:fabric:preview`，返回 `ready`、`unchanged`、`blocked` 三组。问题入口提供明确的非空 `targetDeviceIds`，候选结果不会扩大范围；显式空数组拒绝。手工发起可以省略范围，以检查全部其他 Leaf。候选检查本身不产生可执行任务；预览及执行仍独立检查实际配置。

`Finding.remediation` 包含 `kind`、`vni`、`direction` 和 `targetDeviceIds`，只表示观察到的可补齐问题，不授权写入。客户端不提交任意脚本。所有写入仍要求 `NETWORK_DELIVERY_ENABLED=true`。

`Detail.issueGroups` 提供每个问题的稳定 key、业务范围、四类计数、每台设备的检查结果、证据引用及受限修复建议；`Summary.issueCount` 单独统计组数。相同 VNI 的不同 VRF 分开评估；没有明确业务归属的运行检查保留基础设施范围。前端不按 finding 缺失推断正常。当前配置和运行证据、适用角色及原有检查函数共同决定结果；所有已观测配置变体均标受影响，不采用多数投票。原始发现暂不能明确归属设备时保留未知。

聚合结果纳入不可变 assessment；版本标识 `fabric-issues-v2` 防止复用旧结构快照。设备和 Fabric 的总体健康同时纳入明确业务问题。缺失预期业务接口的运行证据返回未知，配置启用但运行停用的接口返回受影响；配置明确 shutdown 的 NVE 不参与运行接口健康判定，无适用接口配置才是不适用。已有修复目标还需按该业务组重新收窄，不能混入其他 VRF、正常或未知设备。

历史操作采用有界分页：受限权限每次最多读取 100 条，通过 ID 游标遍历并准确计数；完整范围直接使用数据库分页。依赖从 GitHub 下载 `go.mod` 固定的 `go-vxlan` 版本，使用 `go.sum` 校验，无本地 replace；验证命令为 `python3 scripts/verify-delivery-dependencies.py`，版本选择见 [依赖说明](github-dependencies.md)。

## 验证记录

- Fabric / Delivery 测试及 race：`go test -race ./app/net/... -count=1` 已通过，包含精准候选、历史范围和执行前后校验。
- 静态检查：`go vet ./...`、`make fmt-check`、`git diff --check`、`make checksilent UI_DIR=../go-admin-ui/src` 通过；checksilent 为 0 errors / 0 warnings。
- 依赖校验通过：connector 74、expect 18、vxlan 25 个源码输入。
- 最终全仓 `go test -race -cover -p 2 ./...` 通过，退出码 0，日志为 `/tmp/fabric-problems-backend-final.log`。本轮新增 22 项 issue / Detail 回归，覆盖业务分组、四类设备状态、精准目标和接口证据边界。既有并行工作保留。
- 前端完整 gate 通过 68 个文件、466 项测试；类型检查、lint 和 production build 通过。18 项真实浏览器测试使用 mock API，覆盖问题与设备双向导航、业务和状态筛选、URL 刷新与返回、精确 ACL 目标、候选选择、权限、失败刷新、主题与移动端。界面证据位于 `../../output/fabric-ui/problem-*.png`。
- 隔离 SQLite 数据库完整迁移、启动及带认证的实际 HTTP 查询通过；空目标候选返回 400。证据见 `../../output/fabric-api/http-smoke.json`。
- 独立复审已核对 8 类初轮问题修复及候选→预览→执行链路，未发现剩余阻断缺陷。详见 `fabric-review-fixes.md` 与前端 `docs/FABRIC_VALIDATION.md`。
- 问题聚合的独立复审发现预期业务接口缺失、停用接口误报健康，已修正共享接口判断并补充回归；复审通过，未发现剩余阻断。

## 本地开发环境

用户反馈看不到页面后，确认 9527 前端已挂载源码，但 8000 后端旧二进制缺少候选接口，PostgreSQL 缺少 `1793020000000` 迁移。已先用 `pg_dump -Fc` 私有备份本地开发数据库（备份位置记录于 `/tmp/fabric-dev-backup-path`），随后执行迁移并更新后端。更新前无下发任务和设备租约。

已验证菜单组件 `/network/fabric/index`、6 个区域、候选 API 登记完成；`/api/v1/ready` 的 database/cache/schema 均为 OK，未认证的候选请求返回 401。原前端 9527 无需重建，刷新或重新登录即可获取新菜单。真实浏览器访问该入口到达正常登录页；当前浏览器没有本地账号会话，未绕过真实环境登录。自动采集仍关闭（6 个区域均为 false）。

问题聚合与总览→设备→全域对照更新完成后，再次检查没有排队或执行中的下发、Fabric 变更和采集任务，重新构建并重启本地后端。已确认运行二进制包含 `fabric-issues-v1`，database/cache/schema 检查均为 OK；前端继续从 9527 的源码挂载提供新版页面。本次无需新增数据库迁移。登录后的页面交互通过 mock API 浏览器回归验证，未把它表述为真实设备或真实账号的数据验证。

收尾复核时出现环境可用性变化：OrbStack 的 `vmgr.log` 最后记录 `2026-10-26 19:41:13 sleep`，随后 8000、9527、Docker socket `_ping` 及容器查询均超时，`orbctl status` 仍报告 Running。新版加载及 ready 成功的记录发生于此之前；该记录是当时状态；2026-10-28 更新前已复核，前后端服务恢复正常。未强制重启共享虚拟机或改动电源设置。

实际设备采集、设备下发及生产环境尚未验证。本轮没有提交、推送或发布，没有向真实设备下发配置。既有未提交工作保留。

## 2026-10-28 自查整改

- **任务恢复**：中断采集、已尝试变更和变更停止不再因目标改归属、取消归属或成员记录移除而阻塞后续 Fabric。历史任务进入 interrupted / uncertain；失去原区域归属时不写入失效观测，也不污染新区域。真实数据库错误仍返回，不自动重放写入。
- **设备锁**：清理使用独立、最长 5 秒的上下文，精确匹配设备和 token；仅已确认连接关闭时删除锁，未知关闭仍隔离。
- **业务与健康**：矩阵按 VRF＋VNI 分行，相同 VNI 的其他 VRF 标为冲突，不生成补齐建议。Finding 增加可选 scope / vrf / check / direction，IssueGroup 增加 scope；支持证据按完整业务范围关联。无法归属的证据明确显示 Fabric 全域，不显示成默认 VRF。vPC 配对健康在最终设备健康汇总后重新计算。
- **任务自动跟踪**：详情页可见且权限覆盖完整范围时，每 3 秒串行查询已知活动任务。关闭抽屉不停止跟踪；隐藏、离开、切换区域或账号时终止旧请求，恢复时重新读取任务。终态只触发一次 Fabric 和任务列表刷新；失败保留已有证据，单任务失败不阻塞其他任务，较旧版本不能覆盖新状态。任务成功不直接认定问题解决。
- **活动查询**：既有 collections / changes 列表支持 `status=active`，表示 queued / running；过滤先于分页和权限计数，其他非空 status 按精确状态过滤。前端恢复跟踪只遍历活动任务，不读取全部历史。
- **响应瘦身**：SegmentCell.configuration 只投影 host / vlan / svi / nve / evpn / missing；完整配置仍在设备证据中。未修改底层 go-vxlan 目录契约，未新增 API 路由或数据库迁移。

### 测量与验证范围

本地合成基准为 4 台设备，分别配置 10 / 100 / 500 个 VNI，仅测网段矩阵生成和 JSON 序列化，不代表真实设备响应时间或完整 Detail 大小：

| VNI 数 | 原矩阵字节数 | 投影后字节数 | 投影后分配字节/次 | 投影后耗时/次 |
| --- | ---: | ---: | ---: | ---: |
| 10 | 53,289 | 9,569 | 169,037 | 0.253 ms |
| 100 | 4,350,737 | 96,337 | 1,638,186 | 2.271 ms |
| 500 | 108,736,697 | 484,697 | 9,097,010 | 8.542 ms |

基线在修复前记录；投影后由主执行者运行 `go test ./app/net/service/fabric -run '^$' -bench BenchmarkAssessmentMatrixPayload -benchtime=3x -count=1`。500 VNI 样本矩阵字节数减少约 99.55%，未引入缓存。

本轮确认回归先失败再通过：历史成员变化恢复、取消时锁清理、业务健康与支持证据隔离、活动任务过滤、旧任务版本覆盖及单任务失败隔离。最终全仓 `go test -race -cover -p 2 ./...`、`go vet ./...`、fmt-check、checksilent、依赖校验通过；日志 `/tmp/fabric-audit-backend-final.log`。前端 `test:ci` 通过 69 文件 / 476 测试（40 个既有 lint 警告、0 错误），直接 Vite 构建和 22 项模拟 API 浏览器回归通过；日志 `/tmp/fabric-audit-ui-ci-final.log`、`/tmp/fabric-audit-browser-final.log`。桌面和移动端截图已人工检查。

任务恢复与锁清理已通过独立复审。其余实现由主执行者接手复核并完成验证，最终独立审查覆盖业务归属、矩阵投影、健康协调、活动查询、任务生命周期与抽屉集成，未发现阻断问题。本轮没有提交、推送或真实设备操作。

2026-10-28 本地更新：确认活动下发 / Fabric 变更 / 采集、设备锁、启用的自动采集区域均为 0 后，构建并重启后端；运行二进制包含 `fabric-issues-v2`。更新后 `/api/v1/ready` 的 database/cache/schema 均 OK，9527 前端 HTTP 200。未新增迁移，自动采集和真实设备下发均未执行。
