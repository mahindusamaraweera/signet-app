# Blockchain automation implementation log

## Cross-plan vesting rules confirmed

Owner confirmed combined qualifying Tier investments, Option 1 priority, Option 2's 2000-day wait from this package purchase followed by 10000-day release, and an optional existing-user switch of remaining unreleased allocation over 10000 days from switching. Approved ownership-proven account linking, not email matching or account merging. Implemented the companion eligibility calculator and tests; linking and actual schedule changes are unfinished. Daily settlement must remain authoritative while the requested second-by-second odometer is display-only progress. No old schedules, accounts, cap records or production services changed.

Added companion account-link storage and code-exchange foundation with hash-only secret storage, request/code expiry, S256 PKCE, state/source binding, authenticator-aware approval and one-to-one account constraints. These are internal service methods; browser consent and application callback/session integration remain unfinished. No user was linked, merged or provisioned by this development step.

Wired companion authenticated consent GET/POST with CSRF/throttling and restricted begin/exchange APIs. The only permitted redirect is the configured fixed callback; browser identity and callback overrides cannot control approval. Focused link tests passed (23 assertions, local environment warnings). The source-side state/verifier storage and callback UI are not implemented yet. These routes remain disabled by the integration feature flag; no live account linking or redirect/deployment was performed.

Source-side linking now has additive migration 005, authenticated begin/status/complete routes and a wallet link/callback page. State is hashed, bound to the initiating session token and expires; PKCE verifier is AES-GCM encrypted with user/state-associated data, never returned to the browser, and cleared after completion. A strong session secret of at least 32 bytes is required. New logins receive random JWT IDs so session binding distinguishes independent logins. Mutation routes require the configured frontend Origin. The gateway permits only the expected authorization path on its configured financial-service origin. Callback codes are removed from browser history, and ambiguous exchange results use the restricted authoritative status endpoint before local commit. Account IDs remain source-qualified/separate; unique constraints prevent local account reassignment. Crypto tests and Go tests passed; database-backed handler, browser and two-service end-to-end checks remain outstanding. No migrations or live links were executed.

Added companion server-side Tier-investment snapshot retrieval under the restricted credential. Qualifying values come from the verified linked account's original filled purchase records and duration snapshots, not browser claims/current catalog prices. Refunds, pending and missing-snapshot records are excluded; unknown historical evidence requires review. Tested combined-investment evaluation using authoritative database fixtures. No user schedule, allocation or cap was mutated; source-side eligibility UI and voluntary switching remain next work.

Branch: `feature/blockchain-automation`

## Scope

Add user deposit wallets, BSC deposit verification, withdrawals and automated vesting. Preserve existing MLM, commission and placement rules. KYC is outside this integration's scope. Merge requires project-owner approval.

## 2026-10-01 — Registration review

Reviewed the current Vue registration pages, Go authentication handlers, package activation handlers and database migration. This is a static source review; production behavior has not been verified.

### Current registration flow

1. `/register?ref=...` loads referral details through `GET /api/v1/register/referral`. The referral must exist. If the latest referred user has no active package, the page rejects the referral.
2. `POST /api/v1/register/step1` validates credentials, contact fields, referral and optional leader/executive IDs. Binance Pay ID is currently required. Duplicate email, WhatsApp and Binance Pay ID checks run before insertion.
3. One database transaction inserts the pending user, a zero-balance `wallets` row, a six-character referral code and a hashed `user_secret_keys` value. The API returns the numeric `users.id`. It does not issue a registration session or a BSC wallet address.
4. The package-selection page loads active packages and posts `package` and `newUserID` to `POST /api/v1/register/step2`. The backend resolves placement through `tree.ParentFind`, creates placement records, creates or updates a pending first package and records placement logs. Global-share wallet bootstrap and notification follow the commit.
5. The response supplies upline Binance Pay ID, WhatsApp and vacation status. The UI stores these in session storage and displays the upline-contact page.
6. The account-status page fetches `GET /api/v1/register/status?id=...` once on mount. Activation is a separate backend operation; registration itself does not verify a blockchain payment.
7. Login issues a session before checking packages. It returns an error for no package or exactly one pending package; multiple packages bypass that pending-package check.

### Integration implications

- `users.id` is the stable local identity; the display Signet ID and hashed secret-key value are separate representations.
- `wallets` is an internal balance table, not an on-chain deposit-wallet record. A separate address mapping is required.
- Provision a deposit wallet after the step-one transaction commits, using a durable retryable request keyed by local user ID. A remote outage should not invalidate an already-created account.
- Keep deposit verification separate from package activation until the existing activation rules and funding relationship have been traced completely. Receipt of funds alone must not silently invoke commission or placement changes.
- Existing registration continuation endpoints are public and use a client-supplied numeric user ID. They need verified ownership before exposing deposit addresses or accepting financial actions.

