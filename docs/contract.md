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
| Served only to a scraper presenting `Authorization: Bearer <extend.metrics.token>`; `404` when no token is configured, `401` for a wrong one. It used to answer anyone. | `GET /api/v1/metrics` | None in the UI. Configure the scrape job with the token. |
| Requires a Casbin grant (route check) in addition to sign-in; it was excluded from the check. | `POST /api/v1/public/uploadFile` | Used by the system settings page (logo upload); non-admin roles that edit settings need this API granted. |
| Every error envelope is sent with its own HTTP status (4xx/5xx) instead of HTTP 200; the body is unchanged (`{code,msg}` still carries the same code). Covers authentication failure (401), permission denial (403), demo-mode refusal (403, body code was 500), rate limiting (429) and handler errors. MFA challenges stay 200. | All routes | Treat non-2xx as the error path: the HTTP client must read `code`/`msg` from the body of a rejected response instead of assuming every response is 200. |
| Tokens are no longer accepted from the `?token=` query parameter, except on the log websocket upgrade (`/ws/...`), where the token is moved into the header and stripped from the URL. Tokens live for `jwt.timeout` in every mode (dev used to issue 100-year tokens). | All authenticated routes | None for normal requests (the UI sends `Authorization`). Dev sessions now expire after `jwt.timeout`. |
| CORS is granted only to origins listed in `extend.cors.origins` (exact match); responses no longer carry `Access-Control-Allow-Origin: *`. Preflight answers `204`. The shipped dev configs list `http://localhost:9527`. | All routes | A UI served from another origin must be listed. Same-origin deployments need nothing. |
| Generic CRUD update and delete answer `404` "对象不存在或无权操作" for a row that is missing or outside the caller's data permission (was `403` "无权更新/删除该数据"); a request the model cannot be built from answers `422` (was `500`). | Routes mounted with `common/actions` (demo products, jobs and future single-table modules) | Treat 404 on update/delete as "gone or not yours". |
| A password a user sets for themselves must be 12–72 bytes, as at account registration; otherwise `400` "新密码需为 12–72 字节". Administrator-set passwords (create user, reset password) are not checked yet. | `PUT /api/v1/user/pwd/set` | The profile form's 6–20 character rule and hint need updating to 12–72. |
| The role list no longer includes each role's `sysMenu` (it is `null`); the UI loads a role's menus with `menuIds` when editing it. | `GET /api/v1/role` | None: the UI does not read `sysMenu` from the list. |
| A create or update that breaks a unique key (for example a configuration key that already exists) answers `409` "数据已存在" (or code `duplicate` from routes using `request.Fail`) instead of `500`. | Generic CRUD routes; routes using `request.Fail` | Show the conflict to the user instead of a generic failure. |
| Starting and stopping a job use `POST` (were `GET`). Migration `1794787200000_job_state_routes_post` moves existing `sys_api` rows and Casbin grants to `POST`, so roles keep access. Unknown job → `404`; failures no longer echo internal text. | `POST /api/v1/job/start/:id`, `POST /api/v1/job/remove/:id` | Change `startJob`/`removeJob` in `src/api/job/sys-job.ts` to `method: 'post'`. |
| HTTP job targets must be absolute `http`/`https` URLs, and may not be loopback, link-local (including `169.254.169.254`), unspecified or multicast addresses; saving one answers `422`. Private ranges stay allowed. The scheduler also refuses such addresses at connect time, ignores proxy environment variables, and records a job whose target answers 4xx/5xx as failed. | `POST/PUT /api/v1/sysjob` | Show the validation message for a refused target. |
