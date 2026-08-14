package provider_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/provider"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want provider.ErrorKind
	}{
		{
			name: "429",
			err:  &provider.HTTPError{StatusCode: 429},
			want: provider.ErrorKindRetryable,
		},
		{
			name: "500",
			err:  &provider.HTTPError{StatusCode: 500},
			want: provider.ErrorKindRetryable,
		},
		{
			name: "包装后的 429",
			err:  fmt.Errorf("OpenAI/Zhipu API 请求失败: %w", &provider.HTTPError{StatusCode: 429}),
			want: provider.ErrorKindRetryable,
		},
		{
			name: "网络超时",
			err:  timeoutError{},
			want: provider.ErrorKindRetryable,
		},
		{
			name: "401",
			err:  &provider.HTTPError{StatusCode: 401},
			want: provider.ErrorKindFatal,
		},
		{
			name: "400",
			err:  &provider.HTTPError{StatusCode: 400},
			want: provider.ErrorKindFatal,
		},
		{
			name: "普通错误",
			err:  errors.New("构建参数失败"),
			want: provider.ErrorKindFatal,
		},
		{
			name: "取消",
			err:  context.Canceled,
			want: provider.ErrorKindCanceled,
		},
		{
			name: "截止",
			err:  context.DeadlineExceeded,
			want: provider.ErrorKindCanceled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := provider.ClassifyError(tc.err)
			if got != tc.want {
				t.Fatalf("ClassifyError(%v) = %s，期望 %s", tc.err, got, tc.want)
			}
		})
	}
}
