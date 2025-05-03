package policyeval

import (
	"context"
	"slices"
	"strings"
	"time"

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
		return
	}

	shouldRedact := false
	allowedTypes := []string{"m.text", "m.notice", "m.emote"}
	if p.AllowedTypes != nil {
		allowedTypes = *p.AllowedTypes // text-only by default
	}

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

		shouldRedact = !slices.Contains(allowedTypes, msgType)
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
	}
}

type eventWithMentions struct {
	Mentions *event.Mentions `json:"m.mentions"`
}

func MentionProtectionCallback(ctx context.Context, client *mautrix.Client, evt *event.Event, p *config.MaxMentionsProtection) {
	if p.MaxMentions <= 0 {
		return
	}
	protectionLog := zerolog.Ctx(ctx).With().
		Str("protection", "max_mentions").
		Stringer("room", evt.RoomID).
		Stringer("event", evt.ID).
		Stringer("sender", evt.Sender).
		Logger()
	content, ok := evt.Content.Parsed.(*eventWithMentions)
	if !ok || content.Mentions == nil || len(content.Mentions.UserIDs) == 0 {
		// No intentional mentions here, nothing to check
		return
	}
	userMentions := len(content.Mentions.UserIDs)
	powerLevels, err := client.StateStore.GetPowerLevels(ctx, evt.RoomID)
	if err != nil {
		protectionLog.Warn().Err(err).Msg("Failed to get power levels!")
	}
	if p.UserCanBypass(evt.Sender, powerLevels) {
		return
	}

	// TODO: ban instead of redact(?)
	if p.Period <= 0 {
		// Only check the event itself
		if userMentions >= p.MaxMentions {
			if _, err := client.RedactEvent(ctx, evt.RoomID, evt.ID); err != nil {
				protectionLog.Err(err).Msg("Failed to redact message")
			} else {
				protectionLog.Info().Msg("Redacted message")
			}
		}
	} else {
		u := p.IncrementUser(evt.Sender, userMentions)
		if u.Hits >= p.MaxMentions && time.Now().Before(u.Expires) {
			if _, err := client.RedactEvent(ctx, evt.RoomID, evt.ID); err != nil {
				protectionLog.Err(err).Msg("Failed to redact message")
			} else {
				protectionLog.Info().Msg("Redacted message")
			}
		}
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
	pe.protections = content
	// TODO: Diff changes(?)
	output = append(output, "Protections updated")
	return
}
