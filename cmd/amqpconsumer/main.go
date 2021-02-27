package main

import (
	"context"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/qdr"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"log"
	"sync"
	"time"
)

func main() {
	var wg sync.WaitGroup
	//for i := 0; i < 10; i++ {
		runTest(&wg)
		//time.Sleep(10 * time.Second)
	//}
	wg.Wait()
}

func runTest(wg *sync.WaitGroup) {
	//build address
	senderAddress := "/clusternameunknown/nodenameunknown/SYNC/PTP"
	//receiveAddress :="/clusternameunknown/nodenameunknown/SYNC/PTP"
	sub:=types.Subscription{
	SubscriptionID:    "1231231231",
	URILocation:       "",
	ResourceType:      "PTP",
	EndpointURI:       "",
	ResourceQualifier: types.ResourceQualifier{
		NodeName:    "clusternameunknown",
		NameSpace:   "nodenameunknown",
		ClusterName: "",
		Suffix: []string{"SYNC","PTP"},

	},
	EventData:         types.EventDataType{},
	EventTimestamp:    0,
	Error:             "",
}

	event := cloudevents.NewEvent()
	event = cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetSource("https://github.com/aneeshkp/cloud-events/vdu")
	event.SetTime(time.Now())
	event.SetType("com.cloudevents.poc.ptp.status")
	//event.SetSubject("PTPCurrentStatus")
	_ = event.SetData(cloudevents.ApplicationJSON, fmt.Sprintf(`{"address:%s"}`, senderAddress))
	_ = event.SetData(cloudevents.ApplicationJSON, sub)
	ctx := context.Background()

	/*listener, err := qdr.NewReceiver("amqp://localhost", 5672, receiveAddress)
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
		})
		if err != nil {
			log.Printf("Error Dialing AMQP server::%v", err)
		}
	}(wg)

	if err != nil {
		log.Printf("Error Dialing AMQP server::%v", err)
		return
	}*/

	sender, _ := qdr.NewSender("amqp://localhost", 5672, senderAddress)


	for i:=0;i<=200;i++ {
		wg.Add(1)
		go func(wg *sync.WaitGroup,sender *types.AMQPProtocol){
			defer wg.Done()
		ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)

		if result := sender.Client.Send(ctx2, event); cloudevents.IsUndelivered(result) {
			log.Printf("failed to send: %v", result)
			cancel()
			//cancel2()
		} else if cloudevents.IsNACK(result) {
			log.Printf("event not accepted: %v", result)
			cancel()
			//cancel2()
		}else{
			log.Printf("Event sent and ack")
		}

		cancel()
		}(wg,sender)
		//listener.Protocol.Close(ctx2)
		//sender.Protocol.Close(ctx)

	}
	log.Printf("waiting %s","aneesh")
	wg.Wait()
	sender.Protocol.Close(ctx)

}
