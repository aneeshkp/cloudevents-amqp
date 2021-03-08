package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	routes3 "github.com/aneeshkp/cloudevents-amqp/pkg/cnf/routes"
	eventconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config"
	"github.com/aneeshkp/cloudevents-amqp/pkg/report"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/gorilla/mux"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"

	"math/rand"
	"net"
	"os"
	"time"
)

var (
	//SubStore for storing Subscription info that is returned by restapi
	SubStore                  map[string]*types.Subscription
	wg                        sync.WaitGroup
	cfg                       *eventconfig.Config
	defaultSenderSocketPort   = 30001
	defaultListenerSocketPort = 30002

	defaultAPIPort  = 8081
	defaultHostPort = 9091
	latencyCountCh  chan report.Latency
	routes          *routes3.CnfRoutes
)

// init sets initial values for variables used in the function.
func init() {
	rand.Seed(time.Now().UnixNano())
}
func httpClient() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 20,
		},
		Timeout: 10 * time.Second,
	}

	return client
}

func main() {
	var err error
	cfg, err = eventconfig.GetConfig()
	if err != nil {
		log.Printf("Could not load configuration file --config, loading default queue\n")
		cfg = eventconfig.DefaultConfig(defaultHostPort, defaultAPIPort, defaultSenderSocketPort, defaultListenerSocketPort,
			os.Getenv("MY_CLUSTER_NAME"), os.Getenv("MY_NODE_NAME"), os.Getenv("MY_NAMESPACE"))
		cfg.HostPathPrefix = "/api/vdu/v1"
		cfg.APIPathPrefix = "/api/ocloudnotifications/v1"
		cfg.StatusResource.Status.EnableStatusCheck = true
	}
	log.Printf("Framework type :%s\n", cfg.EventHandler)
	SubStore = map[string]*types.Subscription{}
	// if the event handler is socket then do this

	//Prepare for collecting latency
	latencyCountCh = make(chan report.Latency, 10)
	latencyReport := report.New(&wg, "default")
	routes = &routes3.CnfRoutes{LatencyIn: latencyCountCh}

	//Consumer vDU URL event are read from QDR by sidecar and posted to vDU

	//SIDECAR URL: ptp events are published via post to its sidecar
	//eventPublishURL = fmt.Sprintf("http://%s:%d%s/event/create", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE)
	//Start the Rest API server to read ack
	wg.Add(1)
	go func() {
		startServer(&wg, routes)
	}()
	// continue after health check
	healthCheckAPIEndpoints()
	// create various subscriptions that you are interested to subscribe to
	_, _ = createSubscription()
	//check once of often, your choice

	tck := time.NewTicker(time.Second)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var sequenceID = 1
		for range tck.C {
			event, err := checkAllStatus(sequenceID)
			if err != nil {
				log.Printf("error check ptp status %v\n", err)
			}
			log.Printf("ptp status %#v\n", event)
			sequenceID++
		}
	}()

	if cfg.EventHandler == types.SOCKET {
		socketListener(&wg, cfg.Socket.Listener.HostName, cfg.Socket.Listener.Port, latencyCountCh)
	}
	//preparing to collect latency
	log.Printf("collect latency report")
	latencyReport.Collect(latencyCountCh)
	wg.Wait()
}
func startServer(wg *sync.WaitGroup, cnfRoutes *routes3.CnfRoutes) {
	defer wg.Done()

	r := mux.NewRouter()
	api := r.PathPrefix(cfg.HostPathPrefix).Subrouter()
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "api v1")
	})
	api.HandleFunc("/publisher/ack", cnfRoutes.PublisherAck).Methods(http.MethodPost)
	api.HandleFunc("/event/alert", cnfRoutes.EventSubmit).Methods(http.MethodPost)
	api.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "OK") //nolint:errcheck
	}).Methods(http.MethodGet)
	err := r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
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

	http.Handle("/", r)

	log.Print("Started Rest API Server")

	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", cfg.Host.HostName, cfg.Host.Port), api))
}

//Side car publishers
func healthCheckAPIEndpoints() {
	//health check the webserver
	log.Printf("Health check on hosted services %s", fmt.Sprintf("http://%s:%d%s/health", cfg.Host.HostName, cfg.Host.Port, cfg.HostPathPrefix))
	log.Printf("waiting for self hosted rest service to start\n")
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", cfg.Host.HostName, cfg.Host.Port, cfg.HostPathPrefix))
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if response != nil && response.StatusCode == http.StatusOK {
			response.Body.Close()
			break
		}
		response.Body.Close()
		time.Sleep(2 * time.Second)
	}

	//health check sidecar rest api
	log.Printf("health check on rest api's %s", fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, cfg.APIPathPrefix))
	log.Printf("waiting for sidecar rest service to start\n")
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, cfg.APIPathPrefix))
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if response != nil && response.StatusCode == http.StatusOK {
			response.Body.Close()
			break
		}
		response.Body.Close()
		time.Sleep(2 * time.Second)
	}
	log.Printf("Ready...")
}

