package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const descriptionPrefix = "[homelab-manager]"

// Client is the OPNsense API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClientFromEnv reads GATEWAY_URL, GATEWAY_KEY, GATEWAY_SECRET from the environment.
func NewClientFromEnv() *Client {
	return NewClientWithURL(
		os.Getenv("GATEWAY_URL"),
		os.Getenv("GATEWAY_KEY"),
		os.Getenv("GATEWAY_SECRET"),
	)
}

// NewClientWithURL creates a client with the given base URL and HTTP Basic credentials.
// Used by tests to inject an httptest server.
func NewClientWithURL(baseURL, key, secret string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: &basicAuthTransport{key: key, secret: secret},
		},
	}
}

// Enabled reports whether a gateway URL is configured.
func (c *Client) Enabled() bool {
	return c.baseURL != ""
}

type basicAuthTransport struct {
	key    string
	secret string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.SetBasicAuth(t.key, t.secret)
	return http.DefaultTransport.RoundTrip(req)
}

func (c *Client) get(req *http.Request, out interface{}) error {
	return c.do(req, out)
}

func (c *Client) newGET(path string) (*http.Request, error) {
	return http.NewRequest(http.MethodGet, c.baseURL+path, nil)
}

func (c *Client) newPOST(path string, body interface{}) (*http.Request, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) do(req *http.Request, out interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OPNsense %s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(body))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response from %s %s: %w", req.Method, req.URL.Path, err)
		}
	}
	return nil
}
