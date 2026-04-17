package gateway

import (
	"context"
	"fmt"
	"strings"
)

// AddHostOverride creates an Unbound DNS host override resolving hostname.domain to server.
// Returns the UUID assigned by OPNsense.
func (c *Client) AddHostOverride(ctx context.Context, hostname, domain, server, description string) (string, error) {
	req, err := c.newPOST("/api/unbound/settings/add_host_override", HostOverrideRequest{
		Host: HostOverride{
			Enabled:     "1",
			Hostname:    hostname,
			Domain:      domain,
			RR:          "A",
			Server:      server,
			Description: description,
		},
	})
	if err != nil {
		return "", err
	}
	req = req.WithContext(ctx)

	var result OPNsenseResult
	if err := c.do(req, &result); err != nil {
		return "", fmt.Errorf("add host override %s.%s: %w", hostname, domain, err)
	}
	if result.UUID == "" {
		return "", fmt.Errorf("add host override %s.%s: empty UUID in response", hostname, domain)
	}
	return result.UUID, nil
}

// DelHostOverride deletes the Unbound host override with the given UUID.
func (c *Client) DelHostOverride(ctx context.Context, uuid string) error {
	req, err := c.newPOST("/api/unbound/settings/del_host_override/"+uuid, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	var result OPNsenseResult
	if err := c.do(req, &result); err != nil {
		return fmt.Errorf("del host override %s: %w", uuid, err)
	}
	return nil
}

// SearchHostOverrides returns all Unbound host overrides.
func (c *Client) SearchHostOverrides(ctx context.Context) ([]HostOverrideRow, error) {
	req, err := c.newGET("/api/unbound/settings/search_host_override")
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	var result HostOverrideSearchResult
	if err := c.do(req, &result); err != nil {
		return nil, fmt.Errorf("search host overrides: %w", err)
	}
	return result.Rows, nil
}

// ReconfigureUnbound applies pending Unbound changes.
func (c *Client) ReconfigureUnbound(ctx context.Context) error {
	req, err := c.newPOST("/api/unbound/service/reconfigure", nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	var result map[string]string
	if err := c.do(req, &result); err != nil {
		return fmt.Errorf("reconfigure unbound: %w", err)
	}
	return nil
}

// OwnedByUs reports whether a host override description marks it as ours.
func OwnedByUs(description string) bool {
	return strings.HasPrefix(description, descriptionPrefix)
}
