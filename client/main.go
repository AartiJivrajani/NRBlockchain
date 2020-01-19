package main

import (
	"NRBlockchain/client/consensus"
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"

	log "github.com/Sirupsen/logrus"
)

func configureLogger(level string) {
	log.SetOutput(os.Stderr)
	switch strings.ToLower(level) {
	case "panic":
		log.SetLevel(log.PanicLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warning", "warn":
		log.SetLevel(log.WarnLevel)
	}
}

/**
1. When the client first comes up, register the client with the other clients.
2. Do this by establishing a TCP/UDP connection with each of the clients.
3. Once done, sleep for a couple minutes.
*/
func main() {
	var (
		portNumber, clientId int
		logLevel             string
	)

	// parse all the command line arguments
	flag.StringVar(&logLevel, "level", "DEBUG", "Set log level.")
	flag.IntVar(&portNumber, "port_number", 8000, "port on which the client needs to start")
	flag.IntVar(&clientId, "client_id", 1, "client ID(1, 2, 3)")
	flag.Parse()

	// perform the pre-app kick start ceremonies
	ctx, cancel := context.WithCancel(context.Background())
	configureLogger(logLevel)

	consensus.Client = consensus.GetClient(ctx, clientId, portNumber)
	consensus.Client.Start(ctx)

	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		for _ = range signalChan {
			log.Info("Received an interrupt, stopping all connections...")
			cancel()
			cleanupDone <- true
		}
	}()
	<-cleanupDone
}
