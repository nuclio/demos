/*
Copyright 2017 The Nuclio Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// @nuclio.configure
//
// function.yaml:
//   spec:
//     build:
//       commands:
//       - apk --update --no-cache add ca-certificates
//     triggers:
//       eventhub:
//         kind: eventhub
//         attributes:
//           sharedAccessKeyName: < your value here >
//           sharedAccessKeyValue: < your value here >
//           namespace: < your value here >
//           eventHubName: fleet
//           consumerGroup: < your value here >
//           partitions:
//           - 0
//           - 1
//     dataBindings:
//       alarmsEventhub: 
//         class: eventhub
//         attributes:
//           sharedAccessKeyName: < your value here >
//           sharedAccessKeyValue: < your value here >
//           eventHubName: alarms
//           namespace: < your value here >
//       enrichedFleetEventhub: 
//         class: eventhub
//         attributes:
//           sharedAccessKeyName: < your value here >
//           sharedAccessKeyValue: < your value here >
//           eventHubName: enrichedfleet
//           namespace: < your value here >
//

package main

import (
    "bytes"
    ctx "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"

    "github.com/nuclio/nuclio-sdk-go"
    "github.com/nuclio/amqp"
)

type metric struct {
    ID                       string  `json:"id"`
    Latitude                 string  `json:"latitude"`
    Longitude                string  `json:"longitude"`
    TirePressure             float32 `json:"tirePressure"`
    FuelEfficiencyPercentage float32 `json:"fuelEfficiencyPercentage"`
    Temperature              int     `json:"temperature"`
    WeatherCondition         string  `json:"weatherCondition"`
}

type alarm struct {
    ID    string 
    Type  string 
}

type weather struct {
    Temperature int `json:"temperature"`
    WeatherCondition string `json:"weatherCondition"`
}

func SensorHandler(context *nuclio.Context, event nuclio.Event) (interface{}, error) {

    // get alarms eventhub 
    alarmsEventhub := context.DataBinding["alarmsEventhub"].(*amqp.Sender)

    // get enriched fleet eventhub
    enrichedFleetEventhub := context.DataBinding["enrichedFleetEventhub"].(*amqp.Sender)
    
    // unmarshal the eventhub metric
    eventHubMetric := metric{}
    if err := json.Unmarshal(event.GetBody(), &eventHubMetric); err != nil {
        return nil, err
    }
    
    // send alarm if tire pressure < threshold
    var MinTirePressureThreshold float32 = 2
    if eventHubMetric.TirePressure < MinTirePressureThreshold {
        marshaledAlarm, err := json.Marshal(alarm{ID: eventHubMetric.ID, Type: "LowTirePressue"})
        if err != nil {
            return nil, err
        }
        
        // send alarm to event hub
        if err := sendToEventHub(context, marshaledAlarm, alarmsEventhub); err != nil {
            return nil, err
        }
    }
    
    // prepare to send to spark via eventhub
    // call weather station for little enrichment
    temperature, weatherCondtion, err := getWeather(context, eventHubMetric)
    if err != nil {
        return nil, err
    }
    
    context.Logger.DebugWith("Got weather", "temp", temperature, "weather", weatherCondtion)
    
    // assign return values
    eventHubMetric.Temperature = temperature
    eventHubMetric.WeatherCondition = weatherCondtion
    
    // send to spark
    marshaledMetric, err := json.Marshal(eventHubMetric)
    if err != nil {
        return nil, err
    }
    
    if err := sendToEventHub(context, marshaledMetric, enrichedFleetEventhub); err != nil {
        return nil, err
    }
    
    return nil, nil
}

func sendToEventHub(context *nuclio.Context, data []byte, hub *amqp.Sender) error {
    
    // create an amqp message with the body
    message := amqp.Message{
        Data: data,
    }

    // send the metric
    if err := hub.Send(ctx.Background(), &message); err != nil {
        context.Logger.WarnWith("Failed to send message to eventhub", "err", err)

        return err
    }

    return nil
}

func getWeather(context *nuclio.Context, m metric) (int, string, error) {
    
    // call the weather function to get the current weather
    marshalledWeatherItem, err := callFunction(context, "weather", m)
    if err != nil {
        return 0, "", err
    }
    
    // parse the JSON to get current weather
    var weatherItem weather
    err = json.Unmarshal(marshalledWeatherItem, &weatherItem)
    if err != nil {
        return 0, "", err
    }

    return weatherItem.Temperature, weatherItem.WeatherCondition, nil
}


func callFunction(context *nuclio.Context, name string, body interface{}) ([]byte, error) {
    context.Logger.DebugWith("Calling function", "name", name, "body", body)
    
    marshalledBody, err := json.Marshal(body); 
    if err != nil {
        return nil, err
    }
    
    // convert function name to URL (hardcoded nuclio namespace)
    // for local environments, use:
    // url := fmt.Sprintf("http://172.17.0.1:<local port>")
    //
    url := fmt.Sprintf("http://%s.nuclio.svc.cluster.local:8080", name)

    response, err := http.Post(url, "content-type/json", bytes.NewBuffer(marshalledBody))
    if err != nil {
        return nil, err
    }
    
    defer response.Body.Close()

    if response.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("Got unexpected status code: %d", response.StatusCode)
    }
    
    bodyContents, err := ioutil.ReadAll(response.Body)
    if err != nil {
        return nil, err
    }
    
    return bodyContents, nil
}
