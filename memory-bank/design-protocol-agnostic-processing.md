# Design: Protocol-Agnostic Processing

## Problem Statement

Currently, both the `gpsevent.Dispatcher` and `gpscfg` package have direct knowledge of specific GPS protocols (UBX, NMEA, RTCM). This creates tight coupling between these components and protocol implementations, making it difficult to:

1. Add new protocols without modifying multiple components
2. Test components in isolation
3. Maintain a clean separation of concerns
4. Handle stateful protocols like NMEA GSV messages consistently

The goal is to refactor both the `gpsevent.Dispatcher` and `gpscfg` package to reduce their knowledge of specific protocols, making the system more modular and maintainable.

## Current Architecture

The current architecture has the following characteristics:

1. `gpsevent.Dispatcher` directly imports `nmea`, `ubx`, and other protocol-specific packages
2. `Dispatcher.handlePacket` contains a switch statement that dispatches based on packet kind
3. `Dispatcher` implements both `gpsprot.MsgHandler` (via embedding `DefaultHandler`) and `ubx.ProtHandler`
4. Protocol-specific processing is done directly in the `Dispatcher`
5. `gpscfg` package has direct knowledge of specific protocols for configuration

```
┌─────────────┐     ┌─────────────┐
│    scan     │────▶│  gpsevent   │
└─────────────┘     │ Dispatcher  │
                    └─────┬───────┘
                          │
                          ▼
      ┌──────────────────┬──────────────────┐
      │                  │                  │
┌─────▼─────┐     ┌──────▼─────┐     ┌──────▼─────┐
│    ubx    │     │    nmea    │     │    rtcm    │
└───────────┘     └────────────┘     └────────────┘


┌─────────────┐
│   gpscfg    │
└─────┬───────┘
      │
      ▼
┌─────────────┐
│    ubx      │
└─────────────┘
```

## Proposed Architecture

The proposed architecture introduces several new interfaces:

1. `Tag` (renamed from `PacketKind`): Identifies the protocol type
2. `PacketProcessor`: Processes packets of a specific protocol
3. `NativeMsgHandler`: Handles protocol-specific messages
4. `PacketExchanger` (renamed from `Protocol`): Exchanges packets with the GPS receiver for configuration

```
┌─────────────┐     ┌─────────────┐
│    scan     │────▶│  gpsevent   │
└─────────────┘     │ Dispatcher  │
                    └─────┬───────┘
                          │
                    ┌─────▼─────┐
                    │  gpsreg   │
                    │ Processors│
                    └─────┬─────┘
                          │
      ┌──────────────────┬──────────────────┐
      │                  │                  │
┌─────▼─────┐     ┌──────▼─────┐     ┌──────▼─────┐
│    ubx    │     │    nmea    │     │    rtcm    │
│ Processor │     │ Processor  │     │ Processor  │
└───────────┘     └────────────┘     └────────────┘


┌─────────────┐
│   gpscfg    │
└─────┬───────┘
      │
      ▼
┌─────────────┐
│  Processors │
└─────┬───────┘
      │
      ▼
┌─────────────┐
│ Exchangers  │
└─────────────┘
```

## Interface Definitions

### 1. Tag (renamed from PacketKind)

```go
// Tag identifies the protocol type
type Tag string

const InvalidTag Tag = "Invalid"
```

### 2. PacketProcessor Interface

```go
// PacketProcessor processes packets of a specific protocol
type PacketProcessor interface {
    // ProcessPacket processes a packet's data and returns any error
    ProcessPacket(data string, tRead time.Time) error
    
    // SetMsgHandler sets the handler for protocol-agnostic messages
    SetMsgHandler(handler MsgHandler)
    
    // SetNativeMsgHandler sets the handler for protocol-specific messages
    SetNativeMsgHandler(handler NativeMsgHandler)
    
    // GetNativeMsgHandler returns the current protocol message handler
    GetNativeMsgHandler() NativeMsgHandler
    
    // CreatePacketExchanger creates a PacketExchanger if configuration is supported
    // Returns nil if configuration is not supported
    CreatePacketExchanger() PacketExchanger
}
```

### 3. NativeMsgHandler Interface

```go
// NativeMsgHandler handles protocol-specific messages
type NativeMsgHandler interface {
    // NativeMsg handles a protocol-specific message
    // Returns true if the message was handled
    NativeMsg(tag Tag, msgType string, msg interface{}, tRead time.Time) bool
}
```

### 4. PacketExchanger Interface (renamed from Protocol)

