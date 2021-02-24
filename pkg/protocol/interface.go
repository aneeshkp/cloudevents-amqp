package protocol

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/uuid"
	"time"
)

//PubSubType specifies type to identify role played by the side car
type PubSubType string

const (
	//PRODUCER is type of cnf either producer or consumer
	PRODUCER PubSubType = "producer"
	//CONSUMER is type of cnf either producer or consumer
	CONSUMER PubSubType = "consumer"
)

//EventStatus specifies status of the event
type EventStatus int

const (
	// SUCCEED if the event is posted successfully
	SUCCEED EventStatus = 1
	//FAILED if the event  failed to post
	FAILED EventStatus = 2
	//NEW if the event is new for the consumer
	NEW EventStatus = 0
)

// DataEvent ...
type DataEvent struct {
	Address     string
	Data        event.Event
	PubSubType  PubSubType
	EventStatus EventStatus
	EndPointURI string
	//fn          func(e cloudevents.Event) //nolint:structcheck
}

//Config stores config data
type Config struct {
	HostName    string
	Port        int
	PubFilePath string //pub.json
	SubFilePath string //sub.json
}

//GetCloudEvent return data wrapped in a cloud events object
func GetCloudEvent(msg []byte) (cloudevents.Event, error) {
	event := cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
	event.SetTime(time.Now())
	event.SetType("com.cloudevents.poc.event.sent")
	err := event.SetData(cloudevents.ApplicationJSON, msg)
	return event, err
}
