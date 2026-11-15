Follow [go-standards/STANDARD.md v1.0.2](https://github.com/gatework/go-standards/blob/v1.0.2/STANDARD.md); run `make check` before delivery.

# motor backend

Use Go and tool versions from `go.mod`; run `make check` before delivery. The frontend is a separate repository and is outside this backend refactor.

## Architecture

Router → Handler → Service → Model. HTTP adapters live in singular `handler` packages; handler types name the business resource, with List/Get/Create/Update/Delete for CRUD operations. API handles are HTTP method plus route pattern, never runtime Go function names. DTOs live under `service/dto`. APIs bind and respond; services own business logic and never take `gin.Context`. Single-table CRUD uses the generic Actions; use explicit services for transactions and external calls. Keep `common/` independent of `app/`. All network capabilities belong under `app/net/{handler,router,service,models}`, following `app/admin`; capability subpackages belong inside the service, DTO and model layers. Read `app/net/README.md` before changing their composition or worker ownership.

Use `motor-core` request APIs with explicit dependencies (`api.New`, `api.Dependencies`, `API.Service()`). Check construction and binding errors. Services take `context.Context` first, use `DB.WithContext(ctx)`, and return errors; they do not accumulate errors on reusable objects. Responses use core `OK`, `PageOK`, and `Error`.

Choose the database from the current request's tenant. Never use a process-global database or driver to decide request queries. Apply data permissions to all applicable reads and writes. Preserve creator checks, auth-version atomic increments and transaction commit errors. Do not use GORM `Save` for updating an existing row: a concurrent delete must not resurrect it.

## Contracts and names

The canonical module is `github.com/gatework/motor`; shared contracts belong to `github.com/gatework/motor-core`. No legacy import aliases or deprecated API forwarding layers. Go names use ID, IDs, API, HTTP and URL. Project-owned sentinel errors use an Err suffix (InvalidErr, invalidUploadErr), explicitly overriding the go-standards Err prefix convention at the owner's request. Error types retain the Error suffix; third-party symbols retain their names. Explicit JSON/form/URI tags and database column names describe the business protocol independently of Go names.

A type representing a core contract must be an alias (`type X = pkg.Y`), not a new defined type that loses methods. `checksilent` checks this and the import boundary; verify its coverage counts rather than accepting an empty scan.

Models have explicit `TableName`. Retain `ControlBy` and millisecond `ModelTime` semantics; active rows use deleted_at=0. Runtime models must not embed frozen migration models. Preserve bigint decimal-string JSON contracts in generated DTOs and frontend-facing responses.

## Startup and ownership

Router and migration registrations finish before startup hooks execute. Registration `init` functions remain intentional; do not replace them without tracing startup order. The host owns its configuration watcher, logger roots, databases, cache, queues and workers. Subscribe queue handlers before Run. Propagate caller cancellation for request work; background work uses an application-owned lifetime, with bounded independent cleanup when durable outcomes must be recorded after cancellation.

Reload only retires resources owned by that generation. Shutdown must respect the configured drain/server/cleanup budget and container grace period. Preserve device leases, quarantine of unknown outcomes, and Fabric locks as distinct domain concepts; read `GLOSSARY.md` before changing them.

## Database and generated code

Product migrations belong in the tracked `cmd/migrate/migration/version/` directory. Filename prefixes are 13-digit millisecond versions. Freeze existing schema/seed effects and version identities; add migrations for corrections. Mechanical references may change to compile against the canonical module and core contracts. After soft-delete conversion, seeds use runtime `app/` models rather than nullable frozen migration models.

PostgreSQL generation uses the request database and `current_schema()`. Validate generated Go, JSON, TypeScript and forms together. SQLite builds require `-tags sqlite3`. The example application and installation tests are separate Go modules; test them explicitly.

## Local delivery

Preserve unrelated staged, modified and untracked work. Do not publish commits, versions or remote changes without authorization. Never record real credentials in source, logs or documentation. Use English Conventional Commit subjects; keep structural changes separate from behavior fixes.
