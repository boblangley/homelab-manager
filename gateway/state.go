package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// State tracks which OPNsense resources homelab-manager owns.
// Rule ownership is encoded in the description field using the format:
//
//	[homelab-manager] <containerID> [optional label text]
//
// This survives restarts: ReconcileOnStartup reads existing rules from OPNsense
// and re-populates the in-memory maps.
type State struct {
	mu sync.Mutex

	dnsUUIDs map[string][]string // containerID → DNS override UUIDs
	natUUIDs map[string][]string // containerID → NAT rule UUIDs

	client *Client
}

// NewState creates a State backed by the given OPNsense client.
func NewState(client *Client) *State {
	return &State{
		dnsUUIDs: make(map[string][]string),
		natUUIDs: make(map[string][]string),
		client:   client,
	}
}

// ReconcileOnStartup loads existing owned OPNsense rules into the state maps.
// It should be called once on startup before the first update cycle.
func (s *State) ReconcileOnStartup(ctx context.Context, log *zap.Logger) error {
	if !s.client.Enabled() {
		return nil
	}

	dnsRows, err := s.client.SearchHostOverrides(ctx)
	if err != nil {
		return fmt.Errorf("reconcile DNS: %w", err)
	}
	s.mu.Lock()
	for _, row := range dnsRows {
		if !OwnedByUs(row.Description) {
			continue
		}
		cid := extractContainerID(row.Description)
		if cid == "" {
			log.Warn("owned DNS override missing container ID in description", zap.String("uuid", row.UUID))
			continue
		}
		s.dnsUUIDs[cid] = append(s.dnsUUIDs[cid], row.UUID)
	}
	s.mu.Unlock()
	log.Info("reconciled DNS overrides from OPNsense", zap.Int("owned", len(dnsRows)))

	natRows, err := s.client.SearchNATRules(ctx)
	if err != nil {
		return fmt.Errorf("reconcile NAT: %w", err)
	}
	s.mu.Lock()
	for _, row := range natRows {
		if !OwnedByUs(row.Description) {
			continue
		}
		cid := extractContainerID(row.Description)
		if cid == "" {
			log.Warn("owned NAT rule missing container ID in description", zap.String("uuid", row.UUID))
			continue
		}
		s.natUUIDs[cid] = append(s.natUUIDs[cid], row.UUID)
	}
	s.mu.Unlock()
	log.Info("reconciled NAT rules from OPNsense", zap.Int("owned", len(natRows)))

	return nil
}

// AddDNS records a newly created DNS override UUID for a container.
func (s *State) AddDNS(containerID, uuid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnsUUIDs[containerID] = append(s.dnsUUIDs[containerID], uuid)
}

// AddNAT records a newly created NAT rule UUID for a container.
func (s *State) AddNAT(containerID, uuid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.natUUIDs[containerID] = append(s.natUUIDs[containerID], uuid)
}

// RemoveDNS removes and returns the DNS UUIDs for a container.
func (s *State) RemoveDNS(containerID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	uuids := s.dnsUUIDs[containerID]
	delete(s.dnsUUIDs, containerID)
	return uuids
}

// RemoveNAT removes and returns the NAT UUIDs for a container.
func (s *State) RemoveNAT(containerID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	uuids := s.natUUIDs[containerID]
	delete(s.natUUIDs, containerID)
	return uuids
}

// HasDNS reports whether any DNS overrides are tracked for a container.
func (s *State) HasDNS(containerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.dnsUUIDs[containerID]) > 0
}

// HasNAT reports whether any NAT rules are tracked for a container.
func (s *State) HasNAT(containerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.natUUIDs[containerID]) > 0
}

// KnownContainerIDs returns all container IDs that have tracked rules.
func (s *State) KnownContainerIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]struct{}, len(s.dnsUUIDs)+len(s.natUUIDs))
	for id := range s.dnsUUIDs {
		seen[id] = struct{}{}
	}
	for id := range s.natUUIDs {
		seen[id] = struct{}{}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids
}

// MakeDescription builds a tagged description embedding the container ID.
// extra is the optional human-readable text from the label (may be empty).
func MakeDescription(containerID, extra string) string {
	if extra == "" {
		return descriptionPrefix + " " + containerID
	}
	return descriptionPrefix + " " + containerID + " " + extra
}

// extractContainerID parses the container ID from a description of the form:
// "[homelab-manager] <containerID> [...]"
func extractContainerID(description string) string {
	after, found := strings.CutPrefix(description, descriptionPrefix)
	if !found {
		return ""
	}
	parts := strings.Fields(after)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
