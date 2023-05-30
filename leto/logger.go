package main

import (
	"io"
	"log"
	"os"
)

var logOut io.Writer = os.Stderr

func NewLogger(domain string) *log.Logger {
	return log.New(logOut, "["+domain+"] ", 0)
}
