package main

import (
//	"encoding/binary"
    "encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"flag"
	"syscall"
	"github.com/Thermi/nfqueue-go/nfqueue"
)

var queueNum int

func real_callback(payload *nfqueue.Payload) int {
    fmt.Println("Real callback")
    fmt.Printf("  id: %d\n", payload.Id)
    fmt.Printf("  mark: %d\n", payload.GetNFMark())
    fmt.Printf("  in  %d      out  %d\n", payload.GetInDev(), payload.GetOutDev())
    fmt.Printf("  Φin %d      Φout %d\n", payload.GetPhysInDev(), payload.GetPhysOutDev())
    fmt.Println(hex.Dump(payload.Data))
    fmt.Println("-- ")
    payload.SetVerdict(nfqueue.NF_ACCEPT)
    return 0
}

func main() {
	flag.IntVar(&queueNum, "queue-num", 0, "The NFQUEQUE number")

    flag.Parse()
	
	var q = new(nfqueue.Queue)

	q.SetCallback(real_callback)

	q.Init()
	defer q.Close()
	
	q.Unbind(syscall.AF_INET)
	q.Bind(syscall.AF_INET)

	q.CreateQueue(queueNum)
	q.SetMode(nfqueue.NFQNL_COPY_PACKET)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill)

	go func() {
        for sig := range sig {
            // sig sigint or sigkill
            _ = sig
            q.StopLoop()
		}
	}()

	q.Loop()
    q.DestroyQueue()
    q.Close()
    os.Exit(0)
}