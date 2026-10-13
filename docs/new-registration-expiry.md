# New-registration expiry and partial-deposit reminders

Implemented on `blockchain-automation`, 2026-10-11. No main merge, deployed database access, real account deletion, financial transfer or signet-token source change was performed.

## Existing users are excluded

Migration `030_new_registration_expiry.sql` creates two empty tables only. It does **not** insert, update, delete or enroll any existing user. Only newly completed email-verified registrations insert `new_unpaid_v1` enrollment in the same account-creation transaction. Never backfill this table from `users`. The scheduler joins enrollment, and the deletion transaction independently checks enrollment again. Existing pending, active and inactive accounts remain untouched.

## New unpaid accounts

The existing 10-hour placement deadline remains. Verified new registrations that never select a package expire 10 hours after account creation. Processing runs every minute on the scheduler-enabled API instance. Candidates rotate by last-check time so protected or unavailable accounts do not starve subsequent candidates.

Only pending ordinary users can expire; company/admin accounts and user ID 1 cannot. Any recorded deposit (including partial deposits), nonzero balance, financial history, activated package, mining record, withdrawal authorization, Tier link, pending/in-progress purchase or affected downstream user prevents deletion. Local integration evidence is checked even if blockchain execution is disabled. With blockchain disabled, account deletion fails closed rather than bypassing custody checks.

The existing signet-token integration API must provide an exact durable custody seal: ready BSC wallet, zero USDT/SIGNET available/locked/withdrawal balances, a healthy caught-up scanner and on-chain zero-USDT check. The source independently rejects any detected deposit after sealing. Source expiry can provision the account's source-qualified integration wallet if needed; it never selects native signet-token users by matching email. Accounts with no placement use a stable high-bit account-only reference (`2^63 + source_user_id`) with the existing seal API, not a fabricated MLM placement. A seal replayed from an earlier attempt is insufficient for deletion: preserve the account for reconciliation. Keep server clocks synchronized.

Local financial/history checks use MySQL locking reads, including inside scalar subqueries, to avoid authorizing deletion from a stale snapshot ([MySQL locking-read documentation](https://dev.mysql.com/doc/refman/8.4/en/innodb-locking-reads.html)). Matching locks are held through local deletion. Deadlocks, API errors, missing/malformed evidence, audit failures and lost deletion ownership roll back local changes. Validate actual MySQL locks and execution plans in staging before enabling; local tests use scripted SQL, not a live MySQL database.

## Audit and email reuse

Before deletion, `registration_account_deletions` stores account metadata (no password or authenticator secret), package/purchase/owned-placement snapshots, deadline, old source identity and custody seal. It has no user foreign key. The existing placement-expiry audit and withdrawal-security audit remain. Purchase intents are retained as expired evidence; enrollment tombstones remain as an additional barrier against reusing an old source user ID.

Delete only the cleared account's referral code, placement/log rows, unpaid package placeholders, zero-balance wallet placeholders, session/security setup and reset proofs, then its `users` row, all in one transaction. Old verified registration proofs are marked `account_expired` and scrubbed of credentials. The unique email is freed for a fresh email-verified registration with a new ID. No other user's placement rows are deleted, even if their IDs appeared in the old placement creation log.

The remote custody mapping, deposit address, ledger and blockchain history are **not** deleted. A transfer sent to an old address after expiry must be reconciled against the old audited source identity; it must never automatically belong to a later account using the same email. An account preserved after a failed attempt that already sealed custody may need support reconciliation before purchase activation. No automatic historical-account repair or late-deposit ownership reassignment is included.

## Partial-deposit reminder

The funding and wallet pages show a popup after a recorded partial deposit, stating that the account is protected and displaying the exact server-calculated remaining USDT. It appears once per purchase/deficit value per mounted page, not every 15-second refresh. Zero/excess balances and processing/completed/expired purchases do not trigger the popup. The existing inline funding notice remains. This is a reminder only; it never submits a transaction or changes package/MLM terms.

## Deployment and test gates

1. Apply migration 030 with the normal signet-app migration command and rebuild/restart the API and frontend. Do not reset the database.
2. Verify existing-user enrollment count is zero. Do not backfill enrollment or alter old account dates/statuses.
3. Enable `BLOCKCHAIN_ENABLED=true`, `REGISTRATION_EXPIRY_ENABLED=true` and `ENABLE_SCHEDULER=true` on exactly one API scheduler instance. Enable the existing integration-only `SIGNET_APP_REGISTRATION_EXPIRY_ENABLED=true` on signet-token with its integration configuration; no native-user configuration or flow changes are required.
4. Use newly created staging accounts: unpaid expiry/email reuse; partial funding protection and popup; one-base-unit deposit; unconfirmed detected deposit; exact/excess funding; swap-confirming protection; custody/scanner outage; concurrent funding; audit rollback; stale-seal reconciliation. Old users must remain unchanged in every case.

No live registration-to-expiry/deposit test was executed by this implementation. Operator staging verification remains necessary before relying on automated deletion.

Local verification: full Go test suite and `go vet -p=1 ./...` passed; focused expiry/registration/migration tests and expiry race tests passed; frontend tests passed (12 tests) and production build passed (existing large-chunk warning). These checks do not replace staging MySQL concurrency or browser verification.
