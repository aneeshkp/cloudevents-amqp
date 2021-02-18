package protocol

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/uuid"
	"time"
)

// DataEvent ...
type DataEvent struct {
	Address string
	Data    event.Event
}

/*eventBus = &EventBus{
sender: nil,
rm:     sync.RWMutex{},
data:   make(chan DataEvent, 10),
}*/

type Transporter string

const (
	QDR  Transporter = "qdr"
	HTTP             = "http"
)

type Config struct {
	HostName    string
	Port        int
	transporter Transporter
}

func GetCloudEvent(msg string) (error, cloudevents.Event) {
	event := cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
	event.SetTime(time.Now())
	event.SetType("com.cloudevents.poc.event.sent")
	err := event.SetData(cloudevents.ApplicationJSON, []byte(msg))
	return err, event
}
