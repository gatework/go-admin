# Network operations

`app/net` owns the network domain and follows the same layer structure as `app/admin`:

```text
app/net/
├── handler/             HTTP handlers, split into resource and capability files
├── router/              Route registration, authentication and permissions
├── service/             Application operations and worker construction
│   ├── dto/             Request and response contracts, grouped by capability
│   ├── inventory/       Inventory operations and source providers
│   ├── fabric/          Fabric operations
│   ├── wireless/        Wireless operations
│   ├── firewall/        Firewall operations and policy engine
│   ├── delivery/        Configuration workflows
│   └── diagnostics/     Diagnostic process management
└── models/              Domain records and persistence
    ├── inventory/       Devices, groups, circuits and inventory failures
    ├── fabric/          Topology and coordinated change records
    ├── wireless/        Wireless observations and collection records
    ├── firewall/        Policy records, value types and projection storage
    ├── delivery/        Workflow and receipt records
    ├── connection/      Shared device connection settings
    ├── grouping/        Device group lookup and persistence
    └── lease/           Device leases and Fabric locks
```

Handlers and routers share one package per layer. Services, DTOs and models retain capability subpackages so concepts such as Fabric and wireless collections remain distinct. Models do not import services or HTTP adapters; services do not import handlers, routers or Gin. `architecture.yaml` enforces these boundaries.

`cmd/api/net.go` is the host entrypoint. It calls `service.Workers` to construct background operations and `router.InitRouter` to register all network routes, then starts each worker with the host lifetime and joins it during shutdown. Worker construction deduplicates shared database handles and selects tenant roots deterministically. The router registers the diagnostics manager's shutdown hook once. The firewall integration fixture lives under `test/firewall/cmd`.

| Capability | Responsibility |
| --- | --- |
| `inventory` | Devices, groups, circuits, source synchronization, device connection settings and shared device leases |
| `fabric` | Fabric topology, assessment, collection and coordinated changes |
| `wireless` | Wireless inventory, client observations, collection and analysis |
| `firewall` | Firewall configuration collection, evidence, policy analysis and design; Firemon supplies the vendor-aware engine |
| `delivery` | Configuration workflows, receipts and device session ownership |
| `diagnostics` | Bounded network diagnostics and process ownership |

Shared access to a physical device goes through `models/lease`; a Fabric lock coordinates Fabric work and does not replace a device lease. No capability imports host commands. The host owns goroutine startup, cancellation and shutdown budgets.

HTTP adapters use `handler` packages and resource-oriented types and operations. Database API handles use `METHOD /route/:parameter`, which remains stable when Go packages or handler methods are renamed. HTTP URLs, request tags and permission scopes are independent of the Go package layout.
