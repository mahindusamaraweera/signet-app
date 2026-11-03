# Saved gratitude recipient and purchase 12 correction

First automatic package activation now resolves its one gratitude recipient from
`super_parent_logs` bound to BOTH buyer and `user_package`. This log survives
signup placement rows that leave the buyer's own node active. A matching legacy
gratitude node is the same entitlement and is not credited twice. Conflicting or
multiple saved recipients stop the activation transaction for review. Ordinary
buyers without a saved gratitude log or gratitude node receive no extra bonus.
The existing 20% rate, income cap routing, purchase lock, outbox identity, and
custody preparation/delivery rules remain in force. Top-up rules are unchanged.
This does not broadly replay old completed purchases.

## Fixed recovery approved by operator

Buyer 470, source package 505, purchase intent 12, recipient 132, USD 200.
Original source log 81 identifies recipient 132. The original commission outbox
contains confirmed direct/leadership/executive terms, no gratitude term.
Migration 040 adds the durable one-operation source record. The separate native
correction preserves the sealed original company manifest and payments.

Recovery checks that the original company settlement is confirmed on BOTH
servers. Source user/intent locks serialize retries. Source preparation refuses
any existing gratitude instruction (including unpaid/diverted instructions),
checks available income cap including reserved top-up commissions, and atomically
records one USD 200 wallet credit, earn log, `recovery_pending` outbox and recovery
record. No signer or token transfer runs on signet-app.

Native recovery validates a 30-minute signed source proof, immutable UUID
bindings, purchase ownership and original confirmed swap basis, and requires a
named native administrator/reason. It records a separate administrator-approved
recovery. One permanent signer operation `gratitude-470-132-505-v1` is recovered
before attempting a transfer; an uncertain/failed operation never gets a new key.
Exact confirmed token receipt and net treasury debit are required before a
single ledger credit. SIGNET amount is USD 200 at the ORIGINAL purchase swap
basis, not current price. This is a treasury-funded correction because the
original company remainder may already have been converted. It counts as
settled earnings under normal allocation-recovery rules and is withdrawal-audited.

## After deploying BOTH repositories

Go migration 040 is applied by deployment's embedded migration runner. Native
Laravel migration `2026_11_03_000001_create_gratitude_recoveries.php` must also run.
The deployment packages `/opt/signet/backend/gratitude-recovery` (no production
Go compilation needed). Do not modify the original outbox or sealed manifest.

On Ubuntu (signet-app), dry-run:

```sh
cd /opt/signet/backend
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/gratitude-recovery
```

Prepare source accounting and export proof (does NOT send tokens):

```sh
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/gratitude-recovery --execute \
  --reason='Approved missing USD 200 gratitude for recipient 132, buyer 470, package 505.' \
  > /opt/signet/backend/gratitude-470-proof.json
base64 -w 76 /opt/signet/backend/gratitude-470-proof.json
```

Copy proof to `/home/forge/gratitude-470-proof.json` on Forge. If copying through
terminal, use `base64 --decode > /home/forge/gratitude-470-proof.json`, paste all
wrapped Base64 lines, Enter then Ctrl+D. Validate `python3 -m json.tool FILE`.
Proofs expire after 30 minutes; repeating source `--execute` exports a fresh proof
from the SAME record without another USD credit.

On Forge (signet-token):

```sh
cd /home/forge/token.signetcoin.net/current
php artisan migrate --force
php artisan horizon:terminate
php artisan signet:recover-gratitude-470 /home/forge/gratitude-470-proof.json
php artisan signet:recover-gratitude-470 /home/forge/gratitude-470-proof.json \
  --execute --admin-email='admin@signetcoin.net' \
  --reason='Approved missing USD 200 gratitude for recipient 132, buyer 470, package 505.'
```

Review dry-run amount/destination first. If confirming, rerun the SAME execute
command to reconcile. Completed reruns do not transfer or credit again. Do not
send a replacement, reset a status, delete a record, or alter the operation key.
A guard failure is a review requirement, not permission to bypass the guard.

After native `completed`, export signed acknowledgement:

```sh
php artisan signet:recover-gratitude-470 --export-completion \
  > /home/forge/gratitude-470-completed.json
base64 -w 76 /home/forge/gratitude-470-completed.json
```

Copy it to `/opt/signet/backend/gratitude-470-completed.json` using the same
wrapped Base64 technique. On Ubuntu:

```sh
cd /opt/signet/backend
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/gratitude-recovery \
  --confirm-proof=/opt/signet/backend/gratitude-470-completed.json
```

Acknowledgement only marks the source outbox confirmed with the existing tx hash;
it cannot credit USD or broadcast. Repeating it is safe; a changed hash refuses.
Fresh acknowledgement can be exported after expiry without another payment.

## Verification limits

Go tests use a scripted SQL driver, including the existing activation lock
concurrency harness. Laravel integration tests use SQLite and mocked RPC/signer
receipts. They do not prove real MySQL concurrency or live on-chain delivery.
Production execution and confirmation are separate operator steps.
