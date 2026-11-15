# Device access

`app/net/service/deviceaccess` is how network work reaches a device. Delivery, Fabric and wireless use all of it; firewall uses its credentials and transport settings but infers its own (firemon) vendor.

- **Credentials and host keys** (`Config`, `FromEnvironment`): `NC_DEVICE_USERNAME` / `NC_DEVICE_PASSWORD` / `NC_DEVICE_ENABLE_PASSWORD`, per-profile overrides `NC_<PROFILE>_USERNAME|PASSWORD|ENABLE_PASSWORD` (field by field), `NC_HOST_KEY_POLICY` (`strict` by default, or `accept_new`) and `NC_KNOWN_HOSTS`. Passwords are `secret.Secret`: printing or logging a `Config` shows `[redacted]`, and they are revealed only into a connector config that is never stored.
- **Profile inference** (`InferProfile`): the connector profile for an inventory row, from its vendor, OS and model. Unsupported, ambiguous or contradictory evidence resolves to no profile rather than a guess. Nothing contacts the device.
- **Resolution** (`Resolve`, `Effective`): the device's stored connection settings (`sys_delivery_connection`), with the inferred profile filled in when none was chosen, `profileSource`, and readiness (`ready`, `reason`).

- **Use** (`Read`, `Write`, `Check`): a session that only reads (firewall, Fabric and wireless collection) may open to a device that is `pending` or `active`; one that writes (delivery, Fabric changes) only to an `active` device. `maintenance`, `disabled` and `retired` devices are opened for nothing. Readiness reasons, in the order checked: `device_unavailable`, `profile_unknown` (connector profiles; firewall checks its own vendors and adds `profile_mismatch`), `connection_invalid`, `address_invalid`, `credentials_missing`, `transport_unavailable`.

Delivery's own switches (`NETWORK_DELIVERY_ENABLED`, `NETWORK_DELIVERY_CONCURRENCY`) stay in `service/delivery`.
