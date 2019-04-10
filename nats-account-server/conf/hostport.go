package conf

import (
	"fmt"
	"net"
)

//HostPort stores a host port pair
type HostPort struct {
	Host string
	Port int
}

// String returns the joined pair
func (hp *HostPort) String() string {
	return net.JoinHostPort(hp.Host, fmt.Sprintf("%d", hp.Port))
}
