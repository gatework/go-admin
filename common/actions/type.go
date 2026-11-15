package actions

import contractactions "github.com/gatework/motor-core/sdk/contract/actions"

// PermissionKey is core's constant itself, not a restated literal:
// PermissionAction sets the gin context key and GetPermissionFromContext reads
// it back, and an independent copy could silently drift from core's.
const PermissionKey = contractactions.PermissionKey
