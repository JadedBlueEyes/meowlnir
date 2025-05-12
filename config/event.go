package config

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"time"

	"maunium.net/go/mautrix"

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
	NoMedia     NoMediaProtection             `json:"no_media"`
	MaxMentions *MaxMentionsProtection        `json:"max_mentions"`
	RegReqs     *ServerRequirementsProtection `json:"server_requirements"`
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

type ServerRequirementsProtection struct {
	Enabled                  bool `json:"enabled"`
	RequireCaptcha           bool `json:"require_captcha"`
	RequireEmail             bool `json:"require_email"`
	RequirePhone             bool `json:"require_phone"`
	RequireRegistrationToken bool `json:"require_registration_token"`
	RequireExternalAuth      bool `json:"require_external_auth"`

	cache map[string]bool
	lock  sync.Mutex
}

func (p *ServerRequirementsProtection) getServer(name string) *bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.cache == nil {
		p.cache = make(map[string]bool)
	}
	pass, ok := p.cache[name]
	if ok {
		return &pass
	}
	return nil
}

func (p *ServerRequirementsProtection) setServer(name string, pass bool) *bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.cache == nil {
		p.cache = make(map[string]bool)
	}
	p.cache[name] = pass
	time.AfterFunc(time.Hour*12, func() { p.popServer(name) })
	return &pass
}

func (p *ServerRequirementsProtection) popServer(name string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.cache == nil {
		p.cache = make(map[string]bool)
	}
	delete(p.cache, name)
}

func (p *ServerRequirementsProtection) MeetsRequirements(reqs *mautrix.RespUserInteractive) bool {
	if reqs == nil || len(reqs.Flows) == 0 {
		// There are no requirements, so we can skip this check
		return false
	}

	// Only the first flow matters
	flow := reqs.Flows[0]
	if len(flow.Stages) == 0 {
		// No stages, so we can skip this check
		// This would be weird, there should be at least `m.login.dummy`, but whatever
		return false
	}
	if p.RequireCaptcha && !slices.Contains(flow.Stages, "m.login.recaptcha") {
		return false
	}
	if p.RequireEmail && !slices.Contains(flow.Stages, "m.login.email.identity") {
		return false
	}
	if p.RequirePhone && !slices.Contains(flow.Stages, "m.login.msisdn") {
		return false
	}
	if p.RequireRegistrationToken && !slices.Contains(flow.Stages, "m.login.registration_token") {
		return false
	}
	return true
}

func (p *ServerRequirementsProtection) CheckServer(ctx context.Context, name string) (*bool, error) {
	pass := p.getServer(name)
	if pass != nil {
		return pass, nil
	}

	discover, err := mautrix.DiscoverClientAPI(ctx, name)
	var baseUrl string
	if err != nil {
		return nil, err
	}
	if discover == nil || discover.Homeserver.BaseURL == "" {
		baseUrl = "https://" + name
	} else {
		baseUrl = discover.Homeserver.BaseURL
	}
	client, err := mautrix.NewClient(baseUrl, "", "")
	if err != nil {
		return nil, err
	}

	if p.RequireExternalAuth {
		_, err = client.MakeRequest(ctx, http.MethodGet, client.BuildClientURL("unstable", "org.matrix.msc2965", "auth_metadata"), nil, nil)
		if err != nil {
			if errors.Is(err, mautrix.MUnrecognized) {
				// Server does not support external auth
				return p.setServer(name, false), nil
			}
			return nil, fmt.Errorf("failed to check external auth: %w", err)
		}
		// Server supports external auth, so we can skip the rest of the checks
		return p.setServer(name, true), nil
	}

	_, uiaa, err := client.Register(ctx, &mautrix.ReqRegister{})
	if err != nil {
		if errors.Is(err, mautrix.MForbidden) {
			// Server is not accepting registrations, automatically fulfill the requirements
			return p.setServer(name, true), nil
		}
		return nil, err
	}
	isOkay := false
	if uiaa != nil {
		isOkay = p.MeetsRequirements(uiaa)
	}
	return p.setServer(name, isOkay), nil
}

func init() {
	event.TypeMap[StateWatchedLists] = reflect.TypeOf(WatchedListsEventContent{})
	event.TypeMap[StateProtectedRooms] = reflect.TypeOf(ProtectedRoomsEventContent{})
	event.TypeMap[StateProtections] = reflect.TypeOf(StateProtectionsEventContent{})
}
