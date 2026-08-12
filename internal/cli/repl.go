package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	runtime "github.com/Sun668/go-tiny-claw/internal/runtime"
)

type Runtime interface {
	Start(parent context.Context, prompt string, reporter reporter.Reporter) (*runtime.Task, error)
	Clear() error
}

type REPL struct {
	reader   *bufio.Reader
	out      io.Writer
	runtime  Runtime
	reporter reporter.Reporter
	approval *TerminalApprovalHandler

	mu     sync.Mutex
	active *runtime.Task
}

func (r *REPL) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		if r.isIdle() {
			fmt.Fprint(r.out, "\nclaw>")
		}

		line, err := r.reader.ReadString('\n')

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		prompt := strings.TrimSpace(line)

		if r.approval != nil && r.approval.HasPending() {
			decision, ok := parseApprovalDecision(prompt)
			if !ok {
				fmt.Fprintln(r.out, "请输入 y / a / n")
				continue
			}
			if err := r.approval.Respond(decision); err != nil {
				fmt.Fprintln(r.out, err)
			}
			continue
		}

		if !r.isIdle() {
			fmt.Fprintln(r.out, "当前任务正在执行，请等待任务完成。")
			continue
		}

		if prompt == "" {
			continue
		}

		switch prompt {
		case "/exit", "/quit":
			return nil
		case "/clear":
			if err := r.runtime.Clear(); err != nil {
				fmt.Fprintln(r.out, err)
				continue
			}
			fmt.Fprintln(r.out, "会话已清空。")
			continue
		case "/help":
			printHelp(r.out)
			continue
		}

		if err := r.runTurn(ctx, prompt); err != nil {
			fmt.Fprintf(r.out, "引擎运行出错: %v\n", err)
		}
	}
}

func (r *REPL) runTurn(parent context.Context, prompt string) error {
	task, err := r.runtime.Start(parent, prompt, r.reporter)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.active = task
	r.mu.Unlock()

	go func() {
		err := task.Wait()
		r.mu.Lock()
		if r.active == task {
			r.active = nil
		}
		r.mu.Unlock()
		switch task.Status() {
		case runtime.TaskCanceled:
			fmt.Fprintln(r.out, "任务已取消。")
		case runtime.TaskTimedOut:
			fmt.Fprintln(r.out, "任务超时。")
		default:
			if err != nil {
				fmt.Fprintf(r.out, "引擎运行出错: %v\n", err)
			}
		}

	}()

	return nil

}

func (r *REPL) Interrupt() {
	r.mu.Lock()
	task := r.active
	r.mu.Unlock()

	if task == nil {
		return
	}

	fmt.Fprintln(r.out, "\n正在取消当前任务...")
	task.Cancel()
}

func NewREPL(reader *bufio.Reader, out io.Writer, rt Runtime, rep reporter.Reporter, approval *TerminalApprovalHandler) *REPL {
	return &REPL{
		reader:   reader,
		out:      out,
		runtime:  rt,
		reporter: rep,
		approval: approval,
	}
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "可用命令:")
	fmt.Fprintln(out, "  /help   查看帮助")
	fmt.Fprintln(out, "  /clear  清空当前会话")
	fmt.Fprintln(out, "  /exit   退出程序")
	fmt.Fprintln(out, "  /quit   退出程序")
}

func (r *REPL) isIdle() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active == nil
}

func parseApprovalDecision(input string) (approval.Decision, bool) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "y":
		return approval.AllowOnce, true
	case "a":
		return approval.AllowSession, true
	case "n", "":
		return approval.Deny, true
	default:
		return "", false
	}
}
