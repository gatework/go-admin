# Fabric operations implementation

## Implemented

- Persistent manual and scheduled collections, default disabled settings (runtime 5 minutes, configuration 30 minutes). Scheduler reconciles inventory names and membership before selecting devices. Duplicate request keys are scoped by actor and immutable payload hash; scheduled keys include Fabric, mode and time bucket.
- Append-only per-dimension observations validate target identity, Fabric identity, capture timestamps, source connection version and membership versions. Older or out-of-order observations cannot supersede newer evidence. Connection failure records failed latest dimensions while prior complete observations remain available. Unsupported/missing-connection members retain explicit target outcomes and do not prevent collecting eligible members.
- Shared delivery device leases prevent collection/change races with generic delivery. Each Fabric uses a persistent coordinator lease for serial execution across processes. Runs have a 10 minute timeout and 15 minute leases. Expired physical device leases remain quarantined; expired coordinator leases can be removed because they own no transport. Interrupted writes become uncertain and pending later targets are cancelled, with no replay.
- Structured change previews select one explicit source Leaf and target Leaves; source configuration supplies every controlled value. Only `vlan_vni_add`, `svi_gateway_add`, and `svi_acl` are accepted. ACL binding requires a source policy and existing matching target ACL; `create_acl=false`; no raw script edits, ACL creation/deletion, startup save or rollback path exists.
- Plans persist source/target identity, connection versions, membership/rule grouping hash, configuration evidence/digests, generated commands and immutable plan hash. Execute and each write revalidate scope/active actor, current role grants, identity, lease ownership and grouping/evidence. Live source and target preflight readback detects unobserved drift. Writes durably record intent before send and acknowledgement afterward. `Eligible`, `Satisfied` and applicable runtime health gates stop later targets on failure or uncertainty.
- Fabric changes create real delivery orders/targets with kind `fabric`. Generic edit/action/retry and generic claim/recover cannot execute or modify Fabric plans. Fabric receipt states and delivery counts/statuses advance together.
- Private target storage has an explicit GORM serializer; JSON-hidden identity/connection/digest/readback fields survive DB reload. Public readback contains dimension status, issue codes and evidence hashes/command/timestamps; raw configuration records remain private.

## Startup and schema

Root startup calls `NewWorker(*Fabric, nil)`, then `Start()` and `Close(context.Context)`. Default native transport uses the shared device access (`app/net/service/deviceaccess`): the same credentials, profile overrides, host-key policy and connection settings as delivery. `NETWORK_DELIVERY_ENABLED=true` is required for writes; read-only collection requires configured device credentials. Tests inject `Transport` and never connect to real devices.

Migrate `models.Collection`, `models.Change`, and updated delivery `Order` (`Kind`, `FabricID`). Observation schema belongs to Fabric core. No extra target/receipt tables are required: operational target evidence is stored in the owning job alongside delivery targets.

## Verification

Focused tests cover DB reload, safe public evidence, duplicate requests, mixed support, failed latest versus previous complete, shared leases, schedules and rename reconciliation, preview provenance, generic delivery bypass, source/target drift, grouping drift, revoked authorization, durable unknown-write intent, stopped later targets, restart recovery, runtime faults and serial successful execution.

Verified locally:

- `go test ./app/net/... -count=1` passed.
- `go test -race ./app/net/service/fabric ./app/net/service/delivery -count=1` passed.
- `go vet ./app/net/...` passed.

Operations include 17 integration/regression tests plus 2 observation/dimension unit tests. Full repository gates, browser validation, migration/startup integration and dependency provenance are owned by the coordinating agent. No production device, remote deployment, commit or push was performed.

An uncertain write immediately appends failed current evidence unless its configuration has already passed Satisfied, preventing preflight evidence from authorizing another plan. Interrupted jobs do the same during recovery. Native transport returns explicit close receipts: confirmed closure releases the device lease even when policy checks fail; unknown closure retains quarantine.
