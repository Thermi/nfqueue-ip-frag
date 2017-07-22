package main

import (
	"fmt"
	gopacket "github.com/google/gopacket"
	flag "github.com/ogier/pflag"
	nfqueue "github.com/Thermi/go-nfqueue"
)

type arguments struct {
	verbose bool
	concurrency uint64
	queueNumber uint64
}

func main() {
	// parse args
	var args arguments
	flag.Uint64Var(&args.concurrency, "concurrency", 1, "The number of concurrent go routines to work on fragmenting packets")
	flag.Uint64Var(&args.queueNumber, "queueNumber", 1, "The nfqueue number that this application should use")
	flag.BoolVar(&args.verbose, "verbose", false, "Enable or disable verbose mode")

	flag.Parse()

}