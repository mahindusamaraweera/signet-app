# Production custody collision recovery

The UUID protocol is implemented in both services. Deploy both together; the proof below reads the actual production UUIDs rather than inferring custody ownership from numeric IDs.

Deploy this branch's migrations 035 (the dev branch UUID migration) and 036 (purchase package snapshot) before the API binary. New purchase intents freeze the selected package ID. App API/backfill requests now send `X-Signet-App-Environment` using `APP_ENV`; configure production custody to require `production`.

CI includes `backend/custody-recovery-proof` and `deploy/custody-recovery-proof.sh`. The wrapper must run with the production API stopped and reads its existing environment file without shell evaluation. It creates a read-only, signed proof of pending, commission-free purchases:

```bash
sudo systemctl stop signet-api.service
umask 077
sudo bash /opt/signet/deploy/custody-recovery-proof.sh --users=459,460,461,462,463 > /tmp/signet-production-recovery-459-463.json
```

Full custody deployment, dry-run, execution and verification instructions are in the Signet-token repository at `docs/operations/PRODUCTION_CUSTODY_COLLISION_RECOVERY.md`. Keep staging disabled and separate its custody credentials/database permanently. This is a recovery of different people sharing numeric IDs, not permission to delete the staging financial records or replay confirmed commission payments.

Production gateway requests now use `users.uuid` as `external_user_id` and stable customer-scoped UUIDv5 purchase/commission/placement identifiers. Numeric foreign keys remain internal. Set custody `SIGNET_APP_REQUIRE_UUID=true`; the full wire contract is in the Signet-token repository at `docs/operations/UUID_INTEGRATION_PROTOCOL.md`. Missing UUIDs and pre-existing numeric custody links fail closed until ownership recovery.
