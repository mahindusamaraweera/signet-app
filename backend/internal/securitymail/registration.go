package securitymail

import (
	"context"
	"fmt"
	"html"
)

func (s *Sender) SendVerification(ctx context.Context, to, link string) error {
	plain := fmt.Sprintf("Verify your SIGNET email\n\nOpen this link to verify your email and continue to package selection:\n%s\n\nThis link expires in 24 hours and can be used once. No payment is needed to verify. If you did not register, ignore this email.\n", link)
	safe := html.EscapeString(link)
	body := `<!doctype html><html><body style="margin:0;background:#071017;color:#eaf6f8;font-family:Arial,sans-serif;padding:32px 16px"><table role="presentation" style="width:100%;max-width:560px;margin:auto;border:1px solid #24414b;border-radius:20px;background:#101c24"><tr><td style="padding:36px"><p style="color:#80e9f4;font-size:12px;letter-spacing:3px">SIGNET · EMAIL VERIFICATION</p><h1 style="font-size:28px;line-height:1.3;margin:24px 0 16px">Your next step starts here</h1><p style="color:#b8cbd4;line-height:1.7">Verify your email to create your account. Then choose your package and fund your personal USDT wallet.</p><table role="presentation" style="margin:28px 0"><tr><td style="background:#00d6eb;border-radius:10px;text-align:center"><a href="` + safe + `" style="display:inline-block;padding:16px 28px;color:#041018;text-decoration:none;font-weight:bold">Verify my email</a></td></tr></table><p style="font-size:13px;color:#b8cbd4;line-height:1.7">This link expires in 24 hours and can be used once. No payment is required for verification.</p><hr style="border:0;border-top:1px solid #24414b;margin:24px 0"><p style="font-size:12px;color:#b8cbd4">Button not working? Copy this link into your browser:</p><p style="word-break:break-all;font-size:12px"><a href="` + safe + `" style="color:#80e9f4">` + safe + `</a></p><p style="font-size:12px;color:#b8cbd4;line-height:1.7">If you didn’t request this account, you can safely ignore this email. Never share your verification link, password or wallet keys.</p></td></tr></table></body></html>`
	return s.SendHTML(ctx, to, "Verify your SIGNET registration", plain, body)
}