### Observations requiring follow-up

- Referral gating is implemented in the referral-loading endpoint but is not repeated in step-one submission.
- Step two calls `tree.ParentFind` before its local transaction and checks active packages after placement work begins. Review retry and transaction behavior before adding automation.
- Step two does not enforce the package's active status when accepting its ID.
- The upline wait component uses `computed` without importing it from Vue.
- User activation and package activation are distinct paths and must be documented before payment automation is connected.

### Changes and verification

- Created the integration branch from a clean `main` checkout.
- Added this implementation record; application behavior is unchanged.
- No runtime tests were performed for this documentation-only step.

### Next work

Trace both package activation paths and the matching wallet-provisioning flow in `signet-token`. Define the registration ownership mechanism and wallet-provisioning contract before implementing financial endpoints.

## 2026-10-01 — Revised registration and funding requirement

Project-owner requirement: users registering through referral links receive their own custodial BSC USDT deposit address. Users fund that address and purchase a package from verified USDT credit. Funds go to treasury rather than the sponsor. Include PancakeSwap execution. Existing MLM calculations and referral placement remain in scope for preservation; sponsor-funded activation must not be reused unchanged for a self-funded purchase.

### Target behavior

1. Validate referral and create the user with verified registration ownership.
2. Provision a unique deposit address through the blockchain service, with a persistent mapping and retryable provisioning state.
3. Show the user's address, network, verified USDT balance and deposit status instead of upline payment instructions.
4. Scan the configured USDT contract, verify confirmations, and credit each transaction-log event exactly once.
5. Accept an explicit package purchase against confirmed available USDT. Reserve funds with a unique purchase key; reject insufficient balances and duplicate spending.
6. Sweep funds to treasury using the blockchain service. Track broadcasts and confirmations independently from user accounting: sweeping must not debit the user twice.
7. Execute the approved PancakeSwap conversion, reconcile the actual result, and finalize the purchase once its required financial steps succeed.
8. Invoke an adapted self-funded activation path that preserves agreed placement and commission calculations without burning sponsor tokens or charging the sponsor wallet.

### Reference implementation findings

- `signet-token/app/Services/Wallet/WalletService.php` provisions deposit addresses.
- `signet-token/app/Console/Commands/SweepDeposits.php` queues already-credited USDT deposits for treasury sweeps independently of package purchase. This timing differs from the requested purchase-then-sweep wording and needs an explicit policy.
- `signet-token/app/Services/Wallet/PurchaseService.php` supports PancakeSwap-backed purchases, including USDT-to-SIGNET quoting, exact-output operations and execution reconciliation.
- `signet-token/app/Services/Dex/PancakeSwapSignerGateway.php` delegates execution to the existing signer.

### Decisions pending

- Confirm whether the purchase swaps USDT to SIGNET, the portion to convert, and the destination/vesting treatment of acquired SIGNET.
- Confirm whether treasury sweeps should happen after every verified deposit (matching the existing service) or only after a package purchase.

Only the implementation record has changed at this stage. No transaction was broadcast and no integration endpoint has been enabled.

## 2026-10-01 — Confirmed policy and gateway foundation

Confirmed by project owner: convert the full package price from USDT to SIGNET; sweep credited deposits automatically like `signet-token`, independently of purchase; use the same user custody and vesting destination as `signet-token` for acquired SIGNET.

Implemented a Go `blockchain.Gateway`, disabled-by-default configuration and an authenticated `POST /api/v1/blockchain/wallet` route. The route derives identity from the local session rather than a client-provided ID. Requests use service authentication and the stable `signet-app:wallet:{id}` idempotency key. The gateway rejects insecure remote transport, redirects and malformed ready addresses. Amounts remain strings.

Configuration: `BLOCKCHAIN_ENABLED=false` by default; enabling requires `BLOCKCHAIN_SERVICE_URL` and `BLOCKCHAIN_SERVICE_TOKEN`. No keys or signer credentials belong in this application.

Required service contract (not yet implemented in `signet-token`): `POST /api/v1/integration/wallets`, Bearer service token, JSON `{ "source": "signet-app", "external_user_id": "42" }`. Return a top-level wallet object containing `address`, `chain_id`, `asset` (`USDT`), `available` (decimal string), and `provisioning_status` (`pending` or `ready`). The service must persist a unique source/user mapping and deduplicate provisioning. This integration route is not one of the existing user APIs.

