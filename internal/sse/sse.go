package sse

import "encoding/json"

type Event struct {
	Event string
	Data  string
}

func Make(event string, v any) (Event, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return Event{}, err
	}
	return Event{
		Event: event,
		Data:  string(b),
	}, nil
}
