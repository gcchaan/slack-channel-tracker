package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

func getHeader(headers map[string]string, key string) string {
	for k, v := range headers {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func handler(ctx context.Context, req events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	slog.Info("Received request body", "body", req.Body)

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		slog.Error("Failed to load AWS config", "error", err)
		return events.LambdaFunctionURLResponse{StatusCode: 500}, nil
	}
	ssmClient := ssm.NewFromConfig(cfg)

	signingSecretPath := os.Getenv("SLACK_SIGNING_SECRET_SSM_PATH")
	if signingSecretPath == "" {
		slog.Error("SLACK_SIGNING_SECRET_SSM_PATH environment variable is required")
		return events.LambdaFunctionURLResponse{StatusCode: 500}, nil
	}

	signingSecret, err := fetchSSMParameter(ctx, ssmClient, signingSecretPath)
	if err != nil {
		slog.Error("Failed to fetch signing secret", "error", err)
		return events.LambdaFunctionURLResponse{StatusCode: 500}, nil
	}

	header := http.Header{}
	header.Set("X-Slack-Request-Timestamp", getHeader(req.Headers, "x-slack-request-timestamp"))
	header.Set("X-Slack-Signature", getHeader(req.Headers, "x-slack-signature"))

	sv, err := slack.NewSecretsVerifier(header, signingSecret)
	if err != nil {
		slog.Error("Failed to initialize secrets verifier", "error", err)
		return events.LambdaFunctionURLResponse{StatusCode: 403}, nil
	}
	sv.Write([]byte(req.Body))
	if err := sv.Ensure(); err != nil {
		slog.Error("Invalid signature or timestamp", "error", err)
		return events.LambdaFunctionURLResponse{StatusCode: 403}, nil
	}

	eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(req.Body), slackevents.OptionNoVerifyToken())
	if err != nil {
		slog.Error("Failed to parse event", "error", err)
		return events.LambdaFunctionURLResponse{StatusCode: 400}, nil
	}

	// Respond to the challenge request (URL Verification)
	if eventsAPIEvent.Type == slackevents.URLVerification {
		slog.Info("respond challenge")
		var r *slackevents.ChallengeResponse
		if err := json.Unmarshal([]byte(req.Body), &r); err != nil {
			return events.LambdaFunctionURLResponse{StatusCode: 500}, nil
		}

		return events.LambdaFunctionURLResponse{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "text/plain"},
			Body:       r.Challenge,
		}, nil
	}

	// Handle regular event callbacks
	if eventsAPIEvent.Type == slackevents.CallbackEvent {
		slog.Info("event_callback")

		channelID := os.Getenv("SLACK_CHANNEL_ID")
		if channelID == "" {
			slog.Error("SLACK_CHANNEL_ID environment variable is required")
			return events.LambdaFunctionURLResponse{StatusCode: 500}, nil
		}

		tokenPath := os.Getenv("SLACK_TOKEN_SSM_PATH")
		if tokenPath == "" {
			slog.Error("SLACK_TOKEN_SSM_PATH environment variable is required")
			return events.LambdaFunctionURLResponse{StatusCode: 500}, nil
		}

		token, err := fetchSSMParameter(ctx, ssmClient, tokenPath)
		if err != nil {
			slog.Error("Failed to fetch Slack token", "error", err)
			return events.LambdaFunctionURLResponse{StatusCode: 500}, nil
		}

		text := channelEventText(eventsAPIEvent.InnerEvent)

		api := slack.New(token)
		if err := postToSlack(api, channelID, text); err != nil {
			slog.Error("Failed to post message to Slack", "error", err)
			return events.LambdaFunctionURLResponse{StatusCode: 500}, nil
		}

		slog.Info("Slack notification sent successfully")
	}

	return events.LambdaFunctionURLResponse{StatusCode: 200}, nil
}

func channelEventText(innerEvent slackevents.EventsAPIInnerEvent) string {
	switch ev := innerEvent.Data.(type) {
	case *slackevents.ChannelCreatedEvent:
		return fmt.Sprintf("<#%s> is created by `<@%s>`.", ev.Channel.ID, ev.Channel.Creator)
	case *slackevents.ChannelDeletedEvent:
		return fmt.Sprintf("<#%s> is deleted.", ev.Channel)
	case *slackevents.ChannelArchiveEvent:
		return fmt.Sprintf("<#%s> is archived by `<@%s>`.", ev.Channel, ev.User)
	case *slackevents.ChannelUnarchiveEvent:
		return fmt.Sprintf("<#%s> is unarchived by `<@%s>`.", ev.Channel, ev.User)
	default:
		return "unknown event.type is found."
	}
}

func fetchSSMParameter(ctx context.Context, client *ssm.Client, paramName string) (string, error) {
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(paramName),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get parameter from SSM (path: %s): %w", paramName, err)
	}
	return *out.Parameter.Value, nil
}

func postToSlack(api *slack.Client, channelID string, message string) error {
	_, _, err := api.PostMessage(
		channelID,
		slack.MsgOptionText(message, false),
	)
	if err != nil {
		return fmt.Errorf("post message failed: %w", err)
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	lambda.Start(handler)
}
