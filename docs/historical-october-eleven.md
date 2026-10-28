# October historical allocation batch: eleven users

Batch `historical-october-eleven-v1` covers source IDs
436,441,442,443,444,446,449,450,455,456,457 and 5,800,000 SIGNET.
The operator corrected CSV ID 499 to 449 (buddikaanura04@gmail.com), and confirmed
456 as bhmadubashi81@gmail.com. Actual package prices are 500/1,000/2,500 USDT,
not CSV gross figures 580/1,330/2,780. Token entitlements remain the CSV amounts.
The listed scope includes source package activations after October 10; the CSV
filename is not a date filter. No already-funded prior batch users are included.

Migration 039 permits a NULL mining ID in account cutover evidence. Users 444 and
446 have no mining rows: the command verifies this absence under a serializable
transaction and creates only the permanent user-level freeze marker. It does not
invent mining records, IDs, accrued tokens, deposits or payments. If a mining row
appears for either user, both dry run and export refuse it. Existing admin mining
and scheduler/webhook guards already enforce the user-level freeze markers.
The other nine rows are archived, corrected and stopped in the same transaction.

Deploy the updated source API, CLI and migration binary (normal deployment runs
the embedded migration). All independent legacy workers must use updated guards.

## Read-only source preview

```bash
cd /opt/signet/backend
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/historical-mining-cutover \
  --batch historical-october-eleven-v1
```

Review all eleven identities and entitlements. No funds or mining change on preview.

## Freeze and sign

Stop ALL API/scheduler/webhook processes and drain in-flight webhooks first.
`--services-paused` is an operator assertion, not automatic process detection.

```bash
sudo systemctl stop signet-api.service
sudo systemd-run --quiet --wait --pipe --collect \
  -p EnvironmentFile=/opt/signet/backend/.env.production \
  -p WorkingDirectory=/opt/signet/backend \
  /opt/signet/backend/historical-mining-cutover \
  --batch historical-october-eleven-v1 --execute --services-paused \
  > /opt/signet/backend/historical-october-eleven-proof.json
```

Only after command success, restart the updated API:

```bash
sudo systemctl start signet-api.service
sudo systemctl is-active signet-api.service
base64 -w 76 /opt/signet/backend/historical-october-eleven-proof.json
```

Transfer the original JSON securely to the token host. Alternatively paste ALL
short Base64 lines (including trailing padding) into this command on Forge, then
Enter and Ctrl+D:

```bash
base64 --decode > /home/forge/historical-october-eleven-proof.json
```

The signature uses the existing shared integration token without revealing it.
Proofs expire in 30 minutes. For a fresh read-only export, rerun the source systemd
command with `--batch historical-october-eleven-v1 --export-proof` and redirect to
the same source file; then copy it again. Re-export neither re-freezes nor funds.
Custody operations and reconciliation are documented in signet-token's
`docs/operations/HISTORICAL_OCTOBER_ELEVEN.md`.
