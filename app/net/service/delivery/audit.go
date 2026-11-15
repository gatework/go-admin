package delivery

import (
	"fmt"
	"slices"
	"strings"

	models "github.com/gatework/motor/app/net/models/delivery"
)

// scriptsDigest identifies the scripts an order approves: a SHA-256 over each
// target's device and script hash, in device order.
func scriptsDigest(targets []models.Target) string {
	lines := make([]string, 0, len(targets))
	for _, t := range targets {
		lines = append(lines, fmt.Sprintf("%d:%s", t.DeviceID, t.ScriptSHA256))
	}
	slices.Sort(lines)
	return digest(strings.Join(lines, "\n"))
}

// finished reports whether an order status is terminal.
func finished(status string) bool {
	switch status {
	case "succeeded", "failed", "partially_failed", "uncertain", "cancelled":
		return true
	}
	return false
}
