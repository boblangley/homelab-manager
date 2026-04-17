package docker

import (
	"fmt"
	"net/http"
	"net/url"

	dockerclient "github.com/docker/docker/client"
)

// hawserTokenHeader is the auth header injected on every request to a Hawser agent.
const hawserTokenHeader = "X-Hawser-Token"

// hawserTransport injects the Hawser token header on every outbound request.
type hawserTransport struct {
	token string
	base  http.RoundTripper
}

func (t *hawserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(hawserTokenHeader, t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// remoteClient is a Docker Client backed by a Hawser Standard Mode HTTP proxy.
type remoteClient struct {
	*clientWrapper
	hostIP string
}

func (r *remoteClient) HostIP() string {
	return r.hostIP
}

// NewRemoteClient creates a Docker Client that proxies through a Hawser agent at rawURL,
// authenticating with token. The host IP is extracted from rawURL and returned by HostIP().
func NewRemoteClient(rawURL, token string) (Client, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid hawser URL %q: %w", rawURL, err)
	}

	hostIP := parsed.Hostname()

	httpClient := &http.Client{
		Transport: &hawserTransport{token: token},
	}

	sdkClient, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHost(rawURL),
		dockerclient.WithHTTPClient(httpClient),
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client for %q: %w", rawURL, err)
	}

	return &remoteClient{
		clientWrapper: &clientWrapper{client: sdkClient},
		hostIP:        hostIP,
	}, nil
}
