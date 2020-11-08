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

package config

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/almunt/go-hms-push/httpclient"
)

type Config struct {
	AppId         string
	AppSecret     string
	AuthUrl       string
	PushUrl       string
	HttpProxyUrl  string
	MaxRetryTimes int
	RetryInterval time.Duration
}

func (c *Config) ToHTTPClientConfig() (*httpclient.HTTPClientConfig, error) {
	if c.MaxRetryTimes < 1 {
		return nil, errors.New(fmt.Sprintf("MaxRetryTimes value is invalid: %d", c.MaxRetryTimes))
	}

	httpClientConfig := httpclient.HTTPClientConfig{
		RetryConfig: &httpclient.HTTPRetryConfig{
			MaxRetryTimes: c.MaxRetryTimes,
			RetryInterval: c.RetryInterval,
		},
	}

	if len(c.HttpProxyUrl) > 0 {
		proxyUrl, err := url.ParseRequestURI(c.HttpProxyUrl)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("HttpProxyUrl value is invalid: %w", err))
		}
		httpClientConfig.ProxyUrl = proxyUrl
	}

	return &httpClientConfig, nil
}
