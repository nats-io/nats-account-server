/*
 * Copyright 2012-2019 The NATS Authors
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
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/nats-io/account-server/nats-account-server/conf"
	"github.com/rs/cors"
)

func (server *AccountServer) startHTTP() error {
	var err error

	conf := server.config.HTTP

	err = server.createHTTPListener()
	if err != nil {
		server.logger.Errorf("error creating listener: %v", err)
		return err
	}

	router, err := server.buildRouter()
	if err != nil {
		server.FatalError("error creating the router: %v", err)
	}

	xrs := cors.New(cors.Options{
		AllowOriginFunc: func(orig string) bool {
			return true
		},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Authorization"},
		AllowCredentials: false,
	})

	httpServer := &http.Server{
		Handler:      xrs.Handler(router),
		ReadTimeout:  time.Duration(conf.ReadTimeout) * time.Millisecond,
		WriteTimeout: time.Duration(conf.WriteTimeout) * time.Millisecond,
	}

	server.http = httpServer

	go func() {
		if err := server.http.Serve(server.listener); err != nil {
			if err != http.ErrServerClosed {
				server.logger.Errorf("error attempting to serve requests: %v", err)
				go server.Stop()
			}
		}
		server.http.Handler = nil
	}()

	server.logger.Noticef("%s listening on port %d\n", server.protocol, server.port)

	return nil
}

func (server *AccountServer) stopHTTP() {
	if server.http != nil {
		server.logger.Noticef("stopping http server")
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
		defer cancel()

		if err := server.http.Shutdown(ctx); err != nil {
			server.logger.Errorf("error closing http server: %v", err)
		} else {
			server.logger.Noticef("http server stopped")
			server.listener = nil
		}
	}

	if server.listener != nil {
		if err := server.listener.Close(); err != nil {
			server.logger.Errorf("error closing listener: %v", err)
		}
		server.listener = nil
	}
	server.logger.Noticef("http stopped")
}

func (server *AccountServer) createHTTPListener() error {
	var listen net.Listener

	conf := server.config.HTTP
	hp := conf.HTTP.String()
	tlsConf := conf.TLS

	if tlsConf.Cert == "" {
		listen, err := net.Listen("tcp", hp)
		if err != nil {
			return err
		}
		server.protocol = "http"
		server.port = listen.Addr().(*net.TCPAddr).Port
		server.hostPort = hp
		if hp == "" || strings.HasPrefix(hp, ":") {
			server.hostPort = fmt.Sprintf("127.0.0.1:%d", server.port)
		}
		server.listener = listen
		return nil
	}

	config, err := server.makeTLSConfig(tlsConf)
	if err != nil {
		return err
	}

	listen, err = tls.Listen("tcp", hp, config)
	if err != nil {
		return err
	}

	server.protocol = "https"
	server.port = listen.Addr().(*net.TCPAddr).Port
	server.hostPort = hp
	if hp == "" || strings.HasPrefix(hp, ":") {
		server.hostPort = fmt.Sprintf("127.0.0.1:%d", server.port)
	}
	server.listener = listen
	return nil
}

func (server *AccountServer) makeTLSConfig(tlsConf conf.TLSConf) (*tls.Config, error) {
	if tlsConf.Cert == "" || tlsConf.Key == "" {
		server.logger.Noticef("TLS is not configured")
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(tlsConf.Cert, tlsConf.Key)
	if err != nil {
		return nil, fmt.Errorf("error loading X509 certificate/key pair: %v", err)
	}
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("error parsing certificate: %v", err)
	}
	config := tls.Config{
		Certificates:             []tls.Certificate{cert},
		MinVersion:               tls.VersionTLS12,
		ClientAuth:               tls.NoClientCert,
		PreferServerCipherSuites: true,
	}
	return &config, nil
}

// BuildRouter creates the http.Router for the NGS server
func (server *AccountServer) buildRouter() (*httprouter.Router, error) {
	r := httprouter.New()

	r.GET("/jwt/v1/help", server.JWTHelp)

	if !server.jwtStore.IsReadOnly() {
		r.POST("/jwt/v1/accounts/:pubkey", server.UpdateAccountJWT)
	}

	r.GET("/jwt/v1/accounts/:pubkey", server.GetAccountJWT)
	r.GET("/jwt/v1/accounts/", server.GetAccountJWT) // Server test point
	r.GET("/jwt/v1/accounts", server.GetAccountJWT)  // Server test point

	return r, nil
}
