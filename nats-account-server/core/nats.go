package core

import (
	"strings"
	"time"

	nats "github.com/nats-io/go-nats"
)

func (server *AccountServer) natsError(nc *nats.Conn, sub *nats.Subscription, err error) {
	server.logger.Warnf("nats error %s", err.Error())
}

func (server *AccountServer) natsDisconnected(nc *nats.Conn) {
	if !server.checkRunning() {
		return
	}
	server.logger.Warnf("nats disconnected")
}

func (server *AccountServer) natsReconnected(nc *nats.Conn) {
	server.logger.Warnf("nats reconnected")
}

func (server *AccountServer) natsClosed(nc *nats.Conn) {
	if server.checkRunning() {
		server.logger.Errorf("nats connection closed, shutting down bridge")
		go server.Stop()
	}
}

func (server *AccountServer) natsDiscoveredServers(nc *nats.Conn) {
	server.logger.Debugf("discovered servers: %v\n", nc.DiscoveredServers())
	server.logger.Debugf("known servers: %v\n", nc.Servers())
}

// assumes the lock is held by the caller
func (server *AccountServer) connectToNATS() error {
	server.runningLock.Lock()
	defer server.runningLock.Unlock()

	if !server.running {
		return nil // already stopped
	}

	server.logger.Noticef("connecting to NATS for notifications")

	config := server.config.NATS
	options := []nats.Option{nats.MaxReconnects(config.MaxReconnects),
		nats.ReconnectWait(time.Duration(config.ReconnectWait) * time.Millisecond),
		nats.Timeout(time.Duration(config.ConnectTimeout) * time.Millisecond),
		nats.ErrorHandler(server.natsError),
		nats.DiscoveredServersHandler(server.natsDiscoveredServers),
		nats.DisconnectHandler(server.natsDisconnected),
		nats.ReconnectHandler(server.natsReconnected),
		nats.ClosedHandler(server.natsClosed),
	}

	if config.TLS.Root != "" {
		options = append(options, nats.RootCAs(config.TLS.Root))
	}

	if config.TLS.Cert != "" {
		options = append(options, nats.ClientCert(config.TLS.Cert, config.TLS.Key))
	}

	nc, err := nats.Connect(strings.Join(config.Servers, ","),
		options...,
	)

	if err != nil {
		return err
	}

	server.nats = nc
	return nil
}
