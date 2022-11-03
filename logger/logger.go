package logger

import (
	"fmt"
	"io"
	"log"
	"os"
)

type Logger struct {
	file io.Writer
	*log.Logger
}

func New(filepath string) (*Logger, error) {
    file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}

	simpleLogger := log.New(file, "", log.Ldate|log.Ltime|log.Lshortfile)
	return &Logger{ file: file, Logger: simpleLogger }, nil
}


// log.New(file, "", log.Ldate|log.Ltime|log.Lshortfile)
