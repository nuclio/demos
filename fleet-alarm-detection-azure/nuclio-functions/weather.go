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

package main

import (
    "encoding/json"
    "net/http"
    "math/rand"

    "github.com/nuclio/nuclio-sdk-go"
)

type weather struct {
    Temperature int `json:"temperature"`
    WeatherCondition string `json:"weatherCondition"`
}

func Handler(context *nuclio.Context, event nuclio.Event) (interface{}, error) {
    context.Logger.InfoWith("Received event", "body", string(event.GetBody()))

    weatheritem := &weather{
        Temperature: randInt(-10,50),
        WeatherCondition: randWeatherCondition(),
    }
    
    json, err := json.Marshal(weatheritem)
    if err != nil {
        return nil, err
    }

    // return the contents as JSON
    return nuclio.Response{
        StatusCode:  http.StatusOK,
        ContentType: "application/json",
        Body:        []byte(json),
    }, nil
}

func randInt(min int, max int) int {
    return min + rand.Intn(max-min)
}

func randWeatherCondition() string {
    weatherConditions := []string{
        "clear",
        "cloudy",
        "rain",
        "snow",
    }
    
    message := weatherConditions[rand.Intn(len(weatherConditions))]
    
    return message
}
