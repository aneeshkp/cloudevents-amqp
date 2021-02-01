package main

import (
	"context"
	"fmt"
	"github.com/Azure/go-amqp"
	amqpconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config/amqp"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/uuid"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	udpPort  = 10001
	eventBus *EventBus
	wg       sync.WaitGroup
	cfg      *amqpconfig.Config
)

// DataEvent ...
type DataEvent struct {
	Data     interface{}
	TimeInMs int64
	//func() int64 {
	//				return time.Now().UnixNano() / int64(time.Millisecond)
	//			}(),
}

// EventBus stores the list of all senders
type EventBus struct {
	sender []*types.AMQPProtocol
	rm     sync.RWMutex
	data   chan DataEvent
}

func main() {
	hostName := "localhost"
	envPort := os.Getenv("PORT")
	if envPort != "" {
		udpPort, _ = strconv.Atoi(envPort)
	}

	eventBus = &EventBus{
		sender: nil,
		rm:     sync.RWMutex{},
		data:   make(chan DataEvent, 10),
	}
	//prepare AMQP
	eventBus.prepareAMQP()

	/** UDP setup */
	udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", hostName, udpPort))
	if err != nil {
		log.Fatal(err)
	}
	// setup listener for incoming UDP connection
	ln, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("UDP server up and listening on port %d\n", udpPort)
	defer ln.Close()

	// loop udp listener
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			// wait for UDP client to connect
			handleUDPConnection(ln)
		}
	}()

	// loop amqp listener
	for { //nolint:gosimple
		select {
		case d := <-eventBus.data:
			s := d.Data.(string)
			event, err := eventBus.WriteMessage(s)
			if err != nil {
				log.Printf("Failed to set data for message %v", err)
				continue
			}
			for _, s := range eventBus.sender {
				wg.Add(1) //for each sender you send a message since its
				go func(s *types.AMQPProtocol, e cloudevents.Event, wg *sync.WaitGroup) {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(s.ParentContext, time.Duration(cfg.TimeOut)*time.Second)
					defer cancel()
					if result := s.Client.Send(ctx, event); cloudevents.IsUndelivered(result) {
						log.Printf("Failed to send: %v", result)
					} else if cloudevents.IsNACK(result) {
						log.Printf("Event not accepted: %v", result)
					} else {
						atomic.AddUint64(&s.MsgReceivedCount, 1)

					}
				}(s, event, &wg)
			}
		}
	}
	//nolint:govet
	wg.Wait()

	for _, s := range eventBus.sender {
		defer s.Protocol.Close(s.ParentContext)
	}

}
func handleUDPConnection(conn *net.UDPConn) {
	buffer := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		log.Print(err)
		return
	}
	d := DataEvent{
		Data:     string(buffer[:n]),
		TimeInMs: 0,
	}
	eventBus.data <- d
	//fmt.Println("UDP client : ", addr)
	//fmt.Println("Received from UDP client :  ", string(buffer[:n]))
}

// WriteMessage ...
func (e *EventBus) WriteMessage(s string) (event.Event, error) {
	//data := types.Message{}
	event := cloudevents.NewEvent()
	//err := json.Unmarshal([]byte(s), &data)

	//if err != nil {
	//	return event, err
	//}

	event.SetID(uuid.New().String())
	event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
	event.SetTime(time.Now())
	event.SetType("com.cloudevents.poc.event.sent")
	err := event.SetData(cloudevents.ApplicationJSON, []byte(s))
	return event, err

}
func (e *EventBus) prepareAMQP() {
	/** AMQP setup */
	var err error
	var p *amqp1.Protocol
	var opts []amqp1.Option

	cfg, err = amqpconfig.GetConfig()
	if err != nil {
		log.Printf("Could not load configuration file --config, loading default queue\n")
		cfg = &amqpconfig.Config{
			TimeOut:  5,
			MsgCount: 0,
			HostName: "amqp://localhost",
			Port:     5672,
			Sender: amqpconfig.Sender{
				Queue: []amqpconfig.Queue{
					{
						Name:  "test/node1",
						Count: 1,
					},
				},
			},
		}
	}
	log.Printf("Connecting to host %s:%d", cfg.HostName, cfg.Port)

	for _, q := range cfg.Sender.Queue {
		for i := 1; i <= q.Count; i++ {
			s := types.AMQPProtocol{}
			s.Queue = q.Name
			s.ID = fmt.Sprintf("%s-#%d", s.Queue, i)
			for {
				p, err = amqp1.NewProtocol2(fmt.Sprintf("%s:%d", cfg.HostName, cfg.Port), s.Queue, "", []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
				if err != nil {
					log.Printf("Failed to create amqp protocol (trying in 5 secs): %v", err)
					time.Sleep(5 * time.Second)
				} else {
					log.Print("Connection established for producer")
					break
				}
			}
			s.Protocol = p
			parent, cancelParent := context.WithCancel(context.Background())
			s.CancelFn = cancelParent
			s.ParentContext = parent
			//defer p.Close(s.ParentContext)
			// Create a new client from the given protocol
			c, err := cloudevents.NewClient(p)
			if err != nil {
				log.Fatalf("Failed to create client: %v", err)
			}
			s.Client = c
			e.sender = append(e.sender, &s)
		}
	}
}
