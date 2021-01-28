package main

import (
	"context"
	"fmt"
	"github.com/Azure/go-amqp"
	amqp_config "github.com/aneeshkp/cloudevents-amqp/pkg/config/amqp"
	sender_type "github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/aneeshkp/cloudevents-amqp/types"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"log"

	"sync/atomic"

	"sync"
	"time"
)

const (
	defaultMsgCount = 10
)

var (
	cfg     *amqp_config.Config
	wg      sync.WaitGroup
	senders []*sender_type.AMQPProtocol
)

func main() {

	var p *amqp1.Protocol
	var err error
	var opts []amqp1.Option

	cfg, err = amqp_config.GetConfig()
	if err != nil {
		log.Print("Could not load configuration file --config, loading default queue\n")
		cfg = &amqp_config.Config{
			HostName: "amqp://localhost",
			Port:     5672,
			Sender: amqp_config.Sender{
				Count: defaultMsgCount,
				Queue: []amqp_config.Queue{
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
			s := sender_type.AMQPProtocol{}
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
			defer p.Close(s.ParentContext)
			// Create a new client from the given protocol
			c, err := cloudevents.NewClient(p)
			if err != nil {
				log.Fatalf("Failed to create client: %v", err)
			}
			s.Client = c
			senders = append(senders, &s)
		}
	}

	// Close the connection when finished
	// Create a new context.

	for _, s := range senders { //i have now lined up  all senders
		log.Printf("preparing data for sender %s", s.ID)
		wg.Add(1)
		go func(wg *sync.WaitGroup, s *sender_type.AMQPProtocol) {
			defer wg.Done()
			SendMessage(wg, s)
		}(&wg, s)
	}

	log.Printf("--------- wait  ----------\n")

	wg.Wait()

	log.Print("Done")
	for _, s := range senders {
		log.Printf("ce-amqp Sender %s - Sent %d to queue %s", s.ID, s.MsgReceivedCount, s.Queue)
		s.CancelFn()
	}

}

// SendMessage sends message to the queue
func SendMessage(wg *sync.WaitGroup, s *sender_type.AMQPProtocol) {
	for i := 1; i <= defaultMsgCount; i++ {
		event := cloudevents.NewEvent()
		event.SetID(uuid.New().String())
		event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
		event.SetTime(time.Now())
		event.SetType("com.cloudevents.poc.event.sent")
		//log.Printf("Setting Data for %d", i)
		msg := types.Message{ID: i, Msg: "Hello world"}
		err := event.SetData(cloudevents.ApplicationJSON, msg)
		if err != nil {
			log.Printf("Failed to set data for message %d: %v", i, err)
			continue
		}

		wg.Add(1)
		go func(s *sender_type.AMQPProtocol, e cloudevents.Event, wg *sync.WaitGroup, index int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(s.ParentContext, 5*time.Second)
			defer cancel()

			if result := s.Client.Send(ctx, event); cloudevents.IsUndelivered(result) {
				log.Printf("Failed to send: %v", result)
			} else if cloudevents.IsNACK(result) {
				log.Printf("Event not accepted: %v", result)
			} else {
				atomic.AddUint64(&s.MsgReceivedCount, 1)
			} /*else {
				m.Lock()
				defer m.Unlock()
				delete(unSettledMsgs, index)
				log.Printf("MESSAGE ID %d SUCCESSFULLY DELIVERED %v pending %d to settle", index, result, len(unSettledMsgs))
			}*/
		}(s, event, wg, i)
	}
}
