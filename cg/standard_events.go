/*
CodeGame v0.3
*/
package cg

// Create a new game.
const EventCreate EventName = "cg_create"

type EventCreateData struct {
	// If public is set to true, the game will be listed publicly.
	Public bool `json:"public"`
}

// The `cg_created` event is the response of the server to the client, which sent the create_game event.
const EventCreated EventName = "cg_created"

type EventCreatedData struct {
	// The ID of the game that was created.
	GameId string `json:"game_id"`
}

// Join an existing game by ID.
const EventJoin EventName = "cg_join"

type EventJoinData struct {
	// The ID of the game to join.
	GameId string `json:"game_id"`
	// The name of your new user.
	Username string `json:"username"`
}

// The `cg_joined` event is used to send a secret to the player that just joined so that they can reconnect and add other clients.
// It also confirms that the game has been joined successfully.
const EventJoined EventName = "cg_joined"

type EventJoinedData struct {
	// The player secret.
	Secret string `json:"secret"`
}

// The `new_player` event is sent to everyone in the game when someone joins it.
const EventNewPlayer EventName = "cg_new_player"

type EventNewPlayerData struct {
	// The username of the newly joined player.
	Username string `json:"username"`
}

// The `cg_leave` event is used to leave a game which is the preferred way to exit a game in comparison to just disconnecting and never reconnecting.
// It is not required to send this event due to how hard it is to detect if the user has disconnected for good or is just re-writing their program.
const EventLeave EventName = "cg_leave"

type EventLeaveData struct {
}

// The `cg_left` event is sent to everyone in the game when someone leaves it.
const EventLeft EventName = "cg_left"

type EventLeftData struct {
}

// The `cg_connect` event is used to associate a client with an existing player.
// This event is used after making changes to ones program and reconnecting to the game or when adding another client like a viewer in the webbrowser.
const EventConnect EventName = "cg_connect"

type EventConnectData struct {
	// The ID of game to connect to.
	GameId string `json:"game_id"`
	// The ID of the player to connect to.
	PlayerId string `json:"player_id"`
	// The secret of the player to connect to.
	Secret string `json:"secret"`
}

// The `cg_connected` event is sent to the socket that has connected.
const EventConnected EventName = "cg_connected"

type EventConnectedData struct {
}

// The `cg_info` event is sent to every player that joins or connects to a game and catches them up
// on things that may have happened before they were connected.
const EventInfo EventName = "cg_info"

type EventInfoData struct {
	// The IDs of all players currently in the game mapped to their respective usernames.
	Players map[string]string `json:"players"`
}

// The error event is sent to the client that triggered the error.
// The error event should only be used for technical errors such as event deserialisation errors.
// If something in the game doesn’t work intentionally or a very specific error that requires
// handeling by the client occurs, a custom event should be used.
const EventError EventName = "cg_error"

type EventErrorData struct {
	// The reason the error occured.
	Reason string `json:"reason"`
}

// IsStandardEvent returns true if eventName is a standard event.
func IsStandardEvent(eventName EventName) bool {
	return eventName == EventCreate || eventName == EventCreated || eventName == EventJoin ||
		eventName == EventJoined || eventName == EventNewPlayer || eventName == EventLeave ||
		eventName == EventLeft || eventName == EventConnect || eventName == EventConnected ||
		eventName == EventInfo || eventName == EventError
}
