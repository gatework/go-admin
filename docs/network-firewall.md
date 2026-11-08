# 防火墙对象、策略查询与设计草稿

支持从台账设备自动登录、只读采集配置，也可导入离线配置文件。前端入口为「网络管理 → 防火墙对象 / 策略查询 / 策略设计」；对象页包含设备、对象浏览和采集记录三个工作区。此阶段只生成结构化设计草稿，不下发配置。

引擎沿用 `github.com/gatework/firemon v0.0.0-20261029132834-19d1fb999feb`；原生适配器固定到兼容已发布响应与操作接口的提交 `b5c7a0653d93`。连接器使用 `go-net-connector v0.4.0`，终端依赖使用 `go-expect v0.4.1`，全部从 GitHub 下载，无本地兼容补丁替换。可用 `python3 scripts/verify-delivery-dependencies.py` 校验，完整版本选择见 [依赖说明](github-dependencies.md)。

## 启用与使用

迁移 `1793268000000_network_firewall.go` 创建业务表、默认网络域、三个菜单和 API 权限；新增迁移 `1793347200000_firewall_collection.go` 添加在线采集任务及连接设置权限。沿用项目现有迁移命令；已有数据库无需清空。完成迁移后重启后端并刷新登录菜单。

1. 在防火墙对象页按区域/设备分组查找设备，再按名称、IP 或厂商筛选。设备类型或角色须为防火墙，待确认和在用状态均可只读采集。驱动从台账厂商、操作系统、型号推断；不支持的平台、缺失凭据和不匹配设置会逐台显示原因。
2. 选择设备后执行「测试登录」或「采集配置」，支持单台及最多 100 个实例。普通设备首次操作可自动关联默认上下文；同一设备有多个实例时必须选择实例。Fortinet 必须先关联明确的 VDOM，其他在线适配器只接受空原生上下文。任务详情可查看阶段、取消、重试失败目标，并跳转到本次快照。离线文件导入继续可用。七个原生适配器为 Palo Alto、SRX、Huawei、Hillstone、Fortinet、NetScreen、ASA。
3. 在「保护范围」点击「从当前配置填入安全区域」，确认需要保护的安全域后保存；自动填入不会替用户勾选保护范围。路由模式从直连及符合条件的原始路由证据推导网段；透明模式填写网段。网络域默认只有一个，可按 VRF 映射；安全域的网络域为 0 表示继承 VRF 映射，再回退实例默认域。
4. 浏览当前或历史快照中的接口、路由、安全域、地址/地址组、服务/服务组、时间计划、策略、策略绑定、静态 NAT、动态 NAT 和地址池。详情提供结构化字段、诊断及双向引用跳转。
5. 在策略查询页填写三列或粘贴 CSV / TSV / Excel 数据，先解析并纠正问题，再保存、开始分析。已有查询可通过 URL 恢复。
6. 从分析结果创建设计草稿，查看结构化变更、原始需求行、基础版本及证明结果。设置、请求或相关设备快照变化后会提示过期；重新分析会创建新的分析与草稿。

权限前缀为 `network:firewall:`，操作分为 `list`、`query`、`import`、`edit`、`design`、`collect`、`connection`。查询、设计和采集包含所需读取权限；关联实例需要 `edit`，执行登录/采集需要 `collect`，修改共享连接参数需要 `connection`。每个设备、快照、对象引用及历史任务继续受台账数据权限限制；保存的需求、草稿及采集批次仅由创建者访问。权限撤销后，含不可见设备证据的历史结果和原始输入会隐藏。

## 登录配置与采集边界

连接参数复用 `sys_delivery_connection`，设备互斥复用 `sys_delivery_device_lease`。页面只保存 SSH/Telnet、端口和驱动覆盖，采用版本比较防止并发覆盖；不接收密码。凭据从服务器端 `NC_DEVICE_USERNAME` / `NC_DEVICE_PASSWORD` 读取，也可按驱动覆盖，例如 `NC_HILLSTONE_USERNAME` / `NC_HILLSTONE_PASSWORD`、`NC_PALO_ALTO_USERNAME` / `NC_PALO_ALTO_PASSWORD`。SSH 主机密钥沿用 `NC_HOST_KEY_POLICY` 和 `NC_KNOWN_HOSTS`。示例与配置下发共用工作区 `.local/network-delivery.env.example`，实际文件应为 0600。修改环境变量后须重建后端容器才能生效。

