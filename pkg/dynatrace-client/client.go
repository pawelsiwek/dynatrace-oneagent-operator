package dynatrace_client

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// Client is the interface for the Dynatrace REST API client.
type Client interface {
	// GetVersionForLatest gets the latest agent version that is available for download.
	GetVersionForLatest() (string, error)

	// GetVersionForIp returns the agent version running on the host with the given IP address.
	GetVersionForIp(ip net.IP) (string, error)
}

// NewClient creates a REST client for the given API base URL and authentication tokens.
//
// The API base URL is different for managed and SaaS environments:
//  - SaaS: https://{environment-id}.live.dynatrace.com/api
//  - Managed: https://{domain}/e/{environment-id}/api
func NewClient(url, apiToken, paasToken string) Client {
	if strings.HasSuffix(url, "/") {
		url = url[:len(url)-1]
	}
	return &client{url, apiToken, paasToken}
}

// client implements the Client interface.
type client struct {
	url       string
	apiToken  string
	paasToken string
}

// GetVersionForLatest gets the latest agent version that is available from the cluster.
// Returns the version as received from the server on success.
//
// Returns an error for the following conditions:
//  - IO error or unexpected response
//  - error response from the server (e.g. authentication failure)
//  - the agent version is not set or empty
func (c *client) GetVersionForLatest() (string, error) {
	// TODO implement when API is available
	return "1.142.0.20180313-173634", nil
}

// GetVersionForIp returns the agent version running on the host with the given IP address.
// Returns the version string formatted as "Major.Minor.Revision.Timestamp" on success.
//
// Returns an error for the following conditions:
//  - IO error or unexpected response
//  - error response from the server (e.g. authentication failure)
//  - a host with the given IP cannot be found
//  - the agent version for the host is not set
func (c *client) GetVersionForIp(ip net.IP) (string, error) {
	url := fmt.Sprintf("%s/v1/entity/infrastructure/hosts?Api-Token=%s", c.url, c.apiToken)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	return readVersionForIp(resp.Body, ip)
}

// serverError represents an error returned from the server (e.g. authentication failure).
type serverError struct {
	Code    float64
	Message string
}

// Error formats the server error code and message.
func (e serverError) Error() string {
	if len(e.Message) == 0 && e.Code == 0 {
		return "unknown server error"
	}
	return fmt.Sprintf("error %d: %s", int64(e.Code), e.Message)
}

// readVersionForIp reads the agent version of the given host from the given server response reader.
func readVersionForIp(r io.Reader, ip net.IP) (string, error) {
	type jsonHost struct {
		IpAddresses  []string
		AgentVersion *struct {
			Major     int
			Minor     int
			Revision  int
			Timestamp string
		}
	}

	buf := bufio.NewReader(r)
	// Server sends an array of hosts or an error object, check which one it is
	switch b, err := buf.Peek(1); {
	case err != nil:
		return "", err

	case b[0] == '{':
		// Try decoding an error response
		var resp struct{ Error *serverError }
		switch err = json.NewDecoder(buf).Decode(&resp); {
		case err != nil:
			return "", err
		case resp.Error != nil:
			return "", resp.Error
		default:
			return "", errors.New("unexpected response from server")
		}

	case b[0] != '[':
		return "", errors.New("unexpected response from server")
	}

	// Try decoding a successful response
	var resp []jsonHost
	if err := json.NewDecoder(buf).Decode(&resp); err != nil {
		return "", err
	}

	ipStr := ip.String()
	for _, host := range resp {
		if containsString(host.IpAddresses, ipStr) {
			v := host.AgentVersion
			if v == nil {
				return "", errors.New("agent version not set for host")
			}
			return fmt.Sprintf("%d.%d.%d.%s", v.Major, v.Minor, v.Revision, v.Timestamp), nil
		}
	}
	return "", errors.New("host not found")
}

// containsString determines whether haystack contains the string needle.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
