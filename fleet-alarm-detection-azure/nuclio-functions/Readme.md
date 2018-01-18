# Real-time alarm conditions based on telemetry from a single car

![image](https://user-images.githubusercontent.com/17064840/35099715-b7306804-fc61-11e7-8629-3874745393f9.png)

## Functions

###  Car fleet simulator (fleet-sim.go)

Generate simulated datapoints periodically and post them, via AMQP (EH Data binding), to event hub (1). A datapoint includes:
1. Car ID (string)
2. Position (lat / long strings)
3. Tire pressure (float)
4. Fuel efficiency percentage (float)

### Sensor handler (sensor-handler.go)

A car sensor handler nuclio function triggered (2) on each datapoint posted to the car sensor event hub. It will then:
1. Invoke the weather station nuclio function (3), sending the position as an argument and receiving the weather at that location (can just return weighted random - weather.go)
2. Perform a lookup on some database (4) to get the driver ID from the car ID (optional)
3. Post the datapoint enriched by (3) and (4) to the enriched car sensor event hub (6)
4. Check if event.tirePressure < someThreshold. If so, write an alarm event (JSON containing Car ID, alarm type and tire pressure) to the alarm event hub

### Weather (weather.go)

1.  Generate random weather data "clear","cloudy","rain","snow"

### Spark job

A Spark job run periodically and read datapoints in a batch from the enriched sensor event hub (7) and simply calculate mean car fuel efficiency per weather condition (based only on the data in the microbatch). 
The Spark job can write alarms (8) to the alarm event hub (e.g. all cars with a fuel efficiency lower than the mean per weather condition)
 
