package main

import (
	"github.com/sirupsen/logrus"
)

func NewLogger(domain string) *logrus.Entry {
	return logrus.WithField("group", domain)
}
