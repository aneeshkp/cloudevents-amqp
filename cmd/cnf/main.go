package main

import (
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/chain"
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

var (
	udpPort                   = 10001
	totalMsgCount       int64 = 0
	totalPerSecMsgCount       = 0
	rollInMsMin               = 1
	rollInMsMax               = 5
)

// init sets initial values for variables used in the function.
func init() {
	rand.Seed(time.Now().UnixNano())
}
func main() {
	//var err error
	states := intiState()
	envRollInMsMin := os.Getenv("ROLL_DICE_MIN")
	if envRollInMsMin != "" {
		rollInMsMin, _ = strconv.Atoi(envRollInMsMin)
	}
	envRollInMsMax := os.Getenv("ROLL_DICE_MAX")
	if envRollInMsMax != "" {
		rollInMsMin, _ = strconv.Atoi(envRollInMsMax)
	}

	transition := [][]float32{
		{
			0.8, 0.1, 0.1, 0.0, 0.0,
		},
		{
			0.1, 0.9, 0.1, 0.0, 0.0,
		},
		{
			0.3, 0.3, 0.2, 0.0, 0.0,
		},
		{
			0.2, 0.3, 0.5, 0.0, 0.0,
		},
		{
			0.3, 0.4, 0.3, 0.0, 0.0,
		},
	}

	//fmt.Printf("Sleeping %d sec...\n", 10)
	//time.Sleep(time.Duration(10) * time.Second)
	diceTicker := time.NewTicker(time.Duration(rollInMsMin) * time.Millisecond)
	avgPerSecTicker := time.NewTicker(time.Duration(5) * time.Second)

	// initialize current state
	currentStateID := 1
	c := chain.Create(transition, states)
	currentStateChoice := c.GetStateChoice(currentStateID)
	currentStateID = currentStateChoice.Item.(int)
	//rand.Seed(time.Now().UnixNano())
	min := rollInMsMin
	max := rollInMsMax

	for { //nolint:gosimple
		select {
		case <-diceTicker.C:
			currentStateChoice := c.GetStateChoice(currentStateID)
			currentStateID = currentStateChoice.Item.(int)
			currentState := c.GetState(currentStateID)
			//	r := rand.Intn(50)
			if currentState.Payload.Probability > 0 {
				//for i := 1; i <= currentState.Payload.ID; i++ {
				_ = Event(currentState.Payload)
				totalMsgCount++
				totalPerSecMsgCount++
				//}
			}
		case <-avgPerSecTicker.C:
			fmt.Printf("Total message sent mps: %2.2f\n", float64(totalPerSecMsgCount)/1)
			totalPerSecMsgCount = 0
			diceTicker.Stop()
			newTime := rand.Intn(max-min+1) + min
			diceTicker = time.NewTicker(time.Duration(newTime) * time.Millisecond)
		}
	}

}

/*func getSupportedEvents() []string {
	return []string{"Forgot Badge", "Internet Down", "Fire Alarm", "Amazon package delivered"}
}*/
func intiState() []chain.State {
	//events := getSupportedEvents()
	states := []chain.State{
		{
			Payload: types.Message{
				ID:      25,
				StateID: 1,
				Source: fmt.Sprintf("Node: %s Pod: %s NameSpace: %s IP:%s",
					os.Getenv("MY_NODE_NAME"),
					os.Getenv("MY_POD_NAME"),
					os.Getenv("MY_POD_NAMESPACE"),
					os.Getenv("MY_POD_IP"),
				),
				Msg:         fmt.Sprintf("Event Occurred %s", "Forgot Badge"),
				Probability: 99,
			},
			ID: 1,
		},
		{
			Payload: types.Message{
				ID:      10,
				StateID: 2,
				Source: fmt.Sprintf("Node: %s Pod: %s NameSpace: %s IP:%s",
					os.Getenv("MY_NODE_NAME"),
					os.Getenv("MY_POD_NAME"),
					os.Getenv("MY_POD_NAMESPACE"),
					os.Getenv("MY_POD_IP"),
				),
				Msg:         fmt.Sprintf("Event Occurred %s:", "Internet Down"),
				Probability: 99,
			},
			ID: 2,
		},
		{
			Payload: types.Message{
				StateID: 1,
				ID:      50,
				Source: fmt.Sprintf("Node: %s Pod: %s NameSpace: %s IP:%s",
					os.Getenv("MY_NODE_NAME"),
					os.Getenv("MY_POD_NAME"),
					os.Getenv("MY_POD_NAMESPACE"),
					os.Getenv("MY_POD_IP"),
				),
				Msg:         fmt.Sprintf("Event Occurred %s:", "Amazon package delivered"),
				Probability: 99,
			},
			ID: 3,
		},
		{
			Payload: types.Message{
				ID:      4,
				StateID: 4,
				Source: fmt.Sprintf("Node: %s Pod: %s NameSpace: %s IP:%s",
					os.Getenv("MY_NODE_NAME"),
					os.Getenv("MY_POD_NAME"),
					os.Getenv("MY_POD_NAMESPACE"),
					os.Getenv("MY_POD_IP"),
				),
				Msg:         fmt.Sprintf("Event Occurred %s:", "Car breakdown"),
				Probability: 99,
			},
			ID: 4,
		},
		{
			Payload: types.Message{
				ID:      5,
				StateID: 5,
				Source: fmt.Sprintf("Node: %s Pod: %s NameSpace: %s IP:%s",
					os.Getenv("MY_NODE_NAME"),
					os.Getenv("MY_POD_NAME"),
					os.Getenv("MY_POD_NAMESPACE"),
					os.Getenv("MY_POD_IP"),
				),
				Msg:         fmt.Sprintf("Event Occurred %s:", "Temp too high"),
				Probability: 99,
			},
			ID: 5,
		},
	}
	return states
}

//Event will generate random events
func Event(payload types.Message) error {
	Conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: udpPort, Zone: ""})
	defer Conn.Close()

	payload.SetTime(time.Now())
	b, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error: %s", err)
		return err
	}
	if _, err = Conn.Write(b); err != nil {
		return err
	}
	//fmt.Printf("Sending %v messages\n", payload)

	return nil
}
