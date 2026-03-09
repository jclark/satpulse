# PQTM response classification

## Context

The PQTM command/response protocol needs proper classification in `qtmmsg` to
replace the ad-hoc string matching in `gps/msgfile/text.go`. This is part of a
broader refactoring to make proprietary NMEA handling vendor-generic via a
dispatch map.

## PQTM message variants

For a given PQTM sentence name (e.g. PQTMCFGPPS), there are multiple variants
distinguished by the second comma-separated field:

### Commands (sent by host)

- **Write**: `PQTMCFGPPS,W,1,1,100000,1000,0,0` -- second field is "W"
- **Read**: `PQTMCFGPPS,R,1` -- second field is "R"
- **Simple command**: `PQTMSAVEPAR` (no second field) or `PQTMSRR,1` (parameter)

### Responses (received from receiver)

- **Ack**: `PQTMCFGPPS,OK` -- second field is "OK", no data
- **Nak**: `PQTMCFGPPS,ERROR,1` -- second field is "ERROR", followed by error code
  - Error codes: 1=invalid params, 2=failed execution, 3=unsupported command
- **Query response**: `PQTMCFGPPS,OK,1,1,100000,1000,0,0` -- second field is
  "OK", followed by data fields

### Exceptions

- **PQTMVERNO**: response has no "OK" field, just data:
  `PQTMVERNO,LG290P03AANR01A03S,2024/04/30,10:53:07`
- **Periodic messages** (PQTMPVT, PQTMVEL, etc.): unsolicited output, not
  responses to commands

## Protocol analysis (from lg290p.md)

Every PQTM command response uses the same sentence name as the command, and the
second comma-separated field discriminates the response type:

**Write commands** (CFG Set, simple commands like SAVEPAR):
- Sent: `PQTM<NAME>,W,...` or `PQTM<NAME>`
- OK: `PQTM<NAME>,OK` (bare OK, no data)
- Error: `PQTM<NAME>,ERROR,<code>`

**Read commands** (CFG Get, queries like UNIQID, SN):
- Sent: `PQTM<NAME>,R,...` or `PQTM<NAME>`
- OK: `PQTM<NAME>,OK,<data>` (OK followed by data fields)
- Error: `PQTM<NAME>,ERROR,<code>`

**Exception -- PQTMVERNO**:
- OK: `PQTMVERNO,<VerStr>,<BuildDate>,<BuildTime>` (no OK field, just data)
- Error: `PQTMVERNO,ERROR,<code>`

**Periodic output** (PQTMPVT, PQTMVEL, etc.):
- Unsolicited, not responses to commands
- Already handled by `ParsePeriodicMsg`

Error codes: 1=invalid parameters, 2=execution failed, 3=unsupported command.

## Classification semantics

AckResponse means it is *just* an acknowledgement -- nothing more to show the
user. OtherResponse means it contains data that should be shown.

Mapping from PQTM response type to ResponseKind:

| Response | Example | ResponseKind | AckError |
|----------|---------|-------------|----------|
| Write ack (OK, no data) | `PQTMCFGPPS,OK` | AckResponse | "" |
| Nak (ERROR) | `PQTMCFGPPS,ERROR,1` | AckResponse | error text |
| Query response (OK + data) | `PQTMCFGPPS,OK,1,1,...` | OtherResponse | |
| Data-only response | `PQTMVERNO,LG290P03...` | OtherResponse | |
| Periodic message | `PQTMPVT,1,1000,...` | NotResponse | |

Key insight: the qtmmsg function must distinguish OK-with-no-data (pure ack)
from OK-with-data (query response with content to display).

## qtmmsg API: CheckResponse

Matching: extract the sentence name from both sent and recv -- the part after
"PQTM" and before the first comma (e.g. "CFGPPS" from "PQTMCFGPPS,OK"). If
these match, it's a response.

Then classify by the second field of recv:

```go
type ResponseKind int

const (
	NotResponse   ResponseKind = iota
	ResponseOK    // bare OK, no data
	ResponseData  // OK followed by data, or data without OK (e.g. PQTMVERNO)
	ResponseError // ERROR with error code
)

// CheckResponse checks whether recv is a response to sent.
// Both are PQTM payloads (content between $ and *).
// Returns NotResponse if the sentence names do not match.
// For ResponseError, the string is the human-readable error message.
func CheckResponse(sent, recv string) (ResponseKind, string)
```

Second field logic:
- "OK" with no further fields -> ResponseOK
- "OK" with more fields -> ResponseData
- "ERROR" -> ResponseError, translate error code
- Anything else -> ResponseData (covers PQTMVERNO-style data-only responses)

Error codes: 1="invalid parameters", 2="execution failed",
3="unsupported command".

Implementation sketch:
```go
func CheckResponse(sent, recv string) (ResponseKind, string) {
	sentName, _, _ := strings.Cut(sent, ",")
	recvName, rest, hasRest := strings.Cut(recv, ",")
	if sentName != recvName {
		return NotResponse, ""
	}
	if !hasRest {
		return ResponseData, ""
	}
	second, errCode, hasMore := strings.Cut(rest, ",")
	switch second {
	case "OK":
		if hasMore {
			return ResponseData, ""
		}
		return ResponseOK, ""
	case "ERROR":
		return ResponseError, errorMessage(errCode)
	}
	return ResponseData, ""
}
```

Mapping to msgfile ResponseKind:
- NotResponse -> caller decides (typically OtherResponse for non-periodic)
- ResponseOK -> AckResponse, ""
- ResponseData -> OtherResponse
- ResponseError -> AckResponse, error message

## Planned changes

### In `gps/lib/qtmmsg`

Add `ResponseKind`, `CheckResponse`, and `errorMessage` to a new file
`response.go` (or to `periodic.go`).

### In `gps/msgfile/text.go`

1. Add `nmeaVendor(flags, payload) string` -- extracts 3-letter vendor ID
2. Add vendor classifier map:
   ```go
   type proprietaryClassifier func(sentPayload, recvPayload string) (ResponseKind, string)
   var proprietaryClassifiers = map[string]proprietaryClassifier{
       "QTM": classifyPQTMResponse,
   }
   ```
3. Add `classifyPQTMResponse` using `qtmmsg.CheckResponse`
4. Update `nmeaMatcher.match` to dispatch via vendor map
5. Delete `classifyPQTM`, `classifyPQTMWithAck`, `pqtmSentenceName`

### Verification

```
go test -v ./gps/lib/qtmmsg/
go test -v ./gps/msgfile/
```
