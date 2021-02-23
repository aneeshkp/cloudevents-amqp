package types

import (
	"context"
	"encoding/json"
	"fmt"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"
)

type EventHandler string

const (
	UDP EventHandler="UDP"
	HTTP EventHandler="HTTP"
)
// AMQPProtocol loads clients
type AMQPProtocol struct {
	ID               string
	MsgCount         int
	Protocol         *amqp1.Protocol
	Ctx              context.Context
	ParentContext    context.Context
	CancelFn         context.CancelFunc
	Client           cloudevents.Client
	Queue            string
	MaxDiff          int64
	MinDiff          int64
	MaxDiff2         int64
	MinDiff2         int64
	CurrentMax       int64
	CurrentMax2      int64
	MsgReceivedCount uint64
}

//Result ...
type Result struct {
	ID                    string
	FromSourceMaxDiff     int64
	FromSourceMinDiff     int64
	FromSourceCurrentMax  int64
	FromSideCarMinDiff    int64
	FromSideCarMaxDiff    int64
	FromSideCarCurrentMax int64
	MsgReceivedCount      int64
	LatencyBin            map[int64]int64
}

// Message defines the DataIn of CloudEvent
type Message struct {
	// Msg holds the message from the event
	ID          int              `json:"id,omitempty,string"`
	Source      string           `json:"source,omitempty,string"`
	Msg         string           `json:"msg,omitempty,string"`
	Time        *types.Timestamp `json:"time,omitempty"`
	Probability int              `json:"probability,omitempty"`
	StateID     int              `json:"stateid,omitempty"`
}

//GetTime ...
func (m *Message) GetTime() time.Time {
	if m.Time != nil {
		return m.Time.Time
	}
	return time.Time{}
}

// SetTime implements EventContextWriter.SetTime
func (m *Message) SetTime(t time.Time) {
	if t.IsZero() {
		m.Time = nil
	} else {
		m.Time = &types.Timestamp{Time: t}
	}
}

type PubSub interface {
	ReadFromFile() (b []byte, err error)
	WriteToFile() error
	MarshalJSON() (b []byte, err error)
}

/*{
“SubscriptionId”: “789be75d-7ac3-472e-bbbc-6d62878aad4a”,
“UriLocation”: “http://localhost:8080/ocloudNotifications/v1/subsciptions/789be75d-7ac3-472e-bbbc-6d62878aad4a”,
"ResourceType": "PTP",
"EndpointUri ": "http://localhost:9090/resourcestatus/ptp",
"ResourceQualifier": {
"NodeName":"worker-1"
}
}

*/
//ResourceQualifierType ... Qualifier to scope the resource of interest, specific to each resource type.
//For example: subscribe to PTP status notifications from a particular worker node.
type ResourceQualifierType string

//PTPState PTP Synchronization state
type PTPState string

const (
	// FREERUN Clock is out of sync state
	FREERUN PTPState = "Freerun"
	// LOCKED ...Clock is in sync state
	LOCKED PTPState = "Locked"
	// HOLDOVER Clock is in holdover state
	HOLDOVER PTPState = "Holdover"
)

//The resource to subscribe to, currently only PTP is supported.
const (
	PTP ResourceQualifierType = "PTP"
)

//Subscription ...
// SubscriptionId Identifier for the created subscription resource.
// The API client can ignore it in the POST body when creating a subscription resource (this will be sent to the client after the resource is created).
// UriLocation ./subscriptions/{subscriptionId}
// The API client can ignore it in the POST body when creating a subscription resource (this will be sent to the client after the resource is created).
// See note 1 below.
// EndpointUri (a.k.a callback URI), e.g. http://localhost:8080/resourcestatus/ptp
// Please note that ‘localhost’ is a mandatory and cannot be replaced by an IP or FQDN.
type Subscription struct {
	SubscriptionId    string            `json:"subscriptionid,omitempty,string"`
	UriLocation       string            `json:"urilocation,omitempty,string"`
	ResourceType      string            `json:"resourcetype,omitempty,string"`
	EndpointUri       string            `json: "endpointuri,omitempty,string"`
	ResourceQualifier ResourceQualifier `json:"resourcequalifier,omitempty,string"`
	EventData         EventDataType     `json:"eventdata,omitempty,string"`
	EventTimestamp    int64           `json:"eventtimestamp,omitempty,string"`
	Error             string            `json:"error,omitempty,string"`
}


