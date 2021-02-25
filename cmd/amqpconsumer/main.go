package main

import (
	"context"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/qdr"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"log"
	"sync"
	"time"
)

func main() {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		runTest(&wg)
		time.Sleep(10 * time.Second)
	}
}

func runTest(wg *sync.WaitGroup) {
	//build address
	senderAddress := "/clusternamenotknown/nodenamenotknown/CurrentStatus"
	receiveAddress := "/clusternamenotknown/nodenamenotknown/CurrentStatus"

	event := cloudevents.NewEvent()
	event = cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetSource("https://github.com/aneeshkp/cloud-events/vdu")
	event.SetTime(time.Now())
	event.SetType("com.cloudevents.poc.ptp.status")
	event.SetSubject("PTPCurrentStatus")
	_ = event.SetData(cloudevents.ApplicationJSON, fmt.Sprintf(`{"address:%s"}`, receiveAddress))
	ctx := context.Background()

	listener, err := qdr.NewReceiver("amqp://localhost", 5672, receiveAddress)
	if err != nil {
		log.Printf("Error Dialing AMQP server::%v", err)
		return
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel2()
	}()
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		err = listener.Client.StartReceiver(ctx2, func(e cloudevents.Event) {
			fmt.Println(e)
			cancel2()
		})
		if err != nil {
			log.Printf("Error Dialing AMQP server::%v", err)
			cancel2()
		}
	}(wg)

	if err != nil {
		log.Printf("Error Dialing AMQP server::%v", err)
		return
	}

	sender, _ := qdr.NewSender("amqp://localhost", 5672, senderAddress)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	if result := sender.Client.Send(ctx2, event); cloudevents.IsUndelivered(result) {
		log.Printf("failed to send: %v", result)
		cancel()
		cancel2()
	} else if cloudevents.IsNACK(result) {
		log.Printf("event not accepted: %v", result)
		cancel()
		cancel2()
	}
	wg.Wait()
	cancel()
	listener.Protocol.Close(ctx2)
	sender.Protocol.Close(ctx)

}
