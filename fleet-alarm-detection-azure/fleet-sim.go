// @nuclio.configure
//
// function.yaml:
//   spec:
//     build:
//       commands:
//       - apk --update --no-cache add ca-certificates
//     triggers:
//       periodic:
//         kind: cron
//         attributes:
//           interval: 3s
//     dataBindings:
//       eh: 
//         class: eventhub
//         attributes:
//           sharedAccessKeyName: < your value here >
//           sharedAccessKeyValue: < your value here >
//           namespace: < your value here >
//           eventHubName: fleet
//

package main

import (
	ctx "context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"sync"

	"github.com/nuclio/nuclio-sdk"
	"pack.ag/amqp"
)

const (
	numberOfCars    = 10
	numberOfSenders = 1
)

type metric struct {
	ID                       string  `json:"id"`
	Latitude                 string  `json:"latitude"`
	Longitude                string  `json:"longitude"`
	TirePressure             float32 `json:"tirePressure"`
	FuelEfficiencyPercentage float32 `json:"fuelEfficiencyPercentage"`
}

func Handler(context *nuclio.Context, event nuclio.Event) (interface{}, error) {
	sender := context.DataBinding["eh"].(*amqp.Sender)

	// create metrics and shove them to a channel
	metricsChannel := make(chan metric)

	// generate metrics into the channel
	go generateRandomMetrics(context.Logger, metricsChannel, numberOfCars)

	// sync for sending
	sendingCompleteWaitGroup := sync.WaitGroup{}
	sendingCompleteWaitGroup.Add(numberOfSenders)

	// spawn N workers to read from the channel and shove to event hub
	for senderIdx := 0; senderIdx < numberOfSenders; senderIdx++ {
		childLogger := context.Logger.GetChild(fmt.Sprintf("w%d", senderIdx))

		go sendMetrics(childLogger, sender, metricsChannel, &sendingCompleteWaitGroup)
	}

	context.Logger.DebugWith("Waiting for metrics to be sent", "num", numberOfSenders)

	// wait for all senders to complete
	sendingCompleteWaitGroup.Wait()

	context.Logger.Debug("Send complete")

	return nil, nil
}

func generateRandomMetrics(logger nuclio.Logger,
	metricsChannel chan metric,
	numberOfCars int) {

	for carIndex := 0; carIndex < numberOfCars; carIndex++ {
		metricsChannel <- metric{
			ID:                       strconv.Itoa(carIndex),
			Latitude:                 "",
			Longitude:                "",
			TirePressure:             generateRandomFloat(0, 5),
			FuelEfficiencyPercentage: generateRandomFloat(0, 100),
		}
	}

	close(metricsChannel)

	logger.DebugWith("Generated metrics", "num", numberOfCars)
}

func generateRandomFloat(min float32, max float32) float32 {
	diff := max - min

	return (rand.Float32() * diff) + min
}

func sendMetrics(logger nuclio.Logger,
	sender *amqp.Sender,
	metricChannel chan metric,
	completionWaitGroup *sync.WaitGroup) {

	for metric := range metricChannel {
		serializedMetric, err := json.Marshal(metric)
		if err != nil {
			logger.WarnWith("Failed to serialize metric, ignoring", "err", err)

			continue
		}

		// create an amqp message with the body of the metric
		message := amqp.Message{
			Data: serializedMetric,
		}

		logger.DebugWith("Sending", "metric", string(serializedMetric))

		// send the metric
		if err := sender.Send(ctx.Background(), &message); err != nil {
			logger.WarnWith("Failed to send message to eventhub", "err", err)
		}

		logger.DebugWith("Sending complete", "id", metric.ID)
	}

	// signal that we're done
	completionWaitGroup.Done()
}