```go
// PacketExchanger manages packet exchange with a GPS receiver
type PacketExchanger interface {
    // ProbePacket returns a packet to be sent to probe the GPS receiver
    ProbePacket() []byte
    
    // ProbeOK returns true if a packet has been processed that indicates the GPS receiver is responding
    ProbeOK() bool
    
    // Configure creates a Configurator for the given target
    Configure(*ConfigTarget) (Configurator, error)
}
```

## Implementation Details

### 1. Protocol-Specific Processors

```go
// UBXProcessor processes UBX packets
type Processor struct {
    handler      gpsprot.MsgHandler
    nativeHandler gpsprot.NativeMsgHandler
}

func NewProcessor() *Processor {
    return &Processor{}
}

func (p *Processor) ProcessPacket(data string, tRead time.Time) error {
    m, err := bin.ParseMsg(data)
    if err != nil {
        return err
    }
    
    // Try to dispatch as a standard message
    if Dispatch(m, tRead, p.handler) {
        return nil
    }
    
    // If not a standard message, try the native handler
    if p.nativeHandler != nil {
        msgType := fmt.Sprintf("UBX-%s-%s", m.Class(), m.ID())
        if p.nativeHandler.NativeMsg(ubx.Tag, msgType, m, tRead) {
            return nil
        }
    }
    
    return nil
}

func (p *Processor) SetMsgHandler(h gpsprot.MsgHandler) {
    p.handler = h
}

func (p *Processor) SetNativeMsgHandler(h gpsprot.NativeMsgHandler) {
    p.nativeHandler = h
}

func (p *Processor) GetNativeMsgHandler() gpsprot.NativeMsgHandler {
    return p.nativeHandler
}

func (p *Processor) CreatePacketExchanger() gpsprot.PacketExchanger {
    return NewPacketExchanger(p)
}
```

### 2. Protocol-Specific Exchangers

```go
// UBXPacketExchanger implements the PacketExchanger interface
type PacketExchanger struct {
    processor *Processor
    ver       *Version
}

func NewPacketExchanger(processor *Processor) *PacketExchanger {
    p := &PacketExchanger{
        processor: processor,
    }
    
    // Save the existing native handler to chain to
    existingHandler := processor.GetNativeMsgHandler()
    
    // Set our own native handler that chains to the existing one
    processor.SetNativeMsgHandler(&exchangerNativeHandler{
        exchanger: p,
        next:     existingHandler,
    })
    
    return p
}

func (p *PacketExchanger) ProbePacket() []byte {
    return bin.Poll(bin.MonVerID)
}

func (p *PacketExchanger) ProbeOK() bool {
    return p.ver != nil
}

func (p *PacketExchanger) Configure(target *ConfigTarget) (Configurator, error) {
    if p.ver == nil {
        panic("Configure called before probe OK")
    }
    
    // Create configurator
    cfg := newConfigurator(target, p.ver)
    
    return cfg, nil
}

// exchangerNativeHandler handles native messages for the PacketExchanger
type exchangerNativeHandler struct {
    exchanger *PacketExchanger
    next     gpsprot.NativeMsgHandler
}

func (h *exchangerNativeHandler) NativeMsg(tag gpsprot.Tag, msgType string, msg interface{}, tRead time.Time) bool {
    // Only handle UBX messages
    if tag != ubx.Tag {
        return false
    }
    
    ubxMsg, ok := msg.(bin.Msg)
    if !ok {
        return false
    }
    
    // Handle specific messages
    switch mt := ubxMsg.(type) {
    case *bin.MonVer:
        h.exchanger.ver = monVer(mt)
        return true
    }
    
    // Pass to the next handler if we didn't handle it
    if h.next != nil {
        return h.next.NativeMsg(tag, msgType, msg, tRead)
    }
    
    return false
}
```

### 3. Registry Function in gpsreg

```go
// In internal/gpsreg/reg.go

// CreateProcessors creates packet processors for all registered protocols
func CreateProcessors() map[gpsprot.Tag]gpsprot.PacketProcessor {
    processors := make(map[gpsprot.Tag]gpsprot.PacketProcessor)
    
    processors[ubx.Tag] = ubx.NewProcessor()
    processors[nmea.Tag] = nmea.NewProcessor()
    processors[rtcm.Tag] = rtcm.NewProcessor()
    
    return processors
}
```

### 4. Dispatcher Implementation

