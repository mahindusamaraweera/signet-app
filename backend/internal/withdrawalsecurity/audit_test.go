package withdrawalsecurity

import (
	"context"
	"testing"
	"time"
)

func TestSecurityAuditReferencesBindUserVersionAndAction(t *testing.T) {
	ref := securityChangeReference(42, 2, "authenticator_confirmed")
	if !challengeIDPattern.MatchString(ref) || ref != securityChangeReference(42, 2, "authenticator_confirmed") || ref == securityChangeReference(43, 2, "authenticator_confirmed") || ref == securityChangeReference(42, 3, "authenticator_confirmed") || ref == securityChangeReference(42, 2, "address_verified") {
		t.Fatal("unstable or unbound audit reference")
	}
	if recordAudit(context.Background(), nil, 42, 2, "authenticator_confirmed", ref, time.Now()) == nil {
		t.Fatal("audit accepted without transaction")
	}
}
