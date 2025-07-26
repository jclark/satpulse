# **Unicore Protocol**

This document describes the protocol supported by the Nebula IV series high precision GNSS receivers, including the UM980.

## **Configuration commands**

### **Command packet format**

Commands are sent as ASCII strings followed by a carriage return and line feed (\\r\\n). Commands can be sent with or without a checksum. By default, the receiver accepts commands without a checksum. To use checksums, the CONFIG CMDFORMAT 1 command must be sent first.

* **Without Checksum:** RESET  
* **With Checksum:** $RESET\*55 (The checksum is a hexadecimal XOR of all characters from $ to \*)

### **Command Acknowledgment**

A critical, undocumented feature of the receiver is that it sends an acknowledgment message in response to every command it receives. This acknowledgment is sent immediately, before any other data that the command might generate.

The general format of this acknowledgment is:
`$command,<original_command_string>,response[[:] <status_message>]*<checksum>`

The `<original_command_string>` is an echo of the command that was sent. The `<status_message>` indicates the result. Note that the colon (`:`) after `response` is present for most acknowledgments but may be omitted in certain error messages.

**Examples of Acknowledgment Responses:**

*   **Successful Command:** The receiver confirms the command was accepted.
    *   `$command,VERSION,response: OK*04`
    *   `$command,GPRMC 1,response: OK*04`

*   **Command with Invalid Parameters:** The receiver indicates the command is recognized but the parameters are incorrect. Note the missing colon after `response`.
    *   `$command,VERSION xyzzy,response can't found device (null)*09`

*   **Unrecognized Command or Message:** The receiver indicates that the command itself is not recognized.
    *   `$command,FOOBARA 1,response: PARSING FAILED NO MATCHING FUNC  FOOBARA*14`
    *   `$command,XYZZY,response: PARSING FAILED NO MATCHING FUNC  XYZZY*05`

### **MODE command**

Sets or queries the receiver's operating mode (e.g., base station, rover).

#### **Setting the Operating Mode**

**Syntax:**  
`MODE [mode] [parameters]`

**Mode Parameters:**

*   **BASE [ID] [lat lon hgt]**: Sets fixed base station mode with precise geodetic coordinates. ID is an optional integer from 0-4095.
*   **BASE [ID] [x y z]**: Sets fixed base station mode with precise ECEF coordinates.
*   **BASE [ID] TIME [T] [Distance]**: Sets self-optimizing base station mode.
    *   T: Time in seconds to average the position (max 3600).
    *   Distance: Optional distance in meters. If the new optimized position is within this distance of a previously saved position, the saved position is used.
*   **ROVER [parameter]**: Sets the rover station mode for different applications.
    *   UAV: For agricultural, surveying, and other UAVs.
    *   SURVEY: For high-precision scenarios with lower dynamics.
    *   AUTOMOTIVE: For passenger vehicles and logistics.
*   **HEADING2 [parameter]**: Enables heading calculation between two receivers (base and rover).
    *   FIXLENGTH: Default mode, for a fixed distance between antennas.
    *   VARIABLELENGTH: For dynamically changing distance between antennas.
    *   STATIC: For both antennas in a static state.

**Examples:**

*   `MODE BASE 40.07898324818 116.23660197714 60.4265`: Sets base station mode with fixed coordinates.
*   `MODE BASE TIME 60`: Sets self-optimizing base station mode, averaging position for 60 seconds.
*   `MODE ROVER`: Sets rover station mode.

#### **Querying the Operating Mode**

**Syntax:**  
`MODE`

**Description:**  
Queries the current operating mode. The receiver responds with an ASCII log message that contains the mode information in its payload.

**Output Format:**  
The response is a standard ASCII log message, which starts with a `#` character.  
`#MODE,<header_fields>;<payload>*<checksum>`

The payload part of the message describes the current mode.

**Example Output:**  
`#MODE,81,GPS,FINE,2230,547967000,0,0,18,518;MODE ROVER SURVEY,*1B`

The payload in the example is `MODE ROVER SURVEY` indicating that the current mode is the same as would result from the command `MODE ROVER SURVEY`.

**Note:** The `MODE` query response, although it is an ASCII log, uses an 8-bit XOR checksum instead of the 32-bit CRC used by other ASCII logs.



### **CONFIG command**

Configures various receiver functions and interfaces.

#### **Querying config**

Queries all current configurations of the receiver.

**Syntax:** CONFIG

Output Format: The receiver responds with a series of ASCII messages, each detailing a specific configuration.  
Example Output:  
$CONFIG,COM1,CONFIG COM1 460800\*65  
$CONFIG,COM2,CONFIG COM2 115200\*23  
$CONFIG,PPS,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0\*6E

#### **Serial port config**

Configures baud rate, data bits, parity, and stop bits for COM ports.

**Syntax:** CONFIG \[port\] \[baud\] \[databits\] \[parity\] \[stopbits\]

**Parameters:**

* **port:** COM1, COM2, COM3  
* **baud:** 9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600  
* **databits:** 8 (only 8 is supported)  
* **parity:** N (None), E (Even), O (Odd)  
* **stopbits:** 1, 2

**Example:** CONFIG COM1 115200 8 n 1

#### **PPS config**

Configures the Pulse Per Second (PPS) output signal.

**Syntax:** CONFIG PPS \[enable/disable\] \[timeref\] \[polarity\] \[period\] \[width\] \[rfdelay\] \[userdelay\]

**Parameters:**

