# One active withdrawal and shared cooldown

Deploy signet-token first, then the signet-app Go backend and frontend. No schema migration or manual balance change is required. The app fails closed for new preparations/queue submissions if the token policy endpoint is missing, malformed or unavailable; existing payout/cancellation workers retain their stable identities.

The token setting `WithdrawalSettings::get('withdrawal_cooldown_minutes')` is authoritative. No separate app-side cooldown is configured. It is measured from the previous custody request's creation time, including cancelled requests, matching the previous token policy. Completing a request does not restart this timer. At the exact deadline the cooldown is over.

Any unresolved custody withdrawal (including audit/security holds, failed broadcasts and reconciliation failures) blocks another request for the same custody user, across assets. Completion or confirmed cancellation allows a new request only after any remaining cooldown. Old failures with a verified fully-refunded historical ledger retain their existing closed treatment.

Both source preparation and final queue consumption check for an existing local pending/cancellation/review outbox entry while holding the owner lock. Token reservation independently checks all custody liabilities while holding its user lock. A direct token request for a source custody account also uses this policy. Two tabs may prepare separate drafts, but cannot enqueue two new intents while one is pending. Repeating the original signed custody authorization returns its existing withdrawal before the new-request gate, without another reserve or fee.

The read-only `/api/v1/integration/withdrawals/admission` endpoint returns only owned active-withdrawal ID, configured cooldown minutes, cooldown deadline, current server time and explicit admission status. The authenticated app user never selects an unrelated custody identity. The UI shows the blocking request or a local-time cooldown countdown. Countdown expiry does not submit a transaction or bypass server checks.

Existing signed/queued requests are not silently cancelled, reset or refunded by deployment. Audit failures still require review and retry of the same withdrawal. Fee processing remains tied to the original completed payout and original deduction.

After deploying token code, refresh cached routes/configuration and restart queue workers before deploying the app:

```bash
cd /home/forge/token.signetcoin.net/current
php artisan route:clear
php artisan config:cache
php artisan horizon:terminate
php artisan route:list --path=api/v1/integration/withdrawals
```

The route listing must include `withdrawals/admission`. Then deploy the app Go binary and frontend. There is no migration to run for this change.

Read current production cooldown on the token server:

```bash
cd /home/forge/token.signetcoin.net/current
php artisan tinker --execute='dump(["withdrawal_cooldown_minutes" => app(\App\Services\Withdrawal\WithdrawalSettings::class)->get("withdrawal_cooldown_minutes")]);'
```

Verification covers blocking active custody states even with zero cooldown, local pending rollback before approval consumption, original authorization retries, signed rejection of a second identity without another reservation/fee, completion/cancellation, live settings changes and the exact cooldown deadline. Local tests use isolated databases and mocked integration transport, not live payouts.
