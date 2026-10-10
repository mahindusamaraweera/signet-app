# Existing-user wallet and vesting migration

Status: source review and implementation specification. No production data has been migrated.

## Legacy release evidence review — 2026-10-11

The current source `mining_update.go` increments `mining_token` by `daily_mining/1440` every scheduled minute and caps it at `total_token`; it does not record a SIGNET payout. The webhook sends that accrual and marks HTTP delivery success, not chain settlement. No matching legacy `mining_token` webhook receiver was found in the current signet-token application/routes. Production records may reflect older implementations or manual changes, so source arithmetic alone cannot certify previously paid amounts.

The read-only inventory now also exports recorded package activation timestamps and mining webhook delivery status/sent timestamps, with explicit evidence limits. It excludes secret keys and raw webhook response bodies. These fields help locate payment evidence but do not authorize import or reconstruct original vesting dates. Required next evidence: the actual legacy webhook receiver/payout ledger, original allocation/payment terms, and manual adjustment history. Existing user rows and accrual behavior were not changed, and no inventory was executed against a real database.

## Wallet backfill audit update — 2026-10-11

The apply command now requires migration 031, which creates an empty independent `blockchain_wallet_provisioning_audit` table. It does not change existing accounts, balances, packages or expiry enrollment. Each ready wallet must have a nonzero valid BSC address, USDT asset and the explicitly expected chain. Default is mainnet `-chain-id 56`; testnet requires `-chain-id 97`. Record source user ID, chain and lowercase public address before advancing the cursor. Retries cannot overwrite an existing audited address or assign it to another source user. Audit/output failures preserve the previous successful cursor; retry the same user through the source-qualified idempotent service. This wallet operation never imports vesting or credits SIGNET. No real backfill was run.

Verification: full Go suite, static checks and focused backfill/audit race tests passed. SQL audit tests use a scripted driver to assert immutable query behavior; real MySQL uniqueness/concurrency, wallet signer provisioning and production user inventory still require staging verification.

## Preserve accounts

Keep `users.id`, credentials, statuses, referrals, package records and MLM placement unchanged. Provision wallets for existing users through the same source-qualified mapping used for new users: `(signet-app, users.id)`. Never match database IDs between services, silently merge accounts by email, or require users to register again.

Wallet provisioning supports on-demand access and an explicit batch command. Pending, inactive and no-package users remain accounts; wallet assignment does not activate a package or grant a balance. Companion mappings are source-qualified and idempotent. Preserve command output for provisioning success/failure and retry the same user after an interruption.

### Wallet backfill command

Run from `backend/` against an approved test database first:

```sh
go run ./cmd/wallet-backfill -limit 100 -after-user-id 0
```

The default is read-only: it inventories IDs without contacting the wallet service. After reviewing that exact range, provisioning requires explicit `-apply` and enabled integration configuration:

```sh
go run ./cmd/wallet-backfill -apply -limit 100 -after-user-id 0
```

Apply advances its printed `resume_after_user_id` only after ready-wallet responses. It stops on the first error or pending response; resume from the last ready cursor to retry the interrupted user. A lost response is safe to retry under the existing source/user idempotency key. Do not use a dry-run cursor to skip users during apply: it indicates reviewed IDs, not provisioned wallets. Limits are bounded to 1..1000; selection is ascending and includes every source account regardless of package/status. Retain stdout/stderr in the deployment record. Do not run against production until migrations, signer/address allocation and custody recovery are approved. No backfill has been executed against a real database in this workspace.

Tests verify dry-run makes no service calls and failures/pending provisioning never advance past the affected user. These are orchestration tests, not MySQL inventory or live signer validation. Historical vesting import remains separate and unfinished.

## Observed legacy data

### Read-only vesting inventory

`go run ./cmd/vesting-inventory -limit 100 -after-user-id 0` (from `backend/`) exports a bounded, repeatable-read, read-only database snapshot as JSON to stdout. It includes source user/record IDs and raw mining/package values without credentials or contact data. It has no apply mode and does not contact custody, allocate tokens, or change accounts. Run only against an approved database; no real database export has been executed here. Its cursor inventories users, not migrated users.

