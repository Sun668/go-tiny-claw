package provider

import (
	"testing"
	"time"
)

func TestBackoffDelayDoublesAndCaps(t *testing.T) {
	p := &RetryingProvider{
		Backoff:    200 * time.Millisecond,
		MaxBackoff: 500 * time.Millisecond,
	}

	if got := p.backoffDelay(1); got != 200*time.Millisecond {
		t.Fatalf("第 1 次失败后应等待 200ms，实际: %s", got)
	}
	if got := p.backoffDelay(2); got != 400*time.Millisecond {
		t.Fatalf("第 2 次失败后应等待 400ms，实际: %s", got)
	}
	if got := p.backoffDelay(3); got != 500*time.Millisecond {
		t.Fatalf("第 3 次失败后应封顶 500ms，实际: %s", got)
	}
}

func TestBackoffDelayZeroMeansNoWait(t *testing.T) {
	p := &RetryingProvider{Backoff: 0}
	if got := p.backoffDelay(1); got != 0 {
		t.Fatalf("Backoff=0 应立即重试，实际: %s", got)
	}
}
