# motor application contracts

The host module is `github.com/gatework/motor`. Shared model, DTO, permission, migration and seed contracts are versioned in `github.com/gatework/motor-core/sdk/contract/`. Applications should consume those canonical packages instead of depending on a particular host checkout. See the pinned core migration guide for the exact public surface.

## Requests and services

Create request-local `api.API` values through `api.New` with explicit database, logger and cache dependencies. `Bind` merges query, the content-type-selected body and URI values, then validates. URI values have final precedence. Multipart upload handlers parse their own bounded body. Services receive `context.Context` as the first argument and return explicit errors.

Database selection is per tenant and per request. Read and write scopes must preserve data permissions; missing permission information is not permission to see all rows. Standard responses use core helpers and the `{code,data,msg}` envelope. The Go identifier convention does not determine JSON tag spelling.

## Models and persistence

Use explicit table names and canonical `ControlBy`/`ModelTime` types. Active soft-delete markers are zero milliseconds, not SQL NULL. Keep creator audit fields and atomic authentication-revocation increments correct. Existing-row updates check query errors and affected rows; never use Save as an update shortcut.

New schema and seed corrections are versioned migrations. Product migrations are tracked; local-only migration files are not a delivery mechanism. Register application routes and migrations before the startup registry is sealed. The example application and its independent installation-test module exercise this contract.

## Runtime ownership

The host owns resources it constructs, including configuration watchers, logger roots, databases, cache, queues and workers. Request cancellation propagates to request I/O. Accepted background work has an application lifetime. Queue handlers use context and Message; Subscribe precedes Run, and Shutdown receives an explicit budget. Resource generations are retired only by their owner.

## Frontend integration

The frontend remains a separate, unchanged repository. Its checks accept explicit backend and core paths supplied by the workspace. Packaged application sources stay outside its generated `src/apps/` directory. Menu component paths, permission identifiers and component names remain business contracts; an internal Go rename is not a reason to change them.

## HTTP contract changes awaiting frontend follow-up

Backend-only changes that alter what a client sees. Each entry names what the frontend must do.

| Change | Endpoint | Frontend follow-up |
| --- | --- | --- |
| Configuration keys not marked frontend (`is_frontend <> '1'`) are returned only to roles allowed `GET /api/v1/config`; otherwise the answer is `404`, the same as a missing key. A missing key used to answer `500`. | `GET /api/v1/configKey/:configKey` | The user page reads `sys_user_initPassword` through this route. Non-admin user managers need the configuration read grant, or the page should stop pre-filling the initial password. |
