# Placement correction 486: hold, recover, reconcile

Fixed scope: source user 486 / UUID 1dde762b-4d54-4529-86ff-28b8cee471b4,
source package 521, source intent 29, custody purchase 59, schedule 5111.
Buyer investment and 355545.377735458375569300 locked SIGNET are preserved.
No refund, new allocation, placement edit, reactivation, or replacement commission
is performed by this tooling. Buyer remains inactive/frozen after recovery.

## Deploy both updates before any execute command

Source branch blockchain-automation. Deployment applies embedded migrations 041
and 042 and installs /opt/signet/backend/hold-486. Native Laravel must apply both
2026_11_20_000001 and 2026_11_20_000002 migrations. Earlier hold-only release is
not sufficient. A prior dry run has not changed an account or reserved funds.

Source preflight verifies four original confirmed payments and two secondary
company accounting entries, their exact earnings IDs, original time, custody
payment IDs, token amounts, recipient UUIDs, current USD wallets and package
counters. It refuses any unbound global-share credit at the original event time:
legacy records cannot reliably identify partially capped contributions. Such a
failure requires a separate attribution audit, not bypassing this guard.
Source schema inspection selects its actual supported inactive package enum.

### 1. Ubuntu source: dry run, then hold and export proof

```sh
cd /opt/signet/backend
sudo systemd-run --quiet --wait --pipe --collect \
 -p EnvironmentFile=/opt/signet/backend/.env.production \
 -p WorkingDirectory=/opt/signet/backend \
 /opt/signet/backend/hold-486
```

Review dry-run accounting plan. No state or balances have changed. Then:

```sh
sudo systemd-run --quiet --wait --pipe --collect \
 -p EnvironmentFile=/opt/signet/backend/.env.production \
 -p WorkingDirectory=/opt/signet/backend \
 /opt/signet/backend/hold-486 --execute \
 --reason='Approved correction of account 486 placement; reverse original commissions and keep buyer inactive.' \
 > /opt/signet/backend/placement-486-hold-proof.json
python3 -m json.tool /opt/signet/backend/placement-486-hold-proof.json > /dev/null
base64 -w 76 /opt/signet/backend/placement-486-hold-proof.json
```

This changes only buyer source status and package status, with a permanent audit
record and accounting plan. It does NOT debit recipients or send tokens. Completed
intent, original commission rows, placement, and financial history are retained.
Repeating execute exports a fresh proof from the same held record. Proofs expire
in 30 minutes. Do not remove the hold while recovery is incomplete.

### 2. Forge: import proof and reserve funds

Copy proof using wrapped Base64 (avoid unwrapped long terminal JSON):

```sh
base64 --decode > /home/forge/placement-486-hold-proof.json
```

Paste all wrapped lines, Enter, Ctrl+D. Then:

```sh
cd /home/forge/token.signetcoin.net/current
python3 -m json.tool /home/forge/placement-486-hold-proof.json > /dev/null
php artisan migrate --force
php artisan horizon:terminate
php artisan signet:hold-placement-486 /home/forge/placement-486-hold-proof.json
```

After reviewing dry run:

```sh
php artisan signet:hold-placement-486 /home/forge/placement-486-hold-proof.json \
 --execute --admin-email='signetcoin@gmail.com' \
 --reason='Approved correction of account 486 placement; reverse original commissions and keep buyer inactive.'
```

Expected held, funds_sent false, commission_recovered false. One transaction
freezes only the buyer, puts schedule 5111 into correction_hold and reserves each
commission in user_correction_reserve ledger escrow. Other recipients' locked
vesting is untouched. Available balances are checked in aggregate (377's direct
and leadership together), and user locks are acquired in ID order. Failures roll
back all reservations. Repeats verify records without additional debits.

### 3. Forge: recover original tokens to treasury

