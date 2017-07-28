package main

import (
	"container/list"
//	"errors"
	"fmt"
	"math"
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

type rawSocket struct {
	fd 			int
	lock 		*sync.Mutex

}
var wg sync.WaitGroup
var args arguments
var socket rawSocket

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
	var ipv4 layers.IPv4

	listOfFragments := list.New()
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &ipv4)

	for {
		packet = <- packetChannel
		if packet == nil {
			return
		}


		// Decode the original packet and take its IP header

		decoded := []gopacket.LayerType{}
		
		err := parser.DecodeLayers(packet.Data, &decoded)
		if err != nil {
			fmt.Println("Could not decode packet.")
			continue
		}

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
		var firstFragmentPayloadLength uint64 = args.mtu - uint64(headerLength)
		// Split payload at the MTU
		// set length of the IP packet

		ipv4.Length = uint16(firstFragmentPayloadLength)

		layerBuffer := gopacket.NewSerializeBuffer()
		// We need to write the payload first
		bytes, _ := layerBuffer.PrependBytes(int(firstFragmentPayloadLength))

		// Let's hope the offset of one is correct
		copy(bytes, ipv4.Payload[:firstFragmentPayloadLength-1])
		_  = ipv4.SerializeTo(layerBuffer, gopacket.SerializeOptions{ComputeChecksums : true})
		

		// Set verdict on the original packet, but with the new data.
		// Pass pointer to new buffer here
		err = packet.SetVerdictModified(nfqueue.NF_ACCEPT, layerBuffer.Bytes())

		if err != nil {
			fmt.Println("An error occured setting the verdict for packet ", ipv4.Id, " - ", err)
		}

		// The rest length of the IP packet we need to put into fragments
		var alreadySentBytes uint64 = firstFragmentPayloadLength

		numberOfFragments := int(math.Ceil(float64(len(ipv4.Payload)) / float64(args.mtu)))

		var newPayloadLength uint64
		// calculate the number of other fragments we need to send
		for i := 0; i< numberOfFragments; i++ {
			// clear the layerBuffer
			layerBuffer.Clear()
			// clear the slice
			bytes = nil
			
			ipv4.FragOffset = uint16(alreadySentBytes)
			// If this is the last fragment, we need to set some special bits
			if (i + 1 == numberOfFragments) {
				newPayloadLength = uint64(len(ipv4.Payload)) - alreadySentBytes
				// set no more fragments
				ipv4.Flags ^= layers.IPv4MoreFragments
			} else {
				// This is an intermediate fragment, in which we can put the maximum amount of bytes
				// into the packet up until the mtu is reached.
				newPayloadLength = firstFragmentPayloadLength

			}
			bytes = make([]byte, newPayloadLength)
			
			copy(bytes, ipv4.Payload[alreadySentBytes:newPayloadLength])

			alreadySentBytes += firstFragmentPayloadLength

			_  = ipv4.SerializeTo(layerBuffer, gopacket.SerializeOptions{ComputeChecksums : true})
			
			listOfFragments.PushBack(layerBuffer.Bytes())
		}

		// Write fragments to raw socket
		// https://www.darkcoding.net/software/raw-sockets-in-go-link-layer/
		// man 7 raw
		
		err = sendPackets(listOfFragments)
		// Clear the list
		listOfFragments.Init()
	}

}

func sendPackets(packetList *list.List) error {
	var packet		[]byte
	var err 		error
	addr := syscall.SockaddrInet4{
		Port: 0,
		Addr: [4]byte{127, 0, 0, 1},
	}

	socket.lock.Lock()
	defer socket.lock.Unlock()

	for e := packetList.Front(); e != nil; e = e.Next() {
		// destination is irrelevant here, because the socket takes over
		// the whole IP header of our packet in the byte array
		packet = e.Value.([]byte)
		err = syscall.Sendto(socket.fd, packet, 0, &addr)
		if err != nil {
			fmt.Println("An error occured when sending a packet: ", err)
			return err
		}
	}

	return nil
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

	// open the raw socket
	// We only want to send!
	socket.fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)

	if err != nil {
		fmt.Println("An error occurred when the socket was opened: ", err)
		os.Exit(1)
	}

	// We know better. :)
	err = syscall.SetsockoptInt(socket.fd, syscall.SOL_SOCKET, syscall.IP_MTU_DISCOVER, 0)
	if err != nil {
		fmt.Println("An error occurred when the socket option IP_MTU_DISCOVER was set: ", err)
		os.Exit(1)
	}

	socket.lock = new(sync.Mutex)

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