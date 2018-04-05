# S3 Notification Watcher

Listens on S3 bucket update events (via AWS SNS notifications) and run custom actions.

This demo enclude a [function handler](s3watch.go) and [unit testing](s3watch_test.go) which demonstrate the use of 
nuclio. the unit testing code can run inside your favorite IDE, just clone the repo and download the dependent libraries
(can use `go get` to download the libraries), the test lib contains sample S3 and SNS events which you can edit.

In order to run the function against live events you need to:
1. deploy the function on a node/cluster which is accessible from the internet/AWS
2. configure your S3 bucket to generate SNS notifications to a topic, and configure the SNS topic to sent the 
notifications to your function. 

### Deploy the function

once you have tested your function locally it can be deployed in any nuclio cluster or standalone setup, you can copy
the code into nuclio playground UI, or use nuclio CLI commands, follow the 
[instructions in this link](https://github.com/nuclio/nuclio/blob/master/docs/tasks/deploying-functions.md). 

Notice the function include API Gateway configuration (in comment decorations), you can edit that or use the CLI/UI apis
to configure the HTTP trigger. The API gateway configuration depends on having Kubernetes setup with Ingress controller.

### Configure AWS S3 and SNS Notifications

You can read [AWS docs](https://docs.aws.amazon.com/AmazonS3/latest/dev/NotificationHowTo.html) or follow this
[blog-post](http://www.tothenew.com/blog/configuring-sns-notifications-for-s3-put-object-event-operation/) which 
explains how to set up the S3 bucket to generate notifications and how to setup the SNS service to forward them to an 
email/HTTP (switch the email destination in the example with your HTTP end-point).

 