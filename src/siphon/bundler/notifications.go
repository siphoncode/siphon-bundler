package bundler

import (
	"log"
	"os"

	"github.com/streadway/amqp"
)

var (
	amqpURI = "amqp://" + os.Getenv("RABBITMQ_HOST") + ":" +
		os.Getenv("RABBITMQ_PORT")
	amqpExchange     = "siphon.apps.notifications"
	amqpExchangeType = "fanout"
	amqpConsumerTag  = "siphon-bundler"
)

// PostAppUpdated sends an app_updated notification on the app notification
// exchange so that the Siphon Sandbox, simulator or developer device can
// refresh itself accordingly.
func PostAppUpdated(appID string, userID string) {
	log.Printf("Dialing %s", amqpURI)
	conn, err := amqp.Dial(amqpURI)
	if err != nil {
		log.Printf("(ignored) Error opening RMQ connection: %v", err)
		return
	}
	defer conn.Close()
	c, err := conn.Channel()
	if err != nil {
		log.Printf("(ignored) Error opening RMQ channel: %v", err)
		return
	}

	log.Printf("Declaring exchange (%q)", amqpExchange)
	err = c.ExchangeDeclare(
		amqpExchange,     // name of the exchange
		amqpExchangeType, // type
		true,             // durable
		false,            // delete when complete
		false,            // internal
		false,            // noWait
		nil,              // arguments
	)

	// Declare the JSON inline rather than mucking around with a struct
	payload := `{"type": "app_updated", "app_id": "` + appID +
		`", "user_id": "` + userID + `"}`

	log.Printf("Posting payload: %s", payload)
	err = c.Publish(amqpExchange, "", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        []byte(payload),
	})

	if err != nil {
		log.Printf("Error posting notification to RabbitMQ: %v", err)
		return
	}
}
