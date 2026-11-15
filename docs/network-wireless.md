# 无线管理：功能、证据与 API 契约

本模块在 motor 中实现 CLI 优先的无线管理闭环。验收证据见前端 `docs/WIRELESS_VALIDATION.md`；功能实现、离线测试、本地服务就绪和真实设备验证分别记录。所有路径以 `/api/v1/network-wireless` 为前缀，沿用 motor 响应包装及分页（`data.list`、`data.count`、`data.pageIndex`、`data.pageSize`），控制器 ID 为网络设备 ID，AP 路径中的 ID 为持久化资源 ID；业务 `apId`、`radioId`、BSSID 保留字符串。

| 方法 | 路径 | 作用 | 权限 |
| --- | --- | --- | --- |
| GET | `/controllers` | 控制器分页 | list |
| GET | `/groups` | 有权查看的库存分组树 | list |
| GET | `/switches` | 有权选择的接入交换机 | query |
| GET | `/clients` | 跨有权控制器定位客户端 | query |
| GET | `/controllers/:controllerId` | 控制器详情 | query |
| GET | `/controllers/:controllerId/analysis` | 按检查、SSID、频段的四类分析 | query |
| GET | `/controllers/:controllerId/access-points` | AP 分页 | query |
| GET | `/controllers/:controllerId/access-points/:id` | AP 详情及邻居证据 | query |
| GET | `/controllers/:controllerId/radios`、`/bsses`、`/clients` | 资源分页 | query |
| GET | `/controllers/:controllerId/clients/:id` | 持久化客户端详情 | query |
| GET | `/controllers/:controllerId/collections` | 采集历史分页 | query |
| GET | `/controllers/:controllerId/collections/:id` | 采集详情 | query |
| POST | `/controllers/:controllerId/collections` | 创建采集任务 | collect |
| PUT | `/controllers/:controllerId/access-points/:id/management` | 更新人工管理信息 | edit |
| PUT | `/controllers/:controllerId/access-points/:id/neighbor` | 更新人工邻居关联 | edit |
| GET | `/controllers/:controllerId/exports/:kind` | 按范围导出 CSV；kind 为资源类型或 `management` | export |
| POST | `/imports/preview` | 预览 AP 管理 CSV | import |
| POST | `/imports/commit` | 按预览摘要事务提交 | import |

权限前缀为 `network:wireless:`。列表及资源支持 `pageIndex`、`pageSize`、`keyword`、`ip`、`mac`、`ap`、`ssid`、`vlan`、`band`、`status`、`sort`、`direction`；控制器列表 `groupId` 省略代表全部，0 代表未分组，正数代表对应组。资源排序仅支持 `id`；方向为 `asc`/`desc`。`ap` 默认模糊匹配，`exactAp=true` 精确匹配。控制器分页复用库存查询，返回 `groupId` 和 `group`。query、collect、edit、import、export 都授予完成操作所需的读取能力；list 仅允许分组和控制器列表，不授予详情或写入。

采集示例：`{"requestKey":"unique-request-001","dimensions":["lldp"],"apNames":["AP-01"]}`。默认维度不包含 LLDP；LLDP 必须明确指定本控制器现有 AP 名称。服务器自动补充 AP 台账维度作为 LLDP 前置证据；配置派生维度自动补充配置读取。控制器检查建议只补充有关维度，AP 邻居建议只包含该 AP。任务状态为 `queued`、`running`、`succeeded`、`partial`、`failed`、`interrupted`；请求键按发起人去重，相同键不同内容返回冲突。

管理更新：`{"version":1,"localAlias":"办公区","installationLocation":"三层","owner":"网络组","notes":""}`；邻居关联：`{"version":1,"observationVersion":2,"evidenceId":"...","deviceId":42,"interface":"GigabitEthernet1/0/1"}`。版本或证据过期返回 409；邻居关联只更新库存元数据，保留原始证据和候选，**不执行 Ruby 邻居修正脚本**。

CSV 使用 UTF-8，管理模板列严格为 `id,version,localAlias,installationLocation,owner,notes`。预览请求 `{"content":"id,version,localAlias,installationLocation,owner,notes\n1,1,AP,,,\n"}`，响应 `{"digest":"...","rows":1,"errors":[]}`；提交请求将相同 `content` 与 `digest` 一并发送。最大 10 MiB、10000 行；提交重新校验权限与版本，并在事务中全部成功或全部失败。导出时对公式开头字符串转义。

