package internal

import (
	"context"
	"fmt"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/nats-io/nats.go/jetstream"
)

func MessageHandler(ctx context.Context, js jetstream.JetStream) func(event *events.MessageCreate) {
	return func(event *events.MessageCreate) {
		if event.Message.Author.Bot {
			return
		}

		publish, err := js.Publish(ctx, fmt.Sprintf("message.%s", event.Message.Author), []byte(event.Message.Content))

		if err != nil {
			println("oups publish", err.Error())
			return
		}

		println(publish.Sequence)
		// event code here
		_, _ = event.Client().Rest().CreateMessage(event.ChannelID, discord.NewMessageCreateBuilder().SetContent("coucou").Build())
	}
}
