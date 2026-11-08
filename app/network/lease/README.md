# Device leases

`app/network/lease` holds the [device lease](../../../GLOSSARY.md) rules for every network module that opens sessions to devices: delivery, firewall collection, fabric and wireless. The rules are stated in the package documentation in `lease.go`, and only this package reads or writes `sys_delivery_device_lease`. The table keeps its original name because delivery created it first.

A lease lasts the caller's run timeout plus `lease.Margin` (5 minutes). Leases are never renewed. Expiry uses the Go process's UTC clock, so the margin also absorbs clock skew between replicas.

When work reaches a quarantined device, each module fails it visibly with `device_quarantined` instead of leaving it queued. Other devices keep scheduling.

## Reconciling a quarantined device

An expired lease is **never reused automatically**, because the old process might still be able to write. A lease sweep (run by every module's recovery) turns it into a quarantine. A quarantine stays until an operator clears it by hand. This is deliberately not an API button.

1. Disable the module that held the lease on every replica (for delivery, `NETWORK_DELIVERY_ENABLED=false`) and stop all worker processes. Confirm that their SSH/Telnet child processes and sessions have actually ended. A database expiry alone is not evidence that a device session ended.
2. Inspect that module's job rows for the lease, using its `token` and `target_id`. For delivery, inspect `sys_delivery_target` and `sys_delivery_receipt`. Read the actual device configuration through an independently authorised session. Record in the operational change record what was applied, what remains unknown, and who reviewed it. Do not change an uncertain job back to pending.
3. In the correct tenant DB, delete only the reconciled lease, using explicit device ID, token and `quarantined=true` predicates. For example, within a transaction: `DELETE FROM sys_delivery_device_lease WHERE device_id = :device AND token = :token AND quarantined = true;`. Verify exactly one affected row. Leave the original job rows and receipts untouched.
4. If further changes are required, author a new reviewed job based on the device's actual current state. Re-enable the module only after all replica configuration and known-hosts state are consistent.

Fabric locks (negative `device_id`) guard no session and are dropped automatically once expired; they never need this procedure.
