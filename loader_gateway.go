package caddydockerproxy

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/lucaslorentz/caddy-docker-proxy/v2/docker"
	"github.com/lucaslorentz/caddy-docker-proxy/v2/gateway"
	"go.uber.org/zap"
)

// remoteHostEntry holds Hawser agent connection parameters parsed from gateway.remote.N.* labels.
type remoteHostEntry struct {
	URL   string
	Token string
	Name  string
}

// parseRemoteHosts extracts gateway.remote.N.{url,token,name} labels into an ordered slice.
func parseRemoteHosts(labels map[string]string) []remoteHostEntry {
	entries := map[int]*remoteHostEntry{}
	for k, v := range labels {
		const pfx = "gateway.remote."
		if !strings.HasPrefix(k, pfx) {
			continue
		}
		rest := k[len(pfx):]
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) != 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if entries[idx] == nil {
			entries[idx] = &remoteHostEntry{}
		}
		switch parts[1] {
		case "url":
			entries[idx].URL = v
		case "token":
			entries[idx].Token = v
		case "name":
			entries[idx].Name = v
		}
	}

	indices := make([]int, 0, len(entries))
	for idx := range entries {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	result := make([]remoteHostEntry, 0, len(indices))
	for _, idx := range indices {
		if entries[idx].URL != "" {
			result = append(result, *entries[idx])
		}
	}
	return result
}

// discoverRemoteHosts reads the homelab-manager container's own labels to discover Hawser agents.
// It appends remote Docker clients to dockerLoader.dockerClients and their URLs to DockerSockets.
func (dockerLoader *DockerLoader) discoverRemoteHosts(log *zap.Logger) {
	hostname, err := os.Hostname()
	if err != nil {
		log.Warn("gateway: could not determine own hostname for remote host discovery", zap.Error(err))
		return
	}
	if len(dockerLoader.dockerClients) == 0 {
		return
	}

	selfInspect, err := dockerLoader.dockerClients[0].ContainerInspect(context.Background(), hostname)
	if err != nil {
		log.Debug("gateway: could not inspect own container (not running in Docker?)", zap.Error(err))
		return
	}
	if selfInspect.Config == nil {
		return
	}

	remotes := parseRemoteHosts(selfInspect.Config.Labels)
	for _, entry := range remotes {
		remoteClient, err := docker.NewRemoteClient(entry.URL, entry.Token)
		if err != nil {
			log.Error("gateway: failed to create remote Docker client",
				zap.String("url", entry.URL), zap.Error(err))
			continue
		}
		dockerLoader.dockerClients = append(dockerLoader.dockerClients, remoteClient)
		dockerLoader.options.DockerSockets = append(dockerLoader.options.DockerSockets, entry.URL)
		log.Info("gateway: connected to remote Docker host via Hawser",
			zap.String("name", entry.Name),
			zap.String("url", entry.URL),
			zap.String("hostIP", remoteClient.HostIP()))
	}
}