每个维度保留最近一次观测与最近完整观测；不完整观测不能覆盖完整事实，完整的零条观测可标记事实缺失。数据超过 36 小时、来源改变或最近采集不完整时不能认定当前健康。分析须区分 affected、healthy、unknown、notApplicable，不能使用无依据的 RF 门限。

## 页面与数据契约

入口 `/network/wireless`，菜单 **网络管理 → 无线管理**，组件 `NetworkWirelessManage`。左侧设备分组和控制器分页，右侧概览、AP 台账、射频、BSS/SSID、客户端和任务。问题对照按检查项 + SSID + 频段组织四类结果；可从问题打开 AP/客户端，再返回同类问题。URL 保留控制器、分组、页签、问题、AP/客户端与关键词/SSID/频段/AP/VLAN/状态筛选。

- 控制器详情：`{device, platform, supportedDimensions, defaultDimensions, coverage, counts, candidates}`；`candidates` 是最新部分证据，未当作完整台账。
- 资源：持久化 ID、人工字段、观测时间及版本，另有白名单 `fields`、`coverage`、`health`、`controllerName`。不会序列化原始配置、凭据、命令回显或内部来源摘要。
- AP 详情：`{resource, neighbors, neighborCoverage, radios, bsses, clients, counts}`。明细每类最多返回 200 条，数量独立返回；完整明细在对应台账筛选。
- 分析：`{controllerId, groups, ssids, coverage}`；每组返回 `key/check/ssid/band`、四类数量、`outcomes` 和 `suggestion`。
- 任务：`id/controllerId/status/version/dimensions/apNames/results` 与起止时间，失败只含安全原因代码。请求由登录用户发起，客户端不能指定 actor。
- 管理更新返回同一资源 DTO；邻居关联返回 AP 详情。写入使用版本校验；刷新保留草稿，冲突必须读取新记录后处理。

界面可见时每 15 秒刷新证据并发现任务，每 3 秒跟踪已知活动任务；慢详情请求不会阻塞任务观察。隐藏、缓存停用、账号或范围变化会中止/丢弃旧请求。失败刷新保留同一查询已有结果；新查询失败不会显示旧查询的数据。

## 健康与证据

当前健康检查有明确范围：AP 在线状态使用库的状态归一化；射频检查管理启用状态；客户端检查当前 AP 归属。BSS 的正常仅表示完整记录具有 SSID，不能解释为射频质量正常。不设置未经依据验证的信号强度、信噪比或容量阈值。客户端 SSID 分布统计保留的观测记录，须结合时效阅读。

每维度分别保存 latest/last-complete。完整零记录可以退役事实；部分、失败、取消及会话关闭不确定保留上一完整事实并使本次覆盖未知。超过 36 小时或设备/连接来源变化也不能判为健康。射频/BSS/客户端绑定稳定 AP 资源 ID 及来源，AP 退役或同名替换后，旧子记录不会继续证明健康。

邻居完整性按明确选择的 AP 保存。失败没有逐 AP 回执时，也记录该 AP 最新失败，未选择 AP 的证据不受覆盖。邻居状态绑定稳定 AP ID，同名新设备不继承旧设备的邻居事实。人工关联必须使用最新完整证据及管理版本；对无权查看的交换机，响应隐藏已存关联的目标 ID/接口，不删除数据库中的关联。

## 采集与并发

Go 原生调用 GitHub 固定版本的 go-wireless Collector/adapter，无 Ruby 或 CLI 子进程解析。H3C 支持 AP、射频、BSS、客户端、配置派生和 AP LLDP 等库声明维度；Huawei 当前只声明 AP 和客户端，其余明确标记不支持。

控制器复用 delivery 的设备连接和安全凭据配置。无线只读命令使用 3 分钟、16 MiB 输出上限，容纳大型控制器的客户端明细。AP 凭据仅在明确请求 LLDP 时读取 `WIRELESS_AP_USERNAME` / `WIRELESS_AP_PASSWORD`。凭据未配置时任务记录失败，不会把失败解释为空结果。任务持久化入队；单 worker 有界执行，14 分钟网络超时、15 分钟租约，不启用自动调度。

控制器使用共享 `sys_delivery_device_lease`，与 delivery/Fabric 互斥。只按自身 token 释放；无法确认会话关闭时隔离租约。超时运行记录被标为中断；已被其他会话占用的任务不会阻塞其他可执行控制器。网络 IO 在事务外；最终发布在事务内锁定设备及连接版本。连接编辑也锁设备行，因此首次建立连接记录同样受版本保护。采集只更新其拥有的观测字段，不覆盖人工信息、关联或管理版本。

