package uncmsg

// fixupRecTimeValueForAscii converts a RecTime with full binary precision
// to match the limited precision values that come from ASCII parsing.
// This simulates the receiver firmware's float-to-ASCII conversion using %.10g formatting.
func fixupRecTimeValueForAscii(msg Msg) Msg {
	r := msg.(*RecTime)
	result := *r // Copy the struct

	const floatFormat = "%.10g"
	// Simulate receiver's ASCII formatting: binary -> printf("%.10g") -> parse back
	fixupFloat(&result.Offset, floatFormat)
	fixupFloat(&result.OffsetStd, floatFormat)
	// UTCOffset typically stays exact since -18.0 is representable exactly

	return &result
}

// fixupGPSUTCValueForAscii converts a GPSUTC with full binary precision
// to match the limited precision values that come from ASCII parsing.
func fixupGPSUTCValueForAscii(msg Msg) Msg {
	g := msg.(*GPSUTC)
	result := *g // Copy the struct

	const floatFormat = "%.10g"

	// Only A1 needs fixup - A0 maintains full precision in ASCII
	fixupFloat(&result.A1, floatFormat)

	return &result
}

// fixupGALUTCValueForAscii converts a GALUTC with full binary precision
// to match the limited precision values that come from ASCII parsing.
func fixupGALUTCValueForAscii(msg Msg) Msg {
	g := msg.(*GALUTC)
	result := *g // Copy the struct

	const floatFormat = "%.16g"
	fixupFloat(&result.DA0g, floatFormat)

	return &result
}

// fixupBDSUTCValueForAscii converts a BDSUTC with full binary precision
// to match the limited precision values that come from ASCII parsing.
func fixupBDSUTCValueForAscii(msg Msg) Msg {
	b := msg.(*BDSUTC)
	result := *b // Copy the struct

	const floatFormat = "%.10g"
	// Fix up floating point precision differences - ASCII has limited precision
	fixupFloat(&result.A1, floatFormat)
	// A0 maintains precision in ASCII format

	return &result
}

// fixupBD3UTCValueForAscii converts a BD3UTC with full binary precision
// to match the limited precision values that come from ASCII parsing.
func fixupBD3UTCValueForAscii(msg Msg) Msg {
	b := msg.(*BD3UTC)
	result := *b // Copy the struct

	// Different precision for different fields
	fixupFloat(&result.A0, "%.16g")  // A0 matches with 16g
	fixupFloat(&result.A1, "%.10g")  // A1 needs less precision
	// A2 maintains exact precision (0.0)

	return &result
}

