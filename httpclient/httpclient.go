/*
Copyright 2020. Huawei Technologies Co., Ltd. All rights reserved.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/dafanasiev/go-hms-push/push/config"
	"github.com/dafanasiev/go-hms-push/trace"
)

type PushRequest struct {
	Method string
	URL    string
	Body   []byte
	Header []HTTPOption
}

type PushResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

type HTTPTransportConfig struct {
	ProxyUrl  *url.URL
	TrustedCA string
}

type HTTPRetryConfig struct {
	MaxRetryTimes int
	RetryInterval time.Duration
}

type HTTPClientConfig struct {
	TransportConfig *HTTPTransportConfig
	RetryConfig     *HTTPRetryConfig
}

type HTTPClient struct {
	Client      *http.Client
	RetryConfig *HTTPRetryConfig
}

type HTTPOption func(r *http.Request)

func SetHeader(key string, value string) HTTPOption {
	return func(r *http.Request) {
		r.Header.Set(key, value)
	}
}

func NewHTTPClientConfig(c *config.Config) (*HTTPClientConfig, error) {
	if c == nil {
		return nil, errors.New("config is nil")
	}

	httpClientConfig := HTTPClientConfig{
		RetryConfig: &HTTPRetryConfig{
			MaxRetryTimes: c.MaxRetryTimes,
			RetryInterval: c.RetryInterval,
		},
	}

	if len(c.ProxyUrl) > 0 {
		proxyURL, err := url.ParseRequestURI(c.ProxyUrl)
		if err != nil {
			return nil, fmt.Errorf("parse proxy url error: %w", err)
		}
		httpClientConfig.TransportConfig = &HTTPTransportConfig{ProxyUrl: proxyURL, TrustedCA: c.TrustedCA}
	}

	return &httpClientConfig, nil
}

func NewHTTPClient(config *HTTPClientConfig) (*HTTPClient, error) {
	var retryConfig *HTTPRetryConfig = nil

	tr := http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
		TLSClientConfig:    &tls.Config{},
	}

	if config != nil {
		if config.RetryConfig != nil {
			if config.RetryConfig.MaxRetryTimes < 1 || config.RetryConfig.MaxRetryTimes > 5 {
				return nil, errors.New("maximum retry times value cannot be less than 1 and more than 5")
			}
			retryConfig = config.RetryConfig
		}

		if config.TransportConfig != nil {
			if config.TransportConfig.ProxyUrl != nil {
				tr.Proxy = http.ProxyURL(config.TransportConfig.ProxyUrl)
			}

			trustedCaPem := config.TransportConfig.TrustedCA
			if trustedCaPem != "" {
				bytes, err := ioutil.ReadFile(trustedCaPem)
				if err != nil {
					return nil, err
				}

				rootCAs, _ := x509.SystemCertPool()
				if rootCAs == nil {
					rootCAs = x509.NewCertPool()
				}
				if ok := rootCAs.AppendCertsFromPEM(bytes); !ok {
					return nil, errors.New("failed to parse trusted CA certificate")
				}

				tr.TLSClientConfig.RootCAs = rootCAs
			}
		}
	}

	if retryConfig == nil {
		retryConfig = &HTTPRetryConfig{
			MaxRetryTimes: 1,
			RetryInterval: 0,
		}
	}

	httpClient := HTTPClient{Client: &http.Client{Transport: &tr}, RetryConfig: retryConfig}
	return &httpClient, nil
}

func (r *PushRequest) buildHTTPRequest() (*http.Request, error) {
	var body io.Reader

	if r.Body != nil {
		body = bytes.NewBuffer(r.Body)
	}

	req, err := http.NewRequest(r.Method, r.URL, body)
	if err != nil {
		return nil, err
	}

	for _, opt := range r.Header {
		opt(req)
	}

	return req, nil
}

func (c *HTTPClient) doHttpRequest(ctx context.Context, req *PushRequest) (*PushResponse, error) {
	request, err := req.buildHTTPRequest()
	if err != nil {
		return nil, err
	}

	var tr trace.HmsTrace
	if t := ctx.Value(trace.HmsTraceKey); t != nil {
		tr = t.(trace.HmsTrace)
	}

	if tr.GotRequestBody != nil {
		tr.GotRequestBody(req.Body)
	}

	resp, err := c.Client.Do(request.WithContext(ctx))

	if err != nil {
		return nil, err
	}

	if tr.GotResponseStatus != nil {
		tr.GotResponseStatus(resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}

	if tr.GotResponseBody != nil {
		tr.GotResponseBody(body)
	}

	return &PushResponse{
		Status: resp.StatusCode,
		Header: resp.Header,
		Body:   body,
	}, nil
}

func (c *HTTPClient) DoHttpRequest(ctx context.Context, req *PushRequest) (*PushResponse, error) {
	var (
		result *PushResponse
		err    error
	)
	for retryTimes := 0; retryTimes < c.RetryConfig.MaxRetryTimes; retryTimes++ {
		result, err = c.doHttpRequest(ctx, req)

		if err == nil {
			break
		}

		if !c.pendingForRetry(ctx) {
			break
		}
	}
	return result, err
}

func (c *HTTPClient) pendingForRetry(ctx context.Context) bool {
	if c.RetryConfig.RetryInterval > 0 {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(c.RetryConfig.RetryInterval):
			return true
		}
	}
	return false
}
