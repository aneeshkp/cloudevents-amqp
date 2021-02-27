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

var SocketFile = "/tmp/go.sock"

func echoServer(wg *sync.WaitGroup,c net.Conn) {
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
		_, err = c.Write([]byte("PTP says , Houston we got problem"))
		if err != nil {
			log.Printf("Writing client error:%v ", err)
		}
	}
}
func SetUpPTPStatusServer(wg *sync.WaitGroup){
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
		return
	}(ln, sigc)

	for {
		fd, err := ln.Accept()
		if err != nil {
			log.Printf("Accept error: ", err)
		}

		wg.Add(1)
		go echoServer(wg,fd)
	}
}
