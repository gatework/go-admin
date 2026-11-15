package dto

// AccountRequest is the body shared by the public account and MFA routes;
// each operation reads the fields it needs.
type AccountRequest struct {
	Username             string `json:"username"`
	Name                 string `json:"name"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
	Token                string `json:"token"`
	Code                 string `json:"code"`
	RecoveryCode         string `json:"recoveryCode"`
	Enabled              *bool  `json:"enabled"`
}
