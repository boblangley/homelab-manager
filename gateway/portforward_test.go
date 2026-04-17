package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddNATRule(t *testing.T) {
	ms, client := newMockServer(t)
	ctx := context.Background()

	rule := NATRule{
		Interface:   "wan",
		Protocol:    "tcp",
		Source:      NATRuleEndpoint{Network: "any"},
		Destination: NATRuleEndpoint{Port: "2222"},
		Target:      "192.168.1.20",
		LocalPort:   "22",
		Description: "[homelab-manager] cid1 SSH to NAS",
	}

	uuid, err := client.AddNATRule(ctx, rule)
	require.NoError(t, err)
	assert.NotEmpty(t, uuid)

	ms.mu.Lock()
	rows := ms.natRows
	ms.mu.Unlock()
	require.Len(t, rows, 1)
	assert.Equal(t, "wan", rows[0].Interface)
	assert.Equal(t, "tcp", rows[0].Protocol)
	assert.Equal(t, "192.168.1.20", rows[0].Target)
	assert.Equal(t, "22", rows[0].LocalPort)
}

func TestDelNATRule(t *testing.T) {
	ms, client := newMockServer(t)
	ctx := context.Background()

	rule := NATRule{Interface: "wan", Protocol: "tcp", Target: "192.168.1.20", LocalPort: "22"}
	uuid, err := client.AddNATRule(ctx, rule)
	require.NoError(t, err)

	err = client.DelNATRule(ctx, uuid)
	require.NoError(t, err)

	ms.mu.Lock()
	count := len(ms.natRows)
	ms.mu.Unlock()
	assert.Equal(t, 0, count)
}

func TestSearchNATRules(t *testing.T) {
	ms, client := newMockServer(t)
	ctx := context.Background()

	ms.mu.Lock()
	ms.natRows = []NATRuleRow{
		{UUID: "n1", Description: "[homelab-manager] cid1"},
		{UUID: "n2", Description: "external rule"},
	}
	ms.mu.Unlock()

	rows, err := client.SearchNATRules(ctx)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

func TestSavepointAndApply(t *testing.T) {
	ms, client := newMockServer(t)
	ctx := context.Background()

	rev, err := client.Savepoint(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, rev)

	err = client.Apply(ctx, rev)
	require.NoError(t, err)

	ms.mu.Lock()
	saves := ms.SavepointCalls
	applies := ms.ApplyCalls
	ms.mu.Unlock()
	assert.Equal(t, 1, saves)
	assert.Equal(t, 1, applies)
}
