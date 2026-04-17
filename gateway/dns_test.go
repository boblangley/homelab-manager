package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddHostOverride(t *testing.T) {
	ms, client := newMockServer(t)
	ctx := context.Background()

	uuid, err := client.AddHostOverride(ctx, "myapp", "internal", "192.168.1.5", "[homelab-manager] abc123")
	require.NoError(t, err)
	assert.NotEmpty(t, uuid)

	ms.mu.Lock()
	rows := ms.dnsRows
	ms.mu.Unlock()
	require.Len(t, rows, 1)
	assert.Equal(t, "myapp", rows[0].Hostname)
	assert.Equal(t, "internal", rows[0].Domain)
	assert.Equal(t, "192.168.1.5", rows[0].Server)
	assert.Equal(t, "A", rows[0].RR)
	assert.Equal(t, "1", rows[0].Enabled)
}

func TestDelHostOverride(t *testing.T) {
	ms, client := newMockServer(t)
	ctx := context.Background()

	uuid, err := client.AddHostOverride(ctx, "myapp", "internal", "192.168.1.5", "test")
	require.NoError(t, err)

	err = client.DelHostOverride(ctx, uuid)
	require.NoError(t, err)

	ms.mu.Lock()
	count := len(ms.dnsRows)
	ms.mu.Unlock()
	assert.Equal(t, 0, count)
}

func TestSearchHostOverrides(t *testing.T) {
	ms, client := newMockServer(t)
	ctx := context.Background()

	ms.mu.Lock()
	ms.dnsRows = []HostOverrideRow{
		{UUID: "u1", Hostname: "app1", Domain: "home", Server: "10.0.0.1", Description: "[homelab-manager] cid1"},
		{UUID: "u2", Hostname: "app2", Domain: "home", Server: "10.0.0.1", Description: "external"},
	}
	ms.mu.Unlock()

	rows, err := client.SearchHostOverrides(ctx)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

func TestReconfigureUnbound(t *testing.T) {
	ms, client := newMockServer(t)
	ctx := context.Background()

	err := client.ReconfigureUnbound(ctx)
	require.NoError(t, err)

	ms.mu.Lock()
	calls := ms.ReconfigureCalls
	ms.mu.Unlock()
	assert.Equal(t, 1, calls)
}

func TestOwnedByUs(t *testing.T) {
	assert.True(t, OwnedByUs("[homelab-manager] abc"))
	assert.True(t, OwnedByUs("[homelab-manager]"))
	assert.False(t, OwnedByUs("some other description"))
	assert.False(t, OwnedByUs(""))
}
