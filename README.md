# slack-channel-tracker

## Requirements

* AWS CLI already configured with Administrator permission
* [Golang](https://golang.org)
* SAM CLI - [Install the SAM CLI](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-sam-cli-install.html)

## Setup process

### Create a new Slack app

`./assets/manifest.json` file helps you to create a new app with the required permissions.

You need to invite the Bot app to the channel where you want to post messages in advance (/invite @AppName).

### Create a new SSM parameter

You can get your token from [Slack API](https://api.slack.com/apps) page after creating a new app and adding the required permissions.

```bash
aws ssm put-parameter \
  --name "/slack-channel-tracker/slack-bot-user-oauth-token" \
  --value "xoxb-***********-**************-************************" \
  --type "SecureString"
```

```bash
aws ssm put-parameter \
  --name "/slack-channel-tracker/signing_secret" \
  --value "abc123xxx" \
  --type "SecureString"
```

### Build and deploy

```bash
sam build
```

```bash
sam deploy --guided
```

### Registering with your Slack App

Upon successful deployment, the SlackChannelTrackerFunctionUrl will be displayed in the Outputs section of your terminal. Copy this URL and paste it into Event Subscriptions ➔ Request URL in your Slack App management dashboard to complete the integration.
