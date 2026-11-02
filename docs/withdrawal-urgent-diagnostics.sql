-- Read-only; execute in signet-app phpMyAdmin. No credentials or OTPs selected.
SELECT u.id, u.email,
       p.user_id IS NOT NULL AS security_profile_exists,
       p.email_verified_at, (BINARY p.verified_email = BINARY u.email) AS verified_email_matches,
       p.authenticator_confirmed_at, p.withdrawals_frozen, p.security_version,
       p.hold_until, (p.hold_until > CURRENT_TIMESTAMP) AS account_wait_active,
       e.user_id IS NOT NULL AS authenticator_enrollment_exists,
       e.confirmed_at AS enrollment_confirmed_at,
       (BINARY e.email = BINARY u.email) AS enrollment_email_matches,
       (e.confirmed_at = p.authenticator_confirmed_at) AS confirmation_times_match,
       a.id AS address_id, a.verified_at AS address_verified_at, a.eligible_at,
       a.revoked_at, (a.eligible_at > CURRENT_TIMESTAMP) AS address_wait_active,
       CURRENT_TIMESTAMP AS database_now, @@session.time_zone AS database_timezone
FROM signet_last.users u
LEFT JOIN signet_last.withdrawal_security_profiles p ON p.user_id=u.id
LEFT JOIN signet_last.withdrawal_authenticator_enrollments e ON e.user_id=u.id
LEFT JOIN signet_last.withdrawal_security_addresses a ON a.user_id=u.id
WHERE u.email IN ('iduwarasithum50@gmail.com','rayanfernando465@gmail.com')
ORDER BY u.id, a.created_at DESC;
