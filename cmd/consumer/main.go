package main
import (
"context"
"fmt"
"log"
"net/url"
"os"
"strings"
	"time"

	"github.com/Azure/go-amqp"

amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
cloudevents "github.com/cloudevents/sdk-go/v2"
)

// Parse AMQP_URL env variable. Return server URL, AMQP node (from path) and SASLPlain
// option if user/pass are present.
func amqpConfig() (server, node string, opts []amqp1.Option) {
	env := os.Getenv("AMQP_URL")
	if env == "" {
		env = "/test"
	}
	u, err := url.Parse(env)
	if err != nil {
		log.Fatal(err)
	}
	if u.User != nil {
		user := u.User.Username()
		pass, _ := u.User.Password()
		opts = append(opts, amqp1.WithConnOpt(amqp.ConnSASLPlain(user, pass)))
	}
	return env, strings.TrimPrefix(u.Path, "/"), opts
}

func main() {
	host, node, opts := amqpConfig()

	var p *amqp1.Protocol
	var err error
	log.Printf("Connecting to host %s", host)
	for {
		p, err = amqp1.NewProtocol(host, node, []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
		if err != nil {
			log.Printf("Failed to create amqp protocol (trying in 5 secs): %v", err)
			time.Sleep(5 * time.Second)
		}else{
			log.Print("Connection established for consumer")
			break
		}
	}

	// Close the connection when finished
	defer p.Close(context.Background())

	// Create a new client from the given protocol
	c, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	err = c.StartReceiver(context.Background(), func(e cloudevents.Event) {
		fmt.Printf("==== Got CloudEvent\n%+v\n----\n", e)

	})
	if err != nil {
		log.Printf("AMQP receiver error: %v", err)
	}
	log.Print("End Consumer")
}
