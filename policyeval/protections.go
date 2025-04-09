package policyeval

import (
	"context"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type ProtectionConfig map[string]IProtection

type Protections struct {
	Global    *ProtectionConfig               `json:"global,omitempty"`
	Overrides map[id.RoomID]*ProtectionConfig `json:"overrides,omitempty"`
}

type IProtection interface {
	IsEnabled() bool
	Callback(ctx context.Context, client *mautrix.Client, evt *event.Event)
}

type NoMediaProtection struct {
	Enabled     bool `json:"enabled"`
	AllowImages bool `json:"allow_images"`
	AllowVideos bool `json:"allow_videos"`
	AllowFiles  bool `json:"allow_files"`
	AllowAudio  bool `json:"allow_audio"`
}

func (p *NoMediaProtection) IsEnabled() bool {
	// Master toggle + at least one disallowed media type
	return p.Enabled && !(p.AllowImages && p.AllowVideos && p.AllowFiles && p.AllowAudio)
}

func (p *NoMediaProtection) Callback(ctx context.Context, client *mautrix.Client, evt *event.Event) {
	// The room constraints and enabled-ness of the protection are already checked before this callback is called.
	if evt.Type != event.EventMessage {
		return
	}
	shouldRedact := false
	content := evt.Content.AsMessage()
	switch content.MsgType {
	case event.MsgImage:
		if !p.AllowImages {
			shouldRedact = true
		}
	case event.MsgVideo:
		if !p.AllowVideos {
			shouldRedact = true
		}
	case event.MsgFile:
		if !p.AllowFiles {
			shouldRedact = true
		}
	case event.MsgAudio:
		if !p.AllowAudio {
			shouldRedact = true
		}
	}

	if shouldRedact {
		if _, err := client.RedactEvent(ctx, evt.RoomID, evt.ID); err != nil {
			zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to redact media message")
		} else {
			zerolog.Ctx(ctx).Info().
				Stringer("room", evt.RoomID).
				Stringer("event", evt.ID).
				Stringer("sender", evt.Sender).
				Msg("Redacted media message")
		}
	}
}

func (pe *PolicyEvaluator) handleProtections(
	evt *event.Event,
) (output, errors []string) {
	content, ok := evt.Content.Parsed.(*Protections)
	if !ok {
		errors = append(errors, "failed to parse protections")
		return
	}
	pe.protections = content
	return
}
