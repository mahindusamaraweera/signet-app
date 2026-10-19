# Correct historical mining totals: users 435, 439, 448

This batch replaces incorrect paper mining with confirmed, locked custody backing.
It does not create new source packages, income-cap earnings or commissions.
The original first batch (429, 440, 458) is not included or changed.

## Reviewed correction

- 435 / package record 468 / mining record 230: USD 1,000, **500,000 SIGNET**.
  Previous paper total: 2,000,000.
- 439 / package record 473 / mining record 233: USD 5,000, **3,500,000 SIGNET**.
  Previous paper total: 16,000,000.
- 448 / package record 482 / mining record 239: USD 1,000, **500,000 SIGNET**.
  Previous paper total: 2,000,000.

The operator confirmed the old token/mining amounts were incorrect and asked to
stop legacy mining, replace totals, and require normal linked Tier Option 1/2
eligibility and confirmation before releases. Existing active package records,
referral placement and package earnings remain unchanged. Source activation
wall-clock timestamps are retained as reviewed original-purchase date evidence.

## Source cutover

Migration 037 creates an append-only cutover audit table. The command validates
all three current owner/UUID/email/package/mining identities before any writes.
It archives exact current mining amounts and webhook state, then atomically sets
corrected `total_token`, `mining_token=0`, `daily_mining=0`, status `inactive`, and
webhook status `historical_frozen`. A failed batch rolls back all three changes.
The signed snapshot preserves the discarded paper accrual; it is never treated
as paid or released real SIGNET. Prior real payments still require reconciliation.

The scheduler excludes frozen users and uses a conditional per-row update, so
an already-read old mining row cannot overwrite the correction. Legacy webhook
selection and dispatch exclude cutovers. The legacy mining administration route
rejects attempts to re-enable or overwrite migrated records.

Deploy the new source API, migrate with its normal production environment, and
build the command on the source host. Example using the same systemd environment
file as `signet-api.service` (no shell sourcing or printing secrets):

```bash
cd /opt/signet/backend
go build -o bin/migrate ./cmd/migrate
go build -o bin/historical-mining-cutover ./cmd/historical-mining-cutover
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/bin/migrate
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/bin/historical-mining-cutover
```

The last command is read-only and reports exact before/after evidence. After
reviewing it, **stop every source API/scheduler/legacy mining/webhook process and
drain any in-flight webhook** before applying the cutover. For the known API:

```bash
sudo systemctl stop signet-api.service
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/bin/historical-mining-cutover --execute --services-paused \
  > /opt/signet/backend/historical-mining-cutover-proof.json
sudo systemctl start signet-api.service
sudo systemctl is-active signet-api.service
```

Do not restart an old API binary or old cron worker; deploy all updated scheduler
and webhook guards. The flag attests all source workers were stopped/drained;
it cannot detect independently running legacy services. Failed freeze execution
must be investigated before restarting any old mining worker. No tokens are sent
by this source command. Re-running apply is idempotent for already-frozen rows;
it validates their archived snapshot and current stopped state.

Copy the proof securely to the Token host, for example as
`/home/forge/historical-mining-cutover-proof.json`. It includes only this reviewed
batch, its archived mining values and an HMAC signature. It contains no passwords,
OTP codes, private keys or integration credential. The domain-separated signature
uses the existing shared integration service token on both servers.

If the proof expires after 30 minutes, export a fresh one without changing mining:

```bash
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/bin/historical-mining-cutover --export-proof \
  > /opt/signet/backend/historical-mining-cutover-proof.json
```

## Custody dry run and backing

Deploy the matching Signet-token code. The second command is separate from the
original three-user import and requires the signed frozen-source artifact.

```bash
cd /home/forge/token.signetcoin.net/current
php artisan config:cache
php artisan horizon:terminate
php artisan signet:import-production-historical-mining-packages \
  /home/forge/historical-mining-cutover-proof.json
```

Dry run never derives wallets, signs, credits or resets balances. It verifies
signature, scope, freshness and each corrected amount. It reports the actual
UUID custody destinations (or needed provisioning). Existing balances, ledger,
withdrawal or vesting/package history stops import rather than adding duplicate
entitlement. Do not bypass that reconciliation. A webhook delivery flag is not
proof a real transfer was paid: inspect original receiver/custody history before
attesting no real tokens were previously paid or released.

Only after operator review and reconciliation:

```bash
php artisan signet:import-production-historical-mining-packages \
  /home/forge/historical-mining-cutover-proof.json \
  --execute --confirm-source-records
```

This sends **4,500,000 real SIGNET** in total plus BSC gas. It checks the existing
wallet, actual on-chain prebalance, configured production treasury/contract and
confirmed treasury-to-user receipt. Stable per-source-package signer keys and
unique allocation rows resume uncertain/pending operations; do not send a new
manual transfer or alter operation IDs. The batch may be partially confirmed;
rerun the same command to reconcile remaining receipts. Export a fresh proof if
needed; its frozen snapshot stays unchanged. Already completed first-batch and
second-batch entries are never reallocated.

Confirmed transfers create exactly the corrected locked amounts, available=0,
released=0, and `awaiting_tier` schedules. The USD 5,000 package keeps its own
principal for percentage-based eligibility. Users connect their Tier accounts,
check the available option and confirm it through the normal interface. Ordinary
allocation recovery, source income-cap rules and withdrawal audits remain in
force. No legacy paper accrual, swap, new bonus, new commission or company
settlement is generated by this import.