* **enable/disable:** ENABLE, ENABLE2, ENABLE3, DISABLE  
* **timeref:** GPS, BDS, GAL, GLO  
* **polarity:** POSITIVE, NEGATIVE  
* **period:** Pulse period in milliseconds (e.g., 1000 for 1Hz)  
* **width:** Pulse width in microseconds  
* **rfdelay:** RF delay in nanoseconds  
* **userdelay:** User-defined delay in nanoseconds

**Enable Parameter Differences:**

* **ENABLE**: (Default) Outputs PPS after position is fixed and PPS converges. Output stops about 30 seconds after losing satellite lock.  
* **ENABLE2**: Enables PPS output and maintains the output state even after losing satellite lock.  
* **ENABLE3**: Enables PPS output immediately after the receiver starts up.

**Example:** CONFIG PPS ENABLE GPS POSITIVE 1000000 1000 0 0

#### **SIGNALGROUP config**

Sets the combination of signals tracked by the master and slave antennas. This command is saved automatically and causes the receiver to reset.

**Syntax:** CONFIG SIGNALGROUP \[master\_group\] \[slave\_group\]

**Parameters:**

* **master\_group / slave\_group:** A number representing a specific frequency combination. For single-antenna products, only master\_group is used.

**Signal Groups (from Table 4-31):**

* **0:** Disable slave antenna  
* **1:** BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b), QZSS (L1C/A, L2C, L5)  
* **2:** BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L1C, L2C, L2P(Y), L5), GLO (G1, G2, G3), GAL (E1, E5a, E5b, E6), QZSS (L1C/A, L1C, L2C, L5), NavIC (L5)  
* **3:** BDS (B1I, B3I, B1C, B2b-PPP), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b, E6), QZSS (L1C/A, L2C, L5)  
* **4:** BDS (B1I, B2I, B3I), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b), QZSS (L1C/A, L2C, L5)  
* **5:** BDS (B1I, B2I, B3I), GPS (L1C/A, L2C/L2P), GLO (G1, G2), GAL (E1, E5b), QZSS (L1C/A, L2C)  
* **6:** BDS (B1I, B3I), GPS (L1C/A, L2C/L2P), GLO (G1, G2), GAL (E1, E5b), QZSS (L1C/A, L2C)  
* **7:** BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b), QZSS (L1C/A, L2C, L5)  
* **8:** GPS (L1C/A, L2C/L2P, L5), BDS (B1I, B3I, B1C, B2a), GAL (E1, E5a, E5b)  
* **9:** BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L2P(Y)/L2C, L5), GLO (L1C/A, L2C/A), GAL (E1C, E5A, E5B), QZSS (L1C/A, L2C, L5)  
* **10:** BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b), QZSS (L1C/A, L2C, L5, L6)

**Example:** CONFIG SIGNALGROUP 1

### **MASK command**

Disables tracking of specific satellite systems, frequencies, or individual satellites, or sets the elevation mask.

**Syntax:**

* MASK \[system/frequency\]  
* MASK \[elevation\_angle\]  
* MASK \[system\] PRN \[satellite\_id\]

**Frequency Syntax (from Table 5-5):**

* **GPS:** L1, LICA, L1C, L2, L2C, L2P, L5  
* **BDS:** B1, B2, B3, B1I, B2I, B3I, BD3B1C, BD3B2A, BD3B2B  
* **GLO:** R1, R2, R3  
* **GAL:** E1, E5a, E5b, E6C  
* **QZSS:** Q1, Q2, Q5, Q1CA, Q1C, Q2C  
* **IRNSS:** I5

**Examples:**

* MASK GPS: Disables tracking of all GPS satellites.  
* MASK 10: Sets the elevation mask angle to 10 degrees.  
* MASK B1: Disables tracking of BDS B1 signals.  
* MASK GPS PRN 10: Disables tracking of GPS satellite PRN 10\.

### **UNLOG command**

Stops the output of specified or all messages on a given port.

**Syntax:** UNLOG \[port\] \[message\]

**Examples:**

* UNLOG: Stops all messages on the current port.  
* UNLOG GPGGA: Stops GPGGA messages on the current port.  
* UNLOG COM1: Stops all messages on COM1.

### **FRESET command**

Clears all user configurations, ephemeris, and position data from Non-Volatile Memory (NVM) and restarts the receiver. The baud rate is reset to 115200 bps.

**Syntax:** FRESET

### **RESET command**

Restarts the receiver and can optionally clear specific data types.

**Syntax:** RESET \[parameter\]

**Parameters:**

* (none): Restarts the receiver.  
* EPHEM: Clears ephemeris.  
* ALMANAC: Clears almanac.  
* IONUTC: Clears ionosphere and UTC parameters.  
* POSITION: Clears position information.  
* ALL: Clears all of the above.

### **SAVECONFIG command**

Saves the current receiver configuration to NVM.

**Syntax:** SAVECONFIG

### **UNILOGLIST command**

Outputs a list of all currently configured periodic logs.

**Syntax:** UNILOGLIST

Output Format: The receiver responds with a list of active logs.  
Example Output:  
\#UNILOGLIST,66,GPS,FINE,2203,447089000,0,0,18,33;  
\< 3  
\< PSRPOSA COM1 1  
\< GPGGA COM1 1  
\< HWSTATUSA COM1 1

The output shows the number of active logs, followed by each log's command string, port, and rate.

## **Unicore data output messages**

The receiver supports proprietary messages in both binary and ASCII formats for reporting data.

### **Enabling data output messages**

Data output is enabled by sending a command with the message name, an optional port, and an optional output rate. Appending 'A' to the message name requests ASCII format; 'B' requests binary format.

**Syntax:** \[Message Name\]\[A/B\] \[Port\] \[Rate\]

