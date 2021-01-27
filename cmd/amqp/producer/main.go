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
	"net/url"
	"os"
	"sync/atomic"

	//"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultMsgCount = 10
)

var (
	unSettledMsgs map[int]interface{}
	m             sync.RWMutex
	cfg           *amqp_config.Config
	wg            sync.WaitGroup
)

// Parse AMQP_URL env variable. Return server URL, AMQP node (from path) and SASLPlain
// option if user/pass are present.
func amqpConfig() (server, node string, opts []amqp1.Option) {
	env := os.Getenv("AMQP_URL")
	if env == "" {
		env = "/test"
	}
	u, err := url.Parse(env)
	if err != nil {
		log.Fatal(err)
	}
	if u.User != nil {
		user := u.User.Username()
		pass, _ := u.User.Password()
		opts = append(opts, amqp1.WithConnOpt(amqp.ConnSASLPlain(user, pass)))
	}

	//opts = append(opts, amqp1.WithSenderLinkOption(amqp.LinkSenderSettle(amqp.ModeSettled)))
	//opts = append(opts, amqp1.WithReceiverLinkOption(amqp.LinkReceiverSettle(amqp.ModeFirst)))

	return env, strings.TrimPrefix(u.Path, "/"), opts
}

func main() {
	host, _, opts := amqpConfig()
	var p *amqp1.Protocol
	var err error
	log.Printf("Connecting to host %s", host)
	var senders []sender_type.AMQPProtocol

	//ctx := context.Background()

	unSettledMsgs = make(map[int]interface{})

	cfg, err := amqp_config.GetConfig()
	if err != nil {
		log.Print("Could not load configuration file --config, loading default queue\n")
		cfg = &amqp_config.Config{
			Sender: amqp_config.Sender{
				Count: defaultMsgCount,
				Queue: []amqp_config.Queue{
					{
						Name:  "test",
						Count: 1,
					},
				},
			},
		}
	}

	for _, q := range cfg.Sender.Queue {
		for i := 1; i <= q.Count; i++ {
			s := sender_type.AMQPProtocol{}
			s.Queue = q.Name
			s.ID=fmt.Sprintf("%s-#%d",s.Queue,i)
			for {
				p, err = amqp1.NewProtocol2(host, s.Queue, "", []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
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
			senders = append(senders, s)
		}
	}

	// Close the connection when finished
	// Create a new context.

	for _, s := range senders { //i have now lined up  all senders
		log.Printf("preparing data for sender %s", s.ID)
		wg.Add(1)
		go func(wg *sync.WaitGroup,s *sender_type.AMQPProtocol){
			SendMessage(wg,s)
		}(&wg,&s)
	}

	log.Printf("--------- WG Wait has issues  ----------\n")
	//log.Printf("All %d message was sent", count)
	/*if len(unSettledMsgs) > 0 {
		log.Printf("Total %d messages were unsettled\n", len(unSettledMsgs))
		log.Printf("Unsettled messages\n")
		log.Printf("--------------------\n")
		for k := range unSettledMsgs {
			log.Printf("Message id `%d` was not settled and waiting", k)
		}
		log.Printf("--------------------\n")
		log.Printf("Out of %d messages ,Only %d was settled", count, count-len(unSettledMsgs))
	} else {
		log.Printf("%d message was sent and %d was settled", count, count)
	}*/
	//measure
	/*elapsed := time.Since(start)
	log.Printf("ce-amqp Took %s to send %d messsages and settle %d messages", elapsed, count, count-len(unSettledMsgs))
	*/

	wg.Wait()



	//log.Printf("All %d message was sent", count)
	//elapsed = time.Since(start)
	//log.Printf("ce-amqp Took %s to send and settle all messages", elapsed)
	log.Print("Done")
	for _, s := range senders {
		log.Printf("ce-amqp Sender %s - Sent %d to queue %s", s.ID, s.MsgReceivedCount, s.Queue)
	}
	for _, s := range senders {

		s.CancelFn()

	}
}


func SendMessage(wg *sync.WaitGroup,s *sender_type.AMQPProtocol) {
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
				//TODO: This is by value use channel to return the count
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