package leto

type Task interface {
	Run() error
}

func StartTask(t Task) <-chan error {
	return StartTaskFunc(t.Run)
}

func StartTaskFunc(f func() error) <-chan error {
	err := make(chan error)
	go func() {
		defer close(err)
		err <- f()
	}()
	return err
}
