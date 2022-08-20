package main

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var loggerConfig zap.Config

func init() {
	loggerConfig = zap.NewProductionConfig()
	loggerConfig.EncoderConfig.CallerKey = ""
	loggerConfig.EncoderConfig.StacktraceKey = ""
	SetLogLevel("info")
}

func Logger() *zap.SugaredLogger {
	return zap.L().Sugar()
}

// SetLogLevel sets LogLevel, returns undo function and error.
func SetLogLevel(logLevel string) (func(), error) {
	level, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %w", err)
	}

	loggerConfig.Level.SetLevel(level)
	logger, err := loggerConfig.Build()
	if err != nil {
		return nil, err
	}

	undo := zap.ReplaceGlobals(logger)
	return undo, nil
}
