package cg

// Create a new game.
const EventCreateGame EventName = "create_game"

type EventCreateGameData struct {
	// If public is set to true, the game will be listed publicly.
	Public bool `json:"public"`
}

// The `created_game` event is the response of the server to the client, which sent the create_game event.
const EventCreatedGame EventName = "created_game"

type EventCreatedGameData struct {
	// The ID of the game that was created.
	GameId string `json:"game_id"`
}

// Join an existing game by ID.
const EventJoinGame EventName = "join_game"

type EventJoinGameData struct {
	// The ID of the game to join.
	GameId string `json:"game_id"`
	// The name of your new user.
	Username string `json:"username"`
}

// The `joined_game` event is sent to everyone in the game when someone joins it.
const EventJoinedGame EventName = "joined_game"

type EventJoinedGameData struct {
	// The username of the newly joined player.
	Username string `json:"username"`
}

// The `player_secret` event is used to send a secret to the player that just joined so that they can reconnect and add other clients.
const EventPlayerSecret EventName = "player_secret"

type EventPlayerSecretData struct {
	// The player secret.
	Secret string `json:"secret"`
}

// The `leave_game` event is used to leave a game which is the preferred way to exit a game in comparison to just disconnecting and never reconnecting.
// It is not required to send this event due to how hard it is to detect if the user has disconnected for good or is just re-writing their program.
const EventLeaveGame EventName = "leave_game"

type EventLeaveGameData struct {
}

// The `left_game` event is sent to everyone in the game when someone leaves it.
const EventLeftGame EventName = "left_game"

type EventLeftGameData struct {
}

// The `disconnected` event is sent to everyone in the game when someone disconnects from the server.
const EventDisconnected EventName = "disconnected"

type EventDisconnectedData struct {
}

// The `connect` event is used to associate a client with an existing player.
// This event is used after making changes to ones program and reconnecting to the game or when adding another client like a viewer in the webbrowser.
const EventConnect EventName = "connect"

type EventConnectData struct {
	// The ID of game to connect to.
	GameId string `json:"game_id"`
	// The ID of the player to connect to.
	PlayerId string `json:"player_id"`
	// The secret of the player to connect to.
	Secret string `json:"secret"`
}

// The `connected` event is sent to everyone in the game when a player connects a client to the server.
const EventConnected EventName = "connected"

type EventConnectedData struct {
}

// The `game_info` event is sent to every player that joins or connects to a game.
const EventGameInfo EventName = "game_info"

type EventGameInfoData struct {
	// The IDs of all players currently in the game mapped to their respective usernames.
	Players map[string]string `json:"players"`
}

// The error event is sent to the client that triggered the error.
// The error event should only be used for technical errors such as event deserialisation errors.
// If something in the game doesn’t work intentionally or a very specific error that requires handeling by the client occurs,
// a custom event should be used.
const EventError EventName = "error"

type EventErrorData struct {
	// The reason the error occured.
	Reason string `json:"reason"`
}

// Returns true if eventName is a standard event.
func IsStandardEvent(eventName EventName) bool {
	return eventName == EventCreateGame || eventName == EventCreatedGame || eventName == EventDisconnected ||
		eventName == EventError || eventName == EventGameInfo || eventName == EventJoinGame || eventName == EventJoinedGame ||
		eventName == EventLeaveGame || eventName == EventLeftGame || eventName == EventConnect || eventName == EventConnected ||
		eventName == EventPlayerSecret
}
