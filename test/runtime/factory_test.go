package runtime_test

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type fakeProvider struct{}

func (p *fakeProvider) Generate(
	ctx context.Context,
	messages []schema.Message,
	availableTools []schema.ToolDefinition,
) (*schema.Message, error) {
	return &schema.Message{}, nil
}

var _ provider.LLMProvider = (*fakeProvider)(nil)

func TestFactoryCreatesIndependentRuntimes(t *testing.T) {
	factory := runtimepkg.NewRuntimeFactory(
		&fakeProvider{},
		t.TempDir(),
		ctxpkg.GlobalSessionMgr,
	)

	bundleA, err := factory.NewTerminalRuntime(
		"factory-session-a",
		bufio.NewReader(strings.NewReader("")),
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("创建第一个 Runtime 失败: %v", err)
	}

	bundleB, err := factory.NewTerminalRuntime(
		"factory-session-b",
		bufio.NewReader(strings.NewReader("")),
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("创建第二个 Runtime 失败: %v", err)
	}

	if bundleA.Runtime == bundleB.Runtime {
		t.Fatal("不同 Session 不应该共享 Runtime")
	}

	if bundleA.Runtime.SessionID() != "factory-session-a" {
		t.Fatalf("第一个 Runtime Session ID 错误: %s", bundleA.Runtime.SessionID())
	}

	if bundleB.Runtime.SessionID() != "factory-session-b" {
		t.Fatalf("第二个 Runtime Session ID 错误: %s", bundleB.Runtime.SessionID())
	}

	if bundleA.Reporter == bundleB.Reporter {
		t.Fatal("不同 Terminal 不应该共享 Reporter")
	}
}

func TestFactoryReusesSessionByID(t *testing.T) {
	factory := runtimepkg.NewRuntimeFactory(
		&fakeProvider{},
		t.TempDir(),
		ctxpkg.GlobalSessionMgr,
	)

	firstBundle, err := factory.NewTerminalRuntime(
		"factory-reused-session",
		bufio.NewReader(strings.NewReader("")),
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("第一次创建 Runtime 失败: %v", err)
	}

	secondBundle, err := factory.NewTerminalRuntime(
		"factory-reused-session",
		bufio.NewReader(strings.NewReader("")),
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("第二次创建 Runtime 失败: %v", err)
	}

	if firstBundle.Runtime == secondBundle.Runtime {
		t.Fatal("相同 Session ID 也应该创建不同 Runtime 实例")
	}
}

func TestManagerCreatesAndDestroysRuntime(t *testing.T) {
	factory := runtimepkg.NewRuntimeFactory(
		&fakeProvider{},
		t.TempDir(),
		nil,
	)
	manager := runtimepkg.NewManagerWithFactory(factory)

	bundle, err := manager.Create(
		"managed-terminal",
		bufio.NewReader(strings.NewReader("")),
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("Manager 创建 Runtime 失败: %v", err)
	}

	got, err := manager.Get("managed-terminal")
	if err != nil {
		t.Fatalf("查询 Runtime 失败: %v", err)
	}

	if got != bundle.Runtime {
		t.Fatal("Manager 查询到的 Runtime 与创建结果不一致")
	}

	if err := manager.Destroy("managed-terminal"); err != nil {
		t.Fatalf("销毁 Runtime 失败: %v", err)
	}

	if manager.Count() != 0 {
		t.Fatalf("销毁后 Runtime 数量错误: got=%d want=0", manager.Count())
	}
}