**Parameters:**

* **Port:** COM1, COM2, COM3. If omitted, the current port is used.  
* **Rate:** Output interval in seconds. Common values are 1 (1Hz), 0.5 (2Hz), 0.2 (5Hz), 0.1 (10Hz). ONCHANGED outputs the message only when its content changes.

**Example:** BESTNAVA COM1 1

### **Packet formats**

#### **Binary packet format**

Binary messages consist of a 24-byte header (including 3 sync bytes), a variable-length data payload, and a 4-byte CRC checksum.

Binary Header Structure (24 bytes total):  
| Field | Type | Bytes | Offset | Description |  
|---|---|---|---|---|  
| Sync1 | UCHAR | 1 | 0 | 0xAA |  
| Sync2 | UCHAR | 1 | 1 | 0x44 |  
| Sync3 | UCHAR | 1 | 2 | 0xB5 |  
| CPU Idle | UCHAR | 1 | 3 | CPU idle percentage (0-100) |  
| Message ID | USHORT | 2 | 4 | Message identifier |  
| Message Length| USHORT | 2 | 6 | Length of data payload (not including header or CRC) |  
| Time Ref | UCHAR | 1 | 8 | Reference time (GPST or BDST) |  
| Time Status | UCHAR | 1 | 9 | Time status |  
| Week | USHORT | 2 | 10 | Week number |  
| ms | ULONG | 4 | 12 | Seconds of week (milliseconds) |  
| Reserved | ULONG | 4 | 16 | Reserved |  
| Version | UCHAR | 1 | 20 | Release version |  
| Leap Sec | UCHAR | 1 | 21 | Leap second |  
| Delay ms | USHORT | 2 | 22 | Output delay |

**Time Status Values (GPS Reference Time Status):**

Based on Novatel OEM7 time status values. The Unicore ASCII header shows "Unknown" or "Fine", indicating that only a subset of these values are typically used.

| Decimal | ASCII | Description |
|---|---|---|
| 20 | UNKNOWN | Time validity is unknown |
| 60 | APPROXIMATE | Time is set approximately |
| 80 | COARSEADJUSTING | Time is approaching coarse precision |
| 100 | COARSE | Time is valid to coarse precision |
| 120 | COARSESTEERING | Time is coarse set and is being steered |
| 130 | FREEWHEELING | Position is lost and the range bias cannot be calculated |
| 140 | FINEADJUSTING | Time is adjusting to fine precision |
| 160 | FINE | Time has fine precision |
| 170 | FINEBACKUPSTEERING | Time is fine set and is being steered by the backup system |
| 180 | FINESTEERING | Time is fine set and is being steered |
| 200 | SATTIME | Time from satellite. Only used in logs containing satellite data such as ephemeris and almanac |

#### **ASCII packet format**

ASCII messages start with a \# character, followed by a header, a semicolon ;, the data payload, an asterisk \*, and a 4-byte hexadecimal CRC. Fields are comma-separated.

ASCII Header Structure:  
\#MessageName,Port,Sequence,IdleTime,TimeStatus,Week,ms,ReceiverStatus,Reserved,SWVersion;

#### **Checksum**

