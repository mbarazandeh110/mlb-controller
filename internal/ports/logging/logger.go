package logging

// Field represents a key-value pair for structured logging.
type Field struct {
	Key   string
	Value interface{}
}

// Logger defines the interface for structured logging (Port).
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	With(fields ...Field) Logger
	Sync() error
}
