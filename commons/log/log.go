package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

var Logger zerolog.Logger

func Init(dir, name string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(dir, name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()
	Logger = zerolog.New(io.MultiWriter(os.Stdout, file)).With().Timestamp().Logger()
	return nil
}
