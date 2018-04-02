// Watches and handles changes on S3 (via SNS)
package main

import (
	"encoding/json"
	"net/http"

	"github.com/eawsy/aws-lambda-go-event/service/lambda/runtime/event/snsevt"
	"github.com/eawsy/aws-lambda-go-event/service/lambda/runtime/event/s3evt"
	"github.com/nuclio/nuclio-sdk-go"
	"fmt"
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
		context.Logger.DebugWith("Handle SubscriptionConfirmation",
			"TopicArn", snsEvent.TopicARN,
			"Message", snsEvent.Message)

		resp, err := http.Get(snsEvent.SubscribeURL)
		if err != nil {
			context.Logger.ErrorWith("Failed to confirm SNS Subscription", "resp", resp, "err", err)
		}

		return "", nil
	}

	// Unmarshal S3 event, can add error check to verify snsEvent.TopicArn has the right topic (arn:aws:sns:...)
	myEvent := s3evt.Event{}
	json.Unmarshal([]byte(snsEvent.Message),&myEvent)

	context.Logger.InfoWith("Got S3 Event", "message", myEvent.String())
	record := myEvent.Records[0].S3
	context.Logger.InfoWith("S3 Details", "bucket", record.Bucket.Name, "key", record.Object.Key, "size", record.Object.Size)
	fmt.Println(myEvent.String())

	// handle your S3 event here
	// ...

	return "", nil
}
