# Network delivery

`app/net/service/delivery` owns the `/api/v1/network-delivery` workflow. All routes require JWT, Casbin and the host's DataScope middleware. APIs bind bounded DTOs; services own transactions and database access. Credentials, scripts and device output never enter ordinary API/operation logs. The private delivery tables intentionally retain scripts and output; restrict database and backup access accordingly.

## Configuration

The server defaults to disabled. History and device connection metadata remain available while disabled.

| Variable | Meaning |
| --- | --- |
| `NETWORK_DELIVERY_ENABLED` | Only `true` enables creation and execution; default false |
| `NETWORK_DELIVERY_CONCURRENCY` | Workers per tenant database per process, 1–20, default 5 |
| `NC_DEVICE_USERNAME` / `NC_DEVICE_PASSWORD` | Server-side default login credentials |
| `NC_DEVICE_ENABLE_PASSWORD` | Optional privilege credential |
| `NC_<PROFILE>_USERNAME` / `PASSWORD` / `ENABLE_PASSWORD` | Override defaults for an uppercased profile, e.g. `NC_CISCO_IOS_USERNAME` |
| `NC_HOST_KEY_POLICY` | `strict` by default; explicit `accept_new` supported; replacement/unchecked policies rejected |
| `NC_KNOWN_HOSTS` | Persistent writable known-hosts file for enrolled SSH hosts; mount persistently |

Profiles come from the connector registry. An inventory device needs no connection row: its effective connection defaults to SSH/22, version 0, and a profile inferred from vendor, OS, model and device type. The resolver reuses the connector's Netdisco rules. Cisco IOS/IOS-XE and NX-OS require distinguishing OS or model evidence; H3C wireless types (or wireless roles when type is unset) select `h3c_wireless`. Unsupported families such as ASA, IOS XR and ACI, contradictory OS/model evidence, and unknown inventory remain unavailable with `reason: "profile_unknown"`; a generic Cisco label does not imply IOS. Reads never materialize connection rows.

Connection responses include `profileSource: "automatic" | "manual" | "unknown"` and `inferredProfile` (empty when unresolved). A registered nonempty profile is a manual override. Sending an empty profile restores inference while retaining optimistic version checks; for example `{"version": 2, "profile": "", "protocol": "ssh", "port": 22}`. Restoring automatic mode keeps a versioned row with an empty stored profile, so stale edits cannot reset a newer choice. Protocol and port remain explicit overrides in that row. Explicit Telnet is supported at its configured port; there is no automatic protocol fallback. Readiness checks credentials and the selected transport binary. Container builds install OpenSSH and a Telnet client. IP is always read from inventory; it is never accepted as an execution request field. Only active inventory devices can be scheduled.

The existing options endpoint returns permission-scoped `inventoryGroups: [{deviceType, deviceRole, vendor, model, count}]`, including unclassified values as empty strings. It also returns `deviceGroups: {items, totalDeviceCount, ungroupedDeviceCount}` by reusing the network device-group tree service. Tree nodes retain `directDeviceCount`, subtree `deviceCount`, and `children`; all device counts cover visible, nondeleted inventory in all statuses. The delivery devices endpoint reuses network pagination, keyword, role/type/vendor/model/status and group filtering, then loads connection overrides in one batch. `groupId` selects a region and its descendant groups; `groupId=0` selects ungrouped devices. `deviceRoleEmpty=true`, `deviceTypeEmpty=true` and `vendorEmpty=true` select exact unclassified values (including legacy NULLs); each takes precedence over its corresponding string filter. For example, `?groupId=3&deviceRole=access_switch&vendor=Cisco&model=C9300` filters a region by role and platform inventory, while `?deviceRoleEmpty=true&deviceType=switch` selects switches without a role.

New target snapshots include `deviceRole`, `deviceType` and the effective `profile` for grouped editing. New known-empty role/type values remain empty strings in JSON; historical missing values remain omitted. Historical snapshots with no profile are supplemented only from their saved connection, never current inventory. Create/edit targets may include `expectedProfile`, `expectedDeviceRole` and `expectedDeviceType`; a mismatch with current scoped inventory returns HTTP 409 before the order is written. Omit an expectation when the historical value is unknown; an explicit empty string asserts that the value is unclassified. Existing per-device script payloads remain valid without these optional fields.

