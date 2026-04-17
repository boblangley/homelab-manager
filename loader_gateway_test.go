package caddydockerproxy

import (
	"testing"

	"github.com/lucaslorentz/caddy-docker-proxy/v2/gateway"
	"github.com/stretchr/testify/assert"
)

func TestParseRemoteHosts(t *testing.T) {
	t.Run("returns empty for no labels", func(t *testing.T) {
		assert.Empty(t, parseRemoteHosts(map[string]string{}))
	})

	t.Run("parses single entry", func(t *testing.T) {
		labels := map[string]string{
			"gateway.remote.0.url":   "http://192.168.1.20:2376",
			"gateway.remote.0.token": "tok1",
			"gateway.remote.0.name":  "nas",
		}
		got := parseRemoteHosts(labels)
		assert.Equal(t, []remoteHostEntry{
			{URL: "http://192.168.1.20:2376", Token: "tok1", Name: "nas"},
		}, got)
	})

	t.Run("parses multiple entries in index order", func(t *testing.T) {
		labels := map[string]string{
			"gateway.remote.1.url":   "http://host1:2376",
			"gateway.remote.1.token": "t1",
			"gateway.remote.0.url":   "http://host0:2376",
			"gateway.remote.0.token": "t0",
			"gateway.remote.2.url":   "http://host2:2376",
			"gateway.remote.2.token": "t2",
		}
		got := parseRemoteHosts(labels)
		assert.Equal(t, []remoteHostEntry{
			{URL: "http://host0:2376", Token: "t0"},
			{URL: "http://host1:2376", Token: "t1"},
			{URL: "http://host2:2376", Token: "t2"},
		}, got)
	})

	t.Run("skips entries without a URL", func(t *testing.T) {
		labels := map[string]string{
			"gateway.remote.0.token": "orphan-token",
			"gateway.remote.1.url":   "http://host1:2376",
			"gateway.remote.1.token": "t1",
		}
		got := parseRemoteHosts(labels)
		assert.Equal(t, []remoteHostEntry{
			{URL: "http://host1:2376", Token: "t1"},
		}, got)
	})

	t.Run("ignores non-gateway labels", func(t *testing.T) {
		labels := map[string]string{
			"caddy":                 "myapp.internal",
			"gateway.dns":           "false",
			"gateway.remote.0.url":  "http://host0:2376",
			"gateway.portforward.0": "something",
		}
		got := parseRemoteHosts(labels)
		assert.Equal(t, []remoteHostEntry{
			{URL: "http://host0:2376"},
		}, got)
	})

	t.Run("ignores non-numeric indices", func(t *testing.T) {
		labels := map[string]string{
			"gateway.remote.foo.url": "http://bad:2376",
			"gateway.remote.0.url":   "http://good:2376",
		}
		got := parseRemoteHosts(labels)
		assert.Equal(t, []remoteHostEntry{
			{URL: "http://good:2376"},
		}, got)
	})
}

func TestExtractDNSTargets(t *testing.T) {
	t.Run("returns nil when prefix label missing", func(t *testing.T) {
		assert.Nil(t, extractDNSTargets(map[string]string{"other": "value"}, "caddy"))
	})

	t.Run("returns single domain", func(t *testing.T) {
		labels := map[string]string{"caddy": "myapp.internal"}
		assert.Equal(t, []string{"myapp.internal"}, extractDNSTargets(labels, "caddy"))
	})

	t.Run("splits multiple domains on whitespace", func(t *testing.T) {
		labels := map[string]string{"caddy": "myapp.internal other.internal"}
		assert.Equal(t, []string{"myapp.internal", "other.internal"}, extractDNSTargets(labels, "caddy"))
	})

	t.Run("skips wildcard domains", func(t *testing.T) {
		labels := map[string]string{"caddy": "myapp.internal *.internal"}
		assert.Equal(t, []string{"myapp.internal"}, extractDNSTargets(labels, "caddy"))
	})

	t.Run("returns nil when gateway.dns is false", func(t *testing.T) {
		labels := map[string]string{
			"caddy":       "myapp.internal",
			"gateway.dns": "false",
		}
		assert.Nil(t, extractDNSTargets(labels, "caddy"))
	})

	t.Run("honors custom label prefix", func(t *testing.T) {
		labels := map[string]string{"proxy": "myapp.internal"}
		assert.Equal(t, []string{"myapp.internal"}, extractDNSTargets(labels, "proxy"))
	})
}

