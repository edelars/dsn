package config

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
)

func InitZerolog(programName, logLevel string) zerolog.Logger {
	zerolog.MessageFieldName = "MESSAGE"
	zerolog.LevelFieldName = "LEVEL"
	zerolog.ErrorFieldName = "ERROR"
	zerolog.TimestampFieldName = "TIME"
	zerolog.CallerFieldName = "CALLER"

	l, err := zerolog.ParseLevel(strings.ToLower(logLevel))
	if err != nil || l == zerolog.NoLevel {
		l = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(l)

	return zerolog.New(os.Stderr).With().Str("PROGRAM", programName).Timestamp().Caller().Logger()
}