无线 adapter 使用已发布连接器的借用会话接口，版本及校验和由 `go.mod`、`go.sum` 固定。`python3 scripts/verify-delivery-dependencies.py` 检查模块图和校验和，并拒绝本地替换；版本兼容范围见 [依赖说明](github-dependencies.md)。

## Ruby 对照与本轮边界

源码参照 `Works/ruby/motor/backend/app/controllers/motor/wireless_controller.rb`、`services/motor/wireless/{inventory,detail_inventory,runtime_analysis,spreadsheet,neighbors}.rb` 和前端无线模块。

| Ruby 业务能力 | 本轮 motor 实现 |
| --- | --- |
| 控制器/AP/射频/BSS/客户端台账 | 复用库存控制器，稳定资源 ID、分组筛选、列表/详情、跨控制器定位 |
| 运行分析与证据 | 四类检查结果、完整性/时效/来源、SSID 分布；未扩展未经验证的 RF 阈值 |
| 人工 AP 台账、表格导入导出 | 别名/位置/负责人/备注，CSV 预览、摘要提交、事务版本检查；不兼容 Ruby XLSX 工作簿 |
| AP 邻居与库存关联 | 显式 AP 采集、候选证据、人工交换机/端口关联 |
| Netdisco 无线同步 | 本轮不接入，仅使用已有设备库存 |
| Ruby 邻居修正脚本、配置下发、重启 AP | 本轮不执行，台账关联不产生设备命令 |
| 其他库维度（认证会话、配置派生、冲突等） | 保存维度观测和覆盖状态；专用业务列表未在本轮页面范围内 |

数据库迁移 `1793160000000` 创建 5 张无线表及 20 条 API 登记。迁移和菜单授权幂等；操作权限前缀 `network:wireless:`。初始交付仅验证本地与模拟场景；后续实机范围见下文。远端 CI 和生产发布尚未验证。

## 2026-10-28 控制器树修正与实机采集

无线分组复用授权后的设备聚合查询，但仅统计 `wireless_controller`。从叶子向上累计后裁剪零控制器分支，保留祖先目录和未分组计数；通用设备管理树继续保留空分组供维护。前端桌面与移动端统一显示“控制器分组 / 全部控制器”，搜索和键盘选择沿用共享树。

用户确认 `SZX9-WLC01 / 10.251.8.253`、`SZX10-WLC01 / 10.253.8.253` 后，仅对这两台进行 SSH 只读采集，无 AP LLDP 登录或配置下发。原有清单没有两台记录；通过已认证的本地管理员会话核验身份后，按用户确认值登记本地台账，备注明确为手工登记且尚未由 Netdisco 同步。设备 ID 为 2935、2936，自动归入传音大厦、深圳云科技。

首次接入使用既有 `accept_new` 首次记录策略建立独立主机密钥文件；后续已切回严格校验。凭据保存在 Git 忽略的本地 0600 文件，不进入源码或报告。

实机发现 H3C 登录提示符前有 NUL 填充，而后续命令可不带该字符。修复独立 go-wireless 的精确提示符匹配，保留控制器身份约束；原生传输回归先失败再通过，随后将单文件更新同步至冻结快照并验证哈希。

18:30 左右的真实采集（香港时间）结果：

| 控制器 | AP | 射频 | BSS | 其他结果 |
| --- | ---: | ---: | ---: | --- |
| SZX9-WLC01 | 197 | 589 | 2308 | AP 型号 171；IP 冲突记录 8902 完整 |
| SZX10-WLC01 | 128 | 384 | 1515 | AP 型号 171；IP 冲突候选 406，重复标识导致 partial |

客户端 verbose 输出分别达到约 4.2/5.2 MB 后触发原 60 秒超时；后续配置和认证维度缺少完整证据，不能视为零记录。调整无线命令预算后，重试在 SSH 建连阶段返回 `connection_closed`；裸 TCP 检查也在收到 SSH banner 前关闭，尚未证明扩大预算后客户端能完整采集。

私有原始报告在双仓根目录 `.local/wireless-live-20261028/reports/2026-10-28_10-29-53/`，保留实际采集时间；另导出 `access-points.csv`，包含两台共 325 条 AP 台账。当前本地容器引擎状态查询、数据库查询及 API 无响应。证据导入停在脚本拷贝阶段，已终止本轮挂起的本地 Docker 客户端；导入程序未执行，本轮未创建页面采集任务。结果入库与真实页面验收仍待服务恢复，不能将 CLI 成功维度表述为已在页面展示。
