package ollama

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

const (
	defaultRequestTimeout   = 2 * time.Minute
	defaultMaxResponseBytes = 8 * 1024 * 1024
)

type Config struct {
	RequestTimeout      time.Duration
	MaxResponseBytes    int64
	AllowPrivateNetwork bool
	AllowInsecureHTTP   bool
}

type Adapter struct {
	client            *http.Client
	requestTimeout    time.Duration
	maxResponseBytes  int64
	allowInsecureHTTP bool
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
	Error   string      `json:"error"`
}

func New(config Config) *Adapter {
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = defaultRequestTimeout
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = defaultMaxResponseBytes
	}
	return &Adapter{
		client: &http.Client{
			Transport: newTransport(config.AllowPrivateNetwork, config.RequestTimeout),
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		requestTimeout:    config.RequestTimeout,
		maxResponseBytes:  config.MaxResponseBytes,
		allowInsecureHTTP: config.AllowInsecureHTTP,
	}
}

func (a *Adapter) Generate(ctx context.Context, request entities.GenerationRequest, emit func(string) error) error {
	endpoint, err := chatEndpoint(request.ProviderAccess.BaseURL, a.allowInsecureHTTP)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(chatRequest{
		Model:    request.ModelRequest.Model.ProviderModelID,
		Messages: modelMessages(request.ModelRequest),
		Stream:   true,
	})
	if err != nil {
		return fmt.Errorf("encode Ollama request: %w", err)
	}

	requestContext, cancel := context.WithTimeout(ctx, a.requestTimeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("build Ollama request")
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/x-ndjson")
	if request.ProviderAccess.Credential != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+request.ProviderAccess.Credential)
	}

	response, err := a.client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("send Ollama request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.CopyN(io.Discard, response.Body, 32*1024)
		return fmt.Errorf("Ollama returned HTTP status %d", response.StatusCode)
	}

	limited := &io.LimitedReader{R: response.Body, N: a.maxResponseBytes + 1}
	decoder := json.NewDecoder(limited)
	completed := false
	emitted := false
	for {
		var chunk chatResponse
		if err := decoder.Decode(&chunk); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if limited.N <= 0 {
				return errors.New("Ollama response exceeded the configured size limit")
			}
			return errors.New("decode Ollama streaming response")
		}
		if chunk.Error != "" {
			return errors.New("Ollama reported a generation error")
		}
		if chunk.Message.Content != "" {
			if err := emit(chunk.Message.Content); err != nil {
				return err
			}
			emitted = true
		}
		if chunk.Done {
			completed = true
			break
		}
	}
	if limited.N <= 0 {
		return errors.New("Ollama response exceeded the configured size limit")
	}
	if !completed {
		return errors.New("Ollama streaming response ended before completion")
	}
	if !emitted {
		return errors.New("Ollama returned an empty response")
	}
	return nil
}

func modelMessages(request entities.ModelRequest) []chatMessage {
	result := make([]chatMessage, 0, len(request.Messages)+1)
	if request.SystemPrompt != "" {
		result = append(result, chatMessage{Role: "system", Content: request.SystemPrompt})
	}
	for _, message := range request.Messages {
		result = append(result, chatMessage{Role: message.Role, Content: message.Content})
	}
	return result
}

func chatEndpoint(baseURL string, allowInsecureHTTP bool) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Ollama base URL is invalid")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && allowInsecureHTTP) {
		return "", errors.New("Ollama base URL must use HTTPS")
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/api/chat"):
		parsed.Path = path
	case strings.HasSuffix(path, "/api"):
		parsed.Path = path + "/chat"
	default:
		parsed.Path = path + "/api/chat"
	}
	parsed.RawPath = ""
	return parsed.String(), nil
}

func newTransport(allowPrivateNetwork bool, responseHeaderTimeout time.Duration) *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.New("resolve Ollama address")
			}
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(addresses) == 0 {
				return nil, errors.New("resolve Ollama host")
			}
			for _, address := range addresses {
				if !allowPrivateNetwork && isPrivateOrReserved(address.IP) {
					return nil, errors.New("Ollama host resolves to a private or reserved network")
				}
			}
			return dialer.DialContext(ctx, "tcp", net.JoinHostPort(addresses[0].IP.String(), port))
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
}

func isPrivateOrReserved(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() {
		return true
	}
	for _, prefix := range []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
	} {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
