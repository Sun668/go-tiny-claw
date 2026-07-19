package runtime

import "context"

type Task struct {
	done   <-chan error
	cancel context.CancelFunc
}

func (t *Task) Done() <-chan error {
	return t.done
}

func (t *Task) Cancel() {
	t.cancel()
}
