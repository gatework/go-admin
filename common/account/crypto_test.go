package account

import (
	"strings"
	"testing"
	"time"
)

func TestSecretEncryptionAndTOTPReplay(t *testing.T) {
	s := Service{Key: strings.Repeat("k", 32)}
	sealed, err := s.seal("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "JBSWY") {
		t.Fatal("plaintext secret persisted")
	}
	plain, err := s.open(sealed)
	if err != nil || plain != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("decrypt: %q %v", plain, err)
	}
	s.Key = strings.Repeat("x", 32)
	if _, err = s.open(sealed); err == nil {
		t.Fatal("wrong key accepted")
	}
	now := time.Unix(1700000000, 0)
	code, _ := otpCode("JBSWY3DPEHPK3PXP", now)
	step := verifyStep("JBSWY3DPEHPK3PXP", code, now, 0)
	if step == 0 || verifyStep("JBSWY3DPEHPK3PXP", code, now, step) != 0 {
		t.Fatal("TOTP replay accepted")
	}
}
