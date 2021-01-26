package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/types"
	"log"
	"sync/atomic"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const (
	defaultMsgCount = 1000
	httpPort        = 9093
)

var (
	msgReceivedCount     uint64
	msgCurrentBatchCount uint64 = 0
	maxDiff              int64  = 0
	currentBatchMaxDiff  int64  = 0
)

func main() {
	ctx := context.Background()
	p, err := cloudevents.NewHTTP()
	p.Port = httpPort
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}

	c, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}

	log.Printf("will listen on :%d\n", httpPort)
	log.Fatalf("failed to start receiver: %s", c.StartReceiver(ctx, receive))
}

func receive(ctx context.Context, event cloudevents.Event) {
	//fmt.Printf("%s", event)
	data := types.Message{}
	err := json.Unmarshal(event.Data(), &data)
	if err != nil {
		fmt.Printf("Error marshalling event data %v", err)
	}
	if data.ID == 1 {
		currentBatchMaxDiff = 0
		msgCurrentBatchCount = 0
	}

	diff := time.Since(event.Context.GetTime()).Microseconds()

	if diff > maxDiff {
		maxDiff = diff
	}

	if diff > currentBatchMaxDiff {
		currentBatchMaxDiff = diff
	}
	atomic.AddUint64(&msgReceivedCount, 1)
	atomic.AddUint64(&msgCurrentBatchCount, 1)
	if (msgReceivedCount % defaultMsgCount) == 0 {
		fmt.Printf("\nCE-HTTP: Total message recived %d, maxDiff = %d\n", msgReceivedCount, maxDiff)
		fmt.Printf("\nCE-HTTP: Total current batch message recived %d, maxDiff = %d\n", msgCurrentBatchCount, currentBatchMaxDiff)
	}
	//fmt.Printf("\nTotal message recived %d\n", msgReceivedCount)

}