```sh
php artisan signet:hold-placement-486 --recover
php artisan signet:hold-placement-486 --recover --execute \
 --admin-email='signetcoin@gmail.com' \
 --reason='Approved exact recovery of the four original commissions for placement correction 486.'
```

Dry run verifies original credits and hold integrity. Actual recovery uses each
frozen derived wallet and one permanent signer key per commission. It recovers
an existing completed operation or resumes the SAME prepared/broadcast signed
transaction. Failed/reverted or unknown states require reconciliation; NEVER
invent new attempt keys. A new transfer requires unchanged custody/derivation,
original treasury, asset configuration and sufficient on-chain funds. Exact
sender debit, treasury receipt, contract, hash and confirmation are verified.

Original amounts:
- term 77 / source 471 gratitude: 14221.815109418335022772 SIGNET
- term 78 / source 377 direct: 35554.537773545837556930 SIGNET
- term 79 / source 377 leadership: 3555.453777354583755693 SIGNET
- term 80 / source 378 executive: 3555.453777354583755693 SIGNET
Total: 56887.260437673340091088 SIGNET (original USD 800 entitlement).

If recovering/confirming, repeat the SAME recovery execute command. Each receipt
settles its existing escrow once; it does not debit available funds again. Only
confirmed recoveries reduce the settled-earnings counter. Original payment rows
remain confirmed historical receipts, with separate audited recovery records.

STOP on any error and inspect it. A bad receipt cannot release escrow or create
completion proof. Partial successes are durable and retries skip them. Do not
reset status, delete records, unfreeze or manually transfer funds.

### 4. Forge to Ubuntu: finalize source accounting

Only once status is recovered with all four line recoveries confirmed:

```sh
php artisan signet:hold-placement-486 --export-recovery-proof \
 > /home/forge/placement-486-recovered.json
base64 -w 76 /home/forge/placement-486-recovered.json
```

Copy wrapped Base64 to Ubuntu:

```sh
base64 --decode > /opt/signet/backend/placement-486-recovered.json
```

Paste lines, Enter, Ctrl+D, then:

```sh
cd /opt/signet/backend
python3 -m json.tool /opt/signet/backend/placement-486-recovered.json > /dev/null
sudo systemd-run --quiet --wait --pipe --collect \
 -p EnvironmentFile=/opt/signet/backend/.env.production \
 -p WorkingDirectory=/opt/signet/backend \
 /opt/signet/backend/hold-486 \
 --finalize-proof=/opt/signet/backend/placement-486-recovered.json
```

Expected recovered, source_accounting_reversed true, buyer_still_held true,
funds_sent false. This command sends NO blockchain payment. It atomically:
- Verifies all four distinct signed native recovery receipts, exact amounts and
  original source identities/history.
- Reverses source USD wallets: 377=550, 378=50, 471=200; company bookkeeping=550.
- Offsets first-package earn only for 378=50 and 471=200. 377's two original
  contributions did not increment that first-package counter, corroborated by
  the two company overflow rows; do not subtract those counters again.
- Appends six linked negative earn logs and durable before/after audit evidence.
- Leaves the completed intent and original payments untouched and buyer held.

Repeating finalization cannot repeat the offsets; a changed receipt hash refuses.
Expired completion proof can be re-exported on Forge without another recovery.

### 5. Stop for placement review

After native recovery AND source accounting completion, developer may prepare
placement changes, including the saved gratitude rule for this package and future
top-ups. Do not simply delete the original gratitude history or reset intent 29.
Corrected recipients and the no-gratitude replacement manifest need review before
separate commission reissue and reactivation tooling. These commands deliberately
leave the buyer's deposit/allocation intact and account held.

## Validation limits

Go SQL transaction/race tests and Laravel SQLite integration tests use mocked
signer/RPC receipts. SQLite fixture monetary TEXT columns emulate MySQL DECIMAL
string reads. This is not a live mainnet or real MySQL concurrency test. Source
preflight and native dry run must pass on current production records.
