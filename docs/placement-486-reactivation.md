# Placement 486 reactivation after confirmed commission recovery

Scope: source buyer 486, UUID `1dde762b-4d54-4529-86ff-28b8cee471b4`, source package 521, completed intent 29, native purchase 59, native vesting schedule 5111.

Operator-approved replacement: direct-team 471 USD 500; leadership 377 USD 50; executive 378 USD 50; gratitude zero. Quote each payment using the original confirmed purchase swap output, never a new market price. The recovered gratitude USD 200 equivalent remains in treasury; no separate company transfer or duplicate company wallet credit is created.

This is a separate correction, not purchase replay. The original intent, sealed commission manifest, confirmed original payments, negative reversal logs and buyer allocation remain unchanged. Source package `ref_id` becomes the approved current referral 471. Original intent's saved commission recipient 377 remains historical evidence. Source gratitude row 87 is copied into correction evidence before its removal, including original timestamps, so neither this sale nor future top-ups select that obsolete entitlement. Placement row 674 must remain the sole buyer row with virtual/parent 471 and active node; no placement rewrite is performed.

## Deploy prerequisites

Deploy latest changes in both repositories. Keep signet-app on `blockchain-automation`. Source binary remains `hold-486`, already installed by the existing workflow. Apply source migration **043** and native migration **2026_11_21_000001**. Do not run old binaries with these flags.

Source server:

```bash
cd /opt/signet/backend
SIGNET_RELEASE=$(cat /opt/signet/REVISION)
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  "/opt/signet/releases/$SIGNET_RELEASE/migrate" up
```

Native server:

```bash
cd /home/forge/token.signetcoin.net/current
php artisan migrate --force
php artisan horizon:terminate
```

## 1. Review source preview, then prepare once

```bash
cd /opt/signet/backend
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/hold-486 --reissue
```

Review exact recipients and USD amounts. The preview sends no funds, changes no data and keeps buyer inactive. Source verifies UUIDs, recipient active status, normal active-package income caps with existing top-up reservations deducted, completed original purchase, recovered correction, approved placement and exact obsolete gratitude row. It fails rather than silently rerouting commissions.

After preview passes:

```bash
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/hold-486 --reissue --execute \
  --reason='Approved corrected commissions: direct 471 USD500, leadership 377 USD50, executive 378 USD50, gratitude zero.' \
  > /opt/signet/backend/placement-486-reissue.json
python3 -m json.tool /opt/signet/backend/placement-486-reissue.json > /dev/null
```

This transaction applies source wallet/earn/global-share accounting under existing rules, stores linked earn log IDs and archived gratitude evidence, removes obsolete row 87, updates current package referral, and creates a unique prepared correction. It sends no tokens and keeps the buyer inactive. Normal secondary company accounting, where applicable, creates no second payment entitlement. Repeating this same command exports a fresh proof without crediting source wallets again.

Copy the exact signed JSON file to `/home/forge/placement-486-reissue.json` on native server. Proof lasts 30 minutes; do not edit payloads/signatures. Use scp or encode the entire JSON with Base64, not just its payload.

## 2. Review native preview, then deliver exact three payments

```bash
cd /home/forge/token.signetcoin.net/current
php artisan signet:reissue-placement-486 /home/forge/placement-486-reissue.json
```

Verify all destinations and SIGNET amounts. Execute only with the same source proof:

```bash
php artisan signet:reissue-placement-486 /home/forge/placement-486-reissue.json \
  --execute --admin-email='signetcoin@gmail.com' \
  --reason='Approved corrected placement 486 commissions: 471 USD500, 377 USD50, 378 USD50; no gratitude.'
```

Repeat the **same** command for pending confirmations. Every role has a fixed operation key and unique ledger credit. Completed signer operations reconcile after lost responses; prepared/broadcast operations resume the same transaction. Failed/reverted operations stop for review. Exact successful receipt checks bind contract, treasury debit, recipient credit, amount and transaction hash. No ledger credit occurs on pending/incorrect receipts. All four prior recoveries must be verified. Buyer remains frozen with schedule `correction_hold` and unchanged locked amount.

Only after top-level status is `completed` and all three rows are credited:

```bash
php artisan signet:reissue-placement-486 --export-completion \
  > /home/forge/placement-486-reissue-completed.json
python3 -m json.tool /home/forge/placement-486-reissue-completed.json > /dev/null
```

Copy this file to `/opt/signet/backend/placement-486-reissue-completed.json` on source.

## 3. Activate source existing records and export release proof

```bash
cd /opt/signet/backend
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/hold-486 \
  --reissue-completion=/opt/signet/backend/placement-486-reissue-completed.json \
  > /opt/signet/backend/placement-486-release.json
python3 -m json.tool /opt/signet/backend/placement-486-release.json > /dev/null
```

This verifies the signed three-payment completion, confirms placement/roles and absence of obsolete gratitude, marks user and existing package active, and stores completion once. It does not call purchase activation, add another pool/allocation, change the original activation date or pay commissions. Repeat with fresh native completion proof if needed; a completed retry must match original receipts and only exports a fresh release proof.

Copy release proof to `/home/forge/placement-486-release.json` on native.

## 4. Release native buyer, preserving tier requirements

```bash
cd /home/forge/token.signetcoin.net/current
php artisan signet:reissue-placement-486 /home/forge/placement-486-release.json \
  --release --execute --admin-email='signetcoin@gmail.com' \
  --reason='Approved release after corrected placement 486 commissions and source activation completed.'
```

Expected `status: active`, `vesting_status: awaiting_tier`, buyer allocation preserved `355545.377735458375569300`, `funds_sent: false`. No automatic Option 1 exception, vesting backdating or new treasury allocation. Tier connection/Option 1 or 2 eligibility remain the existing system rules. If another security freeze or schedule change appears before release, stop and investigate; the command requires the original correction hold.

## Verification limits

Go scripted SQL tests cover dry run, exact placement/caps, source preparation, accounting call ordering, rollback and completion retry behavior. They do not replace a real MySQL concurrency exercise. Native tests use exact decimal SQLite fixtures and mocked signer/receipts, cover partial/pending recovery, lost signer response, duplicate prevention, bad receipts, source-proof release and withdrawal-ledger audits. Production success requires the reviewed previews, chain confirmations and both completion outputs above.
