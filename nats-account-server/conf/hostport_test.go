package conf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostPort(t *testing.T) {
	hp := HostPort{
		Host: "localhost",
		Port: 4222,
	}
	require.Equal(t, "localhost:4222", hp.String())
}
