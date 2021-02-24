package routes

import (
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

//CnfRoutes for routes used by CNF
type CnfRoutes struct {
}

//EventAck to receive event ack
func (c CnfRoutes) EventAck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	queries := mux.Vars(r)
	eventID, ok := queries["eventid"]
	if ok {
		log.Println(eventID)
	}
	w.WriteHeader(http.StatusAccepted)
}

//EventAlert to post events to the consumer
func (c CnfRoutes) EventAlert(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "reading request"}`))
		return
	}
	sub := types.Subscription{}
	if err := json.Unmarshal(bodyBytes, &sub); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "reading subscription data "}`))
		return
	}

	now := time.Now()
	nanos := now.UnixNano()
	// Note that there is no `UnixMillis`, so to get the
	// milliseconds since epoch you'll need to manually
	// divide from nanoseconds.
	millis := nanos / 1000000

	w.WriteHeader(http.StatusAccepted)
	latency := millis - sub.EventTimestamp
	_, _ = w.Write([]byte(fmt.Sprintf(`{"time":%d}`, latency)))
}

//Health return the status of the webservice
func (c CnfRoutes) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