只读采集不要求 `NETWORK_DELIVERY_ENABLED=true`，也不会更改台账状态。配置下发仍保持原来的开关及仅在用设备准入。测试登录只打开、关闭原生只读会话；采集再读取原生配置，最多 16 MiB，关闭会话后解析和发布。此阶段没有浏览器终端和定时采集。

每个租户最多同时采集两个目标，单目标期限 5 分钟、租约 6 分钟。运行前及发布前重新核对设备、实例、连接版本、预期快照与租约。采集与配置下发不能同时占用同一物理设备；不能确认会话关闭的设备租约会隔离，不会自动再次登录。取消允许保留已经取得的部分证据。进程恢复先核对原生发布记录和 SQL 投影，不自动重试登录；明确失败后可由用户发起新批次。相同 `requestKey` 与相同实例集合、操作类型返回原批次，改变内容复用键会冲突。

## 输入与结果语义

- 单次最多 1,000 行，草稿最多选择 100 行，原生配置文件最多 16 MiB。限制随 `/options` 返回；列表单页最多 200 条。
- 支持 IPv4/IPv6、CIDR、IPv4 点分掩码、闭区间、短尾地址范围、多地址、多服务和原生对象名称。范围计算不枚举主机。行内组合自动展开，行与行之间保持关联。
- 端口省略协议时，页面明确显示默认 TCP；`80 443` 表示两个 TCP 端口。`udp/53`、`tcp/1024-65535/443`、ICMP 类型/代码保留协议元组。数字 IP 协议使用显式斜杠语法。
- 空白、非法片段、无法解析的对象名以及不兼容的明确地址族保持待确认。只有明确的 `any` / `all` 表示任意；系统不会把错误或空输入扩大为任意访问。
- 原始单元格、物理行号、稳定行键、备注、自动纠正和问题说明一起保存。请求修改保留以前版本的行记录。对象名在设备和原生作用域中解析，歧义可用 `scope::name` 消除。
- 同时定位源和目的端点，保留同一设备的双侧命中。默认路由只用于另一端的出口定位，不扩大受保护范围。各设备命中是独立观察，不能解读为已经证明的跨设备完整转发路径。
- 策略结果按允许、拒绝、未匹配、未知分区；原始报文、策略地址视图、NAT 有序转换链、动态分配不确定性和原始报文状态分别展示。策略命中允许不等于原始报文已证明可通。
- 新关联但未导入配置的实例、缺失名字、部分解析和未定位范围保留未知结果，不丢弃同批已知结果。结果中出现未确认部分会阻止草稿被标为已验证。

## 数据与版本

不可变原生快照是配置事实来源；SQL 原生对象表是可重建的查询投影，不用于拼装替代引擎配置图。

| 表族（均为 `sys_firewall_` 前缀） | 内容 |
| --- | --- |
| `domain`、`instance`、`scope_binding`、`zone_binding`、`network_binding` | 网络域、逻辑实例、人工保护设置与规范化网段区间 |
| `import`、`snapshot`、`coverage` | 导入任务、不可变来源/语义摘要/解析器版本、派生覆盖证据 |
| `collection_batch`、`collection_target` | 持久化采集批次、逐目标准入/阶段/租约/来源版本、快照结果与安全错误码 |
| `interface`、`route`、`zone`、`address`、`address_group` | 原生网络与地址对象 |
| `service`、`service_group`、`schedule`、`rule`、`rule_binding` | 服务、时间计划、保序策略及绑定 |
| `static_nat`、`dynamic_nat`、`nat_pool` | 分开的原生 NAT 类型与池 |
| `reference`、`address_range`、`service_term` | 引用、地址区间和保持端口配对的服务索引 |
| `request`、`request_row`、`analysis`、`analysis_target`、`analysis_result`、`draft` | 版本化输入、权限关联、持久化任务与结构化草稿 |

