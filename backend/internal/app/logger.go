package app

import (
	"backend/internal/config"
	"backend/internal/logging"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger(cfg config.Config) (*zap.Logger, error) {
	var l *zap.Logger
	var err error

	if cfg.Logger.Development {
		l, err = zap.NewDevelopment()
	} else {
		l, err = zap.NewProduction()
	}
	if err != nil {
		return nil, err
	}

	if cfg.Sentry.DSN != "" {
		sentryCore := logging.NewSentryCore(zapcore.WarnLevel)
		l = l.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(core, sentryCore)
		}))
	}

	zap.ReplaceGlobals(l)
	return l, nil
}
