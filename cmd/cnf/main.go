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

/*
This cnf sends events at random time chosen between 0 to 500ms(configurable in pod)
and send events between 1 to 100K



*/

const (
	blastMessage           = 1000
	messageIntervalMaxInMs = 5000
)

var (
	udpPort             = 10001
	messageToSend       = blastMessage
	messageIntervalInMS = messageIntervalMaxInMs
)

// init sets initial values for variables used in the function.
func init() {
	rand.Seed(time.Now().UnixNano())
}
func main() {
	if os.Getenv("MESSAGE_COUNT") != "" {
		messageToSend, _ = strconv.Atoi(os.Getenv("MESSAGE_COUNT"))
	}
	if os.Getenv("MESSAGE_INTERVAL") != "" {
		messageIntervalInMS, _ = strconv.Atoi(os.Getenv("MESSAGE_INTERVAL"))
	}
	fmt.Printf("Sleeping %d sec...\n", 10)
	time.Sleep(time.Duration(10) * time.Second)
	n := rand.Intn(messageIntervalInMS) // n will be between 0 and 10
	fmt.Printf("Sleeping %d Millisecond...\n", n)
	uptimeTicker := time.NewTicker(time.Duration(n) * time.Millisecond)

	for { //nolint:gosimple
		select {
		case <-uptimeTicker.C:
			fmt.Println("Sending events ")
			if err := Event(); err != nil {
				fmt.Printf("Error %v", err)
			}
			uptimeTicker.Stop()
			n := rand.Intn(messageIntervalInMS) // n will be between 0 and 5000
			fmt.Printf("Sleeping %d Millisecond...\n", n)
			uptimeTicker = time.NewTicker(time.Duration(n) * time.Millisecond)
		}
	}

}

func getSupportedEvents() []string {
	return []string{"Forgot Badge", "Internet Down", "Fire Alarm", "Amazon package delivered"}
}

//Event will generate random events
func Event() error {

	Conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: udpPort, Zone: ""})
	defer Conn.Close()
	events := getSupportedEvents()
	messages := rand.Intn(messageToSend)
	fmt.Printf("Sending %d messages\n", messages)
	for i := 1; i <= messages; i++ {
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
