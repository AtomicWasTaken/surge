package httpclient

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientRequestMethodsAndConfiguration(t *testing.T) {
	type capturedRequest struct {
		Method      string
		Path        string
		AuthHeader  string
		UserAgent   string
		ContentType string
		Body        map[string]string
	}

	var requests []capturedRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{}
		if r.Body != nil {
			data, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			if len(data) > 0 {
				require.NoError(t, json.Unmarshal(data, &body))
			}
		}

		requests = append(requests, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			AuthHeader:  r.Header.Get("Authorization"),
			UserAgent:   r.Header.Get("User-Agent"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		})

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer server.Close()

	client := New().WithAuth("secret-token")
	client.SetUserAgent("surge-test")
	client.SetTimeout(5 * time.Second)

	_, err := client.Get(server.URL + "/get")
	require.NoError(t, err)

	_, err = client.Post(server.URL+"/post", map[string]string{"hello": "world"})
	require.NoError(t, err)

	_, err = client.Patch(server.URL+"/patch", map[string]string{"status": "updated"})
	require.NoError(t, err)

	_, err = client.Delete(server.URL + "/delete")
	require.NoError(t, err)

	require.Len(t, requests, 4)
	assert.Equal(t, "Bearer secret-token", requests[0].AuthHeader)
	assert.Equal(t, "surge-test", requests[0].UserAgent)
	assert.Equal(t, http.MethodGet, requests[0].Method)
	assert.Equal(t, "/get", requests[0].Path)
	assert.Empty(t, requests[0].ContentType)

	assert.Equal(t, http.MethodPost, requests[1].Method)
	assert.Equal(t, "application/json", requests[1].ContentType)
	assert.Equal(t, map[string]string{"hello": "world"}, requests[1].Body)

	assert.Equal(t, http.MethodPatch, requests[2].Method)
	assert.Equal(t, "application/json", requests[2].ContentType)
	assert.Equal(t, map[string]string{"status": "updated"}, requests[2].Body)

	assert.Equal(t, http.MethodDelete, requests[3].Method)
	assert.Equal(t, "/delete", requests[3].Path)

	assert.Equal(t, 5*time.Second, client.HTTPClient().Timeout)
	assert.Same(t, client.HTTPClient(), client.BaseClient().GetClient())
}

func TestClientWithoutAuthDoesNotSetAuthorizationHeader(t *testing.T) {
	var authHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New()
	_, err := client.Get(server.URL)
	require.NoError(t, err)

	assert.Empty(t, authHeader)
}

func TestClientRetriesOnServerErrorsAndRateLimits(t *testing.T) {
	t.Run("retries on 500", func(t *testing.T) {
		var hits int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			if hits < 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := New()
		_, err := client.Get(server.URL)
		require.NoError(t, err)
		assert.Equal(t, 2, hits)
	})

	t.Run("retries on 429", func(t *testing.T) {
		var hits int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			if hits < 2 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := New()
		_, err := client.Get(server.URL)
		require.NoError(t, err)
		assert.Equal(t, 2, hits)
	})
}

func TestClientRetryConditionBranches(t *testing.T) {
	client := New()
	require.NotEmpty(t, client.BaseClient().RetryConditions)
	condition := client.BaseClient().RetryConditions[0]

	assert.True(t, condition(nil, errors.New("network down")))

	resp429 := &resty.Response{RawResponse: &http.Response{StatusCode: http.StatusTooManyRequests}}
	assert.True(t, condition(resp429, nil))

	resp500 := &resty.Response{RawResponse: &http.Response{StatusCode: http.StatusInternalServerError}}
	assert.True(t, condition(resp500, nil))

	resp200 := &resty.Response{RawResponse: &http.Response{StatusCode: http.StatusOK}}
	assert.False(t, condition(resp200, nil))
}
