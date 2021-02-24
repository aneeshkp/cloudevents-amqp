package rest

import (
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
)

//createSubscription The POST method creates a subscription resource for the (Event) API consumer.
// SubscriptionInfo  status 201
// Shall be returned when the subscription resource created successfully.
/*Request
   {
	"ResourceType": "PTP",
	"SourceAddress":"/cluster-x/worker-1/SYNC/PTP",
    "EndpointURI ": "http://localhost:9090/resourcestatus/ptp", /// daemon
	"ResourceQualifier": {
			"NodeName":"worker-1"
		}
	}
Response:
		{
		//"SubscriptionID": "789be75d-7ac3-472e-bbbc-6d62878aad4a",
        "PublisherId": "789be75d-7ac3-472e-bbbc-6d62878aad4a",
        "SourceAddress":"/cluster-x/worker-1/SYNC/PTP",
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
func (s *Server) createSubscription(w http.ResponseWriter, r *http.Request) {
	s.createPubSub(w, r, "subscriptions", s.cfg.SubFilePath, protocol.CONSUMER)
}

func (s *Server) createPublisher(w http.ResponseWriter, r *http.Request) {
	s.createPubSub(w, r, "publishers", s.cfg.PubFilePath, protocol.PRODUCER)
}
func (s *Server) createPubSub(w http.ResponseWriter, r *http.Request, resourcePath string, filePath string, psType protocol.PubSubType) {
	log.Println("Entering create pub/sub")
	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sub := types.Subscription{}
	if err := json.Unmarshal(bodyBytes, &sub); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	//TODO: Do a get to call back address to make sure it works
	//check sub.EndpointURI by get
	sub.SubscriptionID = uuid.New().String()
	sub.URILocation = fmt.Sprintf("http://%s:%d%s/%s/%s", s.cfg.HostName, s.cfg.Port, SUBROUTINE, resourcePath, sub.SubscriptionID)

	w.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(&sub)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("Subscription %v", sub)
	// persist the subscription -
	//TODO:might want to use PVC to live beyond pod crash
	err = sub.WriteToFile(filePath)
	log.Printf("What was wriiter to file %v\n", sub)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}
	log.Println("Stored in a file")
	//store the subscription
	if resourcePath == "publishers" {
		PublisherStore[sub.SubscriptionID] = sub
	} else {
		SubscriptionStore[sub.SubscriptionID] = sub
	}
	s.dataOut <- protocol.DataEvent{
		Address:     sub.ResourceQualifier.GetAddress(),
		Data:        event.Event{},
		PubSubType:  psType,
		EndPointURI: sub.EndpointURI,
	}
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(b)
}

func (s *Server) getSubscriptionByID(w http.ResponseWriter, r *http.Request) {
	queries := mux.Vars(r)
	subscriptionID, ok := queries["subscriptionid"]
	w.Header().Set("Content-Type", "application/json")
	if ok {
		log.Printf("Getting subscription by id %s", subscriptionID)
		if sub, ok := SubscriptionStore[subscriptionID]; ok {
			w.WriteHeader(http.StatusOK)
			// w.Write([]byte(fmt.Sprintf("%v", sub))) //Dont use this .. this is ubuggy
			b, err := json.Marshal(&sub)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write(b)
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}

func (s *Server) getPublisherByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	queries := mux.Vars(r)
	PublisherID, ok := queries["publisherid"]
	w.Header().Set("Content-Type", "application/json")
	if ok {
		log.Printf("Getting subscription by id %s", PublisherID)
		if sub, ok := PublisherStore[PublisherID]; ok {
			w.WriteHeader(http.StatusOK)
			b, err := json.Marshal(&sub)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write(b)
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}
func (s *Server) getSubscriptions(w http.ResponseWriter, r *http.Request) {
	s.getPubSub(w, r, s.cfg.SubFilePath)
}

func (s *Server) getPublishers(w http.ResponseWriter, r *http.Request) {
	s.getPubSub(w, r, s.cfg.PubFilePath)
}

func (s *Server) getPubSub(w http.ResponseWriter, r *http.Request, filepath string) {
	var pubSub types.Subscription
	b, err := pubSub.ReadFromFile(filepath)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (s *Server) deletePublisher(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	queries := mux.Vars(r)
	PublisherID, ok := queries["publisherid"]
	w.Header().Set("Content-Type", "application/json")
	if ok {

		if pub, ok := PublisherStore[PublisherID]; ok {
			if err := pub.DeleteFromFile(s.cfg.PubFilePath); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			delete(PublisherStore, PublisherID)
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}

func (s *Server) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	queries := mux.Vars(r)
	subscriptionID, ok := queries["subscriotionid"]
	w.Header().Set("Content-Type", "application/json")
	if ok {

		if sub, ok := SubscriptionStore[subscriptionID]; ok {
			if err := sub.DeleteFromFile(s.cfg.SubFilePath); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			delete(PublisherStore, subscriptionID)
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}
func (s *Server) deleteAllSubscriptions(w http.ResponseWriter, r *http.Request) {
	var sub types.Subscription
	err := sub.DeleteAllFromFile(s.cfg.SubFilePath)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	//empty the store
	SubscriptionStore = make(map[string]types.Subscription)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) deleteAllPublishers(w http.ResponseWriter, r *http.Request) {
	var pubSub types.Subscription
	err := pubSub.DeleteAllFromFile(s.cfg.PubFilePath)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	//empty the store
	PublisherStore = make(map[string]types.Subscription)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	pub := types.Subscription{}
	if err := json.Unmarshal(bodyBytes, &pub); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	pub.SubscriptionID = uuid.New().String()
	w.Header().Set("Content-Type", "application/json")
	var eventData []byte
	if eventData, err = json.Marshal(&pub); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	event, err := protocol.GetCloudEvent(eventData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		s.dataOut <- protocol.DataEvent{PubSubType: protocol.PRODUCER,
			Data: event, Address: pub.ResourceQualifier.GetAddress(), EndPointURI: pub.EndpointURI}
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
