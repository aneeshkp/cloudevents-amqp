package types

import (
	"context"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"io/ioutil"
	"os"
	"strings"
)

// EventHandler types of SOCKET and HTTP
type EventHandler string

const (
	// SOCKET defines type of framework for sending events
	SOCKET EventHandler = "SOCKET"
	// HTTP defines type of framework for sending events
	HTTP EventHandler = "HTTP"
)

// AMQPProtocol loads clients
type AMQPProtocol struct {
	ID            string
	MsgCount      int
	Protocol      *amqp1.Protocol
	Ctx           context.Context
	ParentContext context.Context
	CancelFn      context.CancelFunc
	Client        cloudevents.Client
	Queue         string
	DataIn        <-chan protocol.DataEvent
	DataOut       chan<- protocol.DataEvent
}

/*{
“SubscriptionID”: “789be75d-7ac3-472e-bbbc-6d62878aad4a”,
“URILocation”: “http://localhost:8080/ocloudNotifications/v1/subsciptions/789be75d-7ac3-472e-bbbc-6d62878aad4a”,
"ResourceType": "PTP",
"EndpointURI ": "http://localhost:9090/resourcestatus/ptp",
"ResourceQualifier": {
"NodeName":"worker-1"
}
}
*/

// ResourceQualifierType ... Qualifier to scope the resource of interest, specific to each resource type.
// For example: subscribe to PTP status notifications from a particular worker node.
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

	//ERROR Clock is in holdover state
	ERROR PTPState = "Error reading PTP state"
)

//The resource to subscribe to, currently only PTP is supported.
const (
	PTP ResourceQualifierType = "PTP"
)

//Subscription ...
// SubscriptionID Identifier for the created subscription resource.
// The API client can ignore it in the POST body when creating a subscription resource (this will be sent to the client after the resource is created).
// URILocation ./subscriptions/{subscriptionId}
// The API client can ignore it in the POST body when creating a subscription resource (this will be sent to the client after the resource is created).
// See note 1 below.
// EndpointURI (a.k.a callback URI), e.g. http://localhost:8080/resourcestatus/ptp
// Please note that ‘localhost’ is a mandatory and cannot be replaced by an IP or FQDN.
type Subscription struct {
	SubscriptionID string `json:"subscriptionid,omitempty,string"`
	URILocation    string `json:"urilocation,omitempty,string"`
	ResourceType   string `json:"resourcetype,omitempty,string"`
	EndpointURI    string `json:"endpointuri,omitempty,string"`
	//nolint:staticcheck
	ResourceQualifier ResourceQualifier `json:"resourcequalifier,omitempty,string"`
	//nolint:staticcheck
	EventData      EventDataType `json:"eventdata,omitempty,string"`
	EventTimestamp int64         `json:"eventtimestamp,omitempty,string"`
	Error          string        `json:"error,omitempty,string"`
}

// EventDataType Describes the synchronization state for PTP.
//For example, "EventData": {
//”State”:”Freerun”}
type EventDataType struct {
	SequenceID int      `json:"sequenceid,omitempty,int"` //nolint:staticcheck
	State      PTPState `json:"state,omitempty,string"`
	PTPStatus  string   `json:"ptpstatus,omitempty,string"`
}

// ResourceQualifier ...The node name where PTP resides.
//‘*’ for all worker nodes
//‘.’ For worker node where the vDU resides
//Specific worker node name
type ResourceQualifier struct {
	NodeName    string `json:"nodename,omitempty,string"`
	NameSpace   string `json:"namespace,omitempty,string"`
	ClusterName string `json:"clustername,omitempty,string"`
	//nolint:staticcheck
	Suffix []string `json:"suffix,omitempty,string"`
}

// GetAddress return qdr address
func (r *ResourceQualifier) GetAddress() string {
	return fmt.Sprintf("/%s/%s/%s", r.ClusterName, r.NodeName, strings.Join(r.Suffix, "/"))
}

// SetAddress sets address from string
func (r *ResourceQualifier) SetAddress(address string) {
	splitFn := func(c rune) bool {
		return c == '/'
	}
	s := strings.FieldsFunc(address, splitFn)
	r.Suffix = []string{}
	for i, v := range s {
		if i == 0 {
			r.ClusterName = s[i]
		} else if i == 1 {
			r.NodeName = s[i]
		} else {
			r.Suffix = append(r.Suffix, v)
		}
	}
}

// ReadFromFile is used to read subscription from the file system
func (s *Subscription) ReadFromFile(filePath string) (b []byte, err error) {
	//open file
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	//read file and unmarshall json file to slice of users
	b, err = ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return b, nil
}

//Message ...
type Message struct {
	ID      int
	Message string
}

//ResourceStatus is used for PTP status update
type ResourceStatus struct {
	ReturnAddress string `json:"returnaddress,omitempty,string"`
	Status        string `json:"status,omitempty,string"`
	Message       string `json:"message,omitempty,string"`
}

/*
{
    "SubscriptionID”: “789be75d-7ac3-472e-bbbc-6d62878aad4a",
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
