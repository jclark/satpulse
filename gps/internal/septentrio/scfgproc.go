package septentrio

import (
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// ReplyKind classifies a parsed Septentrio ASCII command reply packet
// (mosaic reference guide sec 3.1.3; framing in ReplyPacketFormat).
type ReplyKind int

const (
	// ReplyAck is a "$R:" or "$R!" reply: the command was accepted; the
	// packet carries the command echo and any state lines.
	ReplyAck ReplyKind = iota
	// ReplyNak is a "$R?" reply: the command was refused and left the
	// configuration unchanged; Error carries the receiver's message.
	ReplyNak
	// ReplyLst is a "$R;" reply opening an lst command's output: the
	// command was accepted and ReplyBlock units follow.
	ReplyLst
	// ReplyBlock is one "$-- BLOCK n / m" unit of an lst reply. The unit
	// ending at the real prompt (Prompt != "") is the last.
	ReplyBlock
)

// String returns the reply kind as a short lower-case name.
func (k ReplyKind) String() string {
	switch k {
	case ReplyAck:
		return "ack"
	case ReplyNak:
		return "nak"
	case ReplyLst:
		return "lst"
	case ReplyBlock:
		return "block"
	}
	return fmt.Sprintf("ReplyKind(%d)", int(k))
}

// Reply is a parsed Septentrio ASCII command reply packet. Which fields are
// set depends on Kind; Prompt is common to all kinds: it names the connection
// descriptor from the closing prompt (e.g. "USB1"), and is empty when the
// packet ends at the "---->" pseudo-prompt of an unfinished lst reply.
type Reply struct {
	Kind       ReplyKind
	Echo       string   // verbatim command echo (ReplyAck, ReplyLst)
	States     []string // trimmed state lines (ReplyAck)
	Error      string   // receiver's error text (ReplyNak)
	Block      string   // block content, lines joined with \n (ReplyBlock)
	BlockNum   int      // block sequence number, from 1 (ReplyBlock)
	BlockTotal int      // total block count; 0 when the receiver does not say (ReplyBlock)
	Prompt     string   // connection descriptor, "" at a "---->" pseudo-prompt
}

// ParseReply parses a framed TagReply packet into a Reply.
func ParseReply(data string) (*Reply, error) {
	lines := strings.Split(data, "\r\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("septentrio reply too short: %q", data)
	}
	r := &Reply{}
	if last := lines[len(lines)-1]; last != "---->" {
		if !strings.HasSuffix(last, ">") {
			return nil, fmt.Errorf("septentrio reply does not end in a prompt: %q", last)
		}
		r.Prompt = last[:len(last)-1]
	}
	head := lines[0]
	body := lines[1 : len(lines)-1]
	if strings.HasPrefix(head, "$--") {
		r.Kind = ReplyBlock
		if _, err := fmt.Sscanf(head, "$-- BLOCK %d / %d", &r.BlockNum, &r.BlockTotal); err != nil {
			return nil, fmt.Errorf("septentrio block header %q: %v", head, err)
		}
		r.Block = strings.Join(body, "\n")
		return r, nil
	}
	if len(head) < 3 || head[0] != '$' || head[1] != 'R' {
		return nil, fmt.Errorf("septentrio reply has no $R header: %q", head)
	}
	rest := strings.TrimSpace(head[3:])
	switch head[2] {
	case ':', '!':
		r.Kind = ReplyAck
		r.Echo = rest
		for _, ln := range body {
			if s := strings.TrimSpace(ln); s != "" {
				r.States = append(r.States, s)
			}
		}
	case ';':
		r.Kind = ReplyLst
		r.Echo = rest
	case '?':
		r.Kind = ReplyNak
		r.Error = strings.TrimSpace(strings.Join(append([]string{rest}, body...), "\n"))
	default:
		return nil, fmt.Errorf("septentrio reply has unknown type %q", head[2])
	}
	return r, nil
}

// ReplyProcessor implements gpsprot.PacketProcessor for TagReply packets,
// delivering each framed reply as a parsed *Reply to the native message
// handler. The configuration protocol consumes these native messages.
type ReplyProcessor struct {
	gpsprot.DefaultPacketProcessor
}

// NewReplyProcessor creates a new Septentrio reply packet processor.
func NewReplyProcessor() *ReplyProcessor {
	return &ReplyProcessor{}
}

// ProcessPacket parses a reply packet and forwards it to the native message handler.
func (p *ReplyProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	r, err := ParseReply(data)
	if err != nil {
		return "", err
	}
	msgID := r.Kind.String()
	if nmh := p.GetNativeMsgHandler(); nmh != nil {
		return msgID, nmh.NativeMsg(TagReply, msgID, r, tRead)
	}
	return msgID, nil
}

// NativeOnly returns true since this processor produces only native messages.
func (p *ReplyProcessor) NativeOnly() bool {
	return true
}
