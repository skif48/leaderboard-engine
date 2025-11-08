package graceful_shutdown

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var inputsShutdownFuncs []func()
var outputShutdownFuncs []func()

func init() {
	inputsShutdownFuncs = make([]func(), 0)
	outputShutdownFuncs = make([]func(), 0)
}

func AddInputShutdownFunc(f func()) {
	inputsShutdownFuncs = append(inputsShutdownFuncs, f)
}

func AddOutputShutdownFunc(f func()) {
	outputShutdownFuncs = append(outputShutdownFuncs, f)
}

func WaitForSignals() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	<-signalChan

	slog.Info("Received shutdown signal, shutting down...")

	for _, f := range inputsShutdownFuncs {
		f()
	}

	time.Sleep(10 * time.Second)

	for _, f := range outputShutdownFuncs {
		f()
	}
}
