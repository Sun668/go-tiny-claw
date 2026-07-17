package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type REPL struct {
	reader   *bufio.Reader
	out      io.Writer
	engine   *engine.AgentEngine
	session  *ctxpkg.Session
	reporter engine.Reporter
}

func (r *REPL) Run(ctx context.Context) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	for {
		fmt.Fprint(r.out, "\nclaw>")

		line, err := r.reader.ReadString('\n')

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		prompt := strings.TrimSpace(line)

		if prompt == "" {
			continue
		}

		switch prompt {
		case "/exit", "/quit":
			return nil
		case "/clear":
			r.session.Clear()
			fmt.Fprintln(r.out, "会话已清空。")
			continue
		case "/help":
			printHelp(r.out)
			continue
		}

		r.session.Append(schema.Message{
			Role:    schema.RoleUser,
			Content: prompt,
		})

		if err := r.runTurn(ctx, signals); err != nil {
			fmt.Fprintf(r.out, "引擎运行出错: %v\n", err)
		}
	}
}

func (r *REPL) runTurn(
	parent context.Context,
	signals <-chan os.Signal,
) error {
	runCtx, cancel := context.WithCancel(parent)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- r.engine.Run(
			runCtx,
			r.session,
			r.reporter,
		)
	}()

	select {
	case err := <-done:
		return err

	case <-signals:
		fmt.Fprintln(r.out, "\n正在取消当前任务...")
		cancel()

		err := <-done
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(r.out, "当前任务已取消。")
			return nil
		}

		return err
	}
}

func NewREPL(reader *bufio.Reader, out io.Writer, engine *engine.AgentEngine, session *ctxpkg.Session, reporter engine.Reporter) *REPL {
	return &REPL{
		reader:   reader,
		out:      out,
		engine:   engine,
		session:  session,
		reporter: reporter,
	}
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "可用命令:")
	fmt.Fprintln(out, "  /help   查看帮助")
	fmt.Fprintln(out, "  /clear  清空当前会话")
	fmt.Fprintln(out, "  /exit   退出程序")
	fmt.Fprintln(out, "  /quit   退出程序")
}
