/*
 * MinIO Go Library for Amazon S3 Compatible Cloud Storage
 * Copyright 2015-2017 MinIO, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package minio

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// MaxRetry is the maximum number of retries before stopping.
var MaxRetry = 10

// MaxJitter will randomize over the full exponential backoff time
const MaxJitter = 1.0

// NoJitter disables the use of jitter for randomizing the exponential backoff time
const NoJitter = 0.0

// DefaultRetryUnit - default unit multiplicative per retry.
// defaults to 200 * time.Millisecond
var DefaultRetryUnit = 200 * time.Millisecond

// DefaultRetryCap - Each retry attempt never waits no longer than
// this maximum time duration.
var DefaultRetryCap = time.Minute

// newRetryTimer creates a timer with exponentially increasing
// delays until the maximum retry attempts are reached.
func (c *Client) newRetryTimer(ctx context.Context, maxRetry int, unit, cap time.Duration, jitter float64) <-chan int {
	attemptCh := make(chan int)

	// computes the exponential backoff duration according to
	// https://www.awsarchitectureblog.com/2015/03/backoff.html
	go func() {
		defer close(attemptCh)
		for i := 0; i < maxRetry; i++ {
			select {
			case attemptCh <- i + 1:
			case <-ctx.Done():
				return
			}

			select {
			case <-time.After(time.Hour):
			case <-ctx.Done():
				return
			}
		}
	}()
	return attemptCh
}

// List of AWS S3 error codes which are retryable.
var retryableS3Codes = map[string]struct{}{
	"RequestError":          {},
	"RequestTimeout":        {},
	"Throttling":            {},
	"ThrottlingException":   {},
	"RequestLimitExceeded":  {},
	"RequestThrottled":      {},
	"InternalError":         {},
	"ExpiredToken":          {},
	"ExpiredTokenException": {},
	"SlowDown":              {},
	// Add more AWS S3 codes here.
}

// isS3CodeRetryable - is s3 error code retryable.
func isS3CodeRetryable(s3Code string) (ok bool) {
	_, ok = retryableS3Codes[s3Code]
	return ok
}

// List of HTTP status codes which are retryable.
var retryableHTTPStatusCodes = map[int]struct{}{
	429:                            {}, // http.StatusTooManyRequests is not part of the Go 1.5 library, yet
	499:                            {}, // client closed request, retry. A non-standard status code introduced by nginx.
	http.StatusInternalServerError: {},
	http.StatusBadGateway:          {},
	http.StatusServiceUnavailable:  {},
	http.StatusGatewayTimeout:      {},
	520:                            {}, // It is used by Cloudflare as a catch-all response for when the origin server sends something unexpected.
	// Add more HTTP status codes here.
}

// isHTTPStatusRetryable - is HTTP error code retryable.
func isHTTPStatusRetryable(httpStatusCode int) (ok bool) {
	_, ok = retryableHTTPStatusCodes[httpStatusCode]
	return ok
}

// For now, all http Do() requests are retriable except some well defined errors
func isRequestErrorRetryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if ue, ok := err.(*url.Error); ok {
		e := ue.Unwrap()
		switch e.(type) {
		// x509: certificate signed by unknown authority
		case x509.UnknownAuthorityError:
			return false
		}
		switch e.Error() {
		case "http: server gave HTTP response to HTTPS client":
			return false
		}
	}
	return true
}
