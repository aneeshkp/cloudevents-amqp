package socket

import (
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

//SocketFile ...
var SocketFile = "/tmp/go.sock"

func echoServer(wg *sync.WaitGroup, c net.Conn, msg types.PTPState) {
	defer wg.Done()
	for {
		buf := make([]byte, 512)
		nr, err := c.Read(buf)
		if err != nil {
			if err == io.EOF {
				log.Println("END OF FILE go socket")
				return
			}
			log.Println("error in trying to read data")
			return
		}

		data := buf[0:nr]
		log.Printf("Server got:  %s", string(data))
		_, err = c.Write([]byte(msg))
		if err != nil {
			log.Printf("Writing client error:%v ", err)
		}
	}
}

//SetUpPTPStatusServer ...
func SetUpPTPStatusServer(wg *sync.WaitGroup) {
	var count = 0
	const max int = 11
	response := [max]types.PTPState{
		types.LOCKED,
		types.FREERUN,
		types.HOLDOVER,
	}
	log.Println("Starting echo server")
	os.Remove(SocketFile)
	ln, err := net.Listen("unix", SocketFile)
	if err != nil {
		log.Fatal("Listen error: ", err)
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func(ln net.Listener, c chan os.Signal) {
		sig := <-c
		log.Printf("Caught signal %s: shutting down.", sig)
		ln.Close()
	}(ln, sigc)

	for {
		if count == max {
			count = 0
		}
		fd, err := ln.Accept()
		if err != nil {
			log.Printf("Accept error:%v ", err)
		}

		wg.Add(1)
		go echoServer(wg, fd, response[count])
		count++
	}
}