func createSubscription() (string, error) {
	log.Printf("creating subscription")
	ptpSubscription := types.Subscription{
		URILocation:  "",
		ResourceType: "ptp",
		EndpointURI:  fmt.Sprintf("http://%s:%d%s/event/alert", cfg.Host.HostName, cfg.Host.Port, cfg.HostPathPrefix),
		ResourceQualifier: types.ResourceQualifier{
			ClusterName: cfg.Cluster.Name,
			NodeName:    cfg.Cluster.Node,
			Suffix:      []string{"SYNC", "ptp"},
		},
		EventData:      types.EventDataType{},
		EventTimestamp: 0,
		Error:          "",
	}
	jsonValue, _ := json.Marshal(ptpSubscription)
	response, err := http.Post(fmt.Sprintf("http://%s:%d%s/subscriptions", cfg.API.HostName, cfg.API.Port, cfg.APIPathPrefix),
		cloudevents.ApplicationJSON, bytes.NewBuffer(jsonValue))

	if err != nil {
		log.Printf("The HTTP request failed with error %s\n", err)
		return "", err
	}
	defer response.Body.Close() // Close body only if response non-nil
	if response.StatusCode == http.StatusCreated {
		defer response.Body.Close() // Close body only if response non-nil
		data, _ := ioutil.ReadAll(response.Body)
		var sub types.Subscription
		err = json.Unmarshal(data, &sub)
		if err != nil {
			log.Printf("2.failed to successfully marshal json data from the response %v\n", err)
		} else {
			SubStore[sub.SubscriptionID] = &sub
			log.Printf("created subscription %v", SubStore)
			return sub.SubscriptionID, nil
		}
	} else {
		log.Printf("failed to create subscription %d", response.StatusCode)
		err = fmt.Errorf("error returning from http post %d", response.StatusCode)
	}
	return "", err
}

func socketListener(wg *sync.WaitGroup, hostname string, listenerPort int, latencyCountChannel chan<- report.Latency) {
	wg.Add(1)
	go func(wg *sync.WaitGroup, hostname string, udpListenerPort int, latencyCountChannel chan<- report.Latency) {
		defer wg.Done()
		ServerConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: []byte{0, 0, 0, 0}, Port: listenerPort, Zone: ""})
		if err != nil {
			log.Fatalf("Error setting up socket %v", err)
		}
		defer ServerConn.Close()
		buffer := make([]byte, 1024)
		for {
			n, _, err := ServerConn.ReadFromUDP(buffer)
			if err != nil {
				log.Printf("socket %v\n", err)
				continue
			}
			//event := cloudevents.NewEvent()
			sub := types.Subscription{}
			err = json.Unmarshal(buffer[:n], &sub)
			if err != nil {
				log.Printf("Error marshalling cloud events")
				continue
			}
			now := time.Now()
			nanos := now.UnixNano()
			// Note that there is no `UnixMillis`, so to get the
			// milliseconds since epoch you'll need to manually
			// divide from nanoseconds.
			millis := nanos / 1000000
			latency := millis - sub.EventTimestamp
			latencyCountChannel <- report.Latency{Time: latency}
		}
	}(wg, hostname, listenerPort, latencyCountChannel)

}

func checkAllStatus(index int) (event types.EventDataType, err error) {
	httpC := httpClient()
	url := fmt.Sprintf("http://%s:%d%s/status/%d", cfg.API.HostName, cfg.API.Port, cfg.APIPathPrefix, index)

	var response *http.Response
	response, err = httpC.Get(url)
	if err != nil {
		log.Printf("The HTTP request for ptp status failed with error %s\n", err)
		return
	}
	// Close body only if response non-nil
	if err != nil {
		log.Printf("3.1failed to successfully marshal ptp status json data from the response  %v ", err)
		return
	} else if response.StatusCode == http.StatusOK {
		var bytes []byte
		bytes, err = ioutil.ReadAll(response.Body)
		if err != nil {
			log.Printf("error %v", err)
			return
		}
		defer response.Body.Close()
		e := types.Subscription{}
		err = json.Unmarshal(bytes, &e)
		if err != nil {
			log.Printf("Failed to parse event data %v, for %s", err, string(bytes))
		} else {
			event = e.EventData
		}

	} else {
		log.Printf("failed to create ptp status %d url %s", response.StatusCode, url)
	}
	return
}