模型入口 `app/firewall/models.Schema()` 共 32 张表，另复用两张已有连接/设备租约表。迁移使用 `models.Migrate`：PostgreSQL / SQLite 为 TEXT，MySQL 的大载荷列为 LONGTEXT，重复迁移不缩窄列。

快照采用预期版本 CAS 发布；SQL 投影提交后才确认文件 outbox。进程重启会重放未完成投影。离线导入中，用户声明不完整或解析不完整的快照只归档，不替换当前版本；在线采集的最新不完整证据会成为当前版本，撤销旧覆盖证据并使旧分析/草稿过期，避免继续拿旧完整配置证明当前访问。页面明确显示不完整状态，未知部分继续阻止设计被标记为已验证。快照记录原文摘要、语义摘要、采集时间、解析器版本与软件版本。

分析固定请求版本、网络域、全部目标的原生快照/设置版本及时间。新增同域目标也会使旧分析过期。编辑采用版本比较；相同 `requestKey` 的重复提交返回已接受任务，不同内容复用键返回冲突。草稿的同键重试可返回后来已过期的原记录，并明确携带 `stale=true`。

离线导入/分析任务由每个租户库的两个工作协程处理，在线采集使用独立的两个工作协程及数据库并发准入。计算期限 5 分钟、任务租约 6 分钟；租约令牌阻止旧进程覆盖重领结果。浏览器关闭不停止任务。排队导入可以取消；运行中导入已进入原生快照发布流程，不提供取消。分析、采集支持排队和运行中取消。

## 文件持久化

`FIREWALL_SNAPSHOT_DIR` 默认是后端工作目录下的 `data/firewall`，内部按租户注册键隔离。此目录已加入后端 Git 忽略列表。上传正文写入私有文件；数据库不保存配置正文，浏览器 API 不返回原始配置或渲染后的 CLI，防火墙接口正文与返回值不进入操作日志。

Docker 开发环境用 `firewall_snapshots` 卷保存 `/var/lib/motor/firewall`。启用新增挂载必须重建 backend 容器，单纯 restart 不会加载新环境变量和卷。SQL 数据库与快照目录必须配套备份；不要只恢复 SQL、删除快照文件或手改 manifest。当前不自动删除历史快照及上传来源，以保留恢复和追溯依据。

## API 与能力边界

所有接口位于 `/api/v1/network-firewall`，沿用 `code/data/msg` 和 `data.list/count/pageIndex/pageSize`。完整方法、路由及权限的单一声明为 `app/firewall/routes.go`，路由注册和菜单迁移共用该声明。主要资源为 `/options`、`/domains`、`/instances`、`/imports`、`/snapshots/:id/objects/:kind`、`/normalize`、`/requests`、`/analyses`、`/drafts`。在线入口为 `GET /devices`、`GET/PUT /devices/:id/connection`、`POST/GET /collection-batches`、`GET /collection-batches/:id`、`POST /collection-batches/:id/cancel`；创建批次输入为 `{requestKey, instanceIds, kind}`，`kind` 为 `connection_test` 或 `capture`。

设计按同一实例累积原生 `Plan.After`，最终重新验证所有目标、目标范围之外的行为和 NAT 保持情况。只有证明成立的结构化候选才显示已验证。涉及活跃 NAT 的编辑、不能证明原始报文行为的厂商/模式、推测的入接口、未知语法或不完整需求均保留待确认草稿；Hillstone 当前继续显示 `needs_review`。此阶段没有命令预览、审批、下发和保存设备配置接口。

验证记录见前端 `docs/FIREWALL_VALIDATION.md`。本地合成配置与真实 HTTP / PostgreSQL 验证不代表真实设备、生产部署或全部厂商配置的验证。
