package http

import (
	"crypto/tls"
	"io"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// RestyAdapter wraps resty.Client to satisfy the domain.HTTPClient interface
// and provides built-in retry logic with exponential backoff.
type RestyAdapter struct {
	client *resty.Client
}

// NewRestyClient creates a new HTTP client using resty with retry support.
// Retries are configured with exponential backoff (3 attempts, 100ms base wait).
func NewRestyClient() *RestyAdapter {
	c := resty.New().
		SetRetryCount(3).
		SetRetryWaitTime(100 * time.Millisecond).
		SetRetryMaxWaitTime(2 * time.Second).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			// Retry on network errors or 5xx status codes
			if err != nil {
				return true
			}
			statusCode := r.StatusCode()
			return statusCode >= 500 && statusCode < 600
		})

	return &RestyAdapter{client: c}
}

// NewRestyClientWithTLS creates a resty-based HTTP client with TLS configuration.
// If insecure is true, certificate verification is skipped.
func NewRestyClientWithTLS(insecure bool) *RestyAdapter {
	c := resty.New().
		SetRetryCount(3).
		SetRetryWaitTime(100 * time.Millisecond).
		SetRetryMaxWaitTime(2 * time.Second).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true
			}
			statusCode := r.StatusCode()
			return statusCode >= 500 && statusCode < 600
		})

	// Configure TLS if needed
	if insecure {
		c.SetTLSClientConfig(&tls.Config{
			InsecureSkipVerify: true,
		})
	}

	return &RestyAdapter{client: c}
}

// Do performs an HTTP request, returning the response or error.
// Retries are handled transparently by the underlying resty client.
// The returned response body can be read by the caller.
func (r *RestyAdapter) Do(req *http.Request) (*http.Response, error) {
	// Extract method, URL from the http.Request
	method := req.Method
	url := req.URL.String()

	// Create a resty request with DoNotParseResponse to preserve the body
	restyReq := r.client.R().SetDoNotParseResponse(true)

	// Copy headers from the original request
	for key, values := range req.Header {
		for _, value := range values {
			restyReq.Header.Add(key, value)
		}
	}

	// Copy context from the original request
	if req.Context() != nil {
		restyReq = restyReq.SetContext(req.Context())
	}

	// Copy body if present - must read body into bytes for resty to accept it
	if req.Body != nil {
		// Read the entire body into memory for resty
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
		restyReq = restyReq.SetBody(body)
	}

	// Execute the request with the appropriate method
	var restyResp *resty.Response
	var err error

	switch method {
	case http.MethodGet:
		restyResp, err = restyReq.Get(url)
	case http.MethodPost:
		restyResp, err = restyReq.Post(url)
	case http.MethodPut:
		restyResp, err = restyReq.Put(url)
	case http.MethodDelete:
		restyResp, err = restyReq.Delete(url)
	case http.MethodHead:
		restyResp, err = restyReq.Head(url)
	case http.MethodPatch:
		restyResp, err = restyReq.Patch(url)
	default:
		restyResp, err = restyReq.Execute(method, url)
	}

	if err != nil {
		return nil, err
	}

	// Return the resty response as an http.Response
	// With SetDoNotParseResponse(true), the body is preserved and can be read
	return restyResp.RawResponse, nil
}

// Client returns the underlying resty.Client for advanced operations
func (r *RestyAdapter) Client() *resty.Client {
	return r.client
}
