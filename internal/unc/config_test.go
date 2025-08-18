package unc

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/uncmsg"
)

// testReceiver simulates a Unicore receiver using nativeConfigProps
type testReceiver struct {
	nativeProps  nativeConfigProps
	version      *uncmsg.Version
	nSent        int
	sentPackets  []string // Track all packets sent for verification
	
	// Error injection
	nakQuery     *string // Query that gets negative ACK
	noAckQuery   *string // Query that gets no ACK  
	noRespQuery  *string // Query that gets ACK but no response
	lateAckQuery *string // Query with ACK after deadline
	
	// Track retry attempts
	retryCount   map[string]int // Count retries per command
}

func runConfiguration(rcvr *testReceiver, target *gpsprot.ConfigTarget) (*Configurator, int, error) {
	cp := &ConfigProtocol{ver: rcvr.version}
	cfgInterface, err := cp.Configure2(target)
	if err != nil {
		return nil, 0, err
	}
	cfg := cfgInterface.(*Configurator)
	
	// Initialize retry count map if needed
	if rcvr.retryCount == nil {
		rcvr.retryCount = make(map[string]int)
	}
	
	processErrors := 0  // Count processing errors

	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	// Run through requests
	maxIterations := 100 // Safety limit to prevent infinite loops
	iterations := 0
	for iterations < maxIterations {
		iterations++
		err := cfg.GenerateRequests()
		if err != nil {
			return nil, 0, err
		}

		count, complete := cfg.GetRequestCount()
		if complete && count == 0 {
			break // Done
		}
		
		// Check if we're done
		if complete && cfg.phase == phaseFinal {
			// Check if all requests are actually done
			allDone := true
			for i := 0; i < count; i++ {
				req := cfg.Request(i)
				state := req.GetState()
				if state != gpsprot.ConfigRequestSucceeded && state != gpsprot.ConfigRequestFailed && state != gpsprot.ConfigRequestSkipped {
					allDone = false
					break
				}
			}
			if allDone {
				break
			}
		}

		// Process requests that are ready
		hasWork := false
		for i := 0; i < count; i++ {
			req := cfg.Request(i)

			switch req.GetState() {
			case gpsprot.ConfigRequestReadyToSend:
				hasWork = true
				pkt := req.GetPacket()
				rcvr.nSent++
				
				// Track sent packet
				pktStr := strings.TrimSuffix(string(pkt), "\r\n")
				rcvr.sentPackets = append(rcvr.sentPackets, pktStr)

				// Simulate send and response
				t0 = t0.Add(10 * time.Millisecond)
				req.SetSentTime(t0)
				
				// Check for error injection
				shouldSkipAck := rcvr.noAckQuery != nil && *rcvr.noAckQuery == pktStr
				shouldSkipResp := rcvr.noRespQuery != nil && *rcvr.noRespQuery == pktStr
				shouldDelayAck := rcvr.lateAckQuery != nil && *rcvr.lateAckQuery == pktStr
				
				if shouldSkipAck {
					// No ACK or response - simulate timeout
					continue
				}
				
				if shouldDelayAck {
					// Delay ACK beyond deadline
					t0 = t0.Add(2 * time.Second) // Beyond maxResponseDelay
				}

				// Generate responses
				responses := rcvr.generateResponses(string(pkt))
				
				// Filter responses based on error injection
				if shouldSkipResp {
					// Keep only ACK, skip other responses
					var ackOnly []interface{}
					for _, resp := range responses {
						if s, ok := resp.(*nmea.Sentence); ok && strings.Contains(s.Payload, "command,") {
							ackOnly = append(ackOnly, resp)
							break
						}
					}
					responses = ackOnly
				}
				
				for _, resp := range responses {
					t0 = t0.Add(5 * time.Millisecond)
					// Call NativeMsg with the correct tag based on response type
					switch r := resp.(type) {
					case *nmea.Sentence:
						err = cp.NativeMsg("NMEA", "", r, t0)
					case *uncmsg.Mode:
						err = cp.NativeMsg("UNCA", "", r, t0)
					}
					if err != nil {
						processErrors++
						// Continue processing even if there's an error
					}
				}

			case gpsprot.ConfigRequestAwaitingResponse:
				// Check for timeout
				if t0.After(req.GetResponseDeadline()) {
					hasWork = true
					req.SetResponseDeadlinePassed()
				}

			case gpsprot.ConfigRequestMaybeComplete:
				// Check idle period  
				deadline := req.GetResponseDeadline()
				if t0.After(deadline) {
					hasWork = true
					req.SetResponseDeadlinePassed()
				}

			case gpsprot.ConfigRequestMayResend:
				hasWork = true
				// Allow one retry for error injection tests
				pkt := req.GetPacket()
				cmd := strings.TrimSuffix(string(pkt), "\r\n")
				rcvr.retryCount[cmd]++
				if rcvr.retryCount[cmd] > 1 {
					req.SetWontResend() // Don't retry more than once
				} else {
					// Retry the request - resend it
					rcvr.nSent++
					
					// Track sent packet
					pktStr := strings.TrimSuffix(string(pkt), "\r\n")
					rcvr.sentPackets = append(rcvr.sentPackets, pktStr)
					
					// Simulate send
					t0 = t0.Add(10 * time.Millisecond)
					req.SetSentTime(t0)
					
					// For retries, don't inject errors anymore  
					// (assume the retry succeeds)
					responses := rcvr.generateResponses(string(pkt))
					for _, resp := range responses {
						t0 = t0.Add(5 * time.Millisecond)
						switch r := resp.(type) {
						case *nmea.Sentence:
							err = cp.NativeMsg("NMEA", "", r, t0)
						case *uncmsg.Mode:
							err = cp.NativeMsg("UNCA", "", r, t0)
						}
						if err != nil {
							processErrors++
							// Continue processing even if there's an error
						}
					}
				}
			}
		}
		
		// If no work was done, advance time to prevent infinite loop
		if !hasWork {
			t0 = t0.Add(100 * time.Millisecond)
		}
	}
	
	if iterations >= maxIterations {
		return nil, 0, fmt.Errorf("test stuck in loop after %d iterations", iterations)
	}

	return cfg, processErrors, nil
}

