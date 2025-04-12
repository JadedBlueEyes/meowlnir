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
		protectionLog.Warn().Err(err).Msg("Failed to get power levels")
		return
	}
	userPL, ok := powerLevels.Users[evt.Sender]
	if !ok {
		userPL = powerLevels.UsersDefault
		protectionLog.Debug().Msg("Failed to find user, defaulted power level")
	}
	if int64(userPL) > p.IgnorePL {
		protectionLog.Debug().
			Int("user_power_level", userPL).
			Int64("ignore_power_level", p.IgnorePL).
			Msg("Ignoring message from user with sufficient power level")
		return
	}

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