// reconcileGateway diffs current running containers against the gateway state and
// creates or removes OPNsense DNS overrides and NAT rules as needed.
func (dockerLoader *DockerLoader) reconcileGateway() {
	if !dockerLoader.gatewayClient.Enabled() {
		return
	}
	ctx := context.Background()
	log := logger()

	// Collect all running containers across all clients.
	type containerInfo struct {
		labels map[string]string
		hostIP string
	}
	current := make(map[string]containerInfo)
	for _, dc := range dockerLoader.dockerClients {
		hostIP := dc.HostIP()
		if hostIP == "" {
			hostIP = os.Getenv("CADDY_IP")
		}
		containers, err := dc.ContainerList(ctx, container.ListOptions{})
		if err != nil {
			log.Error("gateway: failed to list containers", zap.Error(err))
			continue
		}
		for _, c := range containers {
			current[c.ID] = containerInfo{labels: c.Labels, hostIP: hostIP}
		}
	}

	var dnsChanged bool
	var natRevision string

	// Remove rules for containers that are no longer running.
	for _, cid := range dockerLoader.gatewayState.KnownContainerIDs() {
		if _, running := current[cid]; running {
			continue
		}
		for _, uuid := range dockerLoader.gatewayState.RemoveDNS(cid) {
			if err := dockerLoader.gatewayClient.DelHostOverride(ctx, uuid); err != nil {
				log.Error("gateway: del DNS override failed", zap.String("uuid", uuid), zap.Error(err))
			} else {
				dnsChanged = true
				log.Info("gateway: removed DNS override", zap.String("containerID", cid[:12]), zap.String("uuid", uuid))
			}
		}
		for _, uuid := range dockerLoader.gatewayState.RemoveNAT(cid) {
			if natRevision == "" {
				natRevision, _ = dockerLoader.gatewayClient.Savepoint(ctx)
			}
			if err := dockerLoader.gatewayClient.DelNATRule(ctx, uuid); err != nil {
				log.Error("gateway: del NAT rule failed", zap.String("uuid", uuid), zap.Error(err))
			} else {
				log.Info("gateway: removed NAT rule", zap.String("containerID", cid[:12]), zap.String("uuid", uuid))
			}
		}
	}

	// Create rules for running containers with gateway labels not yet in state.
	caddyIP := os.Getenv("CADDY_IP")
	labelPrefix := dockerLoader.options.LabelPrefix

	for cid, info := range current {
		// DNS overrides — skip if we already track rules for this container.
		if !dockerLoader.gatewayState.HasDNS(cid) {
			domains := extractDNSTargets(info.labels, labelPrefix)
			if len(domains) > 0 && caddyIP == "" {
				log.Warn("gateway: CADDY_IP not set, skipping DNS overrides")
			} else {
				for _, domain := range domains {
					hostname, domainPart, ok := splitDomain(domain)
					if !ok {
						log.Warn("gateway: skipping invalid domain", zap.String("domain", domain))
						continue
					}
					desc := gateway.MakeDescription(cid, "")
					uuid, err := dockerLoader.gatewayClient.AddHostOverride(ctx, hostname, domainPart, caddyIP, desc)
					if err != nil {
						log.Error("gateway: add DNS override failed", zap.String("domain", domain), zap.Error(err))
						continue
					}
					dockerLoader.gatewayState.AddDNS(cid, uuid)
					dnsChanged = true
					log.Info("gateway: added DNS override", zap.String("domain", domain), zap.String("containerID", cid[:12]))
				}
			}
		}

		// NAT port forwarding rules.
		if !dockerLoader.gatewayState.HasNAT(cid) {
			rules := buildNATRules(info.labels, info.hostIP, cid)
			if len(rules) > 0 && info.hostIP == "" {
				log.Warn("gateway: no host IP available, skipping NAT rules",
					zap.String("containerID", cid[:12]))
			} else {
				for _, rule := range rules {
					if natRevision == "" {
						natRevision, _ = dockerLoader.gatewayClient.Savepoint(ctx)
					}
					uuid, err := dockerLoader.gatewayClient.AddNATRule(ctx, rule)
					if err != nil {
						log.Error("gateway: add NAT rule failed", zap.Error(err))
						continue
					}
					dockerLoader.gatewayState.AddNAT(cid, uuid)
					log.Info("gateway: added NAT rule",
						zap.String("containerID", cid[:12]),
						zap.String("target", rule.Target),
						zap.String("dstPort", rule.LocalPort))
				}
			}
		}
	}

	// Apply pending changes.
	if dnsChanged {
		if err := dockerLoader.gatewayClient.ReconfigureUnbound(ctx); err != nil {
			log.Error("gateway: failed to reconfigure Unbound", zap.Error(err))
		}
	}
	if natRevision != "" {
		if err := dockerLoader.gatewayClient.Apply(ctx, natRevision); err != nil {
			log.Error("gateway: failed to apply NAT changes", zap.Error(err))
		}
	}
}

// extractDNSTargets returns site address domains from container labels, unless gateway.dns is "false".
func extractDNSTargets(labels map[string]string, labelPrefix string) []string {
	if labels["gateway.dns"] == "false" {
		return nil
	}
	val, ok := labels[labelPrefix]
	if !ok {
		return nil
	}
	var domains []string
	for _, d := range strings.Fields(val) {
		if d != "" && !strings.Contains(d, "*") {
			domains = append(domains, d)
		}
	}
	return domains
}

// splitDomain splits "myapp.internal" into ("myapp", "internal", true).
// Returns false for values that don't contain a dot or have an empty part.
func splitDomain(fqdn string) (hostname, domain string, ok bool) {
	idx := strings.IndexByte(fqdn, '.')
	if idx < 1 || idx == len(fqdn)-1 {
		return "", "", false
	}
	return fqdn[:idx], fqdn[idx+1:], true
}

type portForwardEntry struct {
	protocol    string
	srcPort     string
	dstPort     string
	description string
}

// buildNATRules parses gateway.portforward.N.* labels and returns NATRule values.
func buildNATRules(labels map[string]string, hostIP, containerID string) []gateway.NATRule {
	entries := map[int]*portForwardEntry{}
	for k, v := range labels {
		const pfx = "gateway.portforward."
		if !strings.HasPrefix(k, pfx) {
			continue
		}
		rest := k[len(pfx):]
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) != 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if entries[idx] == nil {
			entries[idx] = &portForwardEntry{}
		}
		switch parts[1] {
		case "protocol":
			entries[idx].protocol = v
		case "src_port":
			entries[idx].srcPort = v
		case "dst_port":
			entries[idx].dstPort = v
		case "description":
			entries[idx].description = v
		}
	}

	indices := make([]int, 0, len(entries))
	for idx := range entries {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	var rules []gateway.NATRule
	for _, idx := range indices {
		e := entries[idx]
		if e.protocol == "" || e.srcPort == "" || e.dstPort == "" {
			continue
		}
		rules = append(rules, gateway.NATRule{
			Interface:   "wan",
			Protocol:    e.protocol,
			Source:      gateway.NATRuleEndpoint{Network: "any"},
			Destination: gateway.NATRuleEndpoint{Port: e.srcPort},
			Target:      hostIP,
			LocalPort:   e.dstPort,
			Description: gateway.MakeDescription(containerID, e.description),
		})
	}
	return rules
}
