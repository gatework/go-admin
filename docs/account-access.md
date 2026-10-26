# 账户注册、邮件找回与 MFA

前后端分支：`feat/account-access`。参考 Motor 的账户流程；继续使用 Go 用户、角色和 JWT 契约。

## 使用入口

- 公开注册默认关闭。管理员在「系统设置」开启「公开注册」，登录页才显示创建账户入口。
- 注册需要邮箱验证码、用户名、姓名与确认密码。验证码有效期 10 分钟，每分钟最多重发一次；新账户使用按需创建的 `registered` 普通角色，不带管理菜单或授权。业务权限由管理员分配。
- 登录页「找回密码」发送重置链接及验证码。两者有效期 10 分钟，验证成功只能使用一次。新密码为 12–72 UTF-8 字节，旧 JWT 立即失效；MFA 保留。
- 「用户中心 → 修改密码 → 多因子认证」可绑定认证器。先验证当前密码，再扫描二维码或输入手动密钥，最后提交六位动态码。
- 启用后提供 10 个恢复码，只显示一次，可下载保存。每个恢复码只能使用一次。关闭 MFA 和重新生成恢复码均需要密码及动态码或恢复码，完成后重新登录。
- 密码验证成功后才进入 MFA 登录挑战，通过第二因素后才签发 JWT。挑战有效期 5 分钟，最多尝试 5 次。动态码防重放，恢复码在数据库事务中原子消费。

## 部署配置

先执行项目现有迁移命令，新增迁移为 `1792972800000_account_access.go`，创建策略、账户安全状态、挑战、恢复码及限流表。已执行的迁移不要修改。旧版没有凭据签名的 JWT 会被拒绝，升级后用户需重新登录。

| 环境变量 | 用途 |
| --- | --- |
| `ACCOUNT_SECURITY_KEY` | Base64 编码的 32 字节随机密钥，用于加密 TOTP 密钥和保护邮箱验证码 |
| `ACCOUNT_PUBLIC_URL` | 用户实际访问的前端基础 URL；生产使用 HTTPS，不从请求 Host 推断 |
| `ACCOUNT_SMTP_HOST` / `ACCOUNT_SMTP_PORT` | SMTP 地址与端口，默认端口 587 |
| `ACCOUNT_SMTP_FROM` | 发件地址 |
| `ACCOUNT_SMTP_USER` / `ACCOUNT_SMTP_PASSWORD` | SMTP 凭据，按邮件服务配置 |
| `ACCOUNT_SMTP_TLS` | `implicit` 使用隐式 TLS，否则要求 STARTTLS |
| `ACCOUNT_SMTP_INSECURE` | 仅本地邮件接收器可设 `true`，允许无 STARTTLS；默认关闭 |

用 `openssl rand -base64 32` 生成密钥，通过部署的私密配置注入，避免放入 Git、日志或截图。所有服务实例使用同一密钥；将其与数据库备份配套保管，不能在每次启动时重新生成。丢失或直接替换密钥会导致已绑定认证器不可解密，轮换应另做迁移。JWT 签名密钥沿用项目配置。

未配置邮件服务时界面明确禁用注册/找回；未配置安全密钥时不能启用 MFA。SMTP 连接有超时，生产默认校验证书。配置完整只表示可以尝试投递，不代表邮件供应商已验证可达。

## 本地验证

工作区根目录的 `docker-compose.account.yml` 为开发环境增加 Mailpit 和后端环境文件：

```sh
docker compose -f docker-compose.dev.yml -f docker-compose.account.yml up -d mailpit backend
```

`.local/account.env` 权限为 0600，目录为 0700。Mailpit 控制台仅绑定本机，打开 `http://localhost:8025` 查看测试邮件。当前本地注册策略保持关闭。此配置不向真实收件人发送邮件；生产 SMTP 需要另行配置验证。

## API

公共路径前缀 `/api/v1/account`：

| 方法与路径 | 输入 |
| --- | --- |
| `GET /settings` | 返回公开注册、邮件配置、MFA 配置状态 |
| `POST /registration/email` | `email`，返回验证挑战 token |
| `POST /register` | `username`, `name`, `email`, `password`, `passwordConfirmation`, `token`, `code` |
| `POST /password/forgot` | `email`；不在响应中返回重置 token |
| `POST /password/reset` | `email`, `token`, `code`, `password`, `passwordConfirmation` |
| `GET /mfa` | 当前账户 MFA 状态与剩余恢复码数量，需要 JWT |
| `POST /mfa/setup` | `password`，返回二维码与手动密钥，需要 JWT |
| `POST /mfa/cancel` | 取消待确认绑定，需要 JWT |
| `POST /mfa/enable` | `code`，返回一次性可见的恢复码，需要 JWT |
| `POST /mfa/disable`、`POST /mfa/regenerate` | `password` 和 `code` 或 `recoveryCode`，需要 JWT |
| `GET/PUT /registration-policy` | 管理员专用；PUT 输入 `enabled` |

登录仍用 `/api/v1/login`。首步返回 `mfaRequired/challengeToken/expiresIn` 时不包含 JWT；第二步仅提交 `challengeToken` 和 `totpCode` 或 `recoveryCode`。

## 安全与运维边界

验证码、挑战 token、恢复码只保存摘要；TOTP 密钥使用 AES-GCM 加密。注册只接收白名单字段，不接受客户端的角色或管理权限。敏感账户操作与登录、改密的审计记录不保存请求/响应正文。

限流保存在数据库，跨实例共享；按邮箱、账户和 TCP 对端地址限流。转发头不参与信任判断，反向代理部署需在可信入口配置用户级/IP 限流，避免所有用户共享代理地址的额度。忘记密码接口统一响应，但同步 SMTP 投递可能影响响应时间；高敏感公网部署应增加可靠邮件队列及响应时间隔离。

挑战和限流行含 `expires_at`；定期分批删除过期记录，勿删除未过期挑战或账户安全状态。注册会检查邮箱唯一性；既有重复邮箱不会获得找回邮件，需管理员先清理。失去认证器和全部恢复码时，本轮没有管理员绕过第二因素的入口。

## 验证范围

- Go 全量测试；账户/JWT 测试和 race 检查；迁移集成测试。
- PostgreSQL 同一动态码/恢复码并发消费，只有一个请求成功。
- 本地真实 HTTP + Mailpit：注册、非管理员权限边界、密码找回、旧令牌撤销、MFA 登录、恢复码轮换/关闭、审计脱敏。
- Vue 类型、lint、单测、构建及浏览器注册/找回/MFA/登录/用户中心回归；375px 与桌面截图。

API 令牌、自助会话管理、通知和已有账户联系邮箱变更验证仍未实现。

## 交付证据（2026-10-26）

前端 54 个文件、387 个单测通过；lint 无错误（41 项既有警告）、类型检查、生产构建及 500KB 首屏预算通过（452.53KB gzip）。账户浏览器 12 项、系统设置 6 项通过，包含延迟旧响应隔离、MFA 操作锁定、恢复码离开保护。独立安全复核关闭全部已报告问题，界面复核对本轮五项修复给出 ship。较早综合浏览器运行中两项并行 header 颜色断言失败，不属于本轮账户改动，也未作为通过证据。

配套登录验证码统一为六位，保留前导零并由现有验证码存储完成一次性核验。前后端需配套升级。
