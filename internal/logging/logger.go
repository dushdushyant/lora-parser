package logging

import (
	"io"
	"os"
	"strings"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

type closer func()

func NewLogger(filePath, level string, maxSizeMB, maxBackups, maxAgeDays int) (zerolog.Logger, closer, error) {
	if level == "" {
		level = "info"
	}
	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	if filePath != "" {
		_ = os.MkdirAll(filepath.Dir(filePath), 0o755)
	}
	lj := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
	}
	mw := io.MultiWriter(os.Stdout, lj)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	base := zerolog.New(mw).Level(lvl).With().Timestamp().Caller().Logger()
	log.Logger = base
	return base, func() { _ = lj.Close() }, nil
}
