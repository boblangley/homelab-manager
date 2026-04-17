package gateway

import (
	"context"
	"fmt"
)

// AddNATRule creates an OPNsense NAT port forwarding rule.
// Returns the UUID assigned by OPNsense.
func (c *Client) AddNATRule(ctx context.Context, rule NATRule) (string, error) {
	req, err := c.newPOST("/api/firewall/d_nat/add_rule", NATRuleRequest{Rule: rule})
	if err != nil {
		return "", err
	}
	req = req.WithContext(ctx)

	var result OPNsenseResult
	if err := c.do(req, &result); err != nil {
		return "", fmt.Errorf("add NAT rule: %w", err)
	}
	if result.UUID == "" {
		return "", fmt.Errorf("add NAT rule: empty UUID in response")
	}
	return result.UUID, nil
}

// DelNATRule deletes the NAT rule with the given UUID.
func (c *Client) DelNATRule(ctx context.Context, uuid string) error {
	req, err := c.newPOST("/api/firewall/d_nat/del_rule/"+uuid, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	var result OPNsenseResult
	if err := c.do(req, &result); err != nil {
		return fmt.Errorf("del NAT rule %s: %w", uuid, err)
	}
	return nil
}

// SearchNATRules returns all NAT port forwarding rules.
func (c *Client) SearchNATRules(ctx context.Context) ([]NATRuleRow, error) {
	req, err := c.newGET("/api/firewall/d_nat/search_rule")
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	var result NATRuleSearchResult
	if err := c.do(req, &result); err != nil {
		return nil, fmt.Errorf("search NAT rules: %w", err)
	}
	return result.Rows, nil
}

// Savepoint creates a firewall savepoint and returns the revision token.
func (c *Client) Savepoint(ctx context.Context) (string, error) {
	req, err := c.newPOST("/api/firewall/filter_base/savepoint", nil)
	if err != nil {
		return "", err
	}
	req = req.WithContext(ctx)

	var result map[string]string
	if err := c.do(req, &result); err != nil {
		return "", fmt.Errorf("savepoint: %w", err)
	}
	return result["revision"], nil
}

// Apply commits the firewall changes at the given revision.
func (c *Client) Apply(ctx context.Context, revision string) error {
	req, err := c.newPOST("/api/firewall/filter_base/apply/"+revision, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	var result map[string]string
	if err := c.do(req, &result); err != nil {
		return fmt.Errorf("apply revision %s: %w", revision, err)
	}
	return nil
}
