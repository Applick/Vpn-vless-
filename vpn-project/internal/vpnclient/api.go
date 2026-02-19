package vpnclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func FetchClientConfigWithToken(serverHost, clientID, apiToken string) (ClientConfigResponse, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return ClientConfigResponse{}, errors.New("client ID is empty")
	}

	apiURL, err := BuildClientConfigURL(serverHost, clientID)
	if err != nil {
		return ClientConfigResponse{}, err
	}

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return ClientConfigResponse{}, err
	}
	if token := strings.TrimSpace(apiToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	httpClient := &http.Client{Timeout: 12 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return ClientConfigResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return ClientConfigResponse{}, fmt.Errorf("api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed ClientConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return ClientConfigResponse{}, err
	}
	if strings.TrimSpace(parsed.Config) == "" {
		return ClientConfigResponse{}, errors.New("API returned empty config")
	}
	return parsed, nil
}

func BuildClientConfigURL(serverHost, clientID string) (string, error) {
	host := strings.TrimSpace(serverHost)
	if host == "" {
		host = "127.0.0.1"
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}

	parsed, err := url.Parse(host)
	if err != nil {
		return "", err
	}
	if parsed.Host == "" {
		return "", errors.New("invalid server host")
	}
	if parsed.Port() == "" {
		parsed.Host = net.JoinHostPort(parsed.Hostname(), "8080")
	}
	parsed.Path = fmt.Sprintf("/clients/%s/config", url.PathEscape(clientID))
	return parsed.String(), nil
}
