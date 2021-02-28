package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/qdr"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
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
	s.createPubSub(w, r, "subscriptions", s.cfg.Store.SubFilePath, protocol.CONSUMER)
}

func (s *Server) createPublisher(w http.ResponseWriter, r *http.Request) {
	s.createPubSub(w, r, "publishers", s.cfg.Store.PubFilePath, protocol.PRODUCER)
}
func (s *Server) createPubSub(w http.ResponseWriter, r *http.Request, resourcePath string, filePath string, psType protocol.PubSubType) {
	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sub := types.Subscription{}
	if err := json.Unmarshal(bodyBytes, &sub); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "marshalling error")
		return
	}
	//prevent duplicate creation
	if psType == protocol.CONSUMER {
		if exists, err := s.GetFromSubStore(sub.ResourceQualifier.GetAddress()); err == nil {
			log.Printf("There was already subscription,skipping creation %v", exists)
			s.sendOut(psType, &sub)
			s.respondWithJSON(w, http.StatusCreated, exists)
			return
		}
	} else if psType == protocol.PRODUCER {
		if exists, err := s.GetFromPubStore(sub.ResourceQualifier.GetAddress()); err == nil {
			log.Printf("There was already publisher,skipping creation %v", exists)
			s.sendOut(psType, &sub)
			s.respondWithJSON(w, http.StatusCreated, exists)
			return
		}
	}
	//TODO: Do a get to call back address to make sure it works
	if sub.EndpointURI != "" {
		response, err := http.Post(sub.EndpointURI, "application/json", nil)
		if err != nil {
			log.Printf("There was error validating endpointurl %v", err)
			s.respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			log.Printf("There was error validating endpointurl returned status code %d", response.StatusCode)
			s.respondWithError(w, http.StatusBadRequest, "Return url validation check failed for create subscription.check endpointURI")
			return
		}
	}

	//check sub.EndpointURI by get
	sub.SubscriptionID = uuid.New().String()
	sub.URILocation = fmt.Sprintf("http://%s:%d%s/%s/%s", s.cfg.API.HostName, s.cfg.API.Port, s.cfg.APIPathPrefix, resourcePath, sub.SubscriptionID)

	// persist the subscription -
	//TODO:might want to use PVC to live beyond pod crash
	err = s.writeToFile(sub, filePath)
	if err != nil {
		log.Printf("Error writing to store %v\n", err)
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Println("Stored in a file")
	//store the subscription
	if psType == protocol.CONSUMER {
		s.subscription.Set(sub.SubscriptionID, &sub)
	} else {
		s.publisher.Set(sub.SubscriptionID, &sub)
	}
	// go ahead and create QDR to this address
	s.sendOut(psType, &sub)
	s.respondWithJSON(w, http.StatusCreated, sub)
}

func (s *Server) sendOut(psType protocol.PubSubType, sub *types.Subscription) {
	// go ahead and create QDR to this address
	s.dataOut <- protocol.DataEvent{
		Address:     sub.ResourceQualifier.GetAddress(),
		Data:        event.Event{},
		PubSubType:  psType,
		EndPointURI: sub.EndpointURI,
		EventStatus: protocol.NEW,
	}
}
func (s *Server) getSubscriptionByID(w http.ResponseWriter, r *http.Request) {
	queries := mux.Vars(r)
	subscriptionID, ok := queries["subscriptionid"]
	if ok {
		log.Printf("getting subscription by id %s", subscriptionID)
		if sub, ok := s.subscription.Store[subscriptionID]; ok {
			s.respondWithJSON(w, http.StatusOK, sub)
			return
		}
	}
	s.respondWithError(w, http.StatusBadRequest, "Subscriptions not found")
}

func (s *Server) getPublisherByID(w http.ResponseWriter, r *http.Request) {
	queries := mux.Vars(r)
	PublisherID, ok := queries["publisherid"]
	if ok {
		log.Printf("Getting subscription by id %s", PublisherID)
		if pub, ok := s.publisher.Store[PublisherID]; ok {
			s.respondWithJSON(w, http.StatusOK, pub)
			return
		}
	}
	s.respondWithError(w, http.StatusBadRequest, "Publisher not found")
}
func (s *Server) getSubscriptions(w http.ResponseWriter, r *http.Request) {
	s.getPubSub(w, r, s.cfg.Store.SubFilePath)
}

func (s *Server) getPublishers(w http.ResponseWriter, r *http.Request) {
	s.getPubSub(w, r, s.cfg.Store.PubFilePath)
}

func (s *Server) getPubSub(w http.ResponseWriter, r *http.Request, filepath string) {
	var pubSub types.Subscription
	b, err := pubSub.ReadFromFile(filepath)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "error reading file")
		return
	}
	s.respondWithByte(w, http.StatusOK, b)
}

func (s *Server) deletePublisher(w http.ResponseWriter, r *http.Request) {
	queries := mux.Vars(r)
	PublisherID, ok := queries["publisherid"]
	if ok {

		if pub, ok := s.publisher.Store[PublisherID]; ok {
			if err := s.deleteFromFile(*pub, s.cfg.Store.PubFilePath); err != nil {
				s.respondWithError(w, http.StatusBadRequest, err.Error())
				return
			}
			s.publisher.Delete(PublisherID)
			s.respondWithMessage(w, http.StatusOK, "OK")
			return
		}
	}
	//TODO: close QDR connection for this --> use same method as create
	s.respondWithError(w, http.StatusBadRequest, "publisherid param is missing")
}

