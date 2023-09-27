# Goroutine structure

(Work in progress.)

Normal way the program terminates is via a signal.

## Design bug

When an interrupt signal is received after the SSE broadcast is started but before syncing goroutine has started,
then there's a deadlock. The SSE broadcast goroutine blocks waiting for its input channel to be closed.
But this never happens, because it's usually by the syncing goroutine, which didn't get to start. 

## Main

1. Starts signal monitoring goroutine.
1. Creates wait group
1. Start scanning gorouting in wait group
1. Starts serial packet broadcast goroutine in wait group
1. Starts SSE broadcast goroutine in wait group
1. Does GPS initialization.
1. Start TCP listener in wait group
1. Start HTTP listener in wait group
1. Start ptp4l control in wait group
1. Start PPS reading
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

Send to ptp4l grandmaster control goroutine.

Shuts down when the two receiving channels are closed.

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

### Connection reader

### Connection writer


## HTTP

There's a Listener for each port and then reader/writer for each connection.

### Listener

### Shutdown

Receives cancellation message and does graceful shutdown of the HTP server.

## ptp4l control

Receives update messages from syncing goroutine.
Communicates over a socket with ptp4l process.
Need to be careful that this keeps up with messages sent by sync goroutine,
so that the sync goroutine doesn't get blocked. 

