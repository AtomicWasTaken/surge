package httpclient

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client wraps resty with shared configuration.
type Client struct {
	hc   *resty.Client
	auth string
}

// New creates a new HTTP client with retry and timeout support.
func New() *Client {
	hc := resty.New().
		SetTimeout(60 * time.Second).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: false}).
		SetRetryCount(3).
		SetRetryWaitTime(2 * time.Second).
		SetRetryMaxWaitTime(10 * time.Second).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true // Network errors are retryable
			}
			// Retry on 429 (rate limited) and 5xx server errors
			return r.StatusCode() == 429 || r.StatusCode() >= 500
		})

	return &Client{hc: hc}
}

// WithAuth sets Bearer token authentication.
func (c *Client) WithAuth(token string) *Client {
	c.auth = token
	return c
}

// Get performs a GET request and returns the response.
func (c *Client) Get(url string) (*resty.Response, error) {
	req := c.hc.R()
	if c.auth != "" {
		req.SetAuthToken(c.auth)
	}
	return req.Get(url)
}

// Post performs a POST request with a JSON body and returns the response.
func (c *Client) Post(url string, body interface{}) (*resty.Response, error) {
	req := c.hc.R()
	if c.auth != "" {
		req.SetAuthToken(c.auth)
	}
	return req.SetHeader("Content-Type", "application/json").SetBody(body).Post(url)
}

// Patch performs a PATCH request with a JSON body and returns the response.
func (c *Client) Patch(url string, body interface{}) (*resty.Response, error) {
	req := c.hc.R()
	if c.auth != "" {
		req.SetAuthToken(c.auth)
	}
	return req.SetHeader("Content-Type", "application/json").SetBody(body).Patch(url)
}

// Delete performs a DELETE request and returns the response.
func (c *Client) Delete(url string) (*resty.Response, error) {
	req := c.hc.R()
	if c.auth != "" {
		req.SetAuthToken(c.auth)
	}
	return req.Delete(url)
}

// SetTimeout sets the request timeout.
func (c *Client) SetTimeout(timeout time.Duration) {
	c.hc.SetTimeout(timeout)
}

// SetUserAgent sets a custom User-Agent header.
func (c *Client) SetUserAgent(ua string) {
	c.hc.SetHeader("User-Agent", ua)
}

// HTTPClient returns the underlying http.Client for use with httptest.
func (c *Client) HTTPClient() *http.Client {
	return c.hc.GetClient()
}

// BaseClient returns the underlying resty client.
func (c *Client) BaseClient() *resty.Client {
	return c.hc
}
