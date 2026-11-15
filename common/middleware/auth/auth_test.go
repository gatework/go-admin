package auth

import (
	"testing"
	"time"

	"github.com/gatework/motor/common/account"
)

func TestCredentialStampUsesCanonicalInstants(t *testing.T) {
	instant := time.Date(2026, 10, 25, 1, 2, 3, 123456000, time.UTC)
	var user account.User
	var role account.Role
	user.CreatedAt = instant
	role.CreatedAt = instant
	expected := credentialStamp(user, role, []byte("test-key"))
	user.CreatedAt = instant.In(time.FixedZone("local", 8*60*60))
	role.CreatedAt = user.CreatedAt
	if got := credentialStamp(user, role, []byte("test-key")); got != expected {
		t.Fatal("same instant with another timezone invalidated token")
	}
}