func (r *testReceiver) generateResponses(cmd string) []interface{} {
	cmd = strings.TrimSuffix(cmd, "\r\n")
	
	// Check if this is a retry (second or later attempt)
	isRetry := r.retryCount != nil && r.retryCount[cmd] > 1

	// Check for NAK - return error ACK (only on first attempt)
	if !isRetry && r.nakQuery != nil && cmd == *r.nakQuery {
		return []interface{}{
			&nmea.Sentence{Payload: fmt.Sprintf("command,%s,response: error", cmd)},
		}
	}

	var responses []interface{}

	switch cmd {
	case "CONFIG":
		// First send ACK
		responses = append(responses, &nmea.Sentence{
			Payload: fmt.Sprintf("command,%s,response: OK", cmd),
		})
		// Then send CONFIG responses
		// PPS response - use the stored command or default
		ppsCmd := r.nativeProps.pps.command
		if ppsCmd == "" {
			ppsCmd = "CONFIG PPS DISABLE" // Default state
		}
		responses = append(responses, &nmea.Sentence{
			Payload: fmt.Sprintf("CONFIG,PPS,%s", ppsCmd),
		})
		
		// SIGNALGROUP response - always send something
		sgCmd := r.nativeProps.signalGroup.command  
		if sgCmd == "" {
			sgCmd = "CONFIG SIGNALGROUP 2" // Default group
		}
		responses = append(responses, &nmea.Sentence{
			Payload: fmt.Sprintf("CONFIG,SIGNALGROUP,%s", sgCmd),
		})

	case "MASK":
		// First send ACK
		responses = append(responses, &nmea.Sentence{
			Payload: fmt.Sprintf("command,%s,response: OK", cmd),
		})
		// Then send MASK responses - always send at least one
		maskCmd := r.nativeProps.mask.elevationMask
		if maskCmd == "" {
			maskCmd = "10" // Default elevation mask
		}
		responses = append(responses, &nmea.Sentence{
			Payload: fmt.Sprintf("CONFIG,MASK,MASK %s", maskCmd),
		})
		
		// Add signal masks if configured
		if r.nativeProps.mask.signalMask != 0 {
			responses = append(responses, &nmea.Sentence{
				Payload: "CONFIG,MASK,MASK GPS",
			})
		}

	case "MODE":
		// First send ACK
		responses = append(responses, &nmea.Sentence{
			Payload: fmt.Sprintf("command,%s,response: OK", cmd),
		})
		// Then send MODE response - use stored mode command or default
		modeCmd := r.nativeProps.mode.command
		if modeCmd == "" {
			modeCmd = "MODE ROVER" // Default mode
		}
		responses = append(responses, &uncmsg.Mode{
			Mode: modeCmd,
		})

	default:
		// Commands - just send ACK
		responses = append(responses, &nmea.Sentence{
			Payload: fmt.Sprintf("command,%s,response: OK", cmd),
		})
	}

	return responses
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}

// Test cases

