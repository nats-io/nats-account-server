package logging

// Config defines logging flags for the NATS logger
type Config struct {
	Time   bool
	Debug  bool
	Trace  bool
	Colors bool
	PID    bool
}

// Logger interface
type Logger interface {
	Debugf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
	Fatalf(format string, v ...interface{})
	Noticef(format string, v ...interface{})
	Tracef(format string, v ...interface{})
	Warnf(format string, v ...interface{})

	Close() error
}
