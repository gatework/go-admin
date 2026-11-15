// Package deviceaccess is how network work reaches a device: the server-owned
// credentials and host-key policy, the connector profile inferred from
// inventory, and the device's stored connection settings resolved against
// both. Delivery, Fabric and wireless use it; firewall uses its credentials
// and transport settings but infers its own (firemon) vendor.
//
// Credentials come only from the backend environment and never leave the
// process: they are secret.Secret, revealed only into a connector config.
package deviceaccess
