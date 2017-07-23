package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"sync"
	nfqueue "github.com/Thermi/nfqueue-go/nfqueue"
//	gopacket "github.com/google/gopacket"
	flag "github.com/ogier/pflag"	
)

type arguments struct {
	verbose bool
	concurrency uint64
	queueNumber uint16
	mtu uint64
}

var wg sync.WaitGroup
var args arguments

// Set a static MTU for now. Later, this has to be dynamic and depending on the destination
var defaultMtu uint64 = 1300


//var channel chan *nfqueue.Payload
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

/* Architecture of this application:
 * main -> wait for go funcs to exit
 *      -> go func receivePackets
 *			-> Check length, packets that are too long go into a channel
 *			-> packets that are shorter than the MTU are accepted right away
 *		-> go fund workers
 *			-> receive packets via channel, modify payload, set more frags bit in header, return verdict
 *			-> fragments are sent out via raw IP socket
 *
 * global channel for packets
 *
 *
 */
var buf *nfqueue.Payload

func receivePackets(payload *nfqueue.Payload) error {
 	if buf == nil {
 		fmt.Println("Received first packet")
 		buf = payload
 	} else {
 		fmt.Println("Setting first verdict.")
 		payload.SetVerdict(nfqueue.NF_ACCEPT)
 		fmt.Println("Setting second verdict.")
 		buf.SetVerdict(nfqueue.NF_ACCEPT)
 		fmt.Println("WaitGroup before wg.Done in callback: ", wg)
 		fmt.Println("Running wg.Done().")
 		fmt.Println("WaitGroup after wg.Done in callback: ", wg)
 		fmt.Println("Leaving the callback function finally.")
 		return errors.New("verdicted the two packets.")
 	}
 	fmt.Println("Leaving callback function")
 	return nil
}

func main() {
	// parse args
	var err error
	flag.Uint64Var(&args.concurrency, "concurrency", 1, "The number of concurrent go routines to work on fragmenting packets")
	flag.Uint16Var(&args.queueNumber, "queueNumber", 0, "The nfqueue number that this application should use")
	flag.Uint64Var(&args.mtu, "mtu", defaultMtu, "The maximum size of the IP packets. Bigger ones are fragmented.")
	flag.BoolVar(&args.verbose, "verbose", false, "Enable or disable verbose mode")

	flag.Parse()

	var q = new(nfqueue.Queue)
	// Implicitely starts one function, account for when starting q.Loop()
	q.SetCallback(receivePackets)

	q.Init()
	defer q.Close()

	q.Unbind(syscall.AF_INET)
	q.Bind(syscall.AF_INET)

	if err = q.CreateQueue(args.queueNumber); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	if err = q.SetMode(nfqueue.NFQNL_COPY_PACKET); err != nil {
		fmt.Print(err)
		os.Exit(2)		
	}
	if err = q.SetQueueFlags(nfqueue.NFQA_CFG_F_FAIL_OPEN, nfqueue.NFQA_CFG_F_FAIL_OPEN); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}

	if err = q.SetQueueMaxLen(10*1024*1024); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill)
	wg.Add(1)
	// starts one function
	// now we have two
	go func() {
        for sig := range sig {
            // sig sigint or sigkill
            _ = sig
            fmt.Println("Received signal")
            q.StopLoop()
            fmt.Println("Stopped loop")
            wg.Done()
            // wg -= 1 on exit
            fmt.Println("wg.Done() in sig loop")
            fmt.Println("WaitGroup in sigloop: ", wg)
            break
		}
		fmt.Println("Left sig loop")
	}()
	if err = q.Loop(); err != nil {
		fmt.Println("Error on exit of q.Loop():")
		fmt.Print(err)
		fmt.Println("WaitGroup: ", wg)
	}
	// And three 
	sig <- os.Interrupt
	wg.Wait()
    q.DestroyQueue()
    q.Close()
    os.Exit(0)
}