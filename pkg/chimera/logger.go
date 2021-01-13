package chimera

import (
	"fmt"
	"log"
)

type Logger interface {
	Debug(msg string)
	Debugf(format string, args ...interface{})
	Info(msg string)
	Infof(format string, args ...interface{})
	Error(msg string)
	Errorf(format string, args ...interface{})
}

type simpleLogger struct{}

func (*simpleLogger) Debug(msg string) {
	log.Printf("[DEBUG] %s", msg)
}

func (*simpleLogger) Debugf(format string, args ...interface{}) {
	msg := fmt.Sprintf("[DEBUG] %s", format)
	log.Printf(msg, args...)
}

func (*simpleLogger) Info(msg string) {
	log.Printf("[INFO] %s", msg)
}

func (*simpleLogger) Infof(format string, args ...interface{}) {
	msg := fmt.Sprintf("[INFO] %s", format)
	log.Printf(msg, args...)
}
func (*simpleLogger) Error(msg string) {
	log.Printf("[ERROR] %s", msg)
}

func (*simpleLogger) Errorf(format string, args ...interface{}) {
	msg := fmt.Sprintf("[ERROR] %s", format)
	log.Printf(msg, args...)
}
