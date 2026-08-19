package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
)

type ErrorKind string

const (
	ErrorKindRetryable ErrorKind = "retryable"
	ErrorKindFatal     ErrorKind = "fatal"
	ErrorKindCanceled  ErrorKind = "canceled"
)

type HTTPError struct {
	StatusCode int
	Err        error
}

type PartialResponseError struct {
	Err error
}

func (e *PartialResponseError) Error() string {
	if e == nil {
		return "流式响应在输出中途中断"
	}
	if e.Err != nil {
		return fmt.Sprintf("流式响应在输出中途中断: %v", e.Err)
	}
	return "流式响应在输出中途中断"
}

func (e *PartialResponseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func wrapPartial(err error) error {
	var partial *PartialResponseError
	if errors.As(err, &partial) {
		return err
	}
	return &PartialResponseError{Err: err}
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "模型服务返回未知 HTTP 错误"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("模型服务返回 HTTP %d", e.StatusCode)
}

func (e *HTTPError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ClassifyError(err error) ErrorKind {
	if err == nil {
		return ErrorKindFatal
	}

	var partialErr *PartialResponseError
	if errors.As(err, &partialErr) && partialErr != nil {
		return ErrorKindFatal
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrorKindCanceled
	}

	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		if httpErr.StatusCode == 429 || httpErr.StatusCode >= 500 {
			return ErrorKindRetryable
		}
		return ErrorKindFatal
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrorKindRetryable
	}

	return ErrorKindFatal
}
