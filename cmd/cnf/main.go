package main

import (
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"
)

const (
	blastMessage = 10
)

var (
	udpPort = 10001
)

func main() {
	uptimeTicker := time.NewTicker(5 * time.Second)
	envPort := os.Getenv("PORT")
	if envPort != "" {
		udpPort, _ = strconv.Atoi(envPort)
	}

	for { //nolint:gosimple
		select {
		case <-uptimeTicker.C:
			if err := Event(); err != nil {
				fmt.Printf("Error %v", err)
			}
		}
	}

}

func getSupportedEvents() []string {
	return []string{"Forgot Badge", "Internet Down", "Fire Alarm", "Amazon package delivered"}
}

//Event will generate random events
func Event() error {
	rand.Seed(time.Now().Unix()) // initialize global pseudo random generator

	Conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: udpPort, Zone: ""})
	defer Conn.Close()
	events := getSupportedEvents()

	for i := 1; i <= blastMessage; i++ {
		msg := types.Message{
			ID: i,
			Source: fmt.Sprintf("Node: %s /Pod: %s /NameSpace: %s /IP:%s",
				os.Getenv("MY_NODE_NAME"),
				os.Getenv("MY_POD_NAME"),
				os.Getenv("MY_POD_NAMESPACE"),
				os.Getenv("MY_POD_IP"),
			),
			Msg: fmt.Sprintf("Event Occurred %s", events[rand.Intn(len(events))]),
		}
		msg.SetTime(time.Now())
		b, err := json.Marshal(msg)
		if err != nil {
			fmt.Printf("Error: %s", err)
			return err
		}
		if _, err = Conn.Write(b); err != nil {
			return err
		}
	}
	return nil
}