```go
// In internal/gpsevent/dispatcher.go

type Dispatcher struct {
    gpsprot.DefaultHandler
    // ... existing fields ...
    processors map[gpsprot.Tag]gpsprot.PacketProcessor
    loggedUnknownProtocol bool
}

// Implement NativeMsgHandler
func (d *Dispatcher) NativeMsg(tag gpsprot.Tag, msgType string, msg interface{}, tRead time.Time) bool {
    // Log the message for debugging
    d.lg.Debug("received protocol-specific message", "tag", tag, "type", msgType)
    
    // We don't actually handle the message, just log it
    return false
}

func (d *Dispatcher) handlePacket(pkt scan.Packet) {
    lg := d.lg
    
    processor, ok := d.processors[pkt.Tag]
    if !ok {
        if pkt.Tag == gpsprot.InvalidTag {
            if !d.loggedUnknownProtocol {
                lg.Info("received data from GPS in unknown protocol (serial communication problem?)", 
                       "len", len(pkt.Data), "data", pkt.Data)
                d.loggedUnknownProtocol = true
            }
        } else {
            lg.Error("no processor registered for packet tag", "tag", pkt.Tag)
        }
        return
    }
    
    err := processor.ProcessPacket(pkt.Data, pkt.TRead)
    if err != nil {
        lg.Error("failed to process packet", "tag", pkt.Tag, "err", err)
    }
}
```

### 5. Configuration Flow

```go
// In internal/gpscfg/gpscfg.go

func Configure(ctx context.Context, lg *slog.Logger, target *gpsprot.ConfigTarget, 
              packetCh <-chan scan.Packet, port gpsio.OutPort) (*Result, error) {
    // Create message handler
    mh := msgHandler{}
    mh.init(lg, packetCh)
    
    // Create processors for all supported protocols
    processors := gpsreg.CreateProcessors()
    
    // Set message handler on all processors
    for _, processor := range processors {
        processor.SetMsgHandler(&mh)
        processor.SetNativeMsgHandler(&mh)
    }
    
    // Try to detect and configure using each protocol that supports configuration
    for tag, processor := range processors {
        // Create exchanger if supported
        exchanger := processor.CreatePacketExchanger()
        if exchanger == nil {
            // This processor doesn't support configuration
            continue
        }
        
        // Probe the GPS receiver
        ok, err := mh.probe(ctx, exchanger, port)
        if err != nil {
            return nil, err
        }
        
        if ok {
            // Configure the GPS receiver
            cfg, err := exchanger.Configure(target)
            if err != nil {
                return nil, err
            }
            
            cm, err := mh.configure(ctx, cfg, port)
            if err != nil {
                return nil, err
            }
            
            return mh.finish(cm), nil
        }
    }
    
    // No supported protocol found
    return nil, errors.New("no supported protocol detected")
}
```

## Benefits

1. **Decoupling**:
   - `Dispatcher` no longer has protocol-specific knowledge
   - `gpscfg` no longer has protocol-specific knowledge
   - Protocol-specific code is contained within protocol-specific packages

2. **Extensibility**:
   - New protocols can be added without modifying existing code
   - Protocols can implement just `PacketProcessor` if they don't support configuration

3. **Testability**:
   - Components can be tested with mock processors and exchangers
   - Protocol-specific behavior can be tested in isolation

4. **Maintainability**:
   - Clear separation of concerns
   - Protocol-specific code is contained within its respective package
   - Consistent naming conventions

5. **State Management**:
   - Each processor can maintain its own state
   - Stateful protocols like NMEA GSV can accumulate data across multiple messages

## Considerations

1. **Migration Strategy**:
   - Update core interfaces first
   - Implement protocol-specific processors and exchangers
   - Update `Dispatcher` to use processors
   - Update `gpscfg` to be protocol-agnostic

2. **Performance**:
   - The additional layers of abstraction might have a small performance impact
   - This should be negligible compared to the I/O operations involved

3. **Error Handling**:
   - Consistent error handling across processors
   - Clear logging of protocol-specific errors

4. **Backward Compatibility**:
   - Ensure existing functionality continues to work
   - Provide a smooth transition path

## Future Enhancements

1. **Dynamic Protocol Registration**:
   - Allow protocols to be registered at runtime
   - Support for plugin-based architecture

2. **Enhanced Monitoring**:
   - Protocol-specific monitoring capabilities
   - Detailed statistics for each protocol

3. **Configuration Validation**:
   - Protocol-specific validation of configuration
   - Better error reporting for configuration issues

4. **Protocol Auto-Detection**:
   - Automatically detect the protocol used by a GPS receiver
   - Dynamically select the appropriate processor and exchanger
