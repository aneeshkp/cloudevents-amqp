package routes

import (
	"encoding/json"
	"github.com/aneeshkp/cloudevents-amqp/pkg/report"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"io"
	"log"
	"net/http"
	"time"
)

//CnfRoutes for routes used by CNF
type CnfRoutes struct {
	LatencyIn chan<- report.Latency
}

//PublisherAck to receive event ack SUCCESS =202
func (c *CnfRoutes) PublisherAck(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	pub := types.Subscription{}
	err := json.NewDecoder(r.Body).Decode(&pub)
	switch {
	case err == io.EOF:
		c.respondWithMessage(w, http.StatusNoContent, "No Content")
		return
	case err != nil:
		c.respondWithError(w, http.StatusBadRequest, "reading subscription data")
		return
	}
	c.respondWithMessage(w, http.StatusAccepted, "OK")
}

//EventSubmit to post events to the consumer
func (c *CnfRoutes) EventSubmit(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	sub := types.Subscription{}
	err := json.NewDecoder(r.Body).Decode(&sub)
	switch {
	case err == io.EOF:
		log.Printf("No Contect check")
		c.respondWithMessage(w, http.StatusNoContent, "No Content")
		return
	case err != nil:
		log.Printf("error here %v", err)
		c.respondWithError(w, http.StatusBadRequest, "reading subscription data")
		return
	}
	now := time.Now()
	nanos := now.UnixNano()
	// Note that there is no `UnixMillis`, so to get the
	// milliseconds since epoch you'll need to manually
	// divide from nanoseconds.
	millis := nanos / 1000000

	latency := millis - sub.EventTimestamp
	l := report.Latency{
		Time: latency,
	}

	c.LatencyIn <- l

	//w.Header().Set("Time", strconv.FormatInt(latency, 10))
	c.respondWithJSON(w, http.StatusAccepted, l)
}

func (c *CnfRoutes) respondWithError(w http.ResponseWriter, code int, message string) {
	c.respondWithJSON(w, code, map[string]string{"error": message})
}

func (c *CnfRoutes) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response) //nolint:errcheck
}

func (c *CnfRoutes) respondWithMessage(w http.ResponseWriter, code int, message string) {
	c.respondWithJSON(w, code, map[string]string{"status": message})
}

/*func (c *CnfRoutes) respondWithByte(w http.ResponseWriter, code int, message []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(message) //nolint:errcheck
}*/
