package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	nfqueue "github.com/Thermi/nfqueue-go/nfqueue"
	gopacket "github.com/google/gopacket"
	flag "github.com/ogier/pflag"	
)

type arguments struct {
	verbose bool
	concurrency uint64
	queueNumber uint64
	mtu uint64
}

// Set a static MTU for now. Later, this has to be dynamic and depending on the destination
var defaultMtu uint64 = 1300
var args arguments
/* This method receives packets via the "packetChannel" channel and handles them accordingly
 * (either allows them to pass or drops, gets the outgoing interface to the destination,
 * frags the packets and sends out the fragmentss via a raw socket to the destination)
 * 
 */
// func processPackets(packetChannel chan nfqueue.Packet) {
// 	var pkt nfqueue.Packet
// 	for {
// 		pkt <- packetChannel
// 		// Check the length
// 		if len(pkt) > args.mtu {
			
// 		}
// 	}
// }

func main() {
	// parse args
	flag.Uint64Var(&args.concurrency, "concurrency", 1, "The number of concurrent go routines to work on fragmenting packets")
	flag.Uint64Var(&args.queueNumber, "queueNumber", 1, "The nfqueue number that this application should use")
	flag.Uint64Var(&args.mtu, "mtu", defaultMtu, "The maximum size of the IP packets. Bigger ones are fragmented.")
	flag.BoolVar(&args.verbose, "verbose", false, "Enable or disable verbose mode")

	flag.Parse()

}