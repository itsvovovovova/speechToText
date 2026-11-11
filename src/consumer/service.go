package consumer

import (
	"context"
	"errors"

	listen "github.com/deepgram/deepgram-go-sdk/pkg/api/listen/v1/rest"
	"github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	client "github.com/deepgram/deepgram-go-sdk/pkg/client/listen"
)

var errNoTranscript = errors.New("no transcription result returned by Deepgram")

func NewTranscriber(apiKey string) (func(context.Context, string) (string, error), error) {
	c := client.NewREST(apiKey, &interfaces.ClientOptions{})
	if c == nil {
		return nil, errors.New("Deepgram client initialization failed: check DEEPGRAM_API")
	}
	dg := listen.New(c)
	return func(ctx context.Context, audioURL string) (string, error) {
		options := &interfaces.PreRecordedTranscriptionOptions{
			Model:    "nova-2",
			Language: "en",
		}
		res, err := dg.FromURL(ctx, audioURL, options)
		if err != nil {
			return "", err
		}
		if len(res.Results.Channels) == 0 || len(res.Results.Channels[0].Alternatives) == 0 {
			return "", errNoTranscript
		}
		return res.Results.Channels[0].Alternatives[0].Transcript, nil
	}, nil
}
