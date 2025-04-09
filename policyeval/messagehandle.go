package policyeval

import (
	"context"
	"fmt"
	"strings"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/meowlnir/bot"
)

func (pe *PolicyEvaluator) isMention(content *event.MessageEventContent) bool {
	if content.Mentions != nil {
		return content.Mentions.Has(pe.Bot.UserID)
	}
	return strings.Contains(content.FormattedBody, pe.Bot.UserID.URI().MatrixToURL()) ||
		strings.Contains(content.FormattedBody, pe.Bot.UserID.String())
}

func (pe *PolicyEvaluator) HandleMessage(ctx context.Context, evt *event.Event) {
	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return
	}
	if pe.isMention(content) {
		pe.Bot.SendNoticeOpts(
			ctx, pe.ManagementRoom,
			fmt.Sprintf(
				`@room [%s](%s) [pinged](%s) the bot in [%s](%s)`,
				evt.Sender, evt.Sender.URI().MatrixToURL(),
				evt.RoomID.EventURI(evt.ID).MatrixToURL(),
				evt.RoomID, evt.RoomID.URI().MatrixToURL(),
			),
			&bot.SendNoticeOpts{Mentions: &event.Mentions{Room: true}, SendAsText: true},
		)
	}

	if pe.protections != nil {
		if pe.protections.Global != nil {
			for _, protection := range *pe.protections.Global {
				if protection.IsEnabled() {
					protection.Callback(ctx, pe.Bot.Client, evt)
				}
			}
		}
		if pe.protections.Overrides != nil {
			if protection, ok := pe.protections.Overrides[evt.RoomID]; ok {
				for _, p := range *protection {
					if p.IsEnabled() {
						p.Callback(ctx, pe.Bot.Client, evt)
					}
				}
			}
		}
	}
}