// EventDataType Describes the synchronization state for PTP.
//For example, "EventData": {
//”State”:”Freerun”}
type EventDataType struct {
	State PTPState `json:"state,omitempty,string"`
}

// ResourceQualifier ...The node name where PTP resides.
//‘*’ for all worker nodes
//‘.’ For worker node where the vDU resides
//Specific worker node name
type ResourceQualifier struct {
	NodeName    string   `json:"nodename,omitempty,string"`
	NameSpace string `json:"namespace,omitempty,string"`
	ClusterName string   `json:"clustername,omitempty,string"`
	Suffix      []string `json:"suffix,omitempty,string"`
}

func (r *ResourceQualifier) GetAddress() string {
	return fmt.Sprintf("/%s/%s/%s", r.NodeName, r.ClusterName, strings.Join(r.Suffix, "/"))
}


func (s *Subscription) WriteToFile(filePath string) error {
	//open file
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	defer file.Close()
	if err != nil {
		return err
	}

	//read file and unmarshall json file to slice of users
	b, err := ioutil.ReadAll(file)
	var allSubs []Subscription
	if len(b) > 0 {
		err = json.Unmarshal(b, &allSubs)
		if err != nil {
			log.Println("Error is here")
			return err
		}
	}

	allSubs = append(allSubs, *s)
	newBytes, err := json.MarshalIndent(&allSubs, "", " ")
	if err != nil {
		log.Println("No Error is here")
		return err
	}
	ioutil.WriteFile(filePath, newBytes, 0666)
	return nil

}
func (s *Subscription) DeleteAllFromFile(filePath string) error {
	//open file
	ioutil.WriteFile(filePath, []byte{}, 0666)
	return nil

}
func (s *Subscription) DeleteFromFile(filePath string) error {
	//open file
	file, err := os.OpenFile(filePath,  os.O_CREATE|os.O_RDWR, 0644)
	defer file.Close()
	if err != nil {
		return err
	}

	//read file and unmarshall json file to slice of users
	b, err := ioutil.ReadAll(file)
	var allSubs []Subscription
	if len(b) > 0 {
		err = json.Unmarshal(b, &allSubs)
		if err != nil {
			log.Println("Error is here")
			return err
		}
	}
	for k := range allSubs {
		// Remove the element at index i from a.
		if allSubs[k].SubscriptionId == s.SubscriptionId {
			allSubs[k] = allSubs[len(allSubs)-1]     // Copy last element to index i.
			allSubs[len(allSubs)-1] = Subscription{} // Erase last element (write zero value).
			allSubs = allSubs[:len(allSubs)-1]       // Truncate slice.
			break
		}
	}
	newUserBytes, err := json.MarshalIndent(&allSubs, "", " ")
	if err != nil {
		log.Printf("error deleting sub %v", err)
		return err
	}
	ioutil.WriteFile(filePath, newUserBytes, 0666)
	return nil

}
func (s *Subscription) ReadFromFile(filePath string) (b []byte, err error) {
	//open file
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR,  0644)
	defer file.Close()
	if err != nil {
		return nil, err
	}

	//read file and unmarshall json file to slice of users
	b, err = ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return b, nil

}

/*
{
    "SubscriptionId”: “789be75d-7ac3-472e-bbbc-6d62878aad4a",
	"ResourceType": "PTP ",
	"ResourceQualifier": {
		"NodeName": "worker-1"
        "source": "/cluster-x/nodename/SYNC/PTP"
	},
	"EventData": {
		"State": "Freerun"
	},
	"EventTimestamp": "43232432423"
}
*/
