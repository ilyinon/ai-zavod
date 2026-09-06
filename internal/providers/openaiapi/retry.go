package openaiapi

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"zavod_ai/internal/llm"
)

type retryPolicy struct {
	AttemptTimeout, TotalTimeout time.Duration
	Delays                       []time.Duration
}

func (c *Client) generateWithRetry(ctx context.Context, req llm.Request) (*llm.Response, error) {
	p := c.retry
	if p.TotalTimeout == 0 {
		p = retryPolicy{90 * time.Second, 180 * time.Second, []time.Duration{2 * time.Second, 5 * time.Second}}
	}
	started := time.Now()
	operation, cancel := context.WithTimeout(ctx, p.TotalTimeout)
	defer cancel()
	maxAttempts := len(p.Delays) + 1
	if req.NoRetry {
		maxAttempts = 1
	}
	var last *llm.ProviderError
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if operation.Err() != nil {
			if last == nil {
				last = classifyFailure(operation.Err())
				last.ModelID = req.Model
				last.MaxAttempts = maxAttempts
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					last.Kind = "execution_deadline"
					last.Retryable = false
				}
			} else {
				last.Retryable = false
			}
			last.ElapsedMS = time.Since(started).Milliseconds()
			return nil, last
		}
		attemptCtx, stop := context.WithTimeout(operation, p.AttemptTimeout)
		resp, err := c.generateOnce(attemptCtx, req)
		stop()
		if err == nil {
			return resp, nil
		}
		last = classifyFailure(err)
		last.ModelID = req.Model
		last.Attempt = attempt
		last.MaxAttempts = maxAttempts
		last.ElapsedMS = time.Since(started).Milliseconds()
		if ctx.Err() != nil {
			last = classifyFailure(ctx.Err())
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				last.Kind = "execution_deadline"
				last.Retryable = false
			}
			last.ModelID = req.Model
			last.Attempt = attempt
			last.MaxAttempts = maxAttempts
			last.ElapsedMS = time.Since(started).Milliseconds()
			return nil, last
		}
		if !last.Retryable || attempt == maxAttempts || operation.Err() != nil {
			return nil, last
		}
		delay := p.Delays[attempt-1]
		if last.RetryAfter > delay {
			delay = last.RetryAfter
		}
		deadline, _ := operation.Deadline()
		if time.Until(deadline) <= delay {
			last.Retryable = false
			return nil, last
		}
		llm.NotifyRetry(ctx, llm.RetryEvent{Attempt: attempt + 1, MaxAttempts: maxAttempts, Delay: delay, Failure: last})
		timer := time.NewTimer(delay)
		select {
		case <-operation.Done():
			timer.Stop()
			last.ElapsedMS = time.Since(started).Milliseconds()
			if ctx.Err() != nil {
				last.Kind = "cancelled"
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					last.Kind = "execution_deadline"
				}
				last.Retryable = false
				last.Cause = ctx.Err()
			}
			return nil, last
		case <-timer.C:
		}
	}
	return nil, last
}

func classifyFailure(err error) *llm.ProviderError {
	var p *llm.ProviderError
	if errors.As(err, &p) {
		copy := *p
		return &copy
	}
	if errors.Is(err, context.Canceled) {
		return &llm.ProviderError{Kind: "cancelled", Cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &llm.ProviderError{Kind: "provider_timeout", Retryable: true, Cause: err}
	}
	var cert x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	if errors.As(err, &cert) || errors.As(err, &invalid) || errors.As(err, &host) {
		return &llm.ProviderError{Kind: "provider_invalid_request", Cause: err}
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return &llm.ProviderError{Kind: "provider_timeout", Retryable: true, Cause: err}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsNotFound {
		return &llm.ProviderError{Kind: "provider_invalid_request", Cause: err}
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return &llm.ProviderError{Kind: "provider_unavailable", Retryable: true, Cause: err}
	}
	return &llm.ProviderError{Kind: "provider_connection", Cause: err}
}

func httpFailure(status int, retryAfter string) *llm.ProviderError {
	p := &llm.ProviderError{Kind: "provider_invalid_request", HTTPStatus: status}
	switch status {
	case 401, 403:
		p.Kind = "provider_auth"
	case 429:
		p.Kind = "provider_rate_limited"
		p.Retryable = true
	case 502, 503, 504:
		p.Kind = "provider_unavailable"
		p.Retryable = true
	}
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 && seconds <= 86400 {
		p.RetryAfter = time.Duration(seconds) * time.Second
	} else if at, err := http.ParseTime(retryAfter); err == nil {
		p.RetryAfter = time.Until(at)
	}
	return p
}
