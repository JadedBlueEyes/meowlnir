package config

import (
	"reflect"

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
	Global    Protections               `json:"global"`
	Overrides map[id.RoomID]Protections `json:"overrides"`
}

type Protections struct {
	NoMedia            NoMediaProtection `json:"no_media"`
	IgnoreAfterSeconds int64             `json:"ignore_after_seconds"`
}

type NoMediaProtection struct {
	Enabled              bool     `json:"enabled"`
	IgnorePL             int64    `json:"ignore_power_level"`
	AllowedTypes         []string `json:"allowed_types"`
	AllowInlineImages    bool     `json:"allow_inline_images"`
	AllowCustomReactions bool     `json:"allow_custom_reactions"`
}

func init() {
	event.TypeMap[StateWatchedLists] = reflect.TypeOf(WatchedListsEventContent{})
	event.TypeMap[StateProtectedRooms] = reflect.TypeOf(ProtectedRoomsEventContent{})
	event.TypeMap[StateProtections] = reflect.TypeOf(StateProtectionsEventContent{})
}