Gateway tests cover request identity/idempotency, malformed addresses and insecure URLs. Full chain execution and database integration are not yet verified. Registration session ownership, the replacement funding UI, durable provisioning retries, purchase orchestration, adapted MLM activation, withdrawal and vesting endpoints remain to implement. Existing registration behavior is unchanged by this foundation.

## 2026-10-01 — Existing-user preservation and administrator vesting review

Added `docs/existing-user-wallet-vesting-migration.md` with account preservation, wallet backfill, dry-run import, reconciliation, scheduler cutover and administrator adjustment requirements. Existing accounts must retain credentials, identity and MLM records. Wallet assignment is independent of package activation.

Reviewed legacy mining records and scheduler: the current source has token accrual records but no explicit vesting schedule. Reviewed reference manual lock adjustment: it is audited and transactional but restricted to manual schedules. Purchased/imported schedule adjustment requires an explicit extension rather than blindly reusing that action.

This step changed documentation only. No database migration, historical allocation, scheduler change or chain transfer was executed. Production data and the meaning of historical accrued/paid amounts remain to verify before import.

## 2026-10-10 — Automated unpaid-placement expiry

Implemented the existing 10-hour window as an opt-in scheduler job checked every minute (up to 100 candidates per run). The cutoff is measured from placement-log creation, matching the old listing. Expiry retains the account, passwords, referral identity, packages and financial records; only this user's recorded `deactive` placement rows and live placement log are removed. A new `registration_expiries` table records the removed row IDs and expiry time transactionally.

Local protection checks include active packages, wallet/ROC balances, tokens, mining allocations and global-share balances. Blockchain-enabled deployments additionally require explicit clearance from the authoritative service. Unknown, malformed or unavailable eligibility responses preserve placement. Enabled automation excludes expired registrations from the sponsor's latest-registration gate. The legacy hard-delete action is blocked when automation or blockchain integration is enabled.

Deployment prerequisites: apply the additive SQL migration using the existing migration tool; set `REGISTRATION_EXPIRY_ENABLED=true` on the scheduler instance after validation. It defaults to false and no migration has been executed here. `ENABLE_SCHEDULER` also controls whether the job starts.

Required service endpoint: `POST /api/v1/integration/registration-expiry/check`, using source-qualified identity and service authentication, returns `{ "can_expire": true|false }`. Responses must be recomputed rather than permanently cached under the request's idempotency key. Return false for deposits (including unconfirmed/credited/swept history), any available/locked/pending balance, vesting, purchases or broadcasts in progress. Service-side purchase reservation and expiry coordination are still required to close races between clearance and purchase; this feature must remain disabled for live integrated registrations until that contract is implemented and tested.

Verification: Go test suite passed; explicit-clearance gateway tests cover true, false, missing, null and wrong-type responses. `git diff --check` passed. No local MySQL or Docker executable was available for database-backed expiry tests, so transaction behavior and migration execution remain unverified. No production cleanup was performed.

## 2026-10-10 — Automatic funding and partial-deposit protection

Added durable package purchase intents, authenticated registration continuation, removal of required Binance Pay ID in blockchain mode, a personal-wallet funding screen, remaining-amount display and an automatic reconciliation worker. Package selection captures the price and commission basis and serializes repeat submissions by locking the user. Integrated intents are protected against legacy manual activation.

Any observed deposit permanently protects an intent/placement, including after treasury sweep. Confirmed partial USDT displays the exact remainder; sufficient funds request automatic full-price USDT-to-SIGNET execution. Completed remote execution must confirm swap and vesting allocation before transactional local activation. Reused the existing recipient commission calculations without burning sponsor funding tokens. Commission parity with production is still to verify.

Added `docs/automatic-package-funding-contract.md` specifying the missing financial-service endpoints and deployment prerequisites. The matching `signet-token` integration routes remain absent; this is application-side implementation, not a working live chain integration yet. Configuration stays disabled by default and migrations have not been applied.

Tests cover exact decimal partial/exact/excess amounts, malformed values, stable purchase retry keys, partial-deposit protection and rejection of unconfirmed completion. Frontend dependencies were installed from the lockfile for build verification; no production deployment or financial operation occurred.

Verification results: `go test ./...` passed and `npm run build` passed (existing bundle-size warning). `git diff --check` passed. No MySQL-backed migration/activation tests or blockchain end-to-end tests were performed. The integration remains disabled until the service contract is implemented and those checks are completed.

## 2026-10-10 — Cross-repository linking review

Reviewed the current `signet-token` registration, wallet provisioning, purchase service and scheduler. Created its separate `feature/signet-app-integration` branch from clean main and added `docs/signet-app-integration.md`. No companion runtime code was changed. Both main branches remain untouched.

