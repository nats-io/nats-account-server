/*
 * Copyright 2019-2020 The NATS Authors
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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/mitchellh/go-homedir"
	"github.com/nats-io/nats-account-server/server/core"
)

func expandPath(p string) string {
	var err error
	if p != "" {
		p, err = homedir.Expand(p)
		if err != nil {
			panic(fmt.Sprintf("error parsing path: %s", p))
		}
		p, err = filepath.Abs(p)
		if err != nil {
			panic(fmt.Sprintf("error resolving path: %s", p))
		}
	}
	return p
}

func main() {
	var server *core.AccountServer
	var err error

	flags := core.Flags{}
	flag.StringVar(&flags.ConfigFile, "c", "", "configuration filepath, other flags take precedent over the config file")
	flag.StringVar(&flags.NSCFolder, "nsc", "", "the nsc folder to host accounts from, mutually exclusive from dir, and makes the server read-only")
	flag.StringVar(&flags.Directory, "dir", "", "the directory to store/host accounts with, mututally exclusive from nsc")
	flag.StringVar(&flags.NATSURL, "nats", "", "the NATS server to use for notifications, the default is no notifications")
	flag.StringVar(&flags.Creds, "creds", "", "the creds file for connecting to NATS")
	flag.StringVar(&flags.Primary, "primary", "", "the URL for the primary server, in the form http(s)://host:port/")
	flag.BoolVar(&flags.Debug, "D", false, "turn on debug logging")
	flag.BoolVar(&flags.Verbose, "V", false, "turn on verbose logging")
	flag.BoolVar(&flags.DebugAndVerbose, "DV", false, "turn on debug and verbose logging")
	flag.StringVar(&flags.HostPort, "hp", "", "http hostport, defaults to localhost:9090")
	flag.BoolVar(&flags.ReadOnly, "ro", false, "exclusive to -dir flag, makes the server run in read-only mode, file changes will trigger nats updates (if configured)")
	flag.Parse()

	// resolve paths with dots/tildes
	flags.ConfigFile = expandPath(flags.ConfigFile)
	flags.Creds = expandPath(flags.Creds)
	flags.Directory = expandPath(flags.Directory)
	flags.NSCFolder = expandPath(flags.NSCFolder)

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGHUP)

		for {
			signal := <-sigChan

			if signal == os.Interrupt {
				if server.Logger() != nil {
					fmt.Println() // clear the line for the control-C
					server.Logger().Noticef("received sig-interrupt, shutting down")
				}
				server.Stop()
				os.Exit(0)
			}

			if signal == syscall.SIGHUP {
				if server.Logger() != nil {
					server.Logger().Errorf("received sig-hup, restarting")
				}
				server.Stop()
				server := core.NewAccountServer()
				err = server.InitializeFromFlags(flags)

				if err != nil {
					if server.Logger() != nil {
						server.Logger().Errorf("%s", err.Error())
					} else {
						log.Printf("%s", err.Error())
					}
					server.Stop()
					os.Exit(1)
				}

				err = server.Start()

				if err != nil {
					if server.Logger() != nil {
						server.Logger().Errorf("%s", err.Error())
					} else {
						log.Printf("%s", err.Error())
					}
					server.Stop()
					os.Exit(1)
				}
			}
		}
	}()

	server = core.NewAccountServer()
	err = server.InitializeFromFlags(flags)

	if err != nil {
		if server.Logger() != nil {
			server.Logger().Errorf("%s", err.Error())
		} else {
			log.Printf("%s", err.Error())
		}
		server.Stop()
		os.Exit(1)
	}

	if err := core.Run(server); err != nil {
		if server.Logger() != nil {
			server.Logger().Errorf("%s", err.Error())
		} else {
			log.Printf("%s", err.Error())
		}
		server.Stop()
		os.Exit(1)
	}

	// exit main but keep running goroutines
	runtime.Goexit()
}
