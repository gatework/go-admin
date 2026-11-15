# motor

Network operations backend built with Go, Gin, GORM and Casbin.

Device inventory, diagnostics, configuration delivery, Fabric, wireless and firewall operations are organized under `app/`. Framework contracts come from the versioned `github.com/gatework/motor-core` module.

## Development

Use Go 1.27.4 or newer. Run `make check` for formatting, contract checks, build, vet, race tests and application installation tests. PostgreSQL, MySQL, SQL Server and Redis integration checks require dedicated test services; see the CI workflow for environment variables.

```sh
go build -o motor .
./motor migrate -c config/settings.yml
./motor server -c config/settings.yml
```

SQLite builds require `-tags sqlite3`. Configure real credentials outside version control. Development Compose configuration is maintained in the parent workspace and initializes a separate motor environment.

Read `AGENTS.md`, `docs/contract.md` and the module documentation before changing resource ownership, tenant selection, data permissions or device leases.

Upstream attribution: derived from go-admin-team/go-admin; original MIT license and authorship are retained in LICENSE.md.
