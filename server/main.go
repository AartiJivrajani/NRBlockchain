package main

import (
	"NRBlockchain/server/blockchain"
	"context"
	"flag"
	log "github.com/Sirupsen/logrus"
	"os"
	"os/signal"
	"strings"
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
The function of this process is 2 fold -
1. Maintain a blockchain (linked list DS in order to store the transactions)
2. Start a TCP/UDP Listener in order to receive/process the events which are sent by the clients
*/
func main() {

	var (
		logLevel   string
		portNumber int
	)
	ctx, cancel := context.WithCancel(context.Background())
	// parse all the command line arguments
	flag.StringVar(&logLevel, "level", "DEBUG", "Set log level.")
	flag.IntVar(&portNumber, "port_number", 8003, "port on which the client needs to start")
	flag.Parse()

	configureLogger(logLevel)

	blockchain.Server = blockchain.CreateServer(ctx, portNumber)
	blockchain.BlockChainInit(ctx)
	go blockchain.Server.Start(ctx)

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
