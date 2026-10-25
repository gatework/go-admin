# PostgreSQL 代码生成

代码生成器支持 MySQL 和 PostgreSQL。数据库类型取自当前请求的数据库连接，多租户不会使用进程默认数据库的类型来判断是否支持生成。

## 使用方式

1. 将目标租户配置为 `driver: postgres`，按项目原有迁移流程建立生成器的配置表。
2. 打开「代码生成」，从数据库表列表导入业务表。
3. 配置展示、查询和表单字段，预览代码后生成到现有后端目录及 `gen.frontpath`。

PostgreSQL 只读取当前连接的 `current_schema()`，也就是 `search_path` 中第一个有效 schema；默认通常为 `public`。需要其他 schema 时，在该租户 DSN 中配置，例如：

```yaml
driver: postgres
source: host=127.0.0.1 port=5432 user=app password=CHANGE_ME dbname=app sslmode=disable search_path=business
```

生产环境的 TLS 参数按实际部署设置。生成后的服务必须使用相同的 schema 配置，因为生成模型保留未限定 schema 的表名。不要把 `business.orders` 作为导入表名：schema 由连接决定，表名也用于生成文件路径。`gen.dbname` 保留给 MySQL 使用，不控制 PostgreSQL schema。

## 支持的数据形状

- 普通表和分区表；视图、外部表和临时表不列入候选。
- 单列主键：serial/identity、手动指定整数、UUID、字符串。没有主键或复合主键的表不能生成 CRUD。
- 表/列注释、字段顺序、可空性、默认表达式、字符长度、主键和自动编号信息。
- `smallint`/`integer`、`bigint`、`real`/`double precision`、`numeric`、布尔、字符/文本、UUID、date/timestamp/timestamptz、bytea。
- 可空业务字段使用可空 Go 类型；bytea 的 JSON 值为 Base64 字符串或 null。计算列只读，不进入新增/修改表单，也不写入数据库。

业务字段和主键中的 `bigint` 使用 Go int64，JSON 和前端使用十进制字符串，避免 JavaScript Number 丢失精度；新增、读取、更新、删除均遵守该约定。框架审计字段 `create_by` / `update_by` 保持已有的 int / JSON number 契约。`numeric` 使用字符串保留十进制精度。date 和 timestamp 使用现有日期控件及 RFC3339 时间格式。

数组、JSON/JSONB、范围、domain、enum 和其他自定义类型暂不自动推断表单，导入时明确拒绝。PostgreSQL 表名和字段名使用小写字母开头的小写字母、数字及下划线分隔词，例如 `biz_order`、`order_id`；不接受连续或末尾下划线、带引号的大写名称。表名不能使用 PostgreSQL 保留关键字，例如 `order`。不能表达的名称和转换后重名会返回错误。

## 审计字段

当前生成模板嵌入框架的 `ModelTime` 和 `ControlBy`。业务表须包含以下字段：

| 字段 | PostgreSQL 类型 |
|---|---|
| `create_by`、`update_by` | integer 或 bigint |
| `created_at`、`updated_at` | timestamp 或 timestamptz |
| `deleted_at` | bigint NOT NULL，活跃行为 0 |

这些字段由框架写入，不能是计算列或自动编号列。`deleted_at` 使用框架的整数软删除标记，不能用 PostgreSQL timestamp 类型替代。

生成器会在 PostgreSQL 导入阶段检查这些前提。它只写入生成配置和代码文件，不会自动给业务表添加或修改字段。数据库默认表达式不会直接成为浏览器表单代码；需要表单默认值时在生成配置里显式设置。

## 验证

仅对专用测试数据库运行：

```sh
GO_ADMIN_TEST_POSTGRES_DSN='host=127.0.0.1 port=5432 user=postgres password=test dbname=goadmin_test sslmode=disable' \
  go test -race ./app/other/models/tools ./app/other/apis/tools -count=1
```

PostgreSQL 测试建立并清理独立 schema。端到端测试覆盖表/列列表、导入、九类预览产物、生成代码的 vet/编译，以及生成接口的真实数据库 CRUD。现有 MySQL 回归继续使用 `GO_ADMIN_TEST_MYSQL_DSN`。CI 已提供这两个数据库连接，因此 PostgreSQL 测试不会仅依赖 SQLite 模拟。

元数据语义依据 PostgreSQL 的 [pg_attribute](https://www.postgresql.org/docs/current/catalog-pg-attribute.html) 与 [系统信息函数](https://www.postgresql.org/docs/current/functions-info.html) 文档。
