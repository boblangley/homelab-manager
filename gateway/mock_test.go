package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// mockServer is an in-process OPNsense API stub for integration tests.
type mockServer struct {
	mu sync.Mutex

	// DNS state
	dnsRows  []HostOverrideRow
	dnsSeq   int

	// NAT state
	natRows  []NATRuleRow
	natSeq   int

	// Last savepoint revision
	lastRevision string

	// Call counters for assertions
	ReconfigureCalls int
	SavepointCalls   int
	ApplyCalls       int
}

func newMockServer(t *testing.T) (*mockServer, *Client) {
	t.Helper()
	ms := &mockServer{}
	mux := http.NewServeMux()
	ms.register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := NewClientWithURL(srv.URL, "testkey", "testsecret")
	return ms, client
}

func (ms *mockServer) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/unbound/settings/add_host_override", ms.handleAddDNS)
	mux.HandleFunc("/api/unbound/settings/del_host_override/", ms.handleDelDNS)
	mux.HandleFunc("/api/unbound/settings/search_host_override", ms.handleSearchDNS)
	mux.HandleFunc("/api/unbound/service/reconfigure", ms.handleReconfigureUnbound)

	mux.HandleFunc("/api/firewall/d_nat/add_rule", ms.handleAddNAT)
	mux.HandleFunc("/api/firewall/d_nat/del_rule/", ms.handleDelNAT)
	mux.HandleFunc("/api/firewall/d_nat/search_rule", ms.handleSearchNAT)
	mux.HandleFunc("/api/firewall/filter_base/savepoint", ms.handleSavepoint)
	mux.HandleFunc("/api/firewall/filter_base/apply/", ms.handleApply)
}

func jsonResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (ms *mockServer) handleAddDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req HostOverrideRequest
	json.NewDecoder(r.Body).Decode(&req)

	ms.mu.Lock()
	ms.dnsSeq++
	uuid := fmt.Sprintf("dns-%04d", ms.dnsSeq)
	ms.dnsRows = append(ms.dnsRows, HostOverrideRow{
		UUID:        uuid,
		Enabled:     req.Host.Enabled,
		Hostname:    req.Host.Hostname,
		Domain:      req.Host.Domain,
		RR:          req.Host.RR,
		Server:      req.Host.Server,
		Description: req.Host.Description,
	})
	ms.mu.Unlock()

	jsonResponse(w, OPNsenseResult{Result: "saved", UUID: uuid})
}

func (ms *mockServer) handleDelDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uuid := strings.TrimPrefix(r.URL.Path, "/api/unbound/settings/del_host_override/")

	ms.mu.Lock()
	filtered := ms.dnsRows[:0]
	for _, row := range ms.dnsRows {
		if row.UUID != uuid {
			filtered = append(filtered, row)
		}
	}
	ms.dnsRows = filtered
	ms.mu.Unlock()

	jsonResponse(w, OPNsenseResult{Result: "deleted"})
}

func (ms *mockServer) handleSearchDNS(w http.ResponseWriter, r *http.Request) {
	ms.mu.Lock()
	rows := make([]HostOverrideRow, len(ms.dnsRows))
	copy(rows, ms.dnsRows)
	ms.mu.Unlock()

	jsonResponse(w, HostOverrideSearchResult{
		Rows:     rows,
		RowCount: len(rows),
		Total:    len(rows),
		Current:  1,
	})
}

func (ms *mockServer) handleReconfigureUnbound(w http.ResponseWriter, r *http.Request) {
	ms.mu.Lock()
	ms.ReconfigureCalls++
	ms.mu.Unlock()
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (ms *mockServer) handleAddNAT(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req NATRuleRequest
	json.NewDecoder(r.Body).Decode(&req)

	ms.mu.Lock()
	ms.natSeq++
	uuid := fmt.Sprintf("nat-%04d", ms.natSeq)
	ms.natRows = append(ms.natRows, NATRuleRow{
		UUID:        uuid,
		Interface:   req.Rule.Interface,
		Protocol:    req.Rule.Protocol,
		Target:      req.Rule.Target,
		LocalPort:   req.Rule.LocalPort,
		Description: req.Rule.Description,
	})
	ms.mu.Unlock()

	jsonResponse(w, OPNsenseResult{Result: "saved", UUID: uuid})
}

func (ms *mockServer) handleDelNAT(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uuid := strings.TrimPrefix(r.URL.Path, "/api/firewall/d_nat/del_rule/")

	ms.mu.Lock()
	filtered := ms.natRows[:0]
	for _, row := range ms.natRows {
		if row.UUID != uuid {
			filtered = append(filtered, row)
		}
	}
	ms.natRows = filtered
	ms.mu.Unlock()

	jsonResponse(w, OPNsenseResult{Result: "deleted"})
}

func (ms *mockServer) handleSearchNAT(w http.ResponseWriter, r *http.Request) {
	ms.mu.Lock()
	rows := make([]NATRuleRow, len(ms.natRows))
	copy(rows, ms.natRows)
	ms.mu.Unlock()

	jsonResponse(w, NATRuleSearchResult{
		Rows:     rows,
		RowCount: len(rows),
		Total:    len(rows),
		Current:  1,
	})
}

func (ms *mockServer) handleSavepoint(w http.ResponseWriter, r *http.Request) {
	ms.mu.Lock()
	ms.SavepointCalls++
	ms.lastRevision = fmt.Sprintf("rev-%04d", ms.SavepointCalls)
	rev := ms.lastRevision
	ms.mu.Unlock()

	jsonResponse(w, map[string]string{"revision": rev})
}

func (ms *mockServer) handleApply(w http.ResponseWriter, r *http.Request) {
	ms.mu.Lock()
	ms.ApplyCalls++
	ms.mu.Unlock()
	jsonResponse(w, map[string]string{"status": "ok"})
}
