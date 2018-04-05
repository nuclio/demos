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
	"testing"
	"github.com/nuclio/nuclio-test-go"
	"fmt"
)

// sample SNS event template (%q is replaced with the escaped S3 event)
const testSNS = `
{
  "Type" : "Notification",
  "MessageId" : "12312345-6666-7777-8888-999999999999",
  "TopicArn" : "arn:aws:sns:us-east-1:<name>",
  "Subject" : "Amazon S3 Notification",
  "Message" : %q,
  "Timestamp" : "2018-03-22T16:23:00.072Z"
}
`

// sample event taken from: https://docs.aws.amazon.com/AmazonS3/latest/dev/notification-content-structure.html
const sampleS3Event = `
{"Records":
	[
		{  
         "eventVersion":"2.0",
         "eventSource":"aws:s3",
         "awsRegion":"us-east-1",
         "eventTime":"1970-01-01T00:00:00.000Z",
         "eventName":"ObjectCreated:Put",
         "userIdentity":{  
            "principalId":"AIDAJDPLRKLG7UEXAMPLE"
         },
         "requestParameters":{  
            "sourceIPAddress":"127.0.0.1"
         },
         "responseElements":{  
            "x-amz-request-id":"C3D13FE58DE4C810",
            "x-amz-id-2":"FMyUVURIY8/IgAtTv8xRjskZQpcIZ9KG4V5Wp6S7S/JRWeUWerMUE5JgHvANOjpD"
         },
         "s3":{  
            "s3SchemaVersion":"1.0",
            "configurationId":"testConfigRule",
            "bucket":{  
               "name":"mybucket",
               "ownerIdentity":{  
                  "principalId":"A3NL1KOZZKExample"
               },
               "arn":"arn:aws:s3:::mybucket"
            },
            "object":{  
               "key":"HappyFace.jpg",
               "size":1024,
               "eTag":"d41d8cd98f00b204e9800998ecf8427e",
               "versionId":"096fKKXTRTtl3on89fVO.nfljtsv6qko",
               "sequencer":"0055AED6DCD90281E5"
            }
         }
      }
   ]
}`

// function unit testing
func TestS3Watch(t *testing.T) {
	// Initialize a test context (verbose = true)
	tc, err := nutest.NewTestContext(Handler, true, nil )
	if err != nil {
		t.Fatal(err)
	}

	// build the simulated S3 message inside the SNS message
	eventString := fmt.Sprintf(testSNS, sampleS3Event)

	// Create a test event (eventString is a simulated event Json)
	testEvent := nutest.TestEvent{
		Path: "",
		Body: []byte(eventString),
	}

	// Invoke the tested function
	resp, err := tc.Invoke(&testEvent)
	tc.Logger.InfoWith("Run complete", "resp", resp, "err", err)
}
