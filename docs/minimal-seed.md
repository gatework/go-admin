# 最小开发种子

新安装的 `config/db.sql` 和 `config/db-sqlserver.sql` 只保留 admin 账户、默认部门、管理员角色和管理员岗位各一条，不再添加演示任务。系统菜单、权限、字典和必要参数保留；示例联系人和外链 logo 清空。密码散列保持原值，时间使用数据库执行时的 `CURRENT_TIMESTAMP`。

PostgreSQL 序列根据实际最大主键初始化，空表的首次编号从 1 开始。新增迁移 `1792940400000_arco_menu_icons.go` 按内置路由名称更新 Arco 图标，不修改自定义菜单；不会自动删除既有用户或重置既有业务时间。

## 已有本地开发库

`scripts/reset-local-seed-data.sql` 是需要明确授权才执行的开发清理脚本。先停止后台服务、备份数据库，再通过 psql 执行，成功后重启服务，避免旧任务继续驻留内存。脚本在事务内校验 admin、默认角色、部门和岗位；保留密码、必要权限关系和调度租约，删除非必要账户/组织、演示任务、生成器暂存和历史日志，并刷新保留种子的创建/更新时间。运行后产生的新日志是正常业务记录。


## 验证

- `go test ./config` 通过。
- `go test -tags integration ./config ./cmd/migrate/migration/version` 通过，覆盖图标迁移不修改自定义菜单和按钮。
- 独立 PostgreSQL 数据库 `motor_seed_check_20261025` 全部 16 个迁移成功；随后执行清理脚本，账户/部门/角色/岗位各 1、任务 0、调度租约 1、孤立菜单关系 0。
- 本地开发库读回相同核心计数。SQL Server 种子已同步修改，但未在 SQL Server 实例执行验证。
The motor development environment starts from fresh volumes and formal migrations. Historical database cleanup records do not describe a migration performed by this refactor.
