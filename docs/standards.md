# Shared engineering checks

motor adopts go-standards v1.0.2. `.golangci.yml` and `standard.mk` are exact published copies distributed by pinned stdsync; do not edit the copies. `make check` is the CI and local entrypoint for consistency, formatting, tidy verification, vet, lint, architecture, shuffled race tests, vulnerability reachability, and host-specific checks through CHECK_EXTRA.

`architecture.yaml` declares all production packages and forbids infrastructure dependencies on application wiring, domain records on HTTP wiring, application services on host commands, and policy engine dependencies on request adapters. New packages must be declared.

## Explicit standard conflicts

MOD-1 specifies Go 1.26.0. motor retains the approved Go 1.27.4 minimum required by motor-core v0.1.0. Downgrading only the directive would produce an invalid dependency graph; resolving this requires an organization-wide standard update.

PKG-1, CMP-3 and LOG-1 still require architecture review against this inherited host: application modules retain router/handler/service/model subpackages, startup registries register before sealing, and logging uses the canonical motor-core logger contract. Existing init registration has narrowly scoped explanations; no lint rule or distributed configuration is disabled. Passing automated checks does not claim all review-only architecture rules have been implemented.

The frontend remains outside this backend work. Its unchanged static checkers still parse prior Go type and seeder spellings and require a later frontend update. Existing migrations preserve schema history while references and type spelling follow the canonical module.

## Project error naming override

At the owner's explicit request, project-owned sentinel error variables use an `Err` suffix: `InvalidErr` when exported and `invalidUploadErr` when private. Error types retain the conventional `Error` suffix. This is a project convention, not the usual Go `ErrInvalid` convention. It overrides the sentinel naming requirement in go-standards and the naming skill. Narrow `errname`, staticcheck `ST1012` and revive `error-naming` directives apply only to the affected variable declarations; the distributed linter configuration is unchanged. The `checksilent` sentinel-error-name check enforces the replacement convention, including nested examples. Dependency symbols such as `gorm.ErrRecordNotFound` and `os.ErrNotExist` retain their upstream names.

## Network composition and HTTP names

Network capabilities follow `app/admin` under `app/net/{handler,router,service,models}`, with one host composition entrypoint. Handler and router files share their respective packages; services, DTOs and models use capability subpackages. Architecture checks prevent model dependencies on services or HTTP wiring and service dependencies on HTTP wiring. HTTP adapters use singular `handler` packages and resource names; legacy Sys prefixes, numeric Get2/Update2 variants and ForService suffixes are removed from the HTTP boundary. API registration persists method plus route pattern rather than runtime function names. The initial motor identity migration normalizes owned legacy API handles while retaining custom metadata.

Frozen migration records retain their original field order. Their positional SQL seed inserts depend on the resulting historical column order; declaration-local embeddedstructfieldcheck exceptions preserve this contract. Fresh installation tests validate it. Runtime models follow the current embedding rule.
