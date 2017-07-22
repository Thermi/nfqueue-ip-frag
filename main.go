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

}