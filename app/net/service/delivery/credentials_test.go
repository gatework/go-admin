package delivery

import (
	"fmt"
	"strings"
	"testing"
)

func TestProfileCredentialsOverrideFieldByFieldAndNeverPrint(t *testing.T) {
	t.Setenv("NC_DEVICE_USERNAME", "shared-user")
	t.Setenv("NC_DEVICE_PASSWORD", "shared-password")
	t.Setenv("NC_DEVICE_ENABLE_PASSWORD", "shared-enable")
	t.Setenv("NC_H3C_PASSWORD", "h3c-password")
	c := FromEnvironment()

	h3c := c.credentials("h3c")
	if h3c.Username != "shared-user" || h3c.Password.Reveal() != "h3c-password" || h3c.EnablePassword.Reveal() != "shared-enable" {
		t.Fatalf("h3c credentials %q %q %q", h3c.Username, h3c.Password.Reveal(), h3c.EnablePassword.Reveal())
	}
	if other := c.credentials("unknown"); other.Password.Reveal() != "shared-password" {
		t.Fatal("a profile without overrides lost the shared credentials")
	}
	for _, printed := range []string{fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), fmt.Sprint(c)} {
		for _, raw := range []string{"shared-password", "shared-enable", "h3c-password"} {
			if strings.Contains(printed, raw) {
				t.Fatalf("printing the config leaked %q: %s", raw, printed)
			}
		}
	}
}
