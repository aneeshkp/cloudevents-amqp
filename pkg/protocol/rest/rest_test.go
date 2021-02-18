package rest_test

import (
	"bytes"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/qdr"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/rest"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"log"
	"net/http"
	"sync"
	"testing"
	"time"
)

var (
	server  *rest.Server
	router  *qdr.Router
	eventCh chan protocol.DataEvent
	wg      sync.WaitGroup
)

func init() {
	eventCh = make(chan protocol.DataEvent, 10)
}


func TestServer_New(t *testing.T) {

	// have one receiver for testing
	router = qdr.InitServer("amqp://localhost", 5672)
	router.Data=eventCh

	wg.Add(1)
	// create a receiver
	err:=router.NewReceiver("test")
	err=router.NewReceiver("test2")
	if err!=nil{
		t.Errorf("assert  error; %v ", err)
	}

	go router.Receive(&wg, "test", func(e cloudevents.Event) {
		log.Printf("Received event  %s", string(e.Data()))
	})
	go router.Receive(&wg, "test2", func(e cloudevents.Event) {
		log.Printf("Received event  %s", string(e.Data()))
	})

	//Sender sitting and waiting either to send or just create address or create address and send
	go router.Sender(&wg)

	server = rest.InitServer("localhost", 8080)
	server.Data=eventCh
	//start http server
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Start("")
	}()

	time.Sleep(3*time.Second)
	// this should actually send an event

	r, _ := http.NewRequest("PUT", "http://localhost:8080/api/v1/address/add/test", nil)
	resp, err := server.HttpClient.Do(r)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	log.Println("Creating Event now")




	var jsonStr2 = []byte(`{"title":"Buy cheese and bread for breakfast day2."}`)
	req, err := http.NewRequest("POST", "http://localhost:8080/api/v1/event/test2/create", bytes.NewBuffer(jsonStr2))
	req.Header.Set("Content-Type", "application/json")
	_, err = server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}



	/*
	r, _ = http.NewRequest("GET", "http://localhost:8080/api/v1/addresses", nil)
	resp, err = client.Do(r)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	bodyString := string(bodyBytes)
	log.Print(bodyString)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	 */

	close(eventCh)

	wg.Wait()

}
