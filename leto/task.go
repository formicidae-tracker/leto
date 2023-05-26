package main

import "fmt"

type Task interface {
	// start the task asynchronously
	Start()
	// Wait termination of the task. Error can report a failure. If
	// the task is not Spawn() Wait will return nil
	Done() <-chan error
}

type functionTask struct {
	err      chan error
	function func() error
}

func NewFunctionTask(function func() error) Task {
	return &functionTask{
		function: function,
		err:      make(chan error),
	}
}

func (t *functionTask) Start() {
	go func() {
		defer close(t.err)
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			t.err <- fmt.Errorf("inner function panic: %+v", r)
		}()
		t.err <- t.function()
	}()
}

func (t *functionTask) Done() <-chan error {
	return t.err
}