Confirmed gaps: restricted integration routes/identity mapping; globally serialized address derivation; fixed package allowance and vesting policy snapshots; external purchase processing without destination MLM; confirmed treasury bonus delivery; expiry/purchase race coordination; existing-user migration; withdrawal and administrator vesting integration; production package audit and database/testnet validation. Companion purchase service currently computes percentage bonuses and dispatches tier/MLM processing, so calling it unchanged would violate the agreed ownership.

## 2026-10-10 — Companion service implementation

Implemented initial wallet mapping/authentication and external fixed-allowance purchase processing in the companion feature branch. Added locked derivation allocation, durable external purchase records, queue processing, treasury shortfall delivery and vesting without companion MLM processing. The application gateway now sends the selected package ID, resolved from its persisted package record.

Companion tests verify mapped identity, authorization, excess USDT, one-time treasury bonus and one-time vesting allocation using SQLite and mocked chain services. Production allowance catalog is still intentionally empty. Expiry remains conservatively deferred pending atomic service coordination. Existing-user migration, withdrawal/admin endpoints, production catalog reconciliation, recovery hardening and full end-to-end validation remain in progress. No main-branch merge or live operation occurred.

## 2026-10-10 — Vesting pages and linking handoff

Added authenticated user vesting/history and company/admin schedule lookup and adjustment pages. Administrator identity comes from the local session, not the browser request. The companion service limits adjustments to mapped external schedules, records before/after terms, preserves released amounts and uses a revision check plus stable adjustment UUID for retries. Dedicated adjustment tests and rejected-versus-uncertain error handling remain before production enablement.

Updated the companion `docs/signet-app-integration.md` with implemented API routes, shared integration-token configuration, migration prerequisites and scheduler/queue responsibilities. Repository ownership stays separate: this application retains registration, placement and MLM; signet-token owns custody and blockchain accounting. Both feature flags remain disabled. Go tests passed; selected companion tests passed with missing-local-env warnings. No merge, production migration, deployment or transfer was performed.

Remaining release gates: production package allowances/vesting policy; recoverable ambiguous chain broadcasts and on-chain amount verification; atomic expiry coordination; commission-parity/database concurrency tests; existing-user wallet backfill and reconciled vesting import; authenticated withdrawal flow and history; focused adjustment tests/error handling; full testnet deposit/sweep/swap/bonus/vesting/withdrawal lifecycle. Do not enable real deposits until these gates pass.

## 2026-10-10 — Adjustment retries and withdrawal security review

Added a companion adjustment test covering preserved released/claimed amounts, one-time retry, changed UUID terms and stale revision rejection (12 assertions passed with one local environment warning). The gateway now distinguishes definitive HTTP 422 rejection from unavailable/uncertain responses. The admin page clears only definitively rejected requests and reloads current terms; uncertain requests retain the original UUID/body for retry. Gateway and handler tests passed.

Withdrawal review: the companion withdrawal service requires verified email, authenticator, device and withdrawal-address verification. Mapped custody principals deliberately have no shared login or inherited security credentials. Therefore it cannot be called unchanged, and these controls must not be bypassed. Implement an explicit external security enrollment/authorization contract, validated at broadcast time, before enabling withdrawals. No withdrawal endpoint or live authorization was enabled in this step.

## 2026-10-10 — On-chain delivery amount gate

The companion external purchase processor now verifies net SIGNET transfer-log amounts to the mapped custody address for both the confirmed swap and treasury bonus before vesting allocation. Missing, wrong-token or mismatched delivery cannot report completion. Tests cover token filtering, incoming-minus-outgoing amounts and decimals; integration tests passed with receipt fixtures (51 assertions, local environment warnings), and the receipt unit test passed (3 assertions). This does not resolve ambiguous signer broadcasts or prove live testnet behavior. Integration remains disabled.

## 2026-10-10 — Stable swap retry terms

Added the companion fourth integration migration to snapshot the swap minimum before broadcast. Ambiguous retries now reuse immutable quote terms and the same operation key. The timeout test verifies one quote, identical retry payloads and no repeated USDT reservation (8 assertions, one local environment warning). This is not proof of live signer idempotency or complete transaction recovery. Fee amount/recipient confirmation remains pending; no fee logic or production operation was enabled.

## 2026-10-10 — Income-cap preservation audit

