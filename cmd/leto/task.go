package main

type Task interface {
	Run() error
}

func Start(t Task) <-chan error {
	return StartFunc(t.Run)
}

func StartFunc(f func() error) <-chan error {
	err := make(chan error)
	go func() {
		defer close(err)
		err <- f()
	}()
	return err
}
