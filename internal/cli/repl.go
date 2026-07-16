package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type REPL struct {
	in       io.Reader
	out      io.Writer
	engine   *engine.AgentEngine
	session  *ctxpkg.Session
	reporter engine.Reporter
}

func (r *REPL) Run(ctx context.Context) error {
	reader := bufio.NewReader(r.in)

	for {
		fmt.Fprint(r.out, "\nclaw>")

		line, err := reader.ReadString('\n')

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

		if err := r.engine.Run(ctx, r.session, r.reporter); err != nil {
			fmt.Fprintf(r.out, "引擎运行出错: %v\n", err)
		}
	}
}

func NewREPL(in io.Reader, out io.Writer, engine *engine.AgentEngine, session *ctxpkg.Session, reporter engine.Reporter) *REPL {
	return &REPL{
		in:       in,
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
