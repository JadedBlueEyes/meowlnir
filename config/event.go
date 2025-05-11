package config

import (
	"reflect"
	"slices"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	StateWatchedLists   = event.Type{Type: "fi.mau.meowlnir.watched_lists", Class: event.StateEventType}
	StateProtectedRooms = event.Type{Type: "fi.mau.meowlnir.protected_rooms", Class: event.StateEventType}
	StateProtections    = event.Type{Type: "fi.mau.meowlnir.protections", Class: event.StateEventType}
)

type WatchedPolicyList struct {
	RoomID       id.RoomID `json:"room_id"`
	Name         string    `json:"name"`
	Shortcode    string    `json:"shortcode"`
	DontApply    bool      `json:"dont_apply"`
	DontApplyACL bool      `json:"dont_apply_acl"`
	AutoUnban    bool      `json:"auto_unban"`
	AutoSuspend  bool      `json:"auto_suspend"`

	DontNotifyOnChange bool `json:"dont_notify_on_change"`
}

type WatchedListsEventContent struct {
	Lists []WatchedPolicyList `json:"lists"`
}

type ProtectedRoomsEventContent struct {
	Rooms []id.RoomID `json:"rooms"`

	// TODO make this less hacky
	SkipACL []id.RoomID `json:"skip_acl"`
}

type StateProtectionsEventContent struct {
	Global    *Protections               `json:"global"`
	Overrides map[id.RoomID]*Protections `json:"overrides"`
}

type Protections struct {
	NoMedia     NoMediaProtection      `json:"no_media"`
	MaxMentions *MaxMentionsProtection `json:"max_mentions"`
	//IgnoreAfterSeconds int64                 `json:"ignore_after_seconds"`
	// ^ TODO: globally ignore people after a certain time, or after a certain number of messages
}

// TODO: perhaps some union type for UserCanBypass for common fields like enabled, ignorehomeservers, ignoreabovepl
// The granularity of having it configurable per-protection is great for allowing fine-grained control over the
// protections, but is resulting in a lot of repeated code chunks and boilerplate.
// Might be worth using an interface to define some common fields and functions for HandleMessage & co to call.
// Should also hopefully prevent an ugly if/else or switch/match statement chain

// NoMediaProtection will automatically redact the messages if they have a message type not contained in AllowedTypes.
// Enabled - whether the protection is enabled
// IgnoreHomeServers - a list of homeservers to ignore for this protection
// IgnoreAbovePowerLevel - a power level above which to ignore this protection (gt, not gte)
// AllowedTypes - a list of message types to allow. If nil, defaults to ["m.text", "m.notice", "m.emote"]
// AllowInlineImages - whether to allow inline images in messages, like emojis.
// AllowCustomReactions - whether to allow custom emoji reactions to messages.
type NoMediaProtection struct {
	Enabled               bool      `json:"enabled"`
	IgnoreHomeServers     []string  `json:"ignore_home_servers"`
	IgnoreAbovePowerLevel *int64    `json:"ignore_power_level_above"`
	AllowedTypes          *[]string `json:"allowed_types"`
	AllowInlineImages     bool      `json:"allow_inline_images"`
	AllowCustomReactions  bool      `json:"allow_custom_reactions"`
}

func (p *NoMediaProtection) UserCanBypass(userID id.UserID, powerLevels *event.PowerLevelsEventContent) bool {
	if len(p.IgnoreHomeServers) > 0 && slices.Contains(p.IgnoreHomeServers, userID.Homeserver()) {
		return true
	}
	if powerLevels != nil {
		userPL, ok := powerLevels.Users[userID]
		if !ok {
			userPL = powerLevels.UsersDefault
		}
		if p.IgnoreAbovePowerLevel != nil && int64(userPL) > *p.IgnoreAbovePowerLevel {
			return true
		}
	}
	return false
}

type MentionCounter struct {
	Hits        int
	Infractions int
	Expires     time.Time
	Start       time.Time
}

// MaxMentionsProtection will automatically redact the messages if the number of mentions exceeds the configured limit
// Enabled - whether the protection is enabled
// MaxMentions - the maximum number of mentions allowed in a message, or in the given period.
// Period - the time period in seconds to count mentions. Set to 0 to only count per-message.
type MaxMentionsProtection struct {
	Enabled               bool     `json:"enabled"`
	MaxMentions           int      `json:"max_mentions"`
	MaxInfractions        *int     `json:"max_infractions"`
	Period                int      `json:"period"`
	IgnoreAbovePowerLevel *int64   `json:"ignore_power_level_above"`
	IgnoreHomeServers     []string `json:"ignore_home_servers"`
	users                 map[id.UserID]*MentionCounter
}

// GetUser fetches the mention counter for a user, deleting it if it is expired
func (p *MaxMentionsProtection) GetUser(user id.UserID) *MentionCounter {
	if p.users == nil {
		p.users = make(map[id.UserID]*MentionCounter)
	}
	userCounter, ok := p.users[user]
	if ok {
		if time.Now().After(userCounter.Expires) {
			delete(p.users, user)
			userCounter = nil
		}
	}
	return userCounter
}

// IncrementUser increments the mention counter for a user by n, creating it if it doesn't exist
func (p *MaxMentionsProtection) IncrementUser(user id.UserID, n int) *MentionCounter {
	c := p.GetUser(user)
	if c == nil {
		c = &MentionCounter{Hits: 0, Expires: time.Now().Add(time.Duration(p.Period) * time.Second), Start: time.Now()}
	}
	c.Hits += n
	p.users[user] = c
	return c
}

// IncrementInfractions increments the infractions for a user by 1, creating it if it doesn't exist
func (p *MaxMentionsProtection) IncrementInfractions(user id.UserID) *MentionCounter {
	c := p.GetUser(user)
	if c == nil {
		c = &MentionCounter{Hits: 0, Expires: time.Now().Add(time.Duration(p.Period) * time.Second), Start: time.Now()}
	}
	if p.MaxInfractions != nil {
		c.Infractions += 1
	}
	p.users[user] = c
	return c
}

func (p *MaxMentionsProtection) UserCanBypass(userID id.UserID, powerLevels *event.PowerLevelsEventContent) bool {
	if len(p.IgnoreHomeServers) > 0 && slices.Contains(p.IgnoreHomeServers, userID.Homeserver()) {
		return true
	}
	if powerLevels != nil {
		userPL, ok := powerLevels.Users[userID]
		if !ok {
			userPL = powerLevels.UsersDefault
		}
		if p.IgnoreAbovePowerLevel != nil && int64(userPL) > *p.IgnoreAbovePowerLevel {
			return true
		}
	}
	return false
}

func init() {
	event.TypeMap[StateWatchedLists] = reflect.TypeOf(WatchedListsEventContent{})
	event.TypeMap[StateProtectedRooms] = reflect.TypeOf(ProtectedRoomsEventContent{})
	event.TypeMap[StateProtections] = reflect.TypeOf(StateProtectionsEventContent{})
}
