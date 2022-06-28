package cg

import (
	"encoding/json"
)

type EventName string

type Event struct {
	Name EventName       `json:"name"`
	Data json.RawMessage `json:"data"`
}

type CommandName string

type Command struct {
	Name CommandName     `json:"name"`
	Data json.RawMessage `json:"data"`
}

type CommandWrapper struct {
	Origin *Player
	Cmd    Command
}

// UnmarshalData decodes the command data into the struct pointed to by targetObjPtr.
func (c *Command) UnmarshalData(targetObjPtr any) error {
	return json.Unmarshal(c.Data, targetObjPtr)
}

// marshalData encodes obj into the Data field of the event.
func (e *Event) marshalData(obj any) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	e.Data = data
	return nil
}
