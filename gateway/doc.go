// Package gateway manages OPNsense router state from Docker container labels.
// It creates and removes Unbound DNS host overrides and NAT port forwarding rules
// as containers start and stop.
//
//go:generate go run ../tools/gen-opnsense-mocks/main.go
package gateway
