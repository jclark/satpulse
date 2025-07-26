package unicore

// ASCII Packet Format Specification
//
// This file implements the gpsprot.PacketFormat interface for Unicore ASCII packets.
//
// VALIDITY GUARANTEES:
// The ASCII packet format scanner guarantees that any packet passed to ParseAsciiMessage
// will satisfy ALL of the following criteria:
//
// 1. Starts with '#' character
// 2. Contains at least one ';' character (header/data separator)
// 3. Ends with CR/LF (\r\n)
// 4. Before the CR/LF, the packet ends with one of:
//    a) The first semicolon in the packet (no checksum)
//    b) '*' followed by exactly 8 lowercase hex digits (32-bit CRC)
//    c) '*' followed by exactly 2 lowercase hex digits (8-bit XOR checksum)
//
// If ParseAsciiMessage receives a packet that does not satisfy these guarantees,
// it should panic as this indicates a bug in the packet format scanner.
//
// ASCII MESSAGE FORMAT:
// #MessageName,header_fields;data_fields*checksum\r\n
//
// Examples:
// #PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,...*0bbaac1a\r\n
// #MODE,81,GPS,FINE,2230,547967000,0,0,18,518;MODE ROVER SURVEY,*1B\r\n (8-bit checksum)