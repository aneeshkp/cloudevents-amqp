package rest

import (
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"sync"
	"time"
)

type Address struct {
	Name string `json:"name"`
}

var (
	//TODO: sync.Map
	PublisherStore    = map[string]types.Subscription{}
	SubscriptionStore = map[string]types.Subscription{}
	wg                sync.WaitGroup
)

const SUBROUTINE = "/api/ocloudnotifications/v1"

type Server struct {
	cfg          protocol.Config
	address      []string
	DataOut      chan<- protocol.DataEvent
	addressStore []Address
	lock         sync.RWMutex
	HttpClient   *http.Client
}

func InitServer(hostname string, port int, pubFile, subFile string) *Server {
	server := Server{
		cfg: protocol.Config{
			HostName:    hostname,
			Port:        port,
			PubFilePath: pubFile,
			SubFilePath: subFile,
		},
		HttpClient: &http.Client{
			Timeout: 1 * time.Second,
		},
	}

	return &server
}
func (s *Server) Start() error {
	// init store
	//load publisher store
	pub := types.Subscription{}
	b, err := pub.ReadFromFile(s.cfg.PubFilePath)
	if err != nil {
		panic(err)
	}
	var pubs []types.Subscription
	if len(b) > 0 {
		if err := json.Unmarshal(b, &pubs); err != nil {
			panic(err)
		}
	}

	//load subscription store
	sub := types.Subscription{}
	b, err = sub.ReadFromFile(s.cfg.SubFilePath)
	if err != nil {
		panic(err)
	}
	var subs []types.Subscription
	if len(b) > 0 {
		if err := json.Unmarshal(b, &subs); err != nil {
			panic(err)
		}
	}

	r := mux.NewRouter()
	api := r.PathPrefix(SUBROUTINE).Subrouter()
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "api v1")
	})
	//The POST method creates a subscription resource for the (Event) API consumer.
	// SubscriptionInfo  status 201
	// Shall be returned when the subscription resource created successfully.
	/*Request
	   {
		"ResourceType": "PTP",
	    "EndpointUri ": "http://localhost:9090/resourcestatus/ptp", /// daemon
		"ResourceQualifier": {
				"NodeName":"worker-1"
				"Source":"/cluster-x/worker-1/SYNC/PTP"
			}
		}
	Response:
			{
			//"SubscriptionId": "789be75d-7ac3-472e-bbbc-6d62878aad4a",
	        "PublisherId": "789be75d-7ac3-472e-bbbc-6d62878aad4a",
			"UriLocation": "http://localhost:8080/ocloudNotifications/v1/subsciptions/789be75d-7ac3-472e-bbbc-6d62878aad4a",
			"ResourceType": "PTP",
	         "EndpointUri ": "http://localhost:9090/resourcestatus/ptp", // address where the event
				"ResourceQualifier": {
				"NodeName":"worker-1"
	              "Source":"/cluster-x/worker-1/SYNC/PTP"
			}
		}*/

	/*201 Shall be returned when the subscription resource created successfully.
		See note below.
	400 Bad request by the API consumer. For example, the endpoint URI does not include ‘localhost’.
	404 Subscription resource is not available. For example, PTP is not supported by the node.
	409 The subscription resource already exists.
	*/
	api.HandleFunc("/subscriptions", s.createSubscription).Methods(http.MethodPost)
	api.HandleFunc("/publishers", s.createPublisher).Methods(http.MethodPost)
	/*
		 this method a list of subscription object(s) and their associated properties
		200  Returns the subscription resources and their associated properties that already exist.
			See note below.
		404 Subscription resources are not available (not created).
	*/
	api.HandleFunc("/subscriptions", s.getSubscriptions).Methods(http.MethodGet)
	api.HandleFunc("/publishers", s.getPublishers).Methods(http.MethodGet)
	// 200 and 404
	api.HandleFunc("/subscriptions/{subscriptionid}", s.getSubscriptionByID).Methods(http.MethodGet)
	api.HandleFunc("/publishers/{publisherid}", s.getPublisherByID).Methods(http.MethodGet)
	// 204 on success or 404
	api.HandleFunc("/subscriptions/{subscriptionid}", s.deleteSubscription).Methods(http.MethodDelete)
	api.HandleFunc("/publishers/{publisherid}", s.deletePublisher).Methods(http.MethodDelete)

	api.HandleFunc("/subscriptions", s.deleteAllSubscriptions).Methods(http.MethodDelete)
	api.HandleFunc("/publishers", s.deleteAllPublishers).Methods(http.MethodDelete)
	// for testing only
	api.HandleFunc("/health", s.health).Methods(http.MethodGet)
	//TODO: Pull Status Notifications Not implementing

	api.HandleFunc("/event/create", s.createEvent).Methods(http.MethodPost)
	//api.HandleFunc("/event/{address}/create", s.sendEvent).Methods(http.MethodPost)
	api.HandleFunc("/", notFound)
	log.Print("Started Rest API Server")
	log.Printf("endpoint %s", SUBROUTINE)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", s.cfg.HostName, s.cfg.Port), api))
	return nil
}




func notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"message": "not found"}`))
}

func (s *Server) read(address string) []byte {
	//s.lock.RLock()
	//defer s.lock.RUnlock()
	jsonString, _ := json.Marshal(s.addressStore)
	return jsonString
}
func (s *Server) write(address string) {
	//s.lock.Lock()
	//defer s.lock.Unlock()
	s.addressStore = append(s.addressStore, Address{Name: address})
}
