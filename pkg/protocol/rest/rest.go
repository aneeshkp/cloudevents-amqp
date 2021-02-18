package rest

import (
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

type Address struct {
	Name string `json:"name"`
}

type Server struct {
	cfg          protocol.Config
	address      []string
	Data         chan<- protocol.DataEvent
	addressStore []Address
	lock         sync.RWMutex
	HttpClient   *http.Client
}

func InitServer(hostname string, port int) *Server {
	server := Server{
		cfg: protocol.Config{
			HostName: hostname,
			Port:     port,
		},
		HttpClient: &http.Client{
			Timeout: 1 * time.Second,
		},
	}

	return &server
}
func (s *Server) Start(address string) error {
	r := mux.NewRouter()
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "api v1")
	})

	api.HandleFunc("/addresses", s.getAllAddresses).Methods(http.MethodGet)
	api.HandleFunc("/address/add/{address}", s.addAddress).Methods(http.MethodPut)
	api.HandleFunc("/event/create", s.sendEventToAll).Methods(http.MethodPost)
	api.HandleFunc("/event/{address}/create", s.sendEvent).Methods(http.MethodPost)
	api.HandleFunc("/", notFound)
	log.Print("Started Rest API Server")
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", s.cfg.HostName, s.cfg.Port), api))
	return nil
}

func (s *Server) getAllAddresses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"message": %v}`, s.ListAddress())))
}

func (s *Server) addAddress(w http.ResponseWriter, r *http.Request) {
	queries := mux.Vars(r)
	val, ok := queries["address"]
	w.Header().Set("Content-Type", "application/json")
	if ok {
		log.Printf("adding address %s", val)
		s.write(val)
		s.Data <- protocol.DataEvent{Address: val}
		log.Printf("status %d", http.StatusOK)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": "address added"}`))
		return
	}
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"error": "not found"}`))
	return
	// DO AMAZING MATH HERE
	// and return the result
}
func (s *Server) sendEvent(w http.ResponseWriter, r *http.Request) {
	queries := mux.Vars(r)
	address, addressOk := queries["address"]
	if !addressOk {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "creating cloud event"}`))
		return
	}
	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "creating cloud event"}`))
		return
	}
	bodyString := string(bodyBytes)
	w.Header().Set("Content-Type", "application/json")

	err, event := protocol.GetCloudEvent(bodyString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "creating cloud event"}`))
	} else {
		s.Data <- protocol.DataEvent{Data: event, Address: address}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": "event sent"}`))
	}

	return
	// DO AMAZING MATH HERE
	// and return the result
}
func (s *Server) sendEventToAll(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "creating cloud event"}`))
		return
	}
	bodyString := string(bodyBytes)

	err, event := protocol.GetCloudEvent(bodyString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "creating cloud event"}`))
	} else {
		s.Data <- protocol.DataEvent{Data: event}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": "event sent"}`))
	}
	return

	// DO AMAZING MATH HERE
	// and return the result
}

func (s *Server) RemoveAddress(address string) {
	// DO AMAZING MATH HERE
	// and return the result
}
func (s *Server) ListAddress() string {
	jsonString, _ := json.Marshal(s.addressStore)
	return string(jsonString)
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
