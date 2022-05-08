package cg

import (
	"encoding/json"
)

type EventName string

type eventWrapper struct {
	Origin string `json:"origin"`
	Event  Event  `json:"event"`
}

type Event struct {
	Name EventName       `json:"name"`
	Data json.RawMessage `json:"data"`
}

// UnmarshalData decodes the event data into the struct pointed to by targetObjPtr.
func (e *Event) UnmarshalData(targetObjPtr interface{}) error {
	return json.Unmarshal(e.Data, targetObjPtr)
}

// marshalData encodes obj into the Data field of the event.
func (e *Event) marshalData(obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	e.Data = data
	return nil
}