func (s *Server) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	queries := mux.Vars(r)
	subscriptionID, ok := queries["subscriotionid"]
	if ok {

		if sub, ok := s.subscription.Store[subscriptionID]; ok {
			if err := s.deleteFromFile(*sub, s.cfg.Store.SubFilePath); err != nil {
				s.respondWithError(w, http.StatusBadRequest, err.Error())
				return
			}

			s.publisher.Delete(subscriptionID)
			s.respondWithMessage(w, http.StatusOK, "Deleted")
			return
		}
	}
	//TODO: close QDR connection for this --> use same method as create
	s.respondWithError(w, http.StatusBadRequest, "subscriotionid param is missing")
}
func (s *Server) deleteAllSubscriptions(w http.ResponseWriter, r *http.Request) {
	err := s.deleteAllFromFile(s.cfg.Store.SubFilePath)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	//empty the store
	s.subscription.Store = make(map[string]*types.Subscription)
	//TODO: close QDR connection for this --> use same method as create
	s.respondWithMessage(w, http.StatusOK, "deleted all subscriptions")
}

func (s *Server) deleteAllPublishers(w http.ResponseWriter, r *http.Request) {
	err := s.deleteAllFromFile(s.cfg.Store.PubFilePath)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	//empty the store
	s.publisher.Store = make(map[string]*types.Subscription)
	//TODO: close QDR connection for this --> use same method as create
	s.respondWithMessage(w, http.StatusOK, "deleted all publishers")
}

// getResourceStatus send cloud events object requesting for status
func (s *Server) getResourceStatus(w http.ResponseWriter, r *http.Request) {
	msgRequest := getStatusMessage()

	//build address
	queries := mux.Vars(r)
	index, ok := queries["index"]
	var i int
	var err error
	if ok {
		i, err = strconv.Atoi(index)
		if err != nil {
			s.respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if ok && len(s.cfg.StatusResource.Name) > i {

		senderAddress := fmt.Sprintf("/%s/%s/%s", s.cfg.Cluster.Name, s.cfg.Cluster.Node, s.cfg.StatusResource.Name[i])
		receiveAddress := fmt.Sprintf("/%s/%s/%s", s.cfg.Cluster.Name, s.cfg.Cluster.Node, "CurrentStatus")

		event := cloudevents.NewEvent()
		event = cloudevents.NewEvent()
		event.SetID(uuid.New().String())
		event.SetSource("https://github.com/aneeshkp/cloud-events/vdu")
		event.SetTime(time.Now())
		event.SetType("com.cloudevents.poc.ptp.status")
		event.SetSubject("PTPCurrentStatus")
		event.SetSpecVersion(cloudevents.VersionV1)
		status := types.ResourceStatus{
			ReturnAddress: receiveAddress,
			Status:        "",
			Message:       msgRequest[rand.Intn(len(msgRequest))],
		}
		_ = event.SetData(cloudevents.ApplicationJSON, status)

		log.Printf("starting a listener at webserver %s\n", receiveAddress)
		listener, err := qdr.NewReceiver(s.cfg.AMQP.HostName, s.cfg.AMQP.Port, receiveAddress)
		if err != nil {
			log.Printf("Error Dialing AMQP server::%v", err)
			s.respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}
		sender, _ := qdr.NewSender(s.cfg.AMQP.HostName, s.cfg.AMQP.Port, senderAddress)

		listenerCtx, listenerCancel := context.WithTimeout(context.Background(), 5*time.Second)
		senderCtx, senderCancel := context.WithTimeout(context.Background(), 5*time.Second)

		defer func() {
			listenerCancel()
			senderCancel()
			listener.Protocol.Close(listenerCtx)
			sender.Protocol.Close(senderCtx)
		}()

		var wg sync.WaitGroup
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			err = listener.Client.StartReceiver(listenerCtx, func(e cloudevents.Event) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(e)
			})
			if err != nil {
				s.respondWithError(w, http.StatusBadRequest, err.Error())
				return
			}

		}(&wg)

		if result := sender.Client.Send(senderCtx, event); cloudevents.IsUndelivered(result) {
			log.Printf("method:getResourceStatus, error:failed to send status: %v", result)
			s.respondWithJSON(w, http.StatusBadRequest, result)
			return
		} else if cloudevents.IsNACK(result) {
			log.Printf("Event not accepted: %v", result)
			s.respondWithJSON(w, http.StatusBadRequest, result)
			return
		}
		wg.Wait()
	} else {
		s.respondWithError(w, http.StatusBadRequest, "Resource not found")
		return
	}

}

func (s *Server) publishEvent(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	pub := types.Subscription{}
	if err := json.Unmarshal(bodyBytes, &pub); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	pub.SubscriptionID = uuid.New().String()
	var eventData []byte
	if eventData, err = json.Marshal(&pub); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, err := protocol.GetCloudEvent(eventData)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
	} else {
		s.dataOut <- protocol.DataEvent{
			PubSubType:  protocol.EVENT,
			Data:        event,
			Address:     pub.ResourceQualifier.GetAddress(),
			EndPointURI: pub.EndpointURI}
		s.respondWithMessage(w, http.StatusAccepted, "Event published")
	}
}

func (s *Server) respondWithError(w http.ResponseWriter, code int, message string) {
	s.respondWithJSON(w, code, map[string]string{"error": message})
}

func (s *Server) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response) //nolint:errcheck
}
func (s *Server) respondWithMessage(w http.ResponseWriter, code int, message string) {
	s.respondWithJSON(w, code, map[string]string{"status": message})
}

func (s *Server) respondWithByte(w http.ResponseWriter, code int, message []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(message) //nolint:errcheck
}

func getStatusMessage() []string {
	return []string{"This is Houston. Say again, please.", "Roger. MAIN B UNDERVOLT.", "Okay, stand by, 13. We're looking at it.",
		"Roger", "Roger. RESTART and a PGNCS light. RESET on a PGNCS, RESET —", "MAIN A UNDERVOLT.", "Roger",
	}
}
