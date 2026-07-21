package runtime

import (
	"errors"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
)

type RuntimeOptions struct {
	ApprovalHandler approval.Handler
	Reporter        reporter.Reporter
}

func (o RuntimeOptions) validate() error {
	if o.ApprovalHandler == nil {
		return errors.New("审批处理器不能为空")
	}

	if o.Reporter == nil {
		return errors.New("事件报告器不能为空")
	}

	return nil
}
