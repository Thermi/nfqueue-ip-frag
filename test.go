package main

import (
//	"encoding/binary"
	"fmt"
	"os"
	"os/signal"
	"flag"
	"github.com/Thermi/go-nfqueue"
)
var queueNum int

/*
func print_packets(qid uint16, pkt *nfqueue.Packet) {
	fmt.Println(pkt)
	pkt.Accept()
}
*/

func main() {
	flag.IntVar(&queueNum, "queue-num", 0, "The NFQUEQUE number")

    flag.Parse()
	var q = nfqueue.NewNFQueue(uint16(queueNum))
	
	defer q.Destroy()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill)
	packets := q.Process()
	fmt.Println("Queue: ", q)

LOOP:
	for {
		select {
		case pkt := <-packets:
			fmt.Println(pkt)
			pkt.Accept()
		case <-sig:
			break LOOP
		}

	}
}