func TestConfigurator(t *testing.T) {
	tests := []struct {
		name           string
		initialState   []string                     // Commands representing receiver state
		targetProps    func(*gpsprot.ConfigProps)   // Target properties (optional)
		targetOpts     func(*gpsprot.ConfigOptions) // Target options (optional)
		
		// Error injection (single query for each error type)
		nakQuery       *string  // Query that gets negative ACK
		noAckQuery     *string  // Query that gets no ACK
		noRespQuery    *string  // Query that gets ACK but no response  
		lateAckQuery   *string  // Query with ACK after deadline
		
		// Expectations
		expectedSent        []string                     // Expected packets sent (in order)
		expectedStates      map[string]configRequestState // Final states by command
		expectProcessErrors int                           // Number of packets that should produce processing errors
	}{
		// Realistic scenarios (like satpulsed would do)
		{
			name: "basic satpulsed configuration - survey mode",
			initialState: []string{
				"MODE ROVER",
			},
			targetProps: func(p *gpsprot.ConfigProps) {
				p.SetTimePulse(gpsprot.TimePulse{
					Width:          100 * time.Millisecond,
					Period:         time.Second,
					PolarityRising: true,
					OnlyWhenLocked: true,
					AlignToGNSS:    true,
				})
				p.SetTimeGNSS(gpsprot.GPS)
				p.SetAntennaCableDelay(50 * time.Nanosecond)
			},
			targetOpts: func(o *gpsprot.ConfigOptions) {
				o.SetStatic = true
				o.Survey.MinDur = 60 * time.Second
			},
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",  // Query phase
				"CONFIG PPS ENABLE GPS POSITIVE 100000 1000 50 0",
				"MODE BASE TIME 60",
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateSucceeded,
				"MASK":   stateSucceeded,
				"MODE":   stateSucceeded,
				"CONFIG PPS ENABLE GPS POSITIVE 100000 1000 50 0": stateSucceeded,
				"MODE BASE TIME 60": stateSucceeded,
			},
		},
		{
			name: "fixed position configuration",
			initialState: []string{
				"MODE ROVER",
			},
			targetProps: func(p *gpsprot.ConfigProps) {
				p.SetMode(gpsprot.Mode{
					Static:  true,
					PosType: gpsprot.PosTypeECEF,
					FixedPosECEF: gpsprot.Point3D{
						gpsprot.Meters(1000000.0),
						gpsprot.Meters(2000000.0),
						gpsprot.Meters(3000000.0),
					},
					FixedPosAcc: gpsprot.Meters(10.0),
				})
				p.SetRTCMBaseID(123)
			},
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",  // Query phase
				"MODE BASE 123 1000000.0000 2000000.0000 3000000.0000",
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateSucceeded,
				"MASK":   stateSucceeded,
				"MODE":   stateSucceeded,
				"MODE BASE 123 1000000.0000 2000000.0000 3000000.0000": stateSucceeded,
			},
		},
		{
			name: "mobile mode configuration",
			initialState: []string{
				"MODE BASE TIME 60",
			},
			targetProps: func(p *gpsprot.ConfigProps) {
				p.SetMode(gpsprot.Mode{
					Static: false,
				})
			},
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",  // Query phase
				"MODE ROVER",
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateSucceeded,
				"MASK":   stateSucceeded,
				"MODE":   stateSucceeded,
				"MODE ROVER": stateSucceeded,
			},
		},
		
		// Query phase only tests
		{
			name: "query phase with multi-response",
			initialState: []string{
				"CONFIG PPS ENABLE GPS POSITIVE 100000 1000000 0 50",
				"CONFIG SIGNALGROUP 2",
				"MASK 5",
				"MODE ROVER",
			},
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateSucceeded,
				"MASK":   stateSucceeded,
				"MODE":   stateSucceeded,
			},
		},
		
		// Error scenarios
		{
			name: "query with no ACK",
			initialState: []string{
				"MODE ROVER",
			},
			noAckQuery: stringPtr("MODE"),
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",
				"MODE",  // Retry after timeout
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateSucceeded,
				"MASK":   stateSucceeded,  
				"MODE":   stateSucceeded,  // Retry succeeds
			},
		},
		{
			name: "query with ACK but no response",
			initialState: []string{
				"MODE ROVER",
			},
			noRespQuery: stringPtr("CONFIG"),
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",
				"CONFIG",  // Retry after timeout
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateSucceeded,  // Retry succeeds
				"MASK":   stateSucceeded,
				"MODE":   stateSucceeded,
			},
		},
		{
			name: "query with NAK",
			initialState: []string{
				"CONFIG PPS ENABLE GPS POSITIVE 100000 1000000 0 50",
				"MASK 5",
			},
			nakQuery: stringPtr("MODE"),
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",
				// No retry for NAK - fails immediately
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateSucceeded,
				"MASK":   stateSucceeded,
				"MODE":   stateFailed,  // NAK means immediate failure
			},
			expectProcessErrors: 0,  // NAK is handled properly, no processing error
		},
		{
			name: "ACK arrives too late",
			initialState: []string{
				"MODE ROVER",
			},
			lateAckQuery: stringPtr("MASK"),
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",
				"MASK",  // Retry after timeout
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateSucceeded,
				"MASK":   stateSucceeded,  // Retry succeeds
				"MODE":   stateSucceeded,
			},
			expectProcessErrors: 2,  // Both late ACK and late query response cause "no matching request" errors
		},
		{
			name: "command gets negative ACK",
			initialState: []string{
				"MODE ROVER",
			},
			targetProps: func(p *gpsprot.ConfigProps) {
				p.SetMode(gpsprot.Mode{
					Static:  true,
					PosType: gpsprot.PosTypeNone,
				})
			},
			targetOpts: func(o *gpsprot.ConfigOptions) {
				o.Survey.MinDur = 60 * time.Second
			},
			nakQuery: stringPtr("MODE BASE TIME 60"),  // NAK the command
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",
				"MODE BASE TIME 60",
				// No retry for NAK - fails immediately
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateSucceeded,
				"MASK":   stateSucceeded,
				"MODE":   stateSucceeded,
				"MODE BASE TIME 60": stateFailed,  // NAK means immediate failure
			},
			expectProcessErrors: 0,  // NAK is handled properly, no processing error
		},
		{
			name: "query fails but config phase still runs",
			initialState: []string{
				"MODE ROVER",
			},
			targetProps: func(p *gpsprot.ConfigProps) {
				p.SetMode(gpsprot.Mode{Static: true})
			},
			nakQuery: stringPtr("CONFIG"),
			expectedSent: []string{
				"CONFIG", "MASK", "MODE",
				// No retry for NAK - CONFIG fails immediately
				"MODE BASE TIME 0",  // Config phase still runs (using defaults since CONFIG failed)
			},
			expectedStates: map[string]configRequestState{
				"CONFIG": stateFailed,  // NAK means immediate failure
				"MASK":   stateSucceeded,
				"MODE":   stateSucceeded,
				"MODE BASE TIME 0": stateSucceeded,
			},
			expectProcessErrors: 0,  // NAK is handled properly
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test receiver with initial state
			rcvr := &testReceiver{
				version:      &uncmsg.Version{},
				nativeProps:  makeNativeProps(),
				nakQuery:     tt.nakQuery,
				noAckQuery:   tt.noAckQuery,
				noRespQuery:  tt.noRespQuery,
				lateAckQuery: tt.lateAckQuery,
				sentPackets:  []string{},
			}
			
			// Set up initial state
			for _, cmd := range tt.initialState {
				fields := strings.Fields(cmd)
				if len(fields) == 0 {
					continue
				}
				var key string
				if fields[0] == "CONFIG" && len(fields) > 1 {
					key = fields[1]
				} else {
					key = fields[0]
				}
				
				err := rcvr.nativeProps.updateFromQueryResponse(key, cmd)
				if err != nil {
					t.Fatalf("failed to set up initial state for command %s: %v", cmd, err)
				}
			}
			// Debug: check what's stored
			// t.Logf("Initial mode command: %q", rcvr.nativeProps.mode.command)
			
			// Create target
			target := &gpsprot.ConfigTarget{}
			if tt.targetProps != nil {
				tt.targetProps(&target.Props)
			}
			if tt.targetOpts != nil {
				tt.targetOpts(&target.Opts)
			}
			
			// Run configuration
			cfg, processErrors, err := runConfiguration(rcvr, target)
			
			// We don't expect runConfiguration to return an error for these test cases
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			
			// Check process errors
			if processErrors != tt.expectProcessErrors {
				t.Errorf("process errors mismatch: got %d, want %d", processErrors, tt.expectProcessErrors)
			}
			
			// Verify sent packets
			if !slices.Equal(rcvr.sentPackets, tt.expectedSent) {
				t.Errorf("sent packets mismatch\ngot:  %v\nwant: %v", rcvr.sentPackets, tt.expectedSent)
			}
			
			// Verify final states  
			if tt.expectedStates != nil {
				// Debug: print all request states
				if t.Failed() {
					t.Logf("All request states:")
					for i, req := range cfg.reqs {
						t.Logf("  [%d] %q: state=%v err=%v", i, req.cmd, req.state, req.err)
					}
					t.Logf("Phase: %v, Complete: %v, nFinished: %v", cfg.phase, cfg.complete, cfg.nFinished)
				}
				
				for _, req := range cfg.reqs {
					if expectedState, ok := tt.expectedStates[req.cmd]; ok {
						if req.state != expectedState {
							t.Errorf("request %q state mismatch: got %v, want %v", req.cmd, req.state, expectedState)
						}
					}
				}
			}
		})
	}
}

