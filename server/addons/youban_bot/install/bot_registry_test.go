package install

import (
	"strings"
	"testing"

	"hotgo/internal/library/botregistry"
)

func TestAuditLegacyBotClaims(t *testing.T) {
	claims := []legacyBotClaim{
		{42, 2, botregistry.RoleCollector, "source 1"},
		{42, 2, botregistry.RoleCollector, "source 2"},
		{42, 2, botregistry.RoleRelay, "relay 1"},
		{43, 3, botregistry.RoleCollector, "source 3"},
		{43, 4, botregistry.RoleCollector, "source 4"},
		{44, 2, botregistry.RoleRelay, "relay 2"},
	}
	unique, issues := auditLegacyBotClaims(claims)
	if len(unique) != 2 || len(issues) != 1 {
		t.Fatalf("expected two safe identities and one conflict, got %d and %d", len(unique), len(issues))
	}
	if unique[0].role != botregistry.RoleCollector || !strings.Contains(issues[0], "机器人 43") {
		t.Fatalf("unexpected conflict report: %v", issues)
	}
}
