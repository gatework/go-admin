# Fabric core delivery evidence

Date: 2026-10-26. Scope: domain, persistence contract, analysis, normalized endpoint lookup, HTTP API and routes. Operations, migration/bootstrap and frontend are separate owners. No commit, push, production access or live-device command was performed.

## Changed paths

- `app/net/models/fabric/models.go`: Fabric settings, singleton versioned rule set, per-device manual/automatic membership, append-only dimension observations, scoped evidence-keyed assessments.
- `app/net/service/dto/fabric/fabric.go`, `validate.go`: authoritative public contract and bounded pagination (default 20, maximum 200).
- `app/net/service/fabric/inventory.go`, `rules.go`: inventory DataPermission scope, actual complete-membership visibility, automatic name reconciliation, manual priority, explicit conflicts/unmatched rows, optimistic rules/settings updates.
- `app/net/service/fabric/evidence.go`, `assessment.go`: bounded latest/last-complete selection, freshness and acquisition-order checks, source/connection/membership/rule-version validation, historical inventory preservation, role-specific go-vxlan review, all-Leaf service matrix, reciprocal-peer vPC pairing, separately classified unknown evidence and faults.
- `app/net/service/fabric/endpoints.go`: IP/MAC/CIDR normalization, VRF-isolated local/remote candidates, historic facts retained after partial failures, verified vPC duplicate candidates. Historical VRF inference uses matching source/version configuration acquired before the endpoint evidence.
- `app/net/handler/fabric.go`, `fabric_operations.go`: request preparation, strict size-bounded JSON decoding, safe public failures, application response envelopes, service delegation and Swagger annotations.
- `app/net/router/fabric.go`, `fabric_permission.go`: JWT, exact registered-route Casbin precheck, AuthCheckRole and DataPermission middleware on every Fabric API. Static action routes cannot inherit a generic detail grant.
- `app/net/service/fabric/domain_test.go`, `inventory_test.go`, `app/net/router/fabric_test.go`: unit, SQLite domain and authenticated Gin integration coverage.

Operations-owned files `models/operations.go`, `service/dto/operations.go`, `service/collection*.go`, `changes*.go`, `transport.go`, `worker.go` are consumed through the agreed service contract; they were not implemented by this owner.

## Important behavior

A failed/latest configuration does not erase observed segments. Its last complete record remains visible with `configState=historical`; matrix cells and health remain unknown. Evidence also becomes unknown after identity, connection, membership or rule-version drift. Current/history payloads remain distinct.

The evidence query returns at most two observations per dimension/device, using portable correlated anti-joins and acquisition-time/ID ordering. It does not load the accumulated collection history for every list/detail request. Assessments include Fabric ID, member identity/role/version, rule version and selected evidence/freshness in the evidence key; scope keys prevent disclosure through shared cached analyses.

Health checks use actual go-vxlan parser output keys. Spare physical ports are excluded; configured Fabric service/OSPF/vPC ports remain relevant. Complete-but-missing configured VNIs and EVPN BGP peers are faults. Empty dynamic neighbor evidence remains unknown. vPC pairing requires matching domains and reciprocal peer addresses; domain ID alone cannot form a pair. Negative keepalive text, down vPC members and mismatched runtime domains are faults.

`evidenceCoverage` is an integer percentage; `checkedCount` and `applicableCount` expose its denominator. Endpoint records include `fabricCode`, `candidateCount`, and confirmed `vpcGroupKey` where applicable.

## Verification

Passed after final core changes:

- `go test ./app/net/... -count=1`
- `go test -race ./app/net/... -count=1`
- `go vet ./app/net/...`
- `git diff --check -- app/net`

Race builds emit existing third-party clang warnings from `github.com/shoenig/go-m1cpu` about folded variable-length arrays; tests pass and no race was reported.

Authenticated Gin tests exercise successful list/detail/empty-Fabric and parsed IP endpoint response envelopes, cross-VRF empty results, partial inventory visibility, static-route permission isolation, bounded pagination, full-scope mutation rejection and unknown JSON field rejection. Domain tests cover rule conflicts/manual membership, renamed inventory, acquisition order, failure/history preservation, source identity drift, all-Leaf matrix coverage, unused-port filtering, actual parsed OSPF/vPC/MAC output, expected VNI/BGP omissions, verified vPC endpoints and bounded observation-history selection.

Full-repository gates, browser acceptance, migration/startup checks and release integration are root-owner responsibilities. Real-device collection and changes remain unverified.
