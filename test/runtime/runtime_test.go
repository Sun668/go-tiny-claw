package runtime_test

import (
	"context"
	"errors"
	"testing"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
)

type fakeRunner struct {
	run func(
		context.Context,
		*ctxpkg.Session,
		reporter.Reporter,
	) error
}

func (r *fakeRunner) Run(
	ctx context.Context,
	session *ctxpkg.Session,
	rep reporter.Reporter,
) error {
	return r.run(ctx, session, rep)
}

func newTestRuntime(runner runtimepkg.AgentRunner) *runtimepkg.Runtime {
	return runtimepkg.NewRuntime(
		runner,
		ctxpkg.NewSession("session-test", "/tmp/workspace"),
	)
}

func TestRuntimeTaskCompleted(t *testing.T) {
	rt := newTestRuntime(&fakeRunner{
		run: func(
			context.Context,
			*ctxpkg.Session,
			reporter.Reporter,
		) error {
			return nil
		},
	})

	task, err := rt.Start(context.Background(), "测试任务", nil)
	if err != nil {
		t.Fatalf("启动任务失败: %v", err)
	}

	if err := task.Wait(); err != nil {
		t.Fatalf("任务不应该失败: %v", err)
	}

	if task.Status() != runtimepkg.TaskCompleted {
		t.Fatalf("期望状态 %s，实际状态 %s", runtimepkg.TaskCompleted, task.Status())
	}

	if task.Err() != nil {
		t.Fatalf("期望任务错误为空，实际错误: %v", task.Err())
	}
}

func TestRuntimeTaskFailed(t *testing.T) {
	expectedErr := errors.New("执行失败")
	rt := newTestRuntime(&fakeRunner{
		run: func(
			context.Context,
			*ctxpkg.Session,
			reporter.Reporter,
		) error {
			return expectedErr
		},
	})

	task, err := rt.Start(context.Background(), "失败任务", nil)
	if err != nil {
		t.Fatalf("启动任务失败: %v", err)
	}

	if err := task.Wait(); !errors.Is(err, expectedErr) {
		t.Fatalf("期望错误 %v，实际错误 %v", expectedErr, err)
	}

	if task.Status() != runtimepkg.TaskFailed {
		t.Fatalf("期望状态 %s，实际状态 %s", runtimepkg.TaskFailed, task.Status())
	}
}

func TestRuntimeTaskCanceled(t *testing.T) {
	rt := newTestRuntime(&fakeRunner{
		run: func(
			ctx context.Context,
			_ *ctxpkg.Session,
			_ reporter.Reporter,
		) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})

	task, err := rt.Start(context.Background(), "取消任务", nil)
	if err != nil {
		t.Fatalf("启动任务失败: %v", err)
	}

	task.Cancel()

	if err := task.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际错误: %v", err)
	}

	if task.Status() != runtimepkg.TaskCanceled {
		t.Fatalf("期望状态 %s，实际状态 %s", runtimepkg.TaskCanceled, task.Status())
	}
}

func TestRuntimeCancelActiveTask(t *testing.T) {
	started := make(chan struct{})

	rt := newTestRuntime(&fakeRunner{
		run: func(
			ctx context.Context,
			_ *ctxpkg.Session,
			_ reporter.Reporter,
		) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	})

	task, err := rt.Start(context.Background(), "需要取消的任务", nil)
	if err != nil {
		t.Fatalf("启动任务失败: %v", err)
	}

	<-started

	if !rt.IsRunning() {
		t.Fatal("任务运行期间 IsRunning 应该为 true")
	}

	if !rt.Cancel() {
		t.Fatal("存在活动任务时 Cancel 应该返回 true")
	}

	if err := task.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际错误: %v", err)
	}

	if rt.Cancel() {
		t.Fatal("任务结束后 Cancel 应该返回 false")
	}
}

func TestRuntimeRejectsConcurrentTasks(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	rt := newTestRuntime(&fakeRunner{
		run: func(
			ctx context.Context,
			_ *ctxpkg.Session,
			_ reporter.Reporter,
		) error {
			close(started)

			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})

	firstTask, err := rt.Start(context.Background(), "任务一", nil)
	if err != nil {
		t.Fatalf("启动第一个任务失败: %v", err)
	}

	<-started

	if _, err := rt.Start(context.Background(), "任务二", nil); !errors.Is(err, runtimepkg.ErrTaskRunning) {
		t.Fatalf("期望 ErrTaskRunning，实际错误: %v", err)
	}

	close(release)

	if err := firstTask.Wait(); err != nil {
		t.Fatalf("第一个任务不应该失败: %v", err)
	}
}

func TestRuntimeClearWhileRunning(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	rt := newTestRuntime(&fakeRunner{
		run: func(
			ctx context.Context,
			_ *ctxpkg.Session,
			_ reporter.Reporter,
		) error {
			close(started)
			<-release
			return nil
		},
	})

	task, err := rt.Start(context.Background(), "运行任务", nil)
	if err != nil {
		t.Fatalf("启动任务失败: %v", err)
	}

	<-started

	if err := rt.Clear(); !errors.Is(err, runtimepkg.ErrClearWhileRunning) {
		t.Fatalf("期望 ErrClearWhileRunning，实际错误: %v", err)
	}

	close(release)

	if err := task.Wait(); err != nil {
		t.Fatalf("任务不应该失败: %v", err)
	}

	if err := rt.Clear(); err != nil {
		t.Fatalf("任务结束后应该允许清空会话: %v", err)
	}
}

func TestRuntimeRejectsCanceledParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	rt := newTestRuntime(&fakeRunner{
		run: func(
			context.Context,
			*ctxpkg.Session,
			reporter.Reporter,
		) error {
			t.Fatal("父上下文已取消，不应该启动 Runner")
			return nil
		},
	})

	if _, err := rt.Start(parent, "取消的父上下文", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际错误: %v", err)
	}
}
