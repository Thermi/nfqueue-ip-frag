package main

import (
//	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"sync"
	"time"
	nfqueue "github.com/Thermi/nfqueue-go/nfqueue"
	gopacket "github.com/google/gopacket"
	"github.com/google/gopacket/layers"
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


var packetChannel chan *nfqueue.Payload
var needMoreWorkers chan bool

/* This method receives packets via the "packetChannel" channel and handles them accordingly
 * (either allows them to pass or drops, gets the outgoing interface to the destination,
 * frags the packets and sends out the fragmentss via a raw socket to the destination)
 * 
 */

/* Architecture of this application:
 * main -> wait for go funcs to exit
 *      -> go func receivePackets
 *			-> Check length, packets that are too long go into a channel
 *			-> packets that are shorter than the MTU are accepted right away
 			-> if the write to the channel blocks more than 10 ms, tell main to create more workers
 			-> by default, start $NumCPU workers and bind them to the CPUs.
 *		-> go func processPackets
 *			-> receive packets via channel, modify payload, set more frags bit in header, return verdict
 *			-> fragments are sent out via raw IP socket
 *
 *
 *
 */

func receivePackets(payload *nfqueue.Payload) error {

	/* Check the length of the IP packet */
	if len(payload.Data) > int(args.mtu) {
		duration, _ := time.ParseDuration("10ms")
		timer := time.NewTimer(duration)
    	select {
    	case packetChannel <- payload:
    		break
		case <- timer.C:
			select {
			case needMoreWorkers <- true:
				break
			default:
    			break
    		}
    	}
	} else {
		payload.SetVerdict(nfqueue.NF_ACCEPT)
	}
 	return nil
}


func processPackets() {
	var packet *nfqueue.Payload
	var newPacket []byte
	for {
		packet = <- packetChannel
		if packet == nil {
			return
		}
		var ipv4 layers.IPv4

		// Decode the original packet and take its IP header

		parser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &ipv4)
		decoded := []gopacket.LayerType{}
		
		err := parser.DecodeLayers(packet.Data, &decoded)
		if err != nil {
			fmt.Println("Could not decode packet.")
			continue
		}
		data := ipv4.Payload
		// Set more fragments
		// Check if don't fragment is set:
		if ipv4.Flags & layers.IPv4DontFragment > 0 {
			// Accept the packet then and continue in the next loop
			packet.SetVerdict(nfqueue.NF_ACCEPT)
			continue
		}

		// Otherwise, we set the "more fragments" bit and then start fraggin'
		ipv4.Flags &= layers.IPv4MoreFragments

		// Calculate the header size and how much Data is allowed to be in this packet
		headerLength := len(ipv4.Contents)
		
		// This is the length of payload section of the new original IP packet
		var newLength uint64 = args.mtu - uint64(headerLength)
		// Split payload at the MTU
		// set length of the IP packet

		ipv4.Length = uint16(newLength)

		layerBuffer := gopacket.NewSerializeBuffer()
		// We need to write the payload first
		bytes, _ := layerBuffer.PrependBytes(int(newLength))
		copy(bytes, ipv4.Payload[:newLength-1])
		_  = ipv4.SerializeTo(layerBuffer, gopacket.SerializeOptions{})
		

		// Set verdict on the original packet, but with the new data.
		// Pass pointer to new buffer here
		err = packet.SetVerdictModified(nfqueue.NF_ACCEPT, layerBuffer.Bytes())

		if err != nil {
			fmt.Println("An error occured setting the verdict for packet ", ipv4.Id, " - ", err)
		}


		// The rest length of the IP packet we need to 
		var restLength uint64 = uint64(len(data)) - newLength


		// calculate the number of other fragments we need to send


		// Set verdict on the original packet, but with the new data.
		// Pass pointer to new buffer here
		//nfqueue.SetVerdictModified(nfqueue.NF_ACCEPT, newPacket)

		// Calculate how many fragments we actually have to make
		// Create the corpus for a new IP packet
		// Set fragment offset and length
		// gopacket.NewPacket(...)
		//
		// gopacket.SerializePacket(...)
		
		//
		// func (p *Payload) SetVerdictModified(verdict int, data []byte) error {
		// Write fragments to raw socket
	}

}

func main() {
	// parse args
	var err error
	var i uint64
	flag.Uint64Var(&args.concurrency, "concurrency", 1, "The number of concurrent go routines to work on fragmenting packets")
	flag.Uint64Var(&args.mtu, "mtu", defaultMtu, "The maximum size of the IP packets. Bigger ones are fragmented.")
	flag.Uint16Var(&args.queueNumber, "queueNumber", 0, "The nfqueue number that this application should use")
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

	for i = 0; i < args.concurrency; i++ {
		go processPackets()
	}
	go func() {
		for {
			select {
				case <- needMoreWorkers:
					fmt.Println("receivePackets indicated we need more workers, chan blocked for more than one 1 ms")
					go processPackets()
					break
				case <- sig:
					break
					break
			}
		}
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