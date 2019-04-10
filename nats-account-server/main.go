package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/nats-io/account-server/nats-account-server/core"
)

var configFile string

func main() {
	var server *core.AccountServer
	var err error

	flag.StringVar(&configFile, "c", "", "configuration filepath")
	flag.Parse()

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
				server.LoadConfigFile(configFile)
				err = server.Start()

				if err != nil {
					if server.Logger() != nil {
						server.Logger().Errorf("error starting bridge, %s", err.Error())
					} else {
						log.Printf("error starting bridge, %s", err.Error())
					}
					server.Stop()
					os.Exit(0)
				}
			}
		}
	}()

	server = core.NewAccountServer()
	server.LoadConfigFile(configFile)
	err = server.Start()

	if err != nil {
		if server.Logger() != nil {
			server.Logger().Errorf("error starting bridge, %s", err.Error())
		} else {
			log.Printf("error starting bridge, %s", err.Error())
		}
		server.Stop()
		os.Exit(0)
	}

	// exit main but keep running goroutines
	runtime.Goexit()
}
