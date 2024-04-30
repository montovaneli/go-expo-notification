package expo

import (
	"encoding/json"
	"errors"
	"fmt"

	fastshot "github.com/opus-domini/fast-shot"
	"github.com/opus-domini/fast-shot/constant/mime"
)

const (
	// DefaultHost is the default Expo host
	DefaultHost = "https://exp.host"
	// DefaultBaseAPIURL is the default path for API requests
	DefaultBaseAPIURL = "/--/api/v2"
)

func DefaultHTTPClient(host, accessToken string) fastshot.ClientHttpMethods {
	builder := fastshot.NewClient(host)
	builder.Header().AddContentType("application/json")
	builder.Header().AddAccept(mime.JSON)
	if accessToken != "" {
		builder.Header().Add("Authorization", "Bearer "+accessToken)
	}
	return builder.Build()
}

// PushClient is an object used for making push notification requests
type PushClient struct {
	host        string
	apiURL      string
	accessToken string
	httpClient  fastshot.ClientHttpMethods
}

// ClientConfig specifies params that can optionally be specified for alternate
// Expo config and path setup when sending API requests
type ClientConfig struct {
	Host        string
	APIURL      string
	AccessToken string
	HTTPClient  fastshot.ClientHttpMethods
}

// NewPushClient creates a new Exponent push client
// See full API docs at https://docs.getexponent.com/versions/v13.0.0/guides/push-notifications.html#http-2-api
func NewPushClient(config *ClientConfig) *PushClient {
	c := new(PushClient)
	var httpClient fastshot.ClientHttpMethods
	host := DefaultHost
	apiURL := DefaultBaseAPIURL
	accessToken := ""
	if config != nil {
		if config.Host != "" {
			host = config.Host
		}
		if config.APIURL != "" {
			apiURL = config.APIURL
		}
		if config.AccessToken != "" {
			accessToken = config.AccessToken
		}
		if config.HTTPClient != nil {
			httpClient = config.HTTPClient
		} else {
			httpClient = DefaultHTTPClient(host, accessToken)
		}
	}
	c.host = host
	c.apiURL = apiURL
	c.httpClient = httpClient
	c.accessToken = accessToken
	return c
}

// Publish sends a single push notification
// @param push_message: A PushMessage object
// @return an array of PushResponse objects which contains the results.
// @return error if any requests failed
func (c *PushClient) Publish(message *PushMessage) (PushResponse, error) {
	responses, err := c.PublishMultiple([]PushMessage{*message})
	if err != nil {
		return PushResponse{}, err
	}
	return responses[0], nil
}

// PublishMultiple sends multiple push notifications at once
// @param push_messages: An array of PushMessage objects.
// @return an array of PushResponse objects which contains the results.
// @return error if the request failed
func (c *PushClient) PublishMultiple(messages []PushMessage) ([]PushResponse, error) {
	return c.publishInternal(messages)
}

func (c *PushClient) publishInternal(messages []PushMessage) ([]PushResponse, error) {
	// Validate the messages
	for _, message := range messages {
		if len(message.To) == 0 {
			return nil, errors.New("no recipients")
		}
		for _, recipient := range message.To {
			if recipient == "" {
				return nil, errors.New("invalid push token")
			}
		}
	}

	// Send request
	resp, err := c.httpClient.POST(fmt.Sprintf("%s/push/send", c.apiURL)).Body().AsJSON(messages).Send()
	if err != nil {
		return nil, err
	}

	// Check that we didn't receive an invalid response
	err = checkStatus(&resp)
	if err != nil {
		return nil, err
	}

	// Ensure body is closed after reading
	defer resp.RawBody().Close()

	// Validate the response format first
	var r *Response
	err = json.NewDecoder(resp.RawBody()).Decode(&r)
	if err != nil {
		// The response isn't json
		return nil, err
	}
	// If there are errors with the entire request, raise an error now.
	if r.Errors != nil {
		return nil, NewPushServerError("Invalid server response", &resp, r, r.Errors)
	}
	// We expect the response to have a 'data' field with the responses.
	if r.Data == nil {
		return nil, NewPushServerError("Invalid server response", &resp, r, nil)
	}
	// Sanity check the response
	if len(messages) != len(r.Data) {
		message := "Mismatched response length. Expected %d receipts but only received %d"
		errorMessage := fmt.Sprintf(message, len(messages), len(r.Data))
		return nil, NewPushServerError(errorMessage, &resp, r, nil)
	}
	// Add the original message to each response for reference
	for i := range r.Data {
		r.Data[i].PushMessage = messages[i]
	}
	return r.Data, nil
}

func checkStatus(resp *fastshot.Response) error {
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 299 {
		return nil
	}
	return fmt.Errorf("invalid response (%d %s)", resp.StatusCode(), resp.Status())
}
