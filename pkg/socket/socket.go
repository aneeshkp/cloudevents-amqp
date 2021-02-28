package socket

import (
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

func echoServer(wg *sync.WaitGroup, c net.Conn, msg string) {
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
	response := [max]string{
		"I believe we've had a problem here",
		"Houston, we've had a problem. We've had a MAIN B BUS UNDERVOLT.",
		"Okay. Right now, Houston, the voltage is—is looking good",
		"In the interim here, we're starting to go ahead and button up the tunnel again.",
		"Yes. That jolt must have rocked the sensor on—see now—O2 QUANTITY 2",
		"And, Houston, we had a RESTART on our computer and we had a PGNCS light and the RESTART RESET",
		"Okay. And we're looking at our S—SERVICE MODULE RCS HELIUM 1.",
		"Okay, AC 2 is showing zip. I'm going to try to reconfigure on that, Jack.",
		"Yes. We got a MAIN BUS A UNDERVOLT now, too, showing.",
		"It's reading about 25-1/2, MAIN B is reading zip right now.",
		"We'd like you to attempt to reconnect fuel cell 1 to MAIN A and fuel cell 3 to MAIN B. Verify that quad Delta is open",
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
