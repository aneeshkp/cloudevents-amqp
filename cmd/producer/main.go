package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Azure/go-amqp"
	"github.com/google/uuid"

	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const (
	count = 10
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
	for {
		p, err = amqp1.NewProtocol(host, node, []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
		if err != nil {
			log.Printf("Failed to create amqp protocol (trying in 5 secs): %v", err)
			time.Sleep(5 * time.Second)
		}else{
			log.Print("Connection established for producer")
			break
		}
	}

	// Close the connection when finished
	defer p.Close(context.Background())

	// Create a new client from the given protocol
	c, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	for i := 0; i < count; i++ {
		log.Printf("preparing data for %d",i)
		event := cloudevents.NewEvent()
		event.SetID(uuid.New().String())
		event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
		event.SetTime(time.Now())
		event.SetType("com.cloudevents.poc.event.sent")

		log.Printf("Setting Data for %d",i)
		err := event.SetData(cloudevents.ApplicationJSON,
			&Message{
				Sequence: i,
				Message:  "Hello world!",
			})

		if err != nil {
			log.Printf("Failed to set data: %v", err)
		}
		log.Printf("MESSAGE Sendng %d",i)

		if result := c.Send(context.Background(), event); cloudevents.IsUndelivered(result) {
			log.Printf("Failed to send: %v", result)
		} else if cloudevents.IsNACK(result) {
			log.Printf("Event not accepted: %v", result)
		}else{
			log.Printf("MESSAGE SENT %v",result)
		}

		time.Sleep(100 * time.Millisecond)
		log.Printf("Moving on to %d",i+1)
	}
}