Owner confirmed existing cap values and behavior must remain unchanged. Traced current-balance-based four-times-active-package checks, package earnings and global-share counters. Found that the initial automatic activation worker calls the private credit path without repeating the manual handler's upstream cap checks. Automation therefore is not cap-parity verified and remains disabled. Documented the blocker and the need for distinct settlement status so a USD cap basis does not become a second withdrawable balance after SIGNET payment. No historical earnings/caps were changed and no new universal cap rule was introduced.

Extracted the unchanged predictive cap comparison into `wallet.CommissionFitsCap` and added boundary regression tests (below/equal/above cap, already over cap, no active package and multiple-package summed cap). Equality remains allowed, matching legacy code. Automatic execution still needs a durable recipient cap reservation coordinated with commission settlement: a precheck without a reservation can race while a remote swap confirms. Do not enable automation based solely on these pure-function tests. No runtime precheck/reservation or payout was represented as complete.

## First-purchase-only vesting clarification

Owner requires vesting and purchaser bonus on first purchase only. At the existing 4x limit, top-ups extend income cap and pay team commissions, with no new purchaser vesting/bonus. Added a companion guard rejecting a second external first-purchase order (or new first purchase after an imported schedule), while preserving same-order retries. This guard does not implement top-up buying. Top-up swap disposition and fee recipient/amount remain unconfirmed; cap eligibility, commission settlement and the actual top-up UI/API are unfinished.

## Deposit history display

Added a restricted, identity-scoped paginated companion deposit-history endpoint and authenticated local gateway route. The funding screen shows recent USDT transaction hashes, confirmations, credit status and sweep status; history failures do not remove the wallet address or funding details. Scope test passed (8 assertions, one local environment warning); Go suite passed. History is scanner-derived and is not a live chain/testnet validation claim. No account data, financial balances or production services were changed.

## First-allocation vesting completion

Owner confirmed vesting stops at the total token allocation of the first investment, including bonus. The companion release engine now explicitly ignores non-active external schedules. Added ledger-backed release testing of halfway release, final completion and a much later repeated release: total unlocked is bounded at the original allocation, remaining lock is zero and no destination MLM commission is generated. Focused test passed (10 assertions, one local environment warning). Top-up implementation and historical import are still unfinished; no live vesting was changed.

## Treasury bonus recovery

Added companion signer-operation lookup for interrupted bonus broadcasts. Recover the original completed hash, wait for in-progress state, reject failed/invalid recovery results, then require chain confirmation and exact receipt delivery. No second transfer is broadcast for recovered completed operations. The recovery fixture verifies the order remains pending without vesting while its recovered transaction lacks finality. Live signer/chain and concurrent-worker behavior still need validation. No production transaction was executed.

## Existing-account wallet access

Added the authenticated `/wallet/deposits` page with idempotent personal wallet provisioning, QR/address display, confirmed USDT balance and paginated deposit history. Uses the existing session identity; it does not create another source account or change credentials, package activation, cap values or vesting. Existing users can provision on demand without registration continuation. The page explicitly says top-up purchases are unavailable and that deposits alone do not authorize one. Gateway wallet responses now require a valid decimal balance. Bulk wallet backfill and historical vesting import remain separate unfinished work; integration must stay disabled until release gates pass.

## Wallet backfill tooling

Added `cmd/wallet-backfill`: default read-only bounded inventory, explicit apply flag, stable ascending source IDs and resume cursor. Apply provisions through the existing authenticated/idempotent gateway, stops on failed/pending results and never changes source account or financial records. Dry-run and stop/resume boundary tests passed with the Go suite. Updated migration handoff with commands and the distinction between reviewed and successfully provisioned cursors. No actual database backfill, production provisioning or vesting import was performed.

## SIGNET deposit double-credit protection

Companion external wallets now retain unknown SIGNET receipts for reconciliation instead of automatically crediting them. Funding remains USDT-only. This prevents swap proceeds arriving before the purchase hash is saved from being credited as a second financial entitlement. Retained evidence can later be recognized as internal by the original credit job after hash recovery. Genuine unexpected SIGNET deposits require operator review. No ordinary user funding policy or historical record was modified.

## Confirmed cross-plan vesting policy and existing-user switch

Owner confirmed Option 2: wait 2,000 days from this package's purchase date, then release daily over 10,000 days. Option 1 takes priority for an exact matching combined qualifying 3,000-day Tier investment; nonqualifying users may retain their existing method. Account ownership must be explicitly linked; an email match does not authorize linking.

Added companion Option 3 preview/commit and authenticated source gateway/UI. Only reconciled historical `external_import` schedules are eligible, not new purchases. A five-minute encrypted preview binds identity, schedule, revision, dates and released amount; explicit consent is required. Commit preserves the total and already released allocation, spreads only the remaining amount over 10,000 days, audits before/after, and uses a stable request UUID for uncertain retries. A unique schedule audit prevents restarting the same switch. A released/claimed discrepancy blocks the switch until reconciled. No historical schedules were imported or changed in production.

