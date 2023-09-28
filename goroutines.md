# Goroutines

This explains the goroutines that are used.

The normal way the program terminates is via a signal.

The trickiest part is making sure everything shuts down cleanly on termination.

## Main

1. Starts signal monitoring goroutine
1. Creates wait group
1. Starts scanning goroutine in wait group
1. Starts serial packet broadcast goroutine in wait group
1. Starts SSE broadcast goroutine in wait group
1. Does GPS initialization.
1. Starts TCP listener in wait group
1. Starts HTTP listener in wait group
1. Starts ptp4l control in wait group
1. Starts PPS reading
1. Starts syncing goroutine in wait group
1. Waits on the wait group

### Broadcast shutdown

Receives cancellation message and closes the serial packet broadcast and SSE broadcast goroutines.

## Signal monitoring

Receives OS signal and cancels context.

## Syncing

This is the central goroutine that does the real work.

- Receives scanned serial packets through subscription to serial packet broadcast
- Receives events from PHC reading.

Adjusts the PHC.

Sends to SSE broadcast goroutine.

Sends to ptp4l control goroutine.

Shuts down when its two receiving channels are closed.

When this is shutdown, sending channel to SSE broadcast and ptp4l control are closed.

## Scanning

Reads from serial port, divides up into packets

Sends to serial packet broadcast.

## PPS reading

Reads events from PTP hardware clock.

Sends to syncing goroutine.

Receives cancellation and shutdowns.

On shutdown closes channel sending to sync goroutine.

## Serial writing

Writes configuration messages to serial device.

## Serial packet broadcast

Receives packets from scanning goroutine and broadcasts to subscribers.

Receives subscribe and unsubscribe requests.

When closed:
- stops broadcasting
- closes every channel used to send to subscribers

Shutdowns when closed and input channel is closed.

## SSE broadcast

Receives SSE events and broadcasts to subscribers.

Receives subscribe and unsubscribe requests.

## TCP

### Listener

Listener per listening endpoint (i.e. per `[[tcp]]` section in the TOML config).

### Connection reader

### Connection writer


## HTTP

Uses Go's standard library HTTP support.

Uses HTML Server Sent Events (SSE) to push data to Web clients.

### Listener

Listener per listening endpoint (i.e. per `[[http]]` section in the TOML config).

### Handler

For a request to the SSE URL, the handler subscribes to SSE events from SSE
broadcast goroutine, and sends to the client until the client closes the HTTP connection.

### Shutdown

Receives cancellation message and does graceful shutdown of the HTTP server.

## ptp4l control

Receives messages to update the grandmaster properties from the syncing goroutine.

Communicates over a socket with ptp4l process.

Shuts down when its receiving channel is closed.

Need to be careful that this keeps up with messages sent by sync goroutine, so that the sync goroutine doesn't get blocked. 

