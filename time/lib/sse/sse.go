package sse

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
)

type Event struct {
	event string
	data  string
}

func Make(event string, v any) (Event, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return Event{}, err
	}
	return Event{
		event: event,
		data:  string(b),
	}, nil
}

func (e Event) Format() string {
	data := formatData(e.data) + "\n"
	if e.event != "" {
		return "event: " + e.event + "\n" + data
	}
	return data
}

func (e Event) IsZero() bool {
	return e.event == "" && e.data == ""
}

func formatData(data string) string {
	if !strings.ContainsAny(data, "\r\n") {
		return "data: " + data + "\n"
	}

	var lines []string

	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		lines = append(lines, "data: "+scanner.Text()+"\n")
	}
	if err := scanner.Err(); err != nil {
		panic(fmt.Errorf("failed to scan data: %v", err))
	}

	return strings.Join(lines, "")
}