Tier ownership linking, investment snapshot and pure Option 1/2 eligibility evaluation exist on the feature branches. Applying Option 1/2 to schedules, trusted historical purchase-date reconciliation, the second-by-second estimated odometer, historical import, cap-safe commission settlement, top-ups, fee routing and full end-to-end validation remain unfinished. Keep integration disabled; this is not a deployment-ready release.

Verification for this addition: full Go suite passed including switch gateway identity/stable-retry coverage; frontend production build passed with the existing chunk-size warning. Selected companion vesting/eligibility tests completed with 36 assertions, three passed and six warning-classified tests (local environment warning remains). Formatting and both repositories' diff whitespace checks passed. These checks do not cover production MySQL concurrency, browser consent, real chain transactions or imported production records.

## Immutable purchase date for Tier eligibility

Added a companion purchase-completion timestamp written atomically with allocation/vesting completion. Retries must not change it. Tier vesting terms now resolve a completed purchase through both the authenticated custody identity and schedule ID, using its original USD amount and completion date. Intent creation, mutable update timestamps and admin-adjusted vesting dates are not substitutes. Missing dates and historical imports fail closed for reconciliation. Option 2's lock date is purchase completion plus 2,000 days; Option 1/2 schedule application is still unfinished. This migration was authored but not applied to real data.

## Tier-qualified initial schedule application

Extended the authenticated preview/confirmation flow with Tier mode. The server evaluates verified linked investments, prioritizes Option 1 and rechecks evidence on confirmation. Option 1 starts on preview activation; Option 2 starts exactly at package completion plus 2,000 days and ends 10,000 days later. Daily settlement uses the full unreleased allocation. Preview expiry is measured from now, not the future release date. Revision checks, explicit consent and stable retry audit apply to all options. Interval/count metadata is reset to daily/10,000 so next-release calculations use the new duration.

Current Tier application supports completed first-purchase schedules with zero prior releases. This restriction is not the final historical migration policy: partially released schedules require an agreed transition calculation, and historical imports still require verified purchase evidence. The existing-user Option 3 remaining-allocation switch is separate. No production financial records changed. Browser and production concurrency checks remain release gates; integration stays disabled.

## Per-second display estimate

Added a server-supplied display-only snapshot for selected 10,000-day options and a decimal-safe (BigInt scaled to 18 decimal places) frontend progress estimate. It updates each second from server time plus monotonic elapsed time, respects the lock, preserves the released baseline, and caps at total allocation. Suspended/non-active schedules show only settled release. Snapshots older than five minutes are hidden; the user page refreshes each minute unless a confirmation is uncertain. The display explicitly says it is not spendable and settlement is daily. Rendering does not execute a release, ledger credit or withdrawal. Timer cleanup is implemented. Four Node tests cover interpolation, lock/end bounds, stale/malformed inputs and large decimal amounts. Build and browser visual validation are separate checks; live browser behavior still needs verification.

## Transaction-scoped legacy cap calculation

Rechecked automatic versus manual activation: the automatic path still bypasses the upstream `CheckWallet` gate and calls the private credit routine. This remains a release blocker, not a completed cap-preservation claim. Refactored the existing cap reader without changing its SQL, latest-active-package commission selection, float semantics, 4x basis or equality boundary; added `CheckWalletTx` for use within a coordinated transaction. Tests cover equality, over/under boundary, original commission-selection query and failed-read rejection. This helper is not yet wired into automatic activation: a standalone read does not reserve income cap for an asynchronous treasury payout. Beneficiary locking, durable commission reservations/outbox and cap-rejection handling still need implementation and production MySQL concurrency tests. Integration must remain disabled.

## Activation commission outbox evidence

Added migration 006 and a transaction-scoped recorder for automatic activation commissions. Each positive USD credit is tied to its exact newly created earn-log ID and purchase intent; the earn-log ID is unique in the outbox. Direct, gratitude/spillover, leadership/executive and the legacy company-overflow credits are recorded without recomputing USD rules. Zero values create no payout instruction; invalid decimal/identity evidence aborts. The record is inserted inside the same transaction as its USD history and activation, so an insert failure rolls back rather than leaving an untracked credit. Manual legacy callers do not use the recorder.

These rows are `awaiting_policy`, not payments or cap reservations. No worker converts or transfers them yet, and no migration was run against a real database. Company overflow semantics, duplicated legacy synthetic token transfers, cap-safe reservation/activation ordering, live-rate snapshots, treasury delivery and immediate-versus-vested commission policy remain release blockers. Production MySQL atomic rollback/concurrent-worker testing is missing. Keep integration disabled; this addition does not make automatic activation production-safe.

