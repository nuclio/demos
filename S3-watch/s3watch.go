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

// nuclio function to watches and handles changes on S3 (via SNS)

package main

import (
	"encoding/json"
	"net/http"

	"github.com/eawsy/aws-lambda-go-event/service/lambda/runtime/event/snsevt"
	"github.com/eawsy/aws-lambda-go-event/service/lambda/runtime/event/s3evt"
	"github.com/nuclio/nuclio-sdk-go"
)

// @nuclio.configure
//
// function.yaml:
//   spec:
//     triggers:
//       myHttpTrigger:
//         maxWorkers: 4
//         kind: "http"
//         attributes:
//           ingresses:
//             http:
//               paths:
//               - "/mys3hook"

func Handler(context *nuclio.Context, event nuclio.Event) (interface{}, error) {

	// non intrusive structured Debug log (runs only if level is set to debug)
	context.Logger.DebugWith("Process document", "path", event.GetPath(), "body", string(event.GetBody()))

	// Get body, assume it is the right HTTP Post event, can add error checking
	body := event.GetBody()

	snsEvent := snsevt.Record{}
	err := json.Unmarshal([]byte(body),&snsEvent)
	if err != nil {
		return "", err
	}

	context.Logger.InfoWith("Got SNS Event", "type", snsEvent.Type)

	if snsEvent.Type == "SubscriptionConfirmation" {

		// need to confirm registration on first time
		context.Logger.DebugWith("Handle Subscription Confirmation",
			"TopicArn", snsEvent.TopicARN,
			"Message", snsEvent.Message)

		resp, err := http.Get(snsEvent.SubscribeURL)
		if err != nil {
			context.Logger.ErrorWith("Failed to confirm SNS Subscription", "resp", resp, "err", err)
		}

		return "", nil
	}

	// Unmarshal S3 event, can add validations e.g. check if snsEvent.TopicArn has the right topic
	myEvent := s3evt.Event{}
	err = json.Unmarshal([]byte(snsEvent.Message),&myEvent)
	if err != nil {
		return "", err
	}

	// Log the details of the S3 Update
	record := myEvent.Records[0].S3
	context.Logger.InfoWith("S3 Details", "bucket", record.Bucket.Name,
		"key", record.Object.Key, "size", record.Object.Size)

	// handle your S3 event here
	// ...

	return "", nil
}
