package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
)

func newManagerTestRuntime(sessionID string) *runtimepkg.Runtime {
	return runtimepkg.NewRuntime(
		&fakeRunner{
			run: func(
				context.Context,
				*ctxpkg.Session,
				reporter.Reporter,
			) error {
				return nil
			},
		},
		ctxpkg.NewSession(sessionID, "/tmp/workspace"),
	)
}

func TestManagerAddGetRemove(t *testing.T) {
	manager := runtimepkg.NewManager()
	rt := newManagerTestRuntime("session-a")

	if err := manager.Add("session-a", rt); err != nil {
		t.Fatalf("添加 Runtime 失败: %v", err)
	}

	got, err := manager.Get("session-a")
	if err != nil {
		t.Fatalf("获取 Runtime 失败: %v", err)
	}

	if got != rt {
		t.Fatal("获取到的 Runtime 不是添加的实例")
	}

	if err := manager.Remove("session-a"); err != nil {
		t.Fatalf("移除 Runtime 失败: %v", err)
	}

	if _, err := manager.Get("session-a"); !errors.Is(err, runtimepkg.ErrRuntimeNotFound) {
		t.Fatalf("期望 ErrRuntimeNotFound，实际错误: %v", err)
	}
}

func TestManagerRejectsDuplicateRuntime(t *testing.T) {
	manager := runtimepkg.NewManager()

	if err := manager.Add(
		"session-a",
		newManagerTestRuntime("session-a"),
	); err != nil {
		t.Fatalf("第一次添加 Runtime 失败: %v", err)
	}

	if err := manager.Add(
		"session-a",
		newManagerTestRuntime("session-a-duplicate"),
	); !errors.Is(err, runtimepkg.ErrRuntimeExists) {
		t.Fatalf("期望 ErrRuntimeExists，实际错误: %v", err)
	}
}

func TestManagerListAndCount(t *testing.T) {
	manager := runtimepkg.NewManager()

	for _, sessionID := range []string{"session-c", "session-a", "session-b"} {
		if err := manager.Add(sessionID, newManagerTestRuntime(sessionID)); err != nil {
			t.Fatalf("添加 %s 失败: %v", sessionID, err)
		}
	}

	if manager.Count() != 3 {
		t.Fatalf("Runtime 数量错误: got=%d want=3", manager.Count())
	}

	actual := manager.List()
	expected := []string{"session-a", "session-b", "session-c"}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Runtime 列表错误: got=%v want=%v", actual, expected)
	}
}

func TestManagerSupportsConcurrentDifferentRuntimes(t *testing.T) {
	manager := runtimepkg.NewManager()
	const runtimeCount = 32

	var wg sync.WaitGroup
	wg.Add(runtimeCount)

	for index := 0; index < runtimeCount; index++ {
		index := index

		go func() {
			defer wg.Done()

			sessionID := fmt.Sprintf("session-%d", index)
			err := manager.Add(
				sessionID,
				newManagerTestRuntime(sessionID),
			)
			if err != nil {
				t.Errorf("添加 %s 失败: %v", sessionID, err)
			}
		}()
	}

	wg.Wait()

	if manager.Count() != runtimeCount {
		t.Fatalf("并发添加后的 Runtime 数量错误: got=%d want=%d", manager.Count(), runtimeCount)
	}
}
