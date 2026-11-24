// Package ai is cockatoo's AI: the Anthropic client (client.go) and the settings that turn it on
// and keep the user's key (service.go). Every feature that uses AI gets its client from
// Service.Client, so the on/off switch and the key are checked in one place.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// DefaultModel is the model a new setup starts with.
const DefaultModel = "claude-opus-5-5"

// maxTokens is the most a reply may use, thinking included. The model always thinks before it
// answers, so a tight cap can leave no room for the answer itself.
const maxTokens = 16000

// Client calls Anthropic with the user's own key. It reads nothing from the environment: an
// ANTHROPIC_API_KEY or ANTHROPIC_BASE_URL set on the machine is not used.
type Client struct {
	c     anthropic.Client
	model string
}

// NewClient returns a client for key and model. extra is for the tests, which point it at a
// stand-in server with option.WithBaseURL.
func NewClient(key, model string, extra ...option.RequestOption) *Client {
	opts := append([]option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(key),
	}, extra...)
	return &Client{c: anthropic.NewClient(opts...), model: model}
}

// Model is the model this client asks.
func (c *Client) Model() string { return c.model }

// Check asks Anthropic about the model. It proves the key is accepted and the model is there
// for it, and costs nothing: no message is sent.
func (c *Client) Check(ctx context.Context) error {
	if _, err := c.c.Models.Get(ctx, c.model, anthropic.ModelGetParams{}); err != nil {
		return explain(err, c.model)
	}
	return nil
}

// Reply is Claude's answer to one message.
type Reply struct {
	Text         string `json:"text"`
	Model        string `json:"model"` // the model that answered; another one when a fallback served it
	InputTokens  int64  `json:"inputTokens"`
	OutputTokens int64  `json:"outputTokens"`
}

// Request is one message to Claude.
type Request struct {
	System string
	Prompt string
	// Schema, when set, is the JSON schema the answer must follow (structured outputs); the
	// reply's Text is then that JSON. Every object needs "additionalProperties": false.
	Schema json.RawMessage
	// Effort is "low", "medium", "high", "xhigh" or "max"; empty leaves the model's default.
	Effort string
}

// Ask sends one message and returns the text of the reply.
func (c *Client) Ask(ctx context.Context, system, prompt string) (Reply, error) {
	return c.Complete(ctx, Request{System: system, Prompt: prompt})
}

// Complete sends one message and returns the reply with the tokens it used. When Anthropic's
// safety checks decline the request on the chosen model, the server retries it on the model
// Anthropic recommends for that case (fallbacks "default"), inside the same call.
func (c *Client) Complete(ctx context.Context, r Request) (Reply, error) {
	params := anthropic.BetaMessageNewParams{
		Model:     c.model,
		MaxTokens: maxTokens,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(r.Prompt))},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
	}
	if r.System != "" {
		params.System = []anthropic.BetaTextBlockParam{{Text: r.System}}
	}
	if r.Effort != "" {
		params.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(r.Effort)
	}
	if len(r.Schema) > 0 {
		params.OutputConfig.Format = anthropic.BetaJSONOutputFormatParam{Schema: r.Schema}
	}
	msg, err := c.c.Beta.Messages.New(ctx, params)
	if err != nil {
		return Reply{}, explain(err, c.model)
	}

	if msg.StopReason == anthropic.BetaStopReasonRefusal {
		why := strings.TrimRight(strings.TrimSpace(msg.StopDetails.Explanation), ".")
		if why == "" {
			why = "no reason was given"
		}
		return Reply{}, fmt.Errorf("Claude declined this request: %s.", why)
	}
	if msg.StopReason == anthropic.BetaStopReasonMaxTokens {
		// A cut-off answer is not an answer, least of all a cut-off JSON one.
		return Reply{}, errors.New("Claude ran out of room before it finished. Nothing was kept.")
	}
	var text strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	reply := Reply{
		Text: strings.TrimSpace(text.String()), Model: string(msg.Model),
		InputTokens: msg.Usage.InputTokens, OutputTokens: msg.Usage.OutputTokens,
	}
	if reply.Text == "" {
		return Reply{}, fmt.Errorf("Claude's reply had no text (stop reason %q)", msg.StopReason)
	}
	return reply, nil
}

// explain turns what the SDK returned into words for the user. The SDK has already retried
// what can be retried (rate limits, overload, lost connections) before this sees it.
func explain(err error, model string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("Anthropic did not answer in time: %w", err)
	}
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		return fmt.Errorf("could not reach Anthropic: %w", err)
	}
	switch code := apierr.StatusCode; {
	case code == http.StatusUnauthorized:
		return errors.New("Anthropic did not accept this API key. Check that it was copied in full and has not been deleted.")
	case code == http.StatusPaymentRequired:
		return errors.New("The Anthropic account behind this key has no credit left. Add credit in the Claude Console.")
	case code == http.StatusForbidden:
		return fmt.Errorf("This API key is not allowed to use %s.", model)
	case code == http.StatusNotFound:
		return fmt.Errorf("Anthropic does not offer the model %s to this key.", model)
	case code == http.StatusTooManyRequests:
		return errors.New("Anthropic is limiting requests on this key right now. Try again in a minute.")
	case code >= 500:
		return fmt.Errorf("Anthropic is busy or down right now (%d). Try again shortly.", code)
	default:
		msg := strings.TrimSpace(apierr.Message)
		if msg == "" {
			msg = http.StatusText(code)
		}
		return fmt.Errorf("Anthropic refused the request (%d): %s", code, msg)
	}
}