A 32-bit CRC is calculated over the entire message (including the header for binary, and from \# for ASCII).

**Checksum Algorithm (C code from Appendix 1):**

const ULONG aulCrcTable\[256\] \=  
{  
 0x00000000UL, 0x77073096UL, 0xee0e612cUL, 0x990951baUL, 0x076dc419UL,  
 0x706af48fUL, 0xe963a535UL, 0x9e6495a3UL, 0x0edb8832UL, 0x79dcb8a4UL,  
 0xe0d5e91eUL, 0x97d2d988UL, 0x09b64c2bUL, 0x7eb17cbdUL, 0xe7b82d07UL,  
 0x90bf1d91UL, 0x1db71064UL, 0x6ab020f2UL, 0xf3b97148UL, 0x84be41deUL,  
 0x1adad47dUL, 0x6ddde4ebUL, 0xf4d4b551UL, 0x83d385c7UL, 0x136c9856UL,  
 0x646ba8c0UL, 0xfd62f97aUL, 0x8a65c9ecUL, 0x14015c4fUL, 0x63066cd9UL,  
 0xfa0f3d63UL, 0x8d080df5UL, 0x3b6e20c8UL, 0x4c69105eUL, 0xd56041e4UL,  
 0xa2677172UL, 0x3c03e4d1UL, 0x4b04d447UL, 0xd20d85fdUL, 0xa50ab56bUL,  
 0x35b5a8faUL, 0x42b2986cUL, 0xdbbbc9d6UL, 0xacbcf940UL, 0x32d86ce3UL,  
 0x45df5c75UL, 0xdcd60dcfUL, 0xabd13d59UL, 0x26d930acUL, 0x51de003aUL,  
 0xc8d75180UL, 0xbfd06116UL, 0x21b4f4b5UL, 0x56b3c423UL, 0xcfba9599UL,  
 0xb8bda50fUL, 0x2802b89eUL, 0x5f058808UL, 0xc60cd9b2UL, 0xb10be924UL,  
 0x2f6f7c87UL, 0x58684c11UL, 0xc1611dabUL, 0xb6662d3dUL, 0x76dc4190UL,  
 0x01db7106UL, 0x98d220bcUL, 0xefd5102aUL, 0x71b18589UL, 0x06b6b51fUL,  
 0x9fbfe4a5UL, 0xe8b8d433UL, 0x7807c9a2UL, 0x0f00f934UL, 0x9609a88eUL,  
 0xe10e9818UL, 0x7f6a0dbbUL, 0x086d3d2dUL, 0x91646c97UL, 0xe6635c01UL,  
 0x6b6b51f4UL, 0x1c6c6162UL, 0x856530d8UL, 0xf262004eUL, 0x6c0695edUL,  
 0x1b01a57bUL, 0x8208f4c1UL, 0xf50fc457UL, 0x65b0d9c6UL, 0x12b7e950UL,  
 0x8bbeb8eaUL, 0xfcb9887cUL, 0x62dd1ddfUL, 0x15da2d49UL, 0x8cd37cf3UL,  
 0xfbd44c65UL, 0x4db26158UL, 0x3ab551ceUL, 0xa3bc0074UL, 0xd4bb30e2UL,  
 0x4adfa541UL, 0x3dd895d7UL, 0xa4d1c46dUL, 0xd3d6f4fbUL, 0x4369e96aUL,  
 0x346ed9fcUL, 0xad678846UL, 0xda60b8d0UL, 0x44042d73UL, 0x33031de5UL,  
 0xaa0a4c5fUL, 0xdd0d7cc9UL, 0x5005713cUL, 0x270241aaUL, 0xbe0b1010UL,  
 0xc90c2086UL, 0x5768b525UL, 0x206f85b3UL, 0xb966d409UL, 0xce61e49fUL,  
 0x5edef90eUL, 0x29d9c998UL, 0xb0d09822UL, 0xc7d7a8b4UL, 0x59b33d17UL,  
 0x2eb40d81UL, 0xb7bd5c3bUL, 0xc0ba6cadUL, 0xedb88320UL, 0x9abfb3b6UL,  
 0x03b6e20cUL, 0x74b1d29aUL, 0xead54739UL, 0x9dd277afUL, 0x04db2615UL,  
 0x73dc1683UL, 0xe3630b12UL, 0x94643b84UL, 0x0d6d6a3eUL, 0x7a6a5aa8UL,  
 0xe40ecf0bUL, 0x9309ff9dUL, 0x0a00ae27UL, 0x7d079eb1UL, 0xf00f9344UL,  
 0x8708a3d2UL, 0x1e01f268UL, 0x6906c2feUL, 0xf762575dUL, 0x806567cbUL,  
 0x196c3671UL, 0x6e6b06e7UL, 0xfed41b76UL, 0x89d32be0UL, 0x10da7a5aUL,  
 0x67dd4accUL, 0xf9b9df6fUL, 0x8ebeeff9UL, 0x17b7be43UL, 0x60b08ed5UL,  
 0xd6d6a3e8UL, 0xa1d1937eUL, 0x38d8c2c4UL, 0x4fdff252UL, 0xd1bb67f1UL,  
 0xa6bc5767UL, 0x3fb506ddUL, 0x48b2364bUL, 0xd80d2bdaUL, 0xaf0a1b4cUL,  
 0x36034af6UL, 0x41047a60UL, 0xdf60efc3UL, 0xa867df55UL, 0x316e8eefUL,  
 0x4669be79UL, 0xcb61b38cUL, 0xbc66831aUL, 0x256fd2a0UL, 0x5268e236UL,  
 0xcc0c7795UL, 0xbb0b4703UL, 0x220216b9UL, 0x5505262fUL, 0xc5ba3bbeUL,  
 0xb2bd0b28UL, 0x2bb45a92UL, 0x5cb36a04UL, 0xc2d7ffa7UL, 0xb5d0cf31UL,  
 0x2cd99e8bUL, 0x5bdeae1dUL, 0x9b64c2b0UL, 0xec63f226UL, 0x756aa39cUL,  
 0x026d930aUL, 0x9c0906a9UL, 0xeb0e363fUL, 0x72076785UL, 0x05005713UL,  
 0x95bf4a82UL, 0xe2b87a14UL, 0x7bb12baeUL, 0x0cb61b38UL, 0x92d28e9bUL,  
 0xe5d5be0dUL, 0x7cdcefb7UL, 0x0bdbdf21UL, 0x86d3d2d4UL, 0xf1d4e242UL,  
 0x68ddb3f8UL, 0x1fda836eUL, 0x81be16cdUL, 0xf6b9265bUL, 0x6fb077e1UL,  
 0x18b74777UL, 0x88085ae6UL, 0xff0f6a70UL, 0x66063bcaUL, 0x11010b5cUL,  
 0x8f659effUL, 0xf862ae69UL, 0x616bffd3UL, 0x166ccf45UL, 0xa00ae278UL,  
 0xd70dd2eeUL, 0x4e048354UL, 0x3903b3c2UL, 0xa7672661UL, 0xd06016f7UL,  
 0x4969474dUL, 0x3e6e77dbUL, 0xaed16a4aUL, 0xd9d65adcUL, 0x40df0b66UL,  
 0x37d83bf0UL, 0xa9bcae53UL, 0xdebb9ec5UL, 0x47b2cf7fUL, 0x30b5ffe9UL,  
 0xbdbdf21cUL, 0xcabac28aUL, 0x53b39330UL, 0x24b4a3a6UL, 0xbad03605UL,  
 0xcdd70693UL, 0x54de5729UL, 0x23d967bfUL, 0xb3667a2eUL, 0xc4614ab8UL,  
 0x5d681b02UL, 0x2a6f2b94UL, 0xb40bbe37UL, 0xc30c8ea1UL, 0x5a05df1bUL,  
 0x2d02ef8dUL  
};

ULONG CalculateCRC32(UCHAR \*szBuf, INT iSize)  
{  
 int iIndex;  
 ULONG ulCRC \= 0;  
 for (iIndex=0; iIndex\<iSize; iIndex++)  
 {  
 ulCRC \= aulCrcTable\[(ulCRC ^ szBuf\[iIndex\]) & 0xff\] ^ (ulCRC \>\> 8);  
 }  
 return ulCRC;  
}

### **Data output message formats**

#### **VERSION**

* **Message ID:** 37  
* Description: Contains product model, software version, serial number, and authorization information.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | Type | Enum | 4 | Product model |  
  | sw version | Char\[33\] | 33 | Firmware version string |  
  | Auth | Char\[129\] | 129 | Authorization type string |  
  | Psn | Char\[66\] | 66 | Part number and Serial number |  
  | efuse ID | Char\[33\] | 33 | Board ID |  
  | comp time | Char\[43\] | 43 | Firmware compile time |

**Product Model Numbers (Enum for Type field):**

* **0:** UNKNOWN  
* **17:** UM982  
* **18:** UM980  
* **19:** UM960  
* **24:** UM960L  
* **26:** UM981  
* **52:** UB9A0

#### **BESTNAV**

* **Message ID:** 2118  
* Description: Best available position and velocity solution from the master antenna.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | p-sol status | Enum | 4 | Position solution status |  
  | pos type | Enum | 4 | Position type |  
  | lat | Double | 8 | Latitude (degrees) |  
  | lon | Double | 8 | Longitude (degrees) |  
  | hgt | Double | 8 | Height above mean sea level (m) |  
  | undulation | Float | 4 | Geoid undulation (m) |  
  | datum id\# | Enum | 4 | Datum ID (61 for WGS84) |  
  | lat σ | Float | 4 | Latitude standard deviation (m) |  
  | lon σ | Float | 4 | Longitude standard deviation (m) |  
  | hgt σ | Float | 4 | Height standard deviation (m) |  
  | stn id | Char\[4\] | 4 | Base station ID |  
  | diff\_age | Float | 4 | Differential age (s) |  
  | sol\_age | Float | 4 | Solution age (s) |  
  | \#SVs | Uchar | 1 | Number of satellites tracked |  
  | \#solnSVs | Uchar | 1 | Number of satellites used in solution |  
  | Reserved | Uchar | 1 | Reserved |  
  | Reserved | Uchar | 1 | Reserved |  
  | Reserved | Uchar | 1 | Reserved |  
  | ext sol stat | Hex 1 | 1 | Extended solution status flags |  
  | Galileo\&BDS3 sig mask | Hex 1 | 1 | Signal mask for Galileo & BDS-3 |  
  | GPS, GLONASS and BDS2 sig mask | Hex 1 | 1 | Signal mask for GPS, GLONASS, BDS-2 |  
  | v-sol status | Enum | 4 | Velocity solution status |  
  | vel type | Enum | 4 | Velocity type |  
  | latency | Float | 4 | Velocity time tag latency (s) |  
  | age | Float | 4 | Differential age for velocity (s) |  
  | hor spd | Double | 8 | Horizontal speed over ground (m/s) |  
  | trk gnd | Double | 8 | Track over ground (degrees True) |  
  | vert spd | Double | 8 | Vertical speed (m/s) |  
  | Verspd std | Float | 4 | Vertical speed standard deviation (m/s) |  
  | Horspd std | Float | 4 | Horizontal speed standard deviation (m/s) |

**Enums for BESTNAV and related messages:**

Solution Status (p-sol status, v-sol status) (Table 0-5):  
| Decimal | ASCII | Description |  
|---|---|---|  
| 0 | SOL\_COMPUTED | Solution computed |  
| 1 | INSUFFICIENT\_OBS | Insufficient observations |  
| 2 | NO\_CONVERGENCE | No convergence, invalid solution |  
| 4 | COV\_TRACE | Covariance matrix trace exceeds maximum |  
Position or Velocity Type (pos type, vel type) (Table 0-4):  
| Decimal | ASCII | Description |  
|---|---|---|  
| 0 | NONE | No solution |  
| 16 | SINGLE | Single point positioning |  
| 17 | PSRDIFF | Pseudorange differential solution |  
| 18 | SBAS | SBAS positioning |  
| 32 | L1\_FLOAT | L1 float solution |  
| 33 | IONOFREE\_FLOAT | Ionosphere-free float solution |  
| 34 | NARROW\_FLOAT | Narrow-lane float solution |  
| 48 | L1\_INT | L1 fixed solution |  
| 49 | WIDE\_INT | Wide-lane fixed solution |  
| 50 | NARROW\_INT | Narrow-lane fixed solution |  
| 68 | PPP\_CONVERGING | PPP solution converging |  
| 69 | PPP | Precise Point Positioning |  
**Extended Solution Status (ext sol stat) (Table 7-89):**

* **Bit 0:** RTK solution verification (0=unchecked, 1=checked)  
* **Bits 1-3:** Pseudorange ionospheric correction (0=Unknown, 1=Klobuchar, 2=SBAS grid, 3=Multi-frequency, 4=Pseudorange differential)

#### **BESTNAVXYZ**

* **Message ID:** 240  
* Description: Best available position and velocity in ECEF coordinates.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | P-sol status | Enum | 4 | Position solution status |  
  | pos type | Enum | 4 | Position type |  
  | P-X | Double | 8 | ECEF X-coordinate of position (m) |  
  | P-Y | Double | 8 | ECEF Y-coordinate of position (m) |  
  | P-Z | Double | 8 | ECEF Z-coordinate of position (m) |  
  | P-X σ | Float | 4 | Standard deviation of P-X (m) |  
  | P-Y σ | Float | 4 | Standard deviation of P-Y (m) |  
  | P-Z σ | Float | 4 | Standard deviation of P-Z (m) |  
  | V-sol status | Enum | 4 | Velocity solution status |  
  | vel type | Enum | 4 | Velocity type |  
  | V-X | Double | 8 | ECEF X-coordinate of velocity (m/s) |  
  | V-Y | Double | 8 | ECEF Y-coordinate of velocity (m/s) |  
  | V-Z | Double | 8 | ECEF Z-coordinate of velocity (m/s) |  
  | V-X σ | Float | 4 | Standard deviation of V-X (m/s) |  
  | V-Y σ | Float | 4 | Standard deviation of V-Y (m/s) |  
  | V-Z σ | Float | 4 | Standard deviation of V-Z (m/s) |  
  | stn ID | Char\[4\] | 4 | Base station ID |  
  | V-latency | Float | 4 | Velocity time tag latency (s) |  
  | diff\_age | Float | 4 | Differential age (s) |  
  | sol\_age | Float | 4 | Solution age (s) |  
  | \#SVs | Uchar | 1 | Number of satellites tracked |  
  | \#solnSVs | Uchar | 1 | Number of satellites used in solution |  
  | \#ggL1 | Uchar | 1 | Number of satellites with L1/G1/B1 signals used |  
  | \#solnMultiSVs | Uchar | 1 | Number of satellites with multi-frequency signals used |  
  | Reserved | Char | 1 | Reserved |  
  | ext sol stat | Hex 1 | 1 | Extended solution status flags |  
  | Galileo\&BDS3 sig mask | Hex 1 | 1 | Signal mask for Galileo & BDS-3 |  
  | GPS, GLONASS and BDS2 sig mask | Hex 1 | 1 | Signal mask for GPS, GLONASS, BDS-2 |

**Note:** The Enums used in BESTNAVXYZ are the same as those specified for BESTNAV.

#### **STADOP**

* **Message ID:** 954  
* Description: Dilution of Precision (DOP) values for the BESTNAV solution.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | Reserved | Ulong | 4 | Reserved |  
  | gdop | Float | 4 | Geometric DOP |  
  | pdop | Float | 4 | Position DOP |  
  | tdop | Float | 4 | Time DOP |  
  | vdop | Float | 4 | Vertical DOP |  
  | hdop | Float | 4 | Horizontal DOP |  
  | ndop | Float | 4 | North DOP |  
  | edop | Float | 4 | East DOP |  
  | cutoff | Float | 4 | Elevation cutoff angle |  
  | Reserved | Float | 4 | Reserved |  
  | \#PRN | UShort | 2 | Number of tracked satellites |  
  | PRN | UShort\[\] | 2 \* \#PRN | List of PRNs of tracked satellites |

#### **SATSINFO**

* **Message ID:** 2124  
* Description: Detailed information for all tracked satellites. The message consists of a header followed by a variable number of satellite blocks.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | Sat number | Byte | 1 | Number of tracked satellites to follow |  
  | Version number| Byte | 1 | Version number, default \= 2 |  
  | reserve | Byte | 1 | Reserved |  
  | reserve | Byte | 1 | Reserved |  
  | reserve | Byte | 1 | Reserved |  
  | Satellite Block (repeats Sat number times) | | | |  
  | Frq flag | Byte | 1 | Frequency flag bitmask for this satellite |  
  | PRN | Byte | 1 | Satellite PRN number |  
  | Azimuth | Short | 2 | Azimuth (degrees) |  
  | Elevation | Byte | 1 | Elevation (degrees) |  
  | Sys status | Byte | 1 | System identifier |  
  | SNR | Byte | 1 | Signal-to-noise ratio |  
  | Freq status | Byte | 1 | Frequency identifier |  
  | Freq No | Byte | 1 | Number of frequencies for this PRN |  
  | Frequency Block (repeats Freq No times) | | | |  
  | SNR | Byte | 1 | Signal-to-noise ratio for this frequency |  
  | Freq status | Byte | 1 | Frequency identifier for this frequency |  
  | Reserved | Short | 2 | Reserved |

#### **BESTSAT**

* **Message ID:** 1041  
* Description: Information about the satellites used in the position solution. The message consists of a header followed by a variable number of satellite blocks.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | \#entries | Ulong | 4 | Number of satellite entries to follow |  
  | Satellite Block (repeats \#entries times) | | | |  
  | Satellite system | Enum | 4 | GNSS system |  
  | Satellite ID | Ulong | 4 | Satellite PRN number |  
  | Status | Enum | 4 | Status (always "GOOD") |  
  | Signal mask | Hex 4 | 4 | Bitmask of signals used from this satellite |

**Satellite System Enum (from Table 7-116):**

* **0:** GPS  
* **1:** GLONASS  
* **2:** SBAS  
* **5:** GALILEO  
* **6:** BEIDOU  
* **7:** QZSS  
* **9:** NAVIC

#### **GPSUTC**

* **Message ID:** 19  
* Description: Parameters for converting GPS Time to UTC.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | utc wn | Ulong | 4 | UTC reference week number |  
  | tot | Ulong | 4 | Reference time of UTC parameters |  
  | A0 | Double | 8 | Clock bias of GPST relative to UTC |  
  | A1 | Double | 8 | Clock rate of GPST relative to UTC |  
  | wn lsf | Ulong | 4 | Future week number for leap second |  
  | dn | Ulong | 4 | Future day number for leap second |  
  | deltat ls | Long | 4 | Current leap seconds |  
  | deltat lsf | Long | 4 | Future leap seconds |  
  | deltat utc | Ulong | 4 | Time offset of GPST relative to UTC |  
  | reserved | Ulong | 4 | Reserved |

#### **BD3UTC**

* **Message ID:** 22  
* Description: Parameters for converting BDS-3 Time to UTC.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | utc wn | Ulong | 4 | UTC reference week number |  
  | tot | Ulong | 4 | Reference time of UTC parameters |  
  | A0 | Double | 8 | Clock bias of BDST relative to UTC |  
  | A1 | Double | 8 | Clock drift of BDST relative to UTC |  
  | A2 | Double | 8 | Clock drift rate of BDST relative to UTC |  
  | wn lsf | Ulong | 4 | Future week number for leap second |  
  | dn | Ulong | 4 | Future day number for leap second |  
  | deltat ls | Long | 4 | Current leap seconds |  
  | deltat lsf | Long | 4 | Future leap seconds |  
  | reserved | ULONG | 4 | Reserved |  
  | reserved | Ulong | 4 | Reserved |

#### **BDSUTC**

* **Message ID:** 2012  
* Description: Parameters for converting BDS Time to UTC.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | Reserved | Ulong | 4 | Reserved |  
  | Reserved | Ulong | 4 | Reserved |  
  | A0 | Double | 8 | Clock bias of BDT relative to UTC |  
  | A1 | Double | 8 | Clock rate of BDT relative to UTC |  
  | wn lsf | Ulong | 4 | Future week number for leap second |  
  | dn | Ulong | 4 | Future day number for leap second |  
  | deltat ls | Long | 4 | Current leap seconds |  
  | deltat lsf | Long | 4 | Future leap seconds |  
  | Reserved | Ulong | 4 | Reserved |  
  | reserved | Ulong | 4 | Reserved |

#### **GALUTC**

* **Message ID:** 20  
* Description: Parameters for converting Galileo Time to UTC.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | A0 | Double | 8 | Clock bias of Galileo time relative to UTC |  
  | A1 | Double | 8 | Clock rate of Galileo time relative to UTC |  
  | deltat ls | long | 4 | Current leap seconds |  
  | tot | Ulong | 4 | Reference time of UTC parameters |  
  | utc wn | Ulong | 4 | UTC reference week number |  
  | ulWNlsf | Ulong | 4 | Future week number for leap second |  
  | dn | Ulong | 4 | Future day number for leap second |  
  | deltat lsf | Long | 4 | Future leap seconds |  
  | dA0g | Long | 8 | Constant term for Galileo to GPS time conversion |  
  | dA1g | Ulong | 8 | First order term for Galileo to GPS time conversion |  
  | ulT0g | Ulong | 4 | Reference second for Galileo to GPS time conversion |  
  | ulWN0g | Ulong | 4 | Reference week for Galileo to GPS time conversion |

#### **RECTIME**

* **Message ID:** 102  
* Description: Receiver time information.  
  | Field | Type | Bytes | Description |  
  |---|---|---|---|  
  | clock status | Enum | 4 | Clock model status |  
  | offset | Double | 8 | Receiver clock offset relative to GPS time (s) |  
  | offset std | Double | 8 | Standard deviation of clock offset (s) |  
  | utc offset | Double | 8 | GPS time offset relative to UTC (s) |  
  | utc year | Ulong | 4 | UTC year |  
  | utc month | Uchar | 1 | UTC month |  
  | utc day | Uchar | 1 | UTC day |  
  | utc hour | Uchar | 1 | UTC hour |  
  | utc min | Uchar | 1 | UTC minute |  
  | utc ms | Ulong | 4 | UTC millisecond |  
  | utc status | Enum | 4 | UTC status |

**Enums for RECTIME:**

* **clock status**: 0=VALID, 3=INVALID  
* **utc status**: 0=INVALID, 1=VALID, 2=WARNING

#### PPSSTATUS

* **Message ID:** 9000
* Description: Contains the PPS status information. It only supports 1 Hz output.
  | Field | Type | Bytes | Description |
  |---|---|---|---|
  | Status | ULONG | 4 | PPS output status. 0: No PPS output, 1: Accuracy not guaranteed, 2: Accuracy ~100-1000 ns, 3: Accuracy < 100 ns |
  | Week | ULONG | 4 | PPS week (aligned with GPS Week) |
  | MsSecInWeek | ULONG | 4 | PPS milliseconds in the week (aligned with GPS MsCount) |
  | PPS PulseErr | INT | 4 | PPS phase error (ns) |
  | offsetTime | INT | 4 | PPS offset time relative to observation time (0.01 ns) |
  | ConfigInfo | ULONG | 4 | PPS configuration information (Hex) |
  | Register | ULONG | 4 | Hardware register status |
  | TimeEstErr | INT | 4 | DeltaT time estimated error |
  | InnerQuality | ULONG | 4 | Inner positioning and time quality indicator |
  | PPSInnerSta | ULONG | 4 | PPS inner status |
  | PPSCfgSta | ULONG | 4 | PPS config status |
  | PPSStage | ULONG | 4 | PPS stage |
  | RSV | ULONG | 4 | Reserved |
  | RSV | ULONG | 4 | Reserved |
  | RSV | ULONG | 4 | Reserved |

**ConfigInfo Field (Bitmask):**

* **Bits 3:0:** PPS configuration type (0: Enable1, 1: Enable2, 2: Enable3)
* **Bit 4:** Polarity (0: Rising edge, 1: Falling edge)
* **Bits 7:5:** GnssRef config
* **Bits 31:16:** Interval (max 20e3)

**Register Field (Bitmask):**

* **Bits 3:0:** PPS_CTRL[3:0]
* **Bits 7:4:** PPS_PULSE_CTRL[3:0]
* **Bits 31:8:** Reserved

**InnerQuality Field (Bitmask):**

* **Bits 3:0:** BB POSQuality
* **Bits 7:4:** GPS TimeQuality
* **Bits 11:8:** BDS TimeQuality
* **Bits 15:12:** GAL TimeQuality
* **Bits 19:16:** GLO TimeQuality
* **Bits 23:20:** IRNSS TimeQuality
* **Bits 31:24:** Reserved

**PPSInnerSta Field (Bitmask):**

* **Bits 7:0:** Reserved
* **Bits 15:8:** System
* **Bits 23:16:** CtrlFlag
* **Bits 31:24:** Status

**PPSCfgSta Field (Bitmask):**

* **Bits 19:0:** CurPPSDelay
* **Bits 23:20:** TimeRef
* **Bits 31:24:** Flag

**PPSStage Field (Bitmask):**

* **Bits 7:0:** StableCount
* **Bits 15:8:** SwitchCount
* **Bits 23:16:** KeepCount
* **Bits 31:24:** HoldCount


### **Identifiers**

#### **PRN numbers (Table 7-54)**

* **BDS:** 1-63  
* **GPS:** 1-32  
* **GLONASS:** 38-61  
* **Galileo:** 1-36  
* **SBAS:** 120-158  
* **QZSS:** 193-202  
* **IRNSS:** 1-15

#### **Frequency Flag (Table 7-111)**

This is a bitmask indicating which frequency bands have data present in the SATSINFO log.

* **Bit 0:** GPS L1C/A, GLO L1, BDS B1I, GAL E1  
* **Bit 1:** GPS L2C, GLO L2, BDS B2I, GAL E5b  
* **Bit 2:** GPS L5, BDS B3I, GAL E5a, IRNSS L5  
* **Bit 3:** BDS B1C, GPS L1C  
* **Bit 4:** BDS B2a, GLO G3, GAL E6  
* **Bit 5:** BDS B2b, GPS L2P

#### **GNSS System Identifiers (Table 7-112)**

* **0:** GPS  
* **1:** GLONASS  
* **2:** SBAS  
* **3:** GAL  
* **4:** BDS  
* **5:** QZSS  
* **6:** IRNSS

#### **Frequency Identifiers (Table 7-113 & 7-56)**

A value indicating the specific signal type.

* **GPS:** 0=L1 C/A, 9=L2P(Y), 3=L1C pilot, 11=L1C data, 6=L5 data, 14=L5 pilot, 17=L2C(L)  
* **GLONASS:** 0=L1 C/A, 5=L2 C/A, 6=G3I, 7=G3Q  
* **Galileo:** 1=E1B, 2=E1C, 12=E5A pilot, 17=E5B pilot, 18=E6B, 22=E6C  
* **BDS:** 0=B1I, 4=B1Q, 8=B1C(Pilot), 23=B1C(Data), 5=B2Q, 17=B2I, 12=B2a(Pilot), 28=B2a(Data), 6=B3Q, 21=B3I, 13=B2b(I)  
* **QZSS:** 0=L1 C/A, 1=L1C/B, 3=L1C pilot, 4=L1S, 6=L5 data, 11=L1C data, 14=L5 pilot, 17=L2C(L), 21=L6D, 27=L6E  
* **SBAS:** 0=L1 C/A, 6=L5(I)  
* **IRNSS:** 6=L5 data, 14=L5 pilot

## **NMEA messages**

The receiver supports standard NMEA 0183 messages. The default version is 4.10.

Supported Messages:  
GPDTM, GPGBS, GPGGA, GPGLL, GPGNS, GPGRS, GPGSA, GPGST, GPGSV, GPTHS, GPRMC, GPROT, GPVTG, GPZDA.  
Enabling NMEA Output:  
NMEA messages are enabled similarly to Unicore messages, by sending a command with the message name, port, and rate.  
**Syntax:** \[Message Name\] \[Port\] \[Rate\]

**Example:** GPGGA COM1 1

## **RTCM messages**

The receiver supports RTCM Version 3 messages for differential corrections.

**Supported Messages (from Appendix 2):**

* **Observables:**  
  * RTCM1004 (GPS L1/L2 Extended)  
  * RTCM1012 (GLONASS L1/L2 Extended)  
  * RTCM1074 (GPS MSM4)  
  * RTCM1075 (GPS MSM5)  
  * RTCM1084 (GLONASS MSM4)  
  * RTCM1085 (GLONASS MSM5)  
  * RTCM1124 (BDS MSM4)  
  * RTCM1125 (BDS MSM5)  
  * RTCM1127 (BDS MSM7)  
* **Base Station Coordinates:**  
  * RTCM1005 (ARP Coordinates)  
  * RTCM1006 (ARP Coordinates with Antenna Height)  
* **Base Station Antenna:**  
  * RTCM1007 (Antenna Description)  
  * RTCM1033 (Receiver and Antenna Descriptors)  
* **Ephemeris:**  
  * RTCM1019 (GPS Ephemeris)  
  * RTCM1020 (GLONASS Ephemeris)  
  * RTCM1042 (BDS Ephemeris)  
  * RTCM1045 (Galileo F/NAV Ephemeris)  
  * RTCM1046 (Galileo I/NAV Ephemeris)

Enabling RTCM Output:  
RTCM messages are enabled by sending a command with the message ID prefixed by "RTCM", followed by the port and rate.  
**Syntax:** RTCM\[Message ID\] \[Port\] \[Rate\]

**Example:** RTCM1074 COM2 1