# Shared Tier identity verification

Signet-token is the authority for Sumsub. Signet-app does not connect to Sumsub, hold
its credentials, embed its SDK, upload documents or manually approve KYC.

## Account and approval flow

1. A user connects their Tier account through the existing `/wallet/tier-link` flow.
   It proves account control with the existing session, PKCE, state and Tier approval.
2. Signet-app requests `/api/v1/integration/tier-link/kyc` using its existing backend
   integration credentials, environment header and UUID identity protocol.
3. Token resolves the stored Tier link, not a browser-supplied account or matching email.
   The result is independent of purchases, Tier level and selected vesting option.
4. A current production approval is recorded in `tier_identity_verifications` and shown
   as verified. Rejected/pending/Sandbox results are recorded but do not count as verified.
5. Unverified linked users continue on the Tier verification page, signing in to their
   connected Tier account if required. The page checks the expected account UUID.
6. Tier and Assets use the same approval when memberships belong to that Laravel user.
   This does not create an Asset membership, combine different users or merge balances.

The local mirror contains identifiers, status, environment, level and decision expiry;
no documents or legal identity details. It is refreshed after linking, on `/kyc/show`,
and by the existing scheduler in bounded batches. An expired mirror is not proof of KYC.
Every withdrawal remains subject to Token's fresh admission, reservation and pre-send
checks. Revocation cannot be bypassed by stale local records or legacy manual approvals.
Legacy `kycs` rows/files remain intact for retention and existing account-history rules.
All old upload/edit/delete/verify/unverify API routes return 410 after authentication;
old page URLs redirect to the new status page. `/storage/kyc/*` is no longer publicly
served by the backend. Ensure any independent CDN/static-file aliases also do not expose
historical documents; the standard Nginx config already proxies this route to the API.

## Deploy

- Deploy signet-token's shared-KYC changes first, then the Assets release sequentially.
- In each Laravel environment set `SUMSUB_WITHDRAWAL_KYC_REQUIRED=true`,
  `SUMSUB_ENVIRONMENT=production`, and configure the production Sumsub credentials.
  Refresh config/Horizon. A new account needs production KYC before withdrawing.
- On Signet-app, rebuild and run the existing migration tool from `backend/`:

```sh
go run ./cmd/migrate status
go run ./cmd/migrate
```

- Migration 038 creates only the mirror table; it does not alter legacy KYC records.
- Build/restart the backend and deploy `frontend/dist` through the normal process.
  Keep `ENABLE_SCHEDULER` enabled for existing-account background synchronization.
- No new Signet-app environment credentials are required. Keep the existing integration
  URL, token, environment and customer UUID settings unchanged.
- Check approved/unlinked/rejected users, an approved link without a vesting option,
  an expired/revoked approval and an upstream outage. Confirm a queued payout is blocked
  after revocation and resumes only through the existing authorized review/retry flow.

There is no legacy upload fallback or local manual override. If Token is unavailable,
verification fails closed; it is never inferred from a successful vesting connection.
