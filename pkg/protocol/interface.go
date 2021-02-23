package protocol

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/uuid"
	"time"
)

type PubSubType string
const (
	PRODUCER PubSubType ="producer"
	CONSUMER PubSubType ="consumer"
)

type EventStatus int
const (
	SUCCEED  EventStatus =1
	FAILED EventStatus = 2
	NEW EventStatus = 0
)

// DataEvent ...
type DataEvent struct {
	Address string
	Data    event.Event
	PubSubType PubSubType
	EventStatus EventStatus
	EndPointURi string
	fn func(e cloudevents.Event)
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
	PubFilePath string //pub.json
	SubFilePath string //sub.json
}

func GetCloudEvent(msg []byte) (error, cloudevents.Event) {
	event := cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
	event.SetTime(time.Now())
	event.SetType("com.cloudevents.poc.event.sent")
	err := event.SetData(cloudevents.ApplicationJSON, msg)
	return err, event
}
