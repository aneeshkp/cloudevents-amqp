package rest

import (
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/config"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/gorilla/mux"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Address of teh QDR
type Address struct {
	// Name of teh QDR address
	Name string `json:"name"`
}

var (
	//TODO: sync.Map

	// PublisherStore stores publishers in a map
	PublisherStore = map[string]types.Subscription{}
	// SubscriptionStore stores subscription in a map
	SubscriptionStore = map[string]types.Subscription{}
)



// Server defines rest api server object
type Server struct {
	cfg        *config.Config
	dataOut    chan<- protocol.DataEvent
	HTTPClient *http.Client
}

// InitServer is used to supply configurations for rest api server
func InitServer(cfg *config.Config, dataOut chan<- protocol.DataEvent) *Server {

	server := Server{
		cfg:     cfg,
		dataOut: dataOut,
		HTTPClient: &http.Client{
			Timeout: 1 * time.Second,
		},
	}

	return &server
}

// Start will start res api service
func (s *Server) Start() {
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
	api := r.PathPrefix(s.cfg.APIPathPrefix).Subrouter()

	//The POST method creates a subscription resource for the (Event) API consumer.
	// SubscriptionInfo  status 201
	// Shall be returned when the subscription resource created successfully.
	/*Request
	   {
		"ResourceType": "PTP",
	    "EndpointURI ": "http://localhost:9090/resourcestatus/ptp", /// daemon
		"ResourceQualifier": {
				"NodeName":"worker-1"
				"Source":"/cluster-x/worker-1/SYNC/PTP"
			}
		}
	Response:
			{
			//"SubscriptionID": "789be75d-7ac3-472e-bbbc-6d62878aad4a",
	        "PublisherId": "789be75d-7ac3-472e-bbbc-6d62878aad4a",
			"URILocation": "http://localhost:8080/ocloudNotifications/v1/subsciptions/789be75d-7ac3-472e-bbbc-6d62878aad4a",
			"ResourceType": "PTP",
	         "EndpointURI ": "http://localhost:9090/resourcestatus/ptp", // address where the event
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

	api.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "OK")
	}).Methods(http.MethodGet)

	//TODO: Pull Status Notifications Not implementing

	api.HandleFunc("/create/event", s.publishEvent).Methods(http.MethodPost)
	api.HandleFunc("/status", s.getResourceStatus).Methods(http.MethodPost)

	err = r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err == nil {
			fmt.Println("ROUTE:", pathTemplate)
		}
		pathRegexp, err := route.GetPathRegexp()
		if err == nil {
			fmt.Println("Path regexp:", pathRegexp)
		}
		queriesTemplates, err := route.GetQueriesTemplates()
		if err == nil {
			fmt.Println("Queries templates:", strings.Join(queriesTemplates, ","))
		}
		queriesRegexps, err := route.GetQueriesRegexp()
		if err == nil {
			fmt.Println("Queries regexps:", strings.Join(queriesRegexps, ","))
		}
		methods, err := route.GetMethods()
		if err == nil {
			fmt.Println("Methods:", strings.Join(methods, ","))
		}
		fmt.Println()
		return nil
	})

	if err != nil {
		fmt.Println(err)
	}
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, r)
	})


	log.Print("Started Rest API Server")
	log.Printf("endpoint %s", s.cfg.APIPathPrefix)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", s.cfg.API.HostName, s.cfg.API.Port), api))

}

func notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
}
