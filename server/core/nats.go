/*
 * Copyright 2019 The NATS Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package core

import (
	"fmt"
	"strings"
	"time"

	nats "github.com/nats-io/go-nats"
	"github.com/nats-io/jwt"
)

const (
	accountNotificationFormat = "$SYS.ACCOUNT.%s.CLAIMS.UPDATE"
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
	if !server.running {
		return nil // already stopped
	}

	config := server.config.NATS

	if len(config.Servers) == 0 {
		server.logger.Noticef("NATS is not configured, server will not fire notifications on update")
		return nil
	}

	server.logger.Noticef("connecting to NATS for notifications")

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

	if config.UserCredentials != "" {
		options = append(options, nats.UserCredentials(config.UserCredentials))
	}

	nc, err := nats.Connect(strings.Join(config.Servers, ","),
		options...,
	)

	if err != nil {
		reconnectWait := config.ReconnectWait
		server.logger.Errorf("failed to connect to NATS, %v", err)
		server.logger.Errorf("will try to connect again in %d milliseconds", reconnectWait)
		server.natsTimer = time.NewTimer(time.Duration(reconnectWait) * time.Millisecond)
		go func() {
			<-server.natsTimer.C
			server.Lock()
			server.connectToNATS()
			server.Unlock()
		}()
		return nil // we will retry, don't stop server running
	}

	if server.primary != "" {
		subject := strings.Replace(accountNotificationFormat, "%s", "*", -1)
		nc.Subscribe(subject, server.handleAccountNotification)
	}

	server.nats = nc
	return nil
}

func (server *AccountServer) getNatsConnection() *nats.Conn {
	server.Lock()
	defer server.Unlock()
	conn := server.nats
	return conn
}

func (server *AccountServer) sendAccountNotification(claim *jwt.AccountClaims, theJWT []byte) error {
	pubKey := claim.Subject

	if server.nats == nil {
		server.logger.Noticef("skipping notification for %s, no NATS configured", ShortKey(pubKey))
		return nil
	}

	subject := fmt.Sprintf(accountNotificationFormat, pubKey)
	return server.nats.Publish(subject, theJWT)
}

func (server *AccountServer) handleAccountNotification(msg *nats.Msg) {
	jwtBytes := msg.Data
	theJWT := string(jwtBytes)
	claim, err := jwt.DecodeAccountClaims(theJWT)

	if err != nil || claim == nil {
		return
	}

	pubKey := claim.Subject
	err = server.jwtStore.Save(pubKey, theJWT)
	if err != nil {
		return
	}

	// Default cache time is 1 hour (see cacheControl)
	server.cacheLock.Lock()
	server.validUntil[pubKey] = time.Now().Add(time.Hour)
	server.cacheLock.Unlock()
}
