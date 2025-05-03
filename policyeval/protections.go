package policyeval

import (
	"context"
	"slices"
	"strings"

	"go.mau.fi/meowlnir/config"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

func MediaProtectionCallback(ctx context.Context, client *mautrix.Client, evt *event.Event, p *config.NoMediaProtection) {
	// The room constraints and enabled-ness of the protection are already checked before this callback is called.
	protectionLog := zerolog.Ctx(ctx).With().
		Str("protection", "no_media").
		Stringer("room", evt.RoomID).
		Stringer("event", evt.ID).
		Stringer("sender", evt.Sender).
		Logger()
	powerLevels, err := client.StateStore.GetPowerLevels(ctx, evt.RoomID)
	if err != nil {
		protectionLog.Warn().Err(err).Msg("Failed to get power levels!")
	}
	if p.UserCanBypass(evt.Sender, powerLevels) {
		protectionLog.Trace().Msg("User can bypass media protection")
		return
	}
	protectionLog.Debug().Msg("Checking if message should be redacted")

	shouldRedact := false

	if evt.Type == event.EventReaction && !p.AllowCustomReactions {
		if strings.HasPrefix(evt.Content.AsReaction().GetRelatesTo().Key, "mxc://") {
			shouldRedact = true
		}
	} else {
		var msgType string
		var msgContent *event.MessageEventContent

		if evt.Type == event.EventSticker {
			msgType = "m.sticker"
			// m.sticker is actually an event type, not message type. But, for all intents
			// and purposes, it's basically just m.image, and here we'll treat it as such
		} else {
			msgContent = evt.Content.AsMessage()
			msgType = string(msgContent.MsgType)
		}

		shouldRedact = !slices.Contains(p.AllowedTypes, msgType) && len(p.AllowedTypes) > 0
		if msgContent != nil && !p.AllowInlineImages {
			// Lazy, but check for <img> tags in the body.
			if strings.Contains(msgContent.FormattedBody, "<img") {
				shouldRedact = true
			}
		}
	}

	if shouldRedact {
		if _, err := client.RedactEvent(ctx, evt.RoomID, evt.ID); err != nil {
			protectionLog.Err(err).Msg("Failed to redact message")
		} else {
			protectionLog.Info().Msg("Redacted message")
		}
	} else {
		protectionLog.Trace().Msg("Message is allowed")
	}
}

func (pe *PolicyEvaluator) handleProtections(
	evt *event.Event,
) (output, errors []string) {
	if evt.Content.Parsed == nil {
		if err := evt.Content.ParseRaw(config.StateProtections); err != nil {
			errors = append(errors, "failed to parse protections")
			return
		}
	}
	content, ok := evt.Content.Parsed.(*config.StateProtectionsEventContent)
	if !ok {
		errors = append(errors, "failed to parse protections")
		return
	}
	//current := pe.protections
	pe.protections = content
	// TODO: Diff changes(?)
	output = append(output, "Protections updated")
	return
}
