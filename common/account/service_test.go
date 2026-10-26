package account

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type inbox struct {
	body string
	fail bool
}

func (m *inbox) Send(_ context.Context, _, _, body string) error {
	if m.fail {
		return errors.New("SMTP unavailable")
	}
	m.body = body
	return nil
}
func accountTest(t *testing.T) (Service, *inbox, *time.Time) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "account.db")), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&User{}, &Role{}, &State{}, &Challenge{}, &Recovery{}, &Limit{}, &Policy{}); e != nil {
		t.Fatal(e)
	}
	db.Create(&Policy{ID: 1})
	db.Create(&Role{RoleId: 1, RoleKey: "admin", Status: "2", Admin: true})
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin-password-123"), bcrypt.MinCost)
	db.Create(&User{UserId: 1, Username: "admin", Email: "admin@example.com", Password: string(hash), Status: "2", RoleId: 1})
	now := time.Unix(1800000000, 0)
	box := &inbox{}
	return Service{DB: db, Key: strings.Repeat("k", 32), Mail: box, BaseURL: "https://motor.example", Now: func() time.Time { return now }}, box, &now
}
func emailCode(body string) string { return regexp.MustCompile(`[0-9]{6}`).FindString(body) }
func TestRegistrationPolicyVerifiedEmailAndOrdinaryRole(t *testing.T) {
	s, box, _ := accountTest(t)
	ctx := context.Background()
	if _, e := s.IssueEmail(ctx, "register", "new@example.com"); !errors.Is(e, ErrDisabled) {
		t.Fatal(e)
	}
	if e := s.SetPolicy(true); e != nil {
		t.Fatal(e)
	}
	token, e := s.IssueEmail(ctx, "register", "NEW@example.com")
	if e != nil {
		t.Fatal(e)
	}
	r := Registration{Username: "new-user", Name: "New", Email: "new@example.com", Password: "new-password-123", PasswordConfirmation: "new-password-123", Token: token, Code: "wrong"}
	if e = s.Register(r); !errors.Is(e, ErrInvalid) {
		t.Fatal("wrong code accepted", e)
	}
	r.Code = emailCode(box.body)
	if e = s.Register(r); e != nil {
		t.Fatal(e)
	}
	var u User
	s.DB.Where("username = ?", r.Username).First(&u)
	var role Role
	s.DB.First(&role, u.RoleId)
	if role.Admin || role.RoleKey != "registered" || u.RoleId == 1 {
		t.Fatal("privileged registration", role)
	}
	if e = s.Register(r); e == nil {
		t.Fatal("replayed verification")
	}
}
func TestResetOneUseAndCredentialRevocation(t *testing.T) {
	s, box, _ := accountTest(t)
	old, _ := s.User(1)
	oldStamp := Stamp("jwt", old.Password, 0)
	if _, e := s.IssueEmail(context.Background(), "reset", old.Email); e != nil {
		t.Fatal(e)
	}
	token := regexp.MustCompile(`token=([^&]+)`).FindStringSubmatch(box.body)[1]
	r := Reset{Email: old.Email, Token: token, Code: emailCode(box.body), Password: "changed-password-123", PasswordConfirmation: "changed-password-123"}
	if e := s.ResetPassword(r); e != nil {
		t.Fatal(e)
	}
	got, _ := s.User(1)
	if bcrypt.CompareHashAndPassword([]byte(got.Password), []byte(r.Password)) != nil || oldStamp == Stamp("jwt", got.Password, 0) {
		t.Fatal("password or stamp unchanged")
	}
	if e := s.ResetPassword(r); e == nil {
		t.Fatal("reset replay accepted")
	}
}
func TestMFALoginReplayRecoveryAndDisable(t *testing.T) {
	s, _, now := accountTest(t)
	setup, e := s.Setup(1, "admin-password-123")
	if e != nil {
		t.Fatal(e)
	}
	code, _ := otpCode(setup.Secret, *now)
	codes, e := s.Enable(1, code)
	if e != nil || len(codes) != 10 {
		t.Fatal(e)
	}
	state, _ := s.State(1)
	u, _ := s.User(1)
	token, e := s.LoginChallenge(u, state)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteLogin(token, code, ""); !errors.Is(e, ErrInvalid) {
		t.Fatal("reused enrollment code", e)
	}
	*now = now.Add(30 * time.Second)
	code, _ = otpCode(setup.Secret, *now)
	if _, e = s.CompleteLogin(token, code, ""); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteLogin(token, "", codes[0]); e == nil {
		t.Fatal("challenge replay")
	}
	token, _ = s.LoginChallenge(u, state)
	if _, e = s.CompleteLogin(token, "", codes[0]); e != nil {
		t.Fatal(e)
	}
	token, _ = s.LoginChallenge(u, state)
	if _, e = s.CompleteLogin(token, "", codes[0]); e == nil {
		t.Fatal("recovery replay")
	}
	if _, e = s.ManageMFA(1, "admin-password-123", "", codes[1], true); e != nil {
		t.Fatal(e)
	}
	state, _ = s.State(1)
	if state.Enabled || state.Secret != "" || state.Version != 2 {
		t.Fatal("MFA not revoked", state)
	}
}
func TestExpiredAndAttemptLimitedChallenges(t *testing.T) {
	s, box, now := accountTest(t)
	s.SetPolicy(true)
	token, e := s.IssueEmail(context.Background(), "register", "new@example.com")
	if e != nil {
		t.Fatal(e)
	}
	r := Registration{Username: "new-user", Name: "New", Email: "new@example.com", Password: "new-password-123", PasswordConfirmation: "new-password-123", Token: token, Code: "wrong"}
	for i := 0; i < 5; i++ {
		if e = s.Register(r); e == nil {
			t.Fatal("bad code accepted")
		}
	}
	r.Code = emailCode(box.body)
	if e = s.Register(r); e == nil {
		t.Fatal("attempt limit bypass")
	}
	*now = now.Add(time.Minute)
	token, e = s.IssueEmail(context.Background(), "register", r.Email)
	if e != nil {
		t.Fatal(e)
	}
	r.Token = token
	r.Code = emailCode(box.body)
	*now = now.Add(11 * time.Minute)
	if e = s.Register(r); e == nil {
		t.Fatal("expired code accepted")
	}
}
func TestDeliveryFailureLeavesNoUsableChallenge(t *testing.T) {
	s, box, _ := accountTest(t)
	box.fail = true
	s.SetPolicy(true)
	if _, e := s.IssueEmail(context.Background(), "register", "new@example.com"); e == nil {
		t.Fatal("delivery failure ignored")
	}
	var count int64
	s.DB.Model(&Challenge{}).Count(&count)
	if count != 0 {
		t.Fatal("undelivered challenge retained")
	}
}

func TestExistingMixedCaseEmailCanReset(t *testing.T) {
	s, box, _ := accountTest(t)
	if e := s.DB.Model(&User{}).Where("user_id = 1").Update("email", "Admin@Example.com").Error; e != nil {
		t.Fatal(e)
	}
	if _, e := s.IssueEmail(context.Background(), "reset", "ADMIN@example.com"); e != nil {
		t.Fatal(e)
	}
	token := regexp.MustCompile(`token=([^&]+)`).FindStringSubmatch(box.body)[1]
	if e := s.ResetPassword(Reset{Email: "admin@example.com", Token: token, Code: emailCode(box.body), Password: "changed-password-123", PasswordConfirmation: "changed-password-123"}); e != nil {
		t.Fatal(e)
	}
}
func TestPasswordChangeInvalidatesPendingMFAEnrollment(t *testing.T) {
	s, _, now := accountTest(t)
	setup, e := s.Setup(1, "admin-password-123")
	if e != nil {
		t.Fatal(e)
	}
	s.DB.Model(&User{}).Where("user_id = 1").Update("password", "changed-hash")
	code, _ := otpCode(setup.Secret, *now)
	if _, e = s.Enable(1, code); e == nil {
		t.Fatal("old password enrollment survived reset")
	}
}
