package main

import (
	"context"
	"fmt"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"log"
	"time"

	"github.com/Azure/go-amqp"
)

var (
	msgRecievedCount = 0
)

// Message is a basic data struct.
type Message struct {
	Sequence int    `json:"id"`
	Message  string `json:"message"`
}

func main() {
	// Create client
	client, err := amqp.Dial("amqp://localhost:5672")
	if err != nil {
		log.Fatal("Dialing AMQP server:", err)
	}
	defer client.Close()

	// Open a session
	session, err := client.NewSession()
	if err != nil {
		log.Fatal("Creating AMQP session:", err)
	}

	ctx := context.Background()

	// Send a message
	go func() {
		//{
		// Create a sender
		for i := 1; i <= 10; i++ {

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
				log.Fatal("Error setting event data:", err)
			}

			sender, err := session.NewSender(
				amqp.LinkTargetAddress("/queue-name"),
			)
			//amqp.LinkSenderSettle(amqp.ModeSettled)
			if err != nil {
				log.Fatal("Creating sender link:", err)
			}

			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)

			// Send message
			err = sender.Send(ctx, amqp.NewMessage([]byte(fmt.Sprintf("Hello! Message id=%d", i))))
			if err != nil {
				log.Fatal("Sending message:", err)
			}
			sender.Close(ctx)
			cancel()
		}

	}()

	// Continuously read messages
	{

		// Create client
		client2, err := amqp.Dial("amqp://localhost:5672")
		if err != nil {
			log.Fatal("Dialing AMQP server:", err)
		}
		defer client2.Close()

		// Open a session
		session2, err := client2.NewSession()
		if err != nil {
			log.Fatal("Creating AMQP session:", err)
		}

		// Create a receiver
		receiver, err := session2.NewReceiver(
			amqp.LinkSourceAddress("/queue-name"),
			amqp.LinkCredit(10),
		)
		if err != nil {
			log.Fatal("Creating receiver link:", err)
		}
		ctx2 := context.Background()
		defer func() {
			ctx2, cancel2 := context.WithTimeout(ctx2, 5*time.Second)
			receiver.Close(ctx2)
			cancel2()
		}()

		for {
			// Receive next message
			msg, err := receiver.Receive(ctx2)
			if err != nil {
				log.Printf("Reading message from AMQP: %v", err)
			}

			// Accept message
			err = msg.Accept()
			if err != nil {
				log.Printf("Reading message from AMQP: %v", err)
			}

			fmt.Printf("Message received: %s\n", msg.GetData())
			msgRecievedCount++
			fmt.Printf("Total message recieved %d\n", msgRecievedCount)
		}

	}
}
