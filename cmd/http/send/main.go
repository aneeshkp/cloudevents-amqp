package main

import (
	"context"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"log"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
)

const (
	defaultMsgCount     = 1000
	httpPort        int = 9093
)

var (
	wg sync.WaitGroup
)

func main() {
	//	portPtr := flag.String("port", "8080", "a string")

	ctx := cloudevents.ContextWithTarget(context.Background(), fmt.Sprintf("rest://localhost:%d/", httpPort))

	p, err := cloudevents.NewHTTP()
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}
	p.Port = httpPort

	c, err := cloudevents.NewClient(p, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}
	start := time.Now()
	for i := 1; i <= defaultMsgCount; i++ {
		wg.Add(1)
		go func(i int, start time.Time) {
			e := cloudevents.NewEvent()
			e.SetType("com.cloudevents.sample.sent")
			e.SetSource("https://github.com/cloudevents/sdk-go/v2/samples/httpb/sender")
			msg := types.Message{ID: i, Msg: "Hello world"}
			_ = e.SetData(cloudevents.ApplicationJSON, msg)

			res := c.Send(ctx, e)
			if cloudevents.IsUndelivered(res) {
				log.Printf("Failed to send: %v", res)
			} else {
				var httpResult *cehttp.Result
				cloudevents.ResultAs(res, &httpResult)
				log.Printf("Sent %d with status code %d", i, httpResult.StatusCode)
			}
			wg.Done()
			elapsed := time.Since(start)
			log.Printf("ce-rest Took %s to send message number %d", elapsed, i)
		}(i, start)
	}
	wg.Wait()
	elapsed := time.Since(start)
	log.Printf("ce-rest Took %s to send all %d messages ", elapsed, defaultMsgCount)

}