func TestSplitDomain(t *testing.T) {
	cases := []struct {
		fqdn         string
		wantHostname string
		wantDomain   string
		wantOK       bool
	}{
		{"myapp.internal", "myapp", "internal", true},
		{"myapp.example.com", "myapp", "example.com", true},
		{"a.b.c.d", "a", "b.c.d", true},
		{"nodot", "", "", false},
		{".internal", "", "", false},
		{"myapp.", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.fqdn, func(t *testing.T) {
			hostname, domain, ok := splitDomain(tc.fqdn)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantHostname, hostname)
			assert.Equal(t, tc.wantDomain, domain)
		})
	}
}

func TestBuildNATRules(t *testing.T) {
	t.Run("returns nil for no portforward labels", func(t *testing.T) {
		assert.Nil(t, buildNATRules(map[string]string{"caddy": "x"}, "10.0.0.1", "cid1"))
	})

	t.Run("builds single rule with defaults", func(t *testing.T) {
		labels := map[string]string{
			"gateway.portforward.0.protocol": "tcp",
			"gateway.portforward.0.src_port": "2222",
			"gateway.portforward.0.dst_port": "22",
		}
		rules := buildNATRules(labels, "192.168.1.20", "cid1")
		assert.Equal(t, []gateway.NATRule{{
			Interface:   "wan",
			Protocol:    "tcp",
			Source:      gateway.NATRuleEndpoint{Network: "any"},
			Destination: gateway.NATRuleEndpoint{Port: "2222"},
			Target:      "192.168.1.20",
			LocalPort:   "22",
			Description: "[homelab-manager] cid1",
		}}, rules)
	})

	t.Run("includes description when provided", func(t *testing.T) {
		labels := map[string]string{
			"gateway.portforward.0.protocol":    "tcp",
			"gateway.portforward.0.src_port":    "2222",
			"gateway.portforward.0.dst_port":    "22",
			"gateway.portforward.0.description": "SSH to NAS",
		}
		rules := buildNATRules(labels, "192.168.1.20", "cid1")
		assert.Len(t, rules, 1)
		assert.Equal(t, "[homelab-manager] cid1 SSH to NAS", rules[0].Description)
	})

	t.Run("builds multiple rules sorted by index", func(t *testing.T) {
		labels := map[string]string{
			"gateway.portforward.1.protocol": "udp",
			"gateway.portforward.1.src_port": "51820",
			"gateway.portforward.1.dst_port": "51820",
			"gateway.portforward.0.protocol": "tcp",
			"gateway.portforward.0.src_port": "2222",
			"gateway.portforward.0.dst_port": "22",
		}
		rules := buildNATRules(labels, "10.0.0.5", "cid2")
		assert.Len(t, rules, 2)
		assert.Equal(t, "tcp", rules[0].Protocol)
		assert.Equal(t, "2222", rules[0].Destination.Port)
		assert.Equal(t, "udp", rules[1].Protocol)
		assert.Equal(t, "51820", rules[1].Destination.Port)
	})

	t.Run("skips incomplete rules missing required fields", func(t *testing.T) {
		labels := map[string]string{
			"gateway.portforward.0.protocol": "tcp",
			"gateway.portforward.0.src_port": "2222",
			// dst_port missing — rule skipped
			"gateway.portforward.1.protocol": "udp",
			"gateway.portforward.1.src_port": "51820",
			"gateway.portforward.1.dst_port": "51820",
		}
		rules := buildNATRules(labels, "10.0.0.5", "cid3")
		assert.Len(t, rules, 1)
		assert.Equal(t, "udp", rules[0].Protocol)
	})

	t.Run("ignores non-numeric indices", func(t *testing.T) {
		labels := map[string]string{
			"gateway.portforward.foo.protocol": "tcp",
			"gateway.portforward.foo.src_port": "1",
			"gateway.portforward.foo.dst_port": "1",
			"gateway.portforward.0.protocol":   "tcp",
			"gateway.portforward.0.src_port":   "2222",
			"gateway.portforward.0.dst_port":   "22",
		}
		rules := buildNATRules(labels, "10.0.0.5", "cid4")
		assert.Len(t, rules, 1)
		assert.Equal(t, "2222", rules[0].Destination.Port)
	})
}
