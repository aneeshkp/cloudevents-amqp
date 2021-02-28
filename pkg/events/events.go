package events

import (
	"bytes"
	"encoding/json"
	"github.com/aneeshkp/cloudevents-amqp/pkg/chain"
	eventconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"log"
	"math/rand"
	"net"
	"net/http"
	"time"
)

//Event  creates event object
type Event struct {
	avgMsgPerSec int
	totalMsgSent int64
	pubStore     map[string]*types.Subscription
	cfg          eventconfig.Config
	subscription types.Subscription
}

//GetTotalMsgSent returns total message sent
func (e *Event) GetTotalMsgSent() int64 {
	return e.totalMsgSent
}

//ResetTotalMsgSent to reset total message sent
func (e *Event) ResetTotalMsgSent() {
	e.totalMsgSent = 0
}

// New create new event object
func New(avgMsgPerSec int, pubStore map[string]*types.Subscription, subscription types.Subscription, cfg eventconfig.Config) *Event {
	return &Event{
		avgMsgPerSec: avgMsgPerSec,
		totalMsgSent: 0,
		pubStore:     pubStore,
		cfg:          cfg,
		subscription: subscription,
	}

}

//GenerateEvents is used to generate mock data
func (e *Event) GenerateEvents(eventPostAddress string, id string) {

	states := e.intiState(e.subscription.EndpointURI)

	transition := [][]float32{
		{
			0.6, 0.1, 0.1, 0.0, 0.2,
		},
		{
			0.1, 0.7, 0.1, 1.0, 1.0,
		},
		{
			0.3, 0.3, 0.1, 0.1, 0.0,
		},
		{
			0.2, 0.3, 0.3, 0.1, 0.1,
		},
		{
			0.3, 0.3, 0.3, 0.1, 0.0,
		},
	}

	//fmt.Printf("Sleeping %d sec...\n", 10)

	// initialize current state
	currentStateID := 1
	c := chain.Create(transition, states)
	currentStateChoice := c.GetStateChoice(currentStateID)
	currentStateID = currentStateChoice.Item.(int)

	avgMsgPeriodMs := 1000 / e.avgMsgPerSec //100
	log.Printf("avgMsgPerMs: %d\n", avgMsgPeriodMs)
	midpoint := avgMsgPeriodMs / 2

	log.Printf("midpoint: %d\n", midpoint)

	tck := time.NewTicker(time.Duration(1000) * time.Microsecond)
	maxCount := avgMsgPeriodMs
	counter := 0
	for range tck.C {
		currentStateChoice := c.GetStateChoice(currentStateID)
		currentStateID = currentStateChoice.Item.(int)
		currentState := c.GetState(currentStateID)
		if counter >= maxCount {
			if currentState.Probability > 0 {
				//for i := 1; i <= currentState.Payload.ID; i++ {
				if e.cfg.EventHandler == types.SOCKET {
					err := e.udpEvent(currentState.Payload, id)
					if err == nil {
						e.totalMsgSent++
					}
				} else {
					err := e.httpEvent(eventPostAddress, currentState.Payload, id)
					if err == nil {
						e.totalMsgSent++
					}
				}
			}
			maxCount = rand.Intn(avgMsgPeriodMs+1) + midpoint
			counter = 0
		}
		counter++
	}
}

