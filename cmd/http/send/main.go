package main

import (
	"context"
	"log"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
)

const (
	DEFAULT_MSG_COUNT = 1000
)
var (
	wg            sync.WaitGroup

)

func main() {
	ctx := cloudevents.ContextWithTarget(context.Background(), "http://localhost:8080/")

	p, err := cloudevents.NewHTTP()
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}

	c, err := cloudevents.NewClient(p, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}
	start := time.Now()
	for i := 1; i <= DEFAULT_MSG_COUNT; i++ {
		wg.Add(1)
		go func(i int,start time.Time){
		e := cloudevents.NewEvent()
		e.SetType("com.cloudevents.sample.sent")
		e.SetSource("https://github.com/cloudevents/sdk-go/v2/samples/httpb/sender")
		_ = e.SetData(cloudevents.ApplicationJSON, map[string]interface{}{
			"id":      i,
			"message": "Hello, World!",
		})

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
			log.Printf("ce-http Took %s to send message number %d", elapsed,i)
		}(i,start)
	}
	wg.Wait()
	elapsed := time.Since(start)
	log.Printf("ce-http Took %s to send all %d messages ", elapsed,DEFAULT_MSG_COUNT)

}