Package prices in this export are explicitly **current catalog prices**, not proof of historical purchase amounts: `user_packages` does not store a historical price snapshot. `mining_token` is exported as recorded, not classified as paid/released. `import_ready=false` remains until original payment/release evidence, purchase dates, lock terms and overrides are reconciled. Each batch is internally consistent; batches across a changing database are not one global cutover snapshot.

`backend/migrations/001_initial.sql` defines `user_minings` with `total_token`, `mining_token`, `daily_mining`, status and timestamps. `backend/internal/jobs/mining_update.go` increments `mining_token` every scheduler run using `daily_mining / 1440` and caps it at `total_token`. `backend/internal/handlers/mining_handler.go` allows the company role to change total allocation, daily rate and status.

No explicit vesting schedule or administrator vesting-date adjustment was found in the current `signet-app` source. These mining records must not be treated automatically as exact vesting histories. Creation/activation timestamps do not prove the original vesting dates, prior pauses, rate changes, or amounts already paid externally.

## Import procedure

1. Produce a read-only snapshot of users, packages, mining records, balance records and relevant payment/withdrawal evidence. Record snapshot time, row counts and checksums.
2. Prepare a per-record migration manifest with local user ID, source table/record ID, total SIGNET entitlement, verified previously released amount, verified previously paid amount, remaining locked amount, vesting start, cliff/end or remaining release terms, status and any administrator overrides. Flag conflicting or missing evidence for review.
3. Deduplicate by source-qualified record ID and migration version. Check for existing mapped schedules and allocations in the destination before importing. Aggregate mining entitlement must not be expanded into a full allocation for each package.
4. Verify token backing for both locked and available obligations. Historical packages do not represent new USDT deposits: do not fabricate deposits, re-charge users, run fresh swaps, or replay activation commissions during import.
5. Dry-run the manifest and reconcile totals per user and globally. Keep already paid amounts distinct from available balances; do not credit historical releases a second time.
6. At the approved cutover, capture a final snapshot and stop legacy accrual/webhook processing for migrated records. Import the final delta and enable the new release scheduler only after reconciliation. Non-migrated records continue on the old path until their own cutover.
7. Resume interrupted imports from the same manifest. Rollback before any new releases may restore the prior snapshot; once chain transfers occur, use reconciliation and compensating records rather than deleting financial history.

## Administrator time adjustments

The reference `signet-token` implementation provides `VestingService::adjustManualLockPeriod` and the Long Term Stake Plans `adjustLock` action. They support adding/subtracting years, months and days, require a reason, lock the schedule during mutation, preserve released amounts, and record administrator identity, old/new lock ends and released-amount snapshot. Reduction into the past is clamped to the current time.

This service currently accepts only pending/active **manual** schedules. Purchased tier schedules use a different policy; do not assume the existing adjustment action supports them. The integration needs a dedicated policy and endpoint for imported and purchased schedules if administrators must adjust both.

Required adjustment behavior:

- Authorized administrators can shorten or extend the remaining lock based on user performance, with a mandatory reason and a preview of the next release.
- Preserve total allocation and every completed release/withdrawal. No clawback of already released tokens through a date edit.
- Serialize edits with automatic release processing and reject stale schedule versions.
- Use an idempotency key for the adjustment and maintain immutable before/after audit records.
- Show adjustment history and effective release dates to administrators; show effective terms to the user.
- Keep manual performance adjustment separate from automatic score-based acceleration. The existing acceleration engine must not run repeatedly against the same basis and compound reductions unintentionally.

## Decisions needed before importing vesting

- Does `mining_token` mean accrued entitlement, released tokens, or tokens already paid? Confirm against production payment evidence.
- Which records contain original lock/release dates and manual performance overrides, if they are not present in this repository?
- What release terms apply to legacy records, and should the administrator time control apply to both purchased and imported schedules?

No conversion percentage, new vesting duration or historical SIGNET price has been assumed.
