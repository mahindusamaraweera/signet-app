# Automatic package funding integration

Implementation branch: `feature/blockchain-automation`. Current application-side implementation requires matching service routes before it can operate. No live deposits or swaps have been executed.

## User behavior

Referral registration issues a session when blockchain mode is enabled. Package selection verifies session ownership and creates one durable intent with a fixed price and commission recipient/rate. The funding page shows a personal USDT address and polls funding status. A minute-based worker reconciles intent state independently of whether the browser is open.

Partial deposits protect the account and pending placement permanently using a monotonic `deposit_seen` flag. Display the remaining USDT amount. Sufficient confirmed funds automatically reserve the package price. Excess stays available; sweeps change custody location, not user credit. Local activation requires confirmed swap and vesting allocation and commits the legacy commission calculation plus intent completion in one transaction. Do not run the destination service's MLM engine for these orders.

Existing legacy activation endpoints reject integrated purchase intents. Historical registrations are not silently enrolled into new purchases. Their wallet backfill and vesting migration remain separate work.

## Required financial-service routes

All routes require a restricted service token and source-qualified user mapping. Never interpret the local ID as a destination database ID. Do not share end-user passwords between services.

- `POST /api/v1/integration/wallets`: idempotently provision the mapped user's BSC USDT address.
- `POST /api/v1/integration/package-purchases/sync`: create/reconcile the same external purchase, never a new purchase on retry. Request includes `source`, `external_user_id`, `external_purchase_id`, `package_price_usdt`, `swap_asset=SIGNET`, `swap_percent=100`, `custody_policy=user_purchase_vesting`, `auto_execute=true`, `run_mlm=false`.
- `POST /api/v1/integration/registration-expiry/check`: deny expiry for any detected deposit, balance, financial entitlement or purchase operation. Missing/unavailable results postpone expiry. Coordinate expiry with purchase reservation to prevent remote races.

Purchase response: `status`, `available_usdt` (current available decimal string), `deposit_seen` (boolean), `order_id`, `swap_confirmed`, `vesting_allocated`. Valid states: `awaiting_deposit`, `reserved`, `pending_sweep`, `pending_swap`, `pending_vesting`, `retrying`, `completed`. A completed result must certify both swap confirmation and vesting allocation. Responses to sync/check must reflect current state rather than return a permanently cached initial response.

The financial service must atomically reserve confirmed USDT, deduplicate scanner events and orders, verify treasury liquidity, sweep with gas funding, execute swaps with quote/slippage controls, allocate custody and vesting, and reconcile ambiguous broadcasts. Do not fabricate historical deposits or silently substitute a simulated swap.

## Deployment and verification

Apply additive migrations `003_registration_expiry.sql` and `004_package_purchase_intents.sql` through the repository's migration tool after database testing. Set `BLOCKCHAIN_ENABLED`, URL/token and scheduler configuration only when the matching service is deployed. Keep expiry disabled until service coordination is tested.

No matching routes have been implemented in `signet-token` during this task. Database-backed transaction, simultaneous deposit/expiry, legacy commission parity and BSC end-to-end tests remain required before production enablement.

## 2026-10-10 cross-repository review

The companion repository now has `docs/signet-app-integration.md` on `feature/signet-app-integration` documenting endpoint contracts, identity/authentication, service reuse and the remaining implementation sequence. That branch currently changes documentation only; the routes are still absent.

New fixed-allowance requirement is not implemented by this contract yet: add a trusted package policy/version and snapshot fixed SIGNET entitlement/vesting terms in both services. Full USDT price must use an exact-input swap; the confirmed treasury top-up equals the fixed entitlement shortfall. Do not reuse the destination purchase service unchanged: it currently applies percentage bonuses and destination MLM/tier processing.

Existing-user backfill, vesting migration, withdrawals, administrator time adjustment, late-deposit recovery and race-safe expiry remain outstanding. Keep integration disabled pending completion and validation.
# Package transaction fee — updated requirement

## Commission settlement update

### Income-cap preservation requirement and source findings

Owner confirmed that existing cap amounts, historical earnings and cap behavior must be retained. SIGNET conversion must not change the USD basis, reset counters or recompute historical balances at market prices.

Source review shows several distinct rules, not one universal lifetime-income cap: `wallet.CheckWallet` compares current wallet balance plus predicted commission against the sum of active package prices times four; `wallet.UpdateWallet` has a current-balance precheck; package activation's private credit function is not independently cap-gated; `user_packages.earn` and the global-share `max_out` are separate counters. The legacy manual activation handler performs eligibility checks before calling that private credit function. The initial automatic worker currently calls it directly and therefore does not yet prove cap parity. This is a release blocker, not a reason to change the existing rules.

Settlement needs a separate paid/pending balance representation so the preserved USD cap basis cannot also be withdrawn as duplicate funds. Never debit/reset a legacy cap basis merely because SIGNET was delivered. Capture eligibility before external settlement and validate parity for boundary, already-over-cap, multiple-package, company overflow and global-share scenarios in database-backed tests. Do not claim that the initial automation already preserves all cap checks.

Owner requirement: preserve existing commission eligibility/rates and calculate commission USD value from the package amount only, excluding the transaction fee. Settle commission in SIGNET using the live SIGNET/USDT rate. Example: a $100 package with a $50 commission pays `50 / SIGNET_USDT_price` SIGNET. This commission is independent of the purchaser's fixed package entitlement and treasury bonus; never deduct it from that vesting allocation.

Current activation still credits legacy internal wallet amounts and is not a SIGNET payout implementation. Before enablement, capture each eligible recipient's commission as a durable outbox item in the same activation transaction, with a unique purchase/recipient/commission-kind identity. The companion service must snapshot a fresh trusted rate, timestamp and resulting SIGNET amount once, deliver from treasury to mapped recipient custody, verify receipt amount and expose pending/confirmed status. Retries must not reprice or repay the same commission. Preserve USD-denominated earnings/cap history but do not leave a second spendable USD balance for the same paid commission. Recipient availability versus commission vesting policy remains to confirm. Historical balances need a separate explicit migration, not silent conversion.

The owner requires separate package and transaction-fee amounts. Only the package amount is swapped USDT→SIGNET. The transaction fee stays USDT and is sent to a designated company BSC address; it must not increase the swap input, SIGNET entitlement or MLM commission basis. The company address has not yet been supplied.

Current source schema contains `packages.price` and `packages.commission`, but no separate transaction-fee field. Do not reinterpret commission as the transaction fee. Confirm whether the fee is added on top of `price` or already included in the displayed plan total, and obtain per-package fee values before changing funding accounting.

Implementation requirements after terms are confirmed: snapshot package amount, fee amount and company recipient on each new intent/order; reserve their sum once; swap only package amount; deliver fee USDT with a distinct stable operation key; verify confirmed configured-token transfer amount to the snapshotted recipient; report complete only when swap, bonus/vesting and fee delivery are confirmed. Show the three amounts (package, fee, total) and calculate partial-deposit remainder from total. Retain excess funds and existing-account protection. Chain gas cost is separate from this commercial transaction fee. Existing orders must not be silently repriced; migration/cutover needs explicit policy.