## Commission recorder transaction tests

Added a scripted SQL-driver test verifying exact earn-log ID/source intent/beneficiary/USD evidence and execution inside the caller's transaction. Simulated outbox failure propagates and the caller rolls back instead of committing. Added a company-overflow case checking its separate earn-log identity; the test documents existing duplicate user/company USD accounting rather than approving a double treasury payout. The fixture checks call order and argument propagation, not MySQL storage-engine atomicity, uniqueness contention or production concurrency. No migration or treasury transfer was executed. Owner questions about fee details, commission release mode and top-up destination remain unanswered.

## Temporary late-cap hold in automatic activation

Automatic non-company activation now locks the selected beneficiary user and existing wallet row, then calls the legacy transaction-scoped `CheckWallet` calculation before private credits. A failed read rolls back. A denied gate persists `awaiting_cap_review` without activating the package or writing activation commissions; the UI says funding is confirmed, warns not to deposit again and directs support review. Subsequent runs retry the original durable purchase, not a new swap. No silent company fallback or dropped commission is introduced.

This hold is deliberately temporary pending the owner's late-cap policy answer: it does not satisfy uninterrupted self-activation. Companion allocation may already exist when the local cap gate holds; release behavior during that interval needs coordination before deployment. Beneficiary row locking is not a complete durable asynchronous payout reservation; existing writers and MySQL lock ordering/deadlock retries require validation. All cap values and legacy query semantics remain unchanged. Integration must stay disabled.

## Confirmed ledger balance view and withdrawal review

Companion wallet response now includes mapped custody-wallet USDT/SIGNET available, locked and pending-withdrawal ledger balances. Gateway validates all decimal fields and consistency with the original USDT funding balance; missing older-service snapshots are not displayed as invented zero balances. Existing-user wallet page labels locked vesting and per-second estimates as non-spendable and explicitly says integrated withdrawal is unavailable. No browser-selected user ID is used.

Reviewed companion withdrawal API/controller: it sends OTP to the authenticated companion account email and verifies that account's OTP/TOTP/security settings. Generated custody accounts have invalid-domain email/random credentials and are not user login accounts. Simply forwarding withdrawal requests with a service bearer would not establish user authorization. Source withdrawal challenge/OTP identity delivery, replay-safe authorization attestation and mapped security controls need implementation; do not bypass these checks or add KYC. Wallet balances are a prerequisite, not completed withdrawal automation. No real transaction or migration was performed.

## Owner-confirmed late-cap company routing

Owner confirmed that a capped recipient without a top-up must have the commission assigned to the company, with an ongoing user income-cap alert. This supersedes the temporary `awaiting_cap_review` hold for the selected direct team recipient. The buyer now activates normally; the selected denied commission is credited once to company account 1 with its own earn log/outbox instruction, and a unique per-user/purchase diversion notice is inserted in the activation transaction. Dedicated company diversion bookkeeping avoids reapplying user earnings/global-share overflow and accidentally duplicating this credit. The capped selected recipient receives no direct credit. Other activation bonus calculations are unchanged and require their separate cap/settlement audit.

Added authenticated source-only cap overview/diversion history and a dashboard-shell alert refreshed each minute. It shows the original USD income balance, current active-package 4x cap, remaining capacity and recent company assignments. Historical notices remain visible after a later top-up as history; the current reached-cap warning derives from current cap/balance. No claim that top-up buying is ready is made. Scripted worker regression checks buyer activation, exact company USD/outbox, notice and completion in one transaction. Real MySQL locking, all bonus-recipient cap cases, treasury conversion/delivery and top-up extension remain unfinished. Company account 1 is an accounting recipient, not an assumed blockchain address. No migration/deployment/chain transaction was executed; flags remain disabled.

## Activation bonus recipient cap routing

Integrated activation now routes each direct-team, spillover, gratitude, leadership and executive USD commission through a locked beneficiary check using the original active-package 4x basis and the actual role's commission amount. A denied role is credited once to company accounting and receives a per-role diversion notice. Migration 008 extends notice uniqueness to user/purchase/role so a beneficiary with multiple roles does not cause an activation rollback merely because two commissions were diverted. The pre-existing direct-recipient gate remains before the role-specific check; no package price or commission percentage is changed. Manual activation callers continue their original credit path.

