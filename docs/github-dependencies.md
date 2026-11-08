# GitHub 依赖

后端的外部 Go 依赖从版本化模块下载。`go.mod` 是版本来源，`go.sum` 保存校验和；
构建不读取同级库目录。

## 当前兼容组合

| 模块 | 固定版本 |
| --- | --- |
| `github.com/gatework/go-net-connector` | `v0.4.0` |
| `github.com/gatework/go-expect` | `v0.4.1` |
| `github.com/gatework/go-vxlan` | `v0.1.0` |
| `github.com/gatework/go-wireless` | `v0.1.0` |
| `github.com/gatework/firemon` | `v0.0.0-20261029132834-19d1fb999feb` |
| `github.com/gatework/firemon/adapter/netconnector` | `v0.0.0-20261107151731-b5c7a0653d93` |
| `github.com/gatework/go-zabbix` | `v0.0.0-20261107160521-182600c51b9f` |

VXLAN 和无线库采用仍提供当前根包及 `adapter` 接口的 `v0.1.0`。
它们的 `v0.2.0` 已拆分能力包并将适配器移到 `adapter/netconnector`，升级需单独迁移调用方。
Firemon 引擎版本保持不变，适配器使用 GitHub 上的兼容提交，接入新版连接器的
响应和操作接口，避免同时改变防火墙领域模型。

## 验证与升级

```sh
GOWORK=off go mod download
python3 scripts/verify-delivery-dependencies.py
GOWORK=off make test
GOWORK=off go vet ./...
GOWORK=off make fmt-check checksilent
```

校验脚本检查实际解析的模块图，拒绝外部模块的本地路径替换，检查图中所有
`github.com/gatework/` 模块的版本与校验和，并运行 `go mod verify`。
新增或移除模块只需更新 `go.mod`，无需维护第二份模块名单。私有仓库需要正常的 GitHub 读取凭据；不要将凭据写入
`go.mod` 或仓库文件。

升级时指定 tag 或提交，运行 `GOWORK=off go mod tidy`，再运行上述验证。
设备类型识别、嵌套命令取消、写入意图、逐步回执、借用会话及关闭结果是兼容检查的一部分。
这些本地测试不代表真实设备验收。

示例和 `test/e2e-apporder` 中指向本项目及其测试应用的相对 `replace` 用于验证当前源码，
不属于外部依赖替换。