Limits: 600-second grace, 100 targets, 64 KiB per script, 4 MiB total scripts, 4 MiB retained output per target, 10-minute target execution timeout, 15-minute lease expiry. JSON input is bounded to 24 MiB plus 64 KiB to accommodate escaped scripts. Script sizes are UTF-8 byte lengths. Blank lines and `#` comments follow the connector parser; invalid or empty scripts reject the entire order before any connection. Saving a scheduled draft restarts the grace period. Scripts are raw CLI: there is no implicit save, rollback or configuration wrapper.

## Durable execution and cancellation

The host constructs delivery workers through `app/net/service.Workers`, starts them with its lifetime and joins them during shutdown. A timer scans due scheduled/queued work independently of browser polling. A conditional order/target update claims each target once; a unique database device lease prevents simultaneous writes from other jobs or processes. Slow network I/O holds no database transaction. One connector operation executes the full parsed script, preserving vendor hook state (including H3C generated rule identifiers). Vendor prepare and nested hook sends use the same write gate. Workers revalidate inventory identity (IP, hostname, vendor, model, active status), effective inferred or manual profile, connection version, lease ownership and cancellation before login, after login, and before every command. New grouped snapshots also recheck role/type; old snapshots without these facts retain their existing identity checks. Inventory OS changes that resolve to a different driver stop execution even when no connection row exists. Fabric reads and source-hash checks use the same effective connection resolver.

Write intent is persisted before entering the connector. Acknowledged command receipts and progress are committed after every command, under a separately bounded persistence context so concurrent cancellation cannot discard an acknowledgement. Any failure after write intent is `uncertain`, even where the device may have rejected the command. This deliberately prefers uncertainty to unsafe replay. Only `failed` targets with no write intent can enter a newly linked retry order; original orders and receipts are preserved. A first retry validates the original IP, hostname, vendor, model, profile and recorded role/type; changed identity or missing legacy grouping returns HTTP 409 and requires selecting the target again in a new draft. An existing retry request key is recovered before current inventory checks, so a lost response remains recoverable after later inventory changes. An idempotency key is unique per creator, and its immutable original payload digest rejects reuse with different content even after the order is edited.

Cancellation marks pending targets cancelled immediately and interrupts active local contexts. It prevents subsequent sends once observed; commands already in flight cannot be recalled. Completed steps/output remain visible. A local transport is closed before its lease is released. Failed close, lost worker or expired lease marks uncertainty and quarantines the device. Neither process recovery nor HTTP retry replays uncertain writes.

## Reconciling a quarantined device

Device leases, quarantine and the manual reconciliation procedure are owned by [`app/net/models/lease`](../../models/lease/README.md), shared with firewall, fabric and wireless. Recovery keeps output and receipts and changes the running target to `uncertain` after its lease expires. Pending targets for a quarantined device fail visibly with `device_quarantined`.

## Dependency provenance

The connector and terminal modules are downloaded from GitHub at the versions pinned in `go.mod`, with checksums in `go.sum`. Delivery uses the published vendor registry and keeps its cancellation, durable write-intent and receipt hooks around the connector's operation scope. Builds need no sibling library checkout or local `replace` directive.

Verify with `python3 scripts/verify-delivery-dependencies.py`. It disables Go workspaces, rejects local replacements and verifies downloaded module checksums. See [GitHub dependencies](../../../../docs/github-dependencies.md) for the compatible version set and upgrade checks.

## Validation

`go test -race ./app/net/... ./common/middleware ./cmd/migrate/migration/version` covers order validation and idempotency, optimistic versions, HTTP bounds/envelopes, DataScope, durable due scanning, competing claims, cross-job device exclusion, slow-login cancellation, identity changes, acknowledged receipts, failed sends, safe retry, conservative recovery, MySQL storage limits, sensitive operation-log suppression and idempotent migration permissions. The connector adapter test uses `net.Pipe` only; worker lifecycle tests use injected local sessions. No real device was connected or modified.

`go test -race ./app/net/... -count=1` additionally verifies registry-backed profile inference, ambiguous/unsupported platforms, row-free defaults, manual overrides and restoring automatic mode, scoped role/type/vendor facets, explicit empty filters, historical grouping snapshots, selection-to-submit changes, inferred driver/role changes during execution, and Fabric source revalidation. The repository gate is `make check`. External integration tests require their dedicated test environment variables and are separate evidence from these offline checks. This defaults follow-up changes no schema or dependency snapshot.

Migration `1793005000000` adds six tables and menu `NetworkDeliveryManage`. Its permissions are `network:delivery:{list,query,add,edit,execute,cancel,retry,connection}`; each action includes its required read dependencies. Run the normal host migration command before server startup.
