package sys

import (
	"context"
	"testing"
)

func TestAccountCapabilityRejectsUnknownApp(t *testing.T) {
	s := &sSysPublish{}
	if _, err := s.AccountCapability(context.Background(), "unknown", 1); err == nil {
		t.Fatal("unknown app must not receive account capabilities")
	}
}

func TestAccountCapabilityAdminRequiresAccountID(t *testing.T) {
	s := &sSysPublish{}
	if _, err := s.AccountCapability(context.Background(), "admin", 0); err == nil {
		t.Fatal("admin capability must require account id")
	}
}
