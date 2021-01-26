package main

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const (
	defaultMsgCount = 1000
)

var (
	msgReceivedCount uint64
	maxDiff          int64 = 0
)

func main() {
	ctx := context.Background()
	p, err := cloudevents.NewHTTP()
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}

	c, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}

	log.Printf("will listen on :8080\n")
	log.Fatalf("failed to start receiver: %s", c.StartReceiver(ctx, receive))
}

func receive(ctx context.Context, event cloudevents.Event) {
	//fmt.Printf("%s", event)
	diff := time.Since(event.Context.GetTime()).Microseconds()
	if diff > maxDiff {
		maxDiff = diff
	}
	atomic.AddUint64(&msgReceivedCount, 1)
	if (msgReceivedCount % defaultMsgCount) == 0 {
		fmt.Printf("\nCE-HTTP: Total message recived %d, maxDiff = %d\n", msgReceivedCount, maxDiff)
	}
	//fmt.Printf("\nTotal message recived %d\n", msgReceivedCount)

}
