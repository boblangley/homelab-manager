package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMakeDescription(t *testing.T) {
	assert.Equal(t, "[homelab-manager] abc123", MakeDescription("abc123", ""))
	assert.Equal(t, "[homelab-manager] abc123 SSH to NAS", MakeDescription("abc123", "SSH to NAS"))
}

func TestExtractContainerID(t *testing.T) {
	assert.Equal(t, "abc123", extractContainerID("[homelab-manager] abc123"))
	assert.Equal(t, "abc123", extractContainerID("[homelab-manager] abc123 extra text"))
	assert.Equal(t, "", extractContainerID("not owned"))
	assert.Equal(t, "", extractContainerID("[homelab-manager]"))
}

func TestStateAddRemove(t *testing.T) {
	_, client := newMockServer(t)
	s := NewState(client)

	s.AddDNS("cid1", "uuid-dns-1")
	s.AddDNS("cid1", "uuid-dns-2")
	s.AddNAT("cid1", "uuid-nat-1")

	assert.True(t, s.HasDNS("cid1"))
	assert.True(t, s.HasNAT("cid1"))
	assert.False(t, s.HasDNS("cid2"))

	dnsUUIDs := s.RemoveDNS("cid1")
	assert.ElementsMatch(t, []string{"uuid-dns-1", "uuid-dns-2"}, dnsUUIDs)
	assert.False(t, s.HasDNS("cid1"))

	natUUIDs := s.RemoveNAT("cid1")
	assert.ElementsMatch(t, []string{"uuid-nat-1"}, natUUIDs)
}

func TestStateKnownContainerIDs(t *testing.T) {
	_, client := newMockServer(t)
	s := NewState(client)

	s.AddDNS("cid1", "u1")
	s.AddNAT("cid2", "u2")
	s.AddDNS("cid2", "u3")

	ids := s.KnownContainerIDs()
	assert.ElementsMatch(t, []string{"cid1", "cid2"}, ids)
}

func TestReconcileOnStartup(t *testing.T) {
	ms, client := newMockServer(t)
	ctx := context.Background()
	log := zap.NewNop()

	ms.mu.Lock()
	ms.dnsRows = []HostOverrideRow{
		{UUID: "d1", Description: "[homelab-manager] cid-a myapp.internal"},
		{UUID: "d2", Description: "[homelab-manager] cid-a other.internal"},
		{UUID: "d3", Description: "not ours"},
	}
	ms.natRows = []NATRuleRow{
		{UUID: "n1", Description: "[homelab-manager] cid-b SSH"},
		{UUID: "n2", Description: "also not ours"},
	}
	ms.mu.Unlock()

	s := NewState(client)
	err := s.ReconcileOnStartup(ctx, log)
	require.NoError(t, err)

	assert.True(t, s.HasDNS("cid-a"))
	assert.False(t, s.HasDNS("cid-b"))
	assert.True(t, s.HasNAT("cid-b"))

	dnsUUIDs := s.RemoveDNS("cid-a")
	assert.ElementsMatch(t, []string{"d1", "d2"}, dnsUUIDs)

	natUUIDs := s.RemoveNAT("cid-b")
	assert.ElementsMatch(t, []string{"n1"}, natUUIDs)
}
