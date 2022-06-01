package cg

// Join an existing game by ID.
const JoinEvent EventName = "cg_join"

type JoinEventData struct {
	// The ID of the game to join.
	GameId string `json:"game_id"`
	// The name of your new user.
	Username string `json:"username"`
}

// The `cg_joined` event is used to send a secret to the player that just joined so that they can reconnect and add other clients.
// It also confirms that the game has been joined successfully.
const JoinedEvent EventName = "cg_joined"

type JoinedEventData struct {
	// The player secret.
	Secret string `json:"secret"`
}

// The `cg_new_player` event is sent to everyone in the game when someone joins it.
const NewPlayerEvent EventName = "cg_new_player"

type NewPlayerEventData struct {
	// The username of the newly joined player.
	Username string `json:"username"`
}

// The `cg_leave` event is used to leave a game which is the preferred way to exit a game in comparison to just disconnecting and never reconnecting.
// It is not required to send this event due to how hard it is to detect if the user has disconnected for good or is just re-writing their program.
const LeaveEvent EventName = "cg_leave"

type LeaveEventData struct {
}

// The `cg_left` event is sent to everyone in the game when someone leaves it.
const LeftEvent EventName = "cg_left"

type LeftEventData struct {
}

// The `cg_connect` event is used to associate a client with an existing player.
// This event is used after making changes to ones program and reconnecting to the game or when adding another client like a viewer in the webbrowser.
const ConnectEvent EventName = "cg_connect"

type ConnectEventData struct {
	// The ID of the game to connect to.
	GameId string `json:"game_id"`
	// The ID of the player to connect to.
	PlayerId string `json:"player_id"`
	// The secret of the player to connect to.
	Secret string `json:"secret"`
}

// The `cg_connected` event is sent to the socket that has connected.
const ConnectedEvent EventName = "cg_connected"

type ConnectedEventData struct {
	// The username of the player.
	Username string `json:"username"`
}

// The `cg_spectate` event is used to spectate a game.
// Spectators receive all public game events but cannot send any.
const SpectateEvent EventName = "cg_spectate"

type SpectateEventData struct {
	// The ID of the game to spectate.
	GameId string `json:"game_id"`
}

// The `cg_info` event is sent to every player that joins, connects to or spectates a game and catches them up
// on things that may have happened before they were connected.
const InfoEvent EventName = "cg_info"

type InfoEventData struct {
	// The IDs of all players currently in the game mapped to their respective usernames.
	Players map[string]string `json:"players"`
}

// The `cg_error` event is sent to the client that triggered the error.
// The error event should only be used for technical errors such as event deserialisation errors.
// If something in the game doesn’t work intentionally or a very specific error that requires
// handeling by the client occurs, a custom event should be used.
const ErrorEvent EventName = "cg_error"

type ErrorEventData struct {
	// The error message.
	Message string `json:"message"`
}

// IsStandardEvent returns true if eventName is a standard event.
func IsStandardEvent(eventName EventName) bool {
	return eventName == JoinEvent || eventName == JoinedEvent || eventName == NewPlayerEvent ||
		eventName == LeaveEvent || eventName == LeftEvent || eventName == ConnectEvent ||
		eventName == ConnectedEvent || eventName == SpectateEvent || eventName == InfoEvent || eventName == ErrorEvent
}
