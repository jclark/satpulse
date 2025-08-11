package unc

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

type Configurator struct {
	nativeProps nativeConfigProps
	reqs        []*ConfigRequest
	target      *gpsprot.ConfigTarget
	complete    bool // no more requests will be added to req
	nFinished   int  // number of finished requests
}

type configRequestState int

const (
	stateReadyToSendCommand configRequestState = iota
	stateReadyToSendQuery
	stateAwaitingAckAndResponse
	stateAwaitingAck
	stateAwaitingResponse
	stateFailed
	stateSucceeded
)

func (state configRequestState) isFinal() bool {
	switch state {
	case stateFailed, stateSucceeded:
		return true
	}
	return false
}

type ConfigRequest struct {
	state configRequestState
	cmd   string           // not including CR/LF
	prop  nativeConfigProp // if non-nil, then this means it's a command that updates that prop
	tSent time.Time
	err   error // error in handling this request
}

func (req *ConfigRequest) Packet() []byte {
	return append([]byte(req.cmd), '\r', '\n')
}

func (req *ConfigRequest) SetSentTime(tSent time.Time) {
	req.tSent = tSent
}

func NewConfigurator(target *gpsprot.ConfigTarget) *Configurator {
	return &Configurator{
		target:      target,
		nativeProps: makeNativeProps(),
	}
}

func (c *Configurator) Request(index int) *ConfigRequest {
	return c.reqs[index]
}

func (c *Configurator) GetRequestCount() (int, bool) {
	return len(c.reqs), c.complete
}

// payload is after $ up to but not including the * before the checksum
func (c *Configurator) NMEAMessage(payload string, tRead time.Time) (error, bool) {
	fields := strings.Split(payload, ",")
	if len(fields) == 0 {
		return nil, false
	}
	format := fields[0]
	data := fields[1:]
	switch format {
	case "command":
		return c.commandResponse(data, tRead), true
	case "CONFIG":
		return c.configQueryResponse(data, tRead), true
	}
	return nil, false
}

var responseFieldRegexp = regexp.MustCompile(`^response(?:: OK|[: ](.+))$`)

// handle a command response of the form
// `$command,CONFIG,response: OK*XX\r\f`
// fields here will be {"CONFIG", "response: OK"}
func (c *Configurator) commandResponse(fields []string, tRead time.Time) error {
	if len(fields) != 2 {
		return fmt.Errorf("invalid command response format: %s", fields)
	}
	cmd := fields[0]
	response := fields[1]
	matches := responseFieldRegexp.FindStringSubmatch(response)
	if matches == nil {
		return fmt.Errorf("invalid command response: %s", response)
	}
	responseErr := matches[1]
	for i := c.nFinished; i < len(c.reqs); i++ {
		req := c.reqs[i]
		if req.matchesAck(cmd, tRead) {
			req.handleAck(responseErr)
			return nil
		}
	}
	return fmt.Errorf("no matching request for command response: %s", cmd)
}

const maxResponseDelay = time.Second * 3 / 2

func (req *ConfigRequest) matchesAck(cmd string, tRead time.Time) bool {
	if req.cmd != cmd {
		return false
	}
	delay := tRead.Sub(req.tSent)
	if delay < 0 || delay > maxResponseDelay {
		return false
	}
	switch req.state {
	case stateAwaitingAck, stateAwaitingAckAndResponse:
		break
	default:
		return false
	}
	return true
}

func (req *ConfigRequest) handleAck(responseErr string) {
	if responseErr != "" {
		req.state = stateFailed
		req.err = fmt.Errorf("command rejected: %s", responseErr)
		return
	}
	// since the request was accepted, update our state to reflect that the
	// command was successfully performed, so our state matches the receiver state
	if req.prop != nil {
		err := req.prop.updateFromCommand(req.cmd)
		if err != nil {
			panic(fmt.Sprintf("could not parse generated command: %s: %v", req.cmd, err))
		}
	}
	if req.state == stateAwaitingAckAndResponse {
		req.state = stateAwaitingResponse
	} else {
		req.state = stateSucceeded
	}
}

// configQueryResponse handles a response to query, where the response has the form
// `$CONFIG,PPS,CONFIG PPS ENABLE GPS POSITIVE 100000 1000 0 0*6A`
// fields here with be {"PPS","CONFIG PPS ENABLE GPS POSITIVE 100000 1000 0 0"}
func (c *Configurator) configQueryResponse(fields []string, tRead time.Time) error {
	// TODO find the matching request and possibly update the state
	return c.nativeProps.updateFromQueryResponse(fields[0], fields[1])
}

func (c *Configurator) GenerateQueryReqs() {
	for _, cmd := range generateQueryCommands(c.target) {
		c.reqs = append(c.reqs, &ConfigRequest{
			cmd:   cmd,
			state: stateReadyToSendQuery,
		})
	}
}

func (c *Configurator) GenerateConfigCommands() []string {
	return c.nativeProps.generateConfigCommands(&c.target.Props)
}