In integrated activation only, the legacy synthetic token-row transfer for spillover/gratitude is skipped: the USD earnings entry and pending SIGNET settlement instruction represent the commission. No real treasury transfer is enabled. Five scripted SQL tests cover denied routing for all role names, with no capped-beneficiary wallet credit. Full activation-role combinations, uncapped role regression, real MySQL concurrency and treasury delivery remain verification gaps. Legacy company earnings-overflow accounting can still generate an additional company earnings entry and requires explicit reconciliation before any outbox payout worker is enabled; do not treat every accounting row as an approved on-chain transfer. Main and production remain unchanged.

## Strict live commission pricing preparation

Companion display `SignetMarketPriceService` can fall back to a manual/default price during live gateway failure, so it must not be used to issue external commission tokens. Added separate read-only `ExternalCommissionQuoteService`: requires live PancakeSwap enabled, BSC chain/asset contract configuration, a fresh gateway spot price and positive decimal USD/price. Returns USD/price/SIGNET terms, token contracts, quote timestamp and a 30-second expiry; no manual price/display-cache fallback, ledger write or signer transfer. The eventual payout worker must persist immutable accepted terms before broadcasting and recover the original signer operation on retry; this quote alone does not authorize transfer. No API or worker uses it to pay yet. Tests cover precise conversion, disabled live pricing, gateway failure, zero price and missing contracts. Real signer/RPC asset mapping and market-price evidence still require deployment validation.

## Company accounting payout quarantine

Recorder now marks additional legacy company accounting rows `requires_reconciliation` in the same transaction as the earnings/outbox insert. The owner's explicitly approved cap diversion remains `awaiting_policy`; neither status sends funds. A failure to set the reconciliation marker propagates and aborts the transaction rather than leaving a payable draft. Migration 009 provides a forward reclassification for untouched company draft rows only (no remote payment ID or transfer hash); it was authored, not executed. Earnings history and company USD balances are not erased or normalized.

Scripted tests cover company-overflow quarantine and marker-failure rollback. Actual cap-diversion worker regression rejects extra unexpected SQL, thereby checking its company instruction is not incorrectly quarantined. Future payout code must exclude `requires_reconciliation` until an audited owner-approved resolution exists. This prevents known ambiguous accounting history from automatically becoming a duplicate treasury payment; it does not yet implement a payout worker, reconciliation UI or real MySQL atomicity testing.

## Durable companion commission terms preparation

Added companion migration/model/private preparation service for immutable source outbox/earn-log/purchase/recipient/role/USD identity and live price/amount/destination snapshots. Preparation is separately disabled by default. It requires a mapped buyer's completed external purchase and an explicitly reconciled `commission_budget_usd` in that purchase's trusted policy snapshot. Locked per-purchase budget aggregation and unique source earnings/role identities reject oversized or duplicate instructions. Company SIGNET address must be explicitly configured; normal recipient addresses come from mapped BSC custody, never a browser-selected address. Retries return the original snapshot and reject changed source terms without requoting or changing destination.

Prepared status is `awaiting_authorization`, not paid or ready to broadcast. There is no source dispatcher/API bridge, treasury debit, signer transfer or ledger entitlement at this stage. Expired quote evidence is retained as history on retry, not silently refreshed or treated as valid transfer authorization. Future delivery must explicitly handle quote acceptance/recovery and enforce the confirmed payout policy. Tests use mocked quotes/signers and do not establish real MySQL concurrency or chain delivery. No production migration or transfer was performed.

## Cross-repository commission preparation bridge

Added restricted companion `/commissions/prepare` and a source preparation worker, separately disabled by `BLOCKCHAIN_COMMISSION_PREPARATION_ENABLED=false` (companion preparation flag is also false). The endpoint derives buyer identity from the restricted source identity field, ignores a caller-supplied alternative buyer and preserves source earnings/outbox/recipient identities. Source worker selects only classified `awaiting_policy` rows belonging to completed local purchases, locks the instruction, submits its original stable identity and persists returned quote/address/remote draft ID as `awaiting_authorization`. It never marks payment completed, broadcasts or credits SIGNET. Failed drafts retain identity/amount and rotate their check timestamp to avoid starving other drafts.

Migration 010 stores explicit role, accepted quote and check timestamp. Newly recorded roles are explicit; historical unclassified drafts require reconciliation rather than inferring roles from descriptions. Company accounting quarantine remains enforced. Gateway checks echoed source identity, recipient, positive price/token amount and USD equality before persisting. API auth/source-binding and scripted worker/gateway tests added. Cross-server live execution, MySQL concurrency, treasury reservation, actual transfer recovery/receipt validation, source payout UI and payout-policy confirmation remain outstanding. None of these flags or migrations were enabled/applied in production.
