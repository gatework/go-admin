package secret

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

type holder struct {
	User     string
	Password Secret
}

func TestSecretNeverPrints(t *testing.T) {
	const raw = "device-password-123"
	h := holder{User: "netops", Password: raw}
	outputs := []string{
		fmt.Sprintf("%v", h), fmt.Sprintf("%+v", h), fmt.Sprintf("%#v", h),
		fmt.Sprintf("%s", h.Password), fmt.Sprintf("%q", h.Password), fmt.Sprintf("%x", h.Password), fmt.Sprint(h.Password),
	}
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	outputs = append(outputs, string(data))
	var logged bytes.Buffer
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("connecting", "password", h.Password, "config", h)
	outputs = append(outputs, logged.String())
	for _, out := range outputs {
		if strings.Contains(out, raw) || strings.Contains(out, fmt.Sprintf("%x", raw)) {
			t.Errorf("credential leaked: %s", out)
		}
	}
	if h.Password.Reveal() != raw || !h.Password.IsSet() {
		t.Fatal("Reveal does not return the credential")
	}
	if Secret("").String() != "" {
		t.Fatal("an absent credential must print as empty, so it can be told apart")
	}
}
