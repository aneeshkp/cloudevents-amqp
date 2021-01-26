package main

import (
	"context"
	"github.com/Azure/go-amqp"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultMsgCount = 1000
)

var (
	unSettledMsgs map[int]interface{}
	m             sync.RWMutex
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

// Message is a basic data struct.
type Message struct {
	Sequence int    `json:"id"`
	Message  string `json:"message"`
}

func main() {
	host, node, opts := amqpConfig()
	var p *amqp1.Protocol
	var err error
	log.Printf("Connecting to host %s", host)

	//ctx := context.Background()

	unSettledMsgs = make(map[int]interface{})
	for {
		p, err = amqp1.NewProtocol2(host, node, "", []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
		if err != nil {
			log.Printf("Failed to create amqp protocol (trying in 5 secs): %v", err)
			time.Sleep(5 * time.Second)
		} else {
			log.Print("Connection established for producer")
			break
		}
	}

	// Close the connection when finished
	// Create a new context.
	parent, cancelParent := context.WithCancel(context.Background())

	defer p.Close(parent)

	// Create a new client from the given protocol

	c, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	count, err := strconv.Atoi(os.Getenv("MSG_COUNT"))
	if err != nil {
		count = defaultMsgCount
	}

	wg.Add(count)
	start := time.Now()
	for i := 1; i <= count; i++ {

		log.Printf("preparing data for %d", i)

		event := cloudevents.NewEvent()
		event.SetID(uuid.New().String())
		event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
		event.SetTime(time.Now())
		event.SetType("com.cloudevents.poc.event.sent")

		log.Printf("Setting Data for %d", i)
		err := event.SetData(cloudevents.ApplicationJSON,
			&Message{
				Sequence: i,
				Message:  "Hello world!",
			})

		if err != nil {
			log.Printf("Failed to set data for message %d: %v", i, err)
			continue
		}

		log.Printf("MESSAGE Sendng %d", i)

		m.Lock()
		unSettledMsgs[i] = string(event.Data())
		m.Unlock()

		go func(c cloudevents.Client, e cloudevents.Event, wg *sync.WaitGroup, index int) {
			//index:=i
			ctx, cancel := context.WithTimeout(parent, 5*time.Second)
			defer cancel()
			if result := c.Send(ctx, event); cloudevents.IsUndelivered(result) {
				log.Printf("Failed to send: %v", result)
			} else if cloudevents.IsNACK(result) {
				log.Printf("Event not accepted: %v", result)
			} else {
				m.Lock()
				delete(unSettledMsgs, index)
				log.Printf("MESSAGE ID %d SUCCESSFULLY DELIVERED %v pending %d to settle", index, result, len(unSettledMsgs))
				m.Unlock()
			}
			wg.Done()
		}(c, event, &wg, i)
		//time.Sleep(100 * time.Millisecond)
	}
	log.Printf("--------- Summary ----------\n")
	log.Printf("All %d message was sent", count)
	if len(unSettledMsgs) > 0 {
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
	}
	//measure
	elapsed := time.Since(start)
	log.Printf("ce-amqp Took %s to send %d messsages and settle %d messages", elapsed, count, count-len(unSettledMsgs))

	wg.Wait()

	elapsed = time.Since(start)
	log.Printf("ce-amqp Took %s to send and settle all messages", elapsed)
	log.Print("Done")
	cancelParent()

}
