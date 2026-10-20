// Preserve leading zeros and normalize grouped clipboard codes before the
// browser's maxlength=6 can truncate a formatted value such as "123 456".
export function pastedAuthenticatorCode(value) {
  return String(value ?? '').replace(/\D/g, '').slice(0, 6)
}
