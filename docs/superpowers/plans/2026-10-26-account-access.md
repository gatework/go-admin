# Account Access Implementation Plan

> Execute with executing-plans; user requested implementation, ordinary choices proceed without another approval gate. Preserve unrelated WIP. User authorized committing and pushing the completed account-access work on 2026-10-26. Use independent security and UI review before completion.

**Goal:** Add email-verified registration, email password recovery and authenticator MFA following Motor.
**Architecture:** Shared common/account domain owns database transactions, one-use challenges, encrypted TOTP secrets, recovery codes and SMTP. Admin API/service adapters preserve project layering. Existing JWT authenticator enforces the second factor before issuing tokens; credential stamps invalidate old tokens after credential changes.
**Tech Stack:** Go/GORM/PostgreSQL, pquerna/otp, AES-GCM, SMTP; Vue/Arco/Pinia.
**Spec:** User request and Motor backend account/registration/password/session services and frontend account-access pages.

## Decisions

User confirmed registration disabled by default, administrator-controlled. Registered accounts get a dedicated ordinary role with no administrative grants, created only when needed. Existing one-role seed remains unchanged until registration is used. Email settings and secret encryption key come from deployment environment, never copied from Motor secrets. Local validation uses a mail sink; production SMTP delivery requires operator configuration.

Operate UI extends the existing login shell, theme and form conventions. Registration collects username/name/email, email code and password confirmation. Recovery sends a link plus code and ends at login, not an automatically authenticated session. Account security presents MFA status, password-confirmed enrollment, authenticator QR/manual key, confirmation, one-time recovery codes, disable and regenerate. Login switches to an explicit challenge state with authenticator/recovery-code choice. Pending/failed/expired/disabled states and keyboard/mobile paths are required.

Security: cryptographic random tokens, keyed hashes of email codes, secret encryption, attempt/expiry limits, per-IP and per-email throttling, atomic one-use consumption and TOTP replay prevention. No role/mfa fields accepted by registration. Recovery does not disable MFA. Existing user password/role/status changes must invalidate or reject stale sessions. Sensitive endpoints redact operation-log bodies/results.

## Tasks
- [x] Domain tests and implementation: policy, registration/reset challenges, MFA enrollment/login/recovery, replay/concurrency and mail failure.
- [x] Versioned migration, API/service adapters, login/JWT integration, logging redaction and admin policy.
- [x] Arco access pages, MFA challenge, profile security and system registration setting, localized copy.
- [x] Local mail-sink end-to-end test and browser state/viewport coverage; Go tests, types/lint/unit/build.
- [x] Independent security/UI review, documentation and final evidence.

## Sources
Motor is the primary product reference. Security cross-checks: https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html and https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html ; TOTP library https://pkg.go.dev/github.com/pquerna/otp/totp .
