package main

import (
    "encoding/json"
    "net/http"
    "math/rand"
    "time"

    "github.com/nuclio/nuclio-sdk"
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
    
    rand.Seed(time.Now().Unix()) // initialize global pseudo random generator
    
    message := weatherConditions[rand.Intn(len(weatherConditions))]
    
    return message
}
