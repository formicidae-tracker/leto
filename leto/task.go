package main

type Task interface {
	Run() error
}

func Start(t Task) <-chan error {
	err := make(chan error)
	go func() {
		defer close(err)
		err <- t.Run()
	}()
	return err
}
