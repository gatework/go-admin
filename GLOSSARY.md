# motor network operations

The backend of motor, a network-operations platform: device inventory, configuration delivery, firewall, fabric and wireless work against real network devices.

## Device access

**Device lease**:
Exclusive, time-limited permission for one piece of work to open sessions to one device. A device has at most one lease at a time, whichever module holds it.
_Avoid_: device lock, reservation, claim

**Quarantine**:
The state of a device lease whose session outcome is unknown, so the device may still hold an unconfirmed change. A quarantined device takes no new work until an operator clears it by hand.
_Avoid_: blocked, frozen, stuck lease

**Fabric lock**:
Exclusive permission to coordinate work on one Fabric as a whole, independent of any device. It guards no device session, so it expires away rather than being quarantined.
_Avoid_: coordinator lease, negative lease

**Lease sweep**:
The pass that turns every expired device lease into a quarantine and drops every expired fabric lock, regardless of which module took them.
_Avoid_: lease recovery, cleanup
