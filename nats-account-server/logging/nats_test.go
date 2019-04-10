package logging

import (
	"testing"
)

func TestNATSForCoverage(t *testing.T) {
	logger := NewNATSLogger(Config{})
	logger.Debugf("test")
	logger.Tracef("test")
	logger.Noticef("test")
	logger.Errorf("test")
	logger.Warnf("test")
	// skip fatal
	logger.Close()
}