func (e *Event) intiState(eventPublisherURL string) []chain.State {
	states := []chain.State{
		{
			ID: 1,
			Payload: types.Subscription{
				SubscriptionID: "",
				URILocation:    "",
				ResourceType:   "",
				EndpointURI:    eventPublisherURL,
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    e.cfg.Cluster.Node,
					NameSpace:   e.cfg.Cluster.NameSpace,
					ClusterName: e.cfg.Cluster.Name,
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.FREERUN},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
		{
			ID: 2,
			Payload: types.Subscription{
				SubscriptionID: "",
				URILocation:    "",
				ResourceType:   "",
				EndpointURI:    eventPublisherURL,
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    e.cfg.Cluster.Node,
					NameSpace:   e.cfg.Cluster.NameSpace,
					ClusterName: e.cfg.Cluster.Name,
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.HOLDOVER},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
		{
			ID: 3,
			Payload: types.Subscription{
				SubscriptionID: "",
				URILocation:    "",
				ResourceType:   "",
				EndpointURI:    eventPublisherURL,
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    e.cfg.Cluster.Node,
					NameSpace:   e.cfg.Cluster.NameSpace,
					ClusterName: e.cfg.Cluster.Name,
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.LOCKED},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
		{
			ID: 4,
			Payload: types.Subscription{
				SubscriptionID: "",
				URILocation:    "",
				ResourceType:   "",
				EndpointURI:    eventPublisherURL,
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    e.cfg.Cluster.Node,
					NameSpace:   e.cfg.Cluster.NameSpace,
					ClusterName: e.cfg.Cluster.Name,
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.FREERUN},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
		{
			ID: 5,
			Payload: types.Subscription{
				SubscriptionID: "",
				URILocation:    "",
				ResourceType:   "",
				EndpointURI:    eventPublisherURL,
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    e.cfg.Cluster.Node,
					NameSpace:   e.cfg.Cluster.NameSpace,
					ClusterName: e.cfg.Cluster.Name,
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.LOCKED},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
	}
	return states
}

//Event will generate random events
func (e *Event) httpEvent(eventPostAddress string, payload types.Subscription, id string) (err error) {
	//payload.EndpointURI = fmt.Sprintf("%s:%d%s/event/ack", "http://localhost", 9090, SUBROUTINE)
	if pub, ok := e.pubStore[id]; ok {
		payload.SubscriptionID = id
		payload.URILocation = pub.URILocation
		now := time.Now()
		nanos := now.UnixNano()
		// Note that there is no `UnixMillis`, so to get the
		// milliseconds since epoch you'll need to manually
		// divide from nanoseconds.
		millis := nanos / 1000000
		payload.EventTimestamp = millis
		b, err := json.Marshal(payload)
		if err != nil {
			log.Printf("The marshal error %s\n", err)
		}

		//log.Printf("Posting event to %s\n", pub.EndpointURI)

		response, err := http.Post(eventPostAddress, "application/json", bytes.NewBuffer(b))
		if err != nil {
			log.Printf("http event:the http request failed with and error %s\n", err)
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			log.Printf("failed to send events via http %s and status %d", eventPostAddress, response.StatusCode)
		}
	} else {
		log.Printf("Could not find data in publisher store for id %s\n", id)
	}

	//fmt.Printf("Sending %v messages\n", payload)
	return err
}

//Event will generate random events
func (e *Event) udpEvent(payload types.Subscription, publisherID string) error {
	if pub, ok := e.pubStore[publisherID]; ok {
		payload.SubscriptionID = publisherID
		payload.URILocation = pub.URILocation
		//payload.EndpointURI = fmt.Sprintf("%s:%d%s/event/ack", "http://localhost", 9090, SUBROUTINE)
		now := time.Now()
		nanos := now.UnixNano()
		// Note that there is no `UnixMillis`, so to get the
		// milliseconds since epoch you'll need to manually
		// divide from nanoseconds.
		millis := nanos / 1000000
		payload.EventTimestamp = millis

		b, err := json.Marshal(payload)
		if err != nil {
			log.Printf("The marshal error %s\n", err)
			return err
		}
		Conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: e.cfg.Socket.Sender.Port, Zone: ""})
		defer Conn.Close()
		if _, err = Conn.Write(b); err != nil {
			log.Printf("The socket error %s\n", err)
			return err
		}
		if err != nil {
			log.Printf("failed to send  event via SOCKET %s\n", err)
		}

	} else {
		log.Printf("Did not find the publisher to send events %s\n", publisherID)
	}
	//fmt.Printf("Sending %v messages\n", payload)
	return nil

}
