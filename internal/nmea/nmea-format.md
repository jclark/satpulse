7 Data format protocol
7.1 Characters
7.1.1 General
All transmitted data shall be interpreted as ASCII characters. The most significant bit of the
eight-bit character shall always be transmitted as zero (D7 = 0).
7.1.2 Reserved characters
The reserved character set consists of those ASCII characters shown in 8.1 (Table 1).
These characters are used for specific formatting purposes, such as sentence and field
delimiting, and except for code delimiting, shall not be used in data fields.
7.1.3 Valid characters
The valid character set consists of all printable ASCII characters (HEX 20 to HEX 7E)
except those defined as reserved characters. The list of the valid character set is given
in 8.1 (Table 2).
7.1.4 Undefined characters
ASCII values not specified as either “reserved characters” or “valid characters” are
excluded and shall not be transmitted at any time.
When it is necessary to communicate an 8-bit character defined by ISO/IEC 8859-1 that is a
reserved character (Table 1) or not listed in Table 2 as a valid character (e.g. in a
proprietary sentence or text sentence), three characters shall be used.
The reserved character “^“ (HEX 5E) is followed by two ASCII characters (0-9, A-F)
representing the HEX value of the character to be communicated. For example:
– to send heading as "127.5°", transmit “127.5 ^F8”;
– to send the reserved characters <CR><LF>, transmit “^0D^0A”;
– to send the reserved character "^", transmit “^5E”.
IEC 60945 states that, as a minimum requirement, English language shall be used for
controls and displays. Other languages/characters are only supported by the TUT sentence.
7.1.5 Character symbols
When individual characters are used in this standard to define units of measurement, to
indicate the type of data field, type of sentence, etc. they shall be interpreted according
to the character symbol in 8.1 (Table 3).
7.2 Fields
7.2.1 String
A field consists of a string of valid characters, or no characters (null field), located between
two appropriate delimiter characters.
61162-1 Ó IEC:2010(E) – 15 –
7.2.2 Address field
7.2.2.1 General
An address field is the first field in a sentence and follows the "$" or “!” delimiter; it serves
to define the sentence. The "$" delimiter identifies sentences that conform to the
conventional parametric and delimited field composition rules as described in 7.3.3. The "!"
delimiter identifies sentences that conform to the special-purpose encapsulation and non-
delimited field composition rules as described in 7.3.3. Characters within the address field
are limited to digits and upper case letters. The address field shall not be a null field. Only
sentences with the following three types of address fields shall be transmitted.
7.2.2.2 Approved address field
Approved address fields consist of five digits and upper case letter characters defined by
this standard. The first two characters are the talker identifier, listed in 8.2 (Table 4). The
talker identifier serves to define the nature of the data being transmitted.
Devices that have the capability to transmit data from multiple sources shall transmit the
appropriate talker identifier (for example a device with both a GPS receiver and a LORAN-
C receiver shall transmit GP when the position is GPS-based, LC when the position is
LORAN-C-based, and IN for integrated navigation shall be used if lines of position from
LORAN-C and GPS are combined into a position fix).
Devices capable of re-transmitting data from other sources shall use the appropriate
identifier (for example GPS receivers transmitting heading data shall not transmit $GPHCD
unless the compass heading is actually derived from the GPS signals).
The next three characters form the sentence formatter used to define the format and the
type of data. A list of sentence formatters is given in 8.3.
7.2.2.3 Query address field
The query address field consists of five characters and is used for the purpose of requesting
transmission of a specific sentence on a separate bus from an identified talker.
The first two characters are the talker identifier of the device requesting data, the next two
characters are the talker identifier of the device being addressed and the final character is
the query character “Q”.
7.2.2.4 Proprietary address field
The proprietary address field consists of the proprietary character “P” followed by a three-
character manufacturer's mnemonic code, used to identify the talker issuing a proprietary
sentence, and any additional characters as required.
NOTE A list of valid manufacturer's mnemonic codes may be obtained from NMEA (see 7.3.6).
7.2.3 Data fields
7.2.3.1 General
Data fields in approved sentences follow a "," delimiter and contain valid characters (and
code delimiters “^”) in accordance with the formats illustrated in 8.2 (Table 5). Data fields in
proprietary sentences contain only valid characters and the delimiter characters “,” and “^”,
but are not defined by this standard.
– 16 – 61162-1 Ó IEC:2010(E)
Because of the presence of variable data fields and null fields, specific data fields shall
only be located within a sentence by observing the field delimiters ",". Therefore, it is
essential for the listener to locate fields by counting delimiters rather than counting the total
number of characters received from the start of the sentence.
7.2.3.2 Variable length fields
Although some data fields are defined to have fixed length, many are of variable length
in order to allow devices to convey information and to provide data with more or less
precision, according to the capability or requirements of a particular device.
Variable length fields may be alphanumeric or numeric fields. Variable numeric fields may
contain a decimal point and may contain leading or trailing zeros.
7.2.3.3 Data field types
Data fields may be alpha, numeric, alphanumeric, variable length, fixed length or fixed/
variable (with a portion fixed in length while the remainder varies). Some fields are
constant, with their value dictated by a specific sentence definition. The allowable field
types are summarized in 8.2 (Table 5).
7.2.3.4 Null fields
A null field is a field of length zero, i.e. no characters are transmitted in the field. Null fields
shall be used when the value is unreliable or not available.
For example, if heading information were not available, sending data of "000" is misleading
because a user cannot distinguish between "000" meaning no data and a legitimate heading
of "000". However, a null field, with no characters at all, clearly indicates that no data is
being transmitted.
Null fields with their delimiters can have the following appearance depending on where they
are located in the sentence:
",," ",*"
The ASCII NULL character (HEX 00) shall not be used as the null field.
7.2.4 Checksum field
A checksum field shall be transmitted in all sentences. The checksum field is the last field
in a sentence and follows the checksum delimiter character "*". The checksum is the
eight-bit exclusive OR (no start or stop bits) of all characters in the sentence, including ","
and “^” delimiters, between but not including the "$" or “!” and the "*" delimiters.
The hexadecimal value of the most significant and least significant four bits of the result is
converted to two ASCII characters (0-9, A-F) for transmission. The most significant
character is transmitted first.
Examples of the checksum field are:
$GPGLL,5057.970,N,00146.110,E,142451,A*27 and
$GPVTG,089.0,T,,,15.2,N,,,*53.
61162-1 Ó IEC:2010(E) – 17 –
7.2.5 Sequential message identifier field
This is a field that is critical to identifying groups of 2 or more sentences that make up a
multi-sentence message. This field applies only to a single sentence formatter, and is not
used to associate different sentence formatters. This field is incremented each time a new
multi-sentence message is generated with the same sentence formatter. This field’s value is
reset to zero when it is incremented beyond the defined maximum value. This field’s
maximum value, size, and format of this field is determined by the applicable sentence
definition in Clause 8 This is one of three key fields supporting the multi-sentence message
capability (see 7.3.9).
7.3 Sentences
7.3.1 General structure
This subclause describes the general structure of sentences. Details of specific sentence
formats are found in 8.3. Some sentences may specify restrictions beyond the general
limitations given in this standard. Such restrictions may include defining some fields as
fixed length, numeric or text only, required to be non-null, transmitted with a certain
frequency, etc.
The maximum number of characters in a sentence shall be 82, consisting of a maximum of
79 characters between the starting delimiter "$" or “!” and the terminating delimiter
<CR><LF>.
The minimum number of fields in a sentence is one (1). The first field shall be an address
field containing the identity of the talker and the sentence formatter which specifies the
number of data fields in the sentence, the type of data they contain and the order in which
the data fields are transmitted. The remaining portion of the sentence may contain zero or
multiple data fields.
The maximum number of fields allowed in a single sentence is limited only by the maximum
sentence length of 82 characters. Null fields may be present in the sentence and shall
always be used if data for that field is unavailable.
All sentences begin with the sentence-starting delimiter character "$" or “!” and end with the
sentence-terminating delimiter <CR><LF>.
7.3.2 Description of approved sentences
Approved sentences are those designed for general use and detailed in this standard.
Approved sentences are listed in 8.3 and shall be used wherever possible. When a
deprecated sentence has been replaced by an approved sentence, this is indicated in 8.3 by
a note.
Other sentences, not recommended for new designs, may be found in practice.
NOTE Such sentences are listed in NMEA 0183. Information on such sentences may be obtained from the National
Marine Electronics Association (NMEA) (see 7.3.6).
An approved sentence contains, in the order shown, the following elements:
ASCII HEX Description
24 or 21 "$" or “!” <address field> – start of sentence
– talker identifier and sentence formatter
["," <data field>] ["," <data field>]
– zero or more data fields
"*" <checksum field> – 18 – – checksum field
<CR><LF> 0D 0A – end of sentence
61162-1 Ó IEC:2010(E)
7.3.3 Parametric sentences
7.3.3.1 Description
These sentences start with the "$" delimiter, and represent the majority of sentences
defined by this standard. This sentence structure, with delimited and defined data fields, is
the preferred method for conveying information.
The basic rules for parametric sentence structures are:
· the sentence begins with the "$" delimiter;
· only approved sentence formatters are allowed. Formatters used by special-purpose
encapsulation sentences cannot be reused. See 8.2;
· only valid characters are allowed. See 8.1 (Tables 1 and 2);
· only approved field types are allowed. See 8.2 (Table 5);
· data fields (parameters) are individually delimited, and their content is identified and
often described in detail by this standard;
· encapsulated non-delimited data fields are NOT ALLOWED.
7.3.3.2 Structure
The following provides a summary explanation of the approved parametric sentence
structure:
$aaccc, c---c*hh<CR><LF>
ASCII HEX Description
"$" 24 Start of sentence: starting delimiter.
aaccc Address field: alphanumeric characters identifying type of talker,
and sentence formatter. The first two characters identify the
talker. The last three are the sentence formatter mnemonic code
identifying the data type and the string format of the successive
fields. Mnemonics will be used as far as possible to facilitate
readouts by users.
"," 2C Field delimiter: starts each field except address and checksum
fields. If it is followed by a null field, it is all that remains to
indicate no data in a field.
c---c Data sentence block: follows address field and is a series of data
fields containing all of the data to be transmitted. Data field
sequence is fixed and identified by the third and subsequent
characters of the address field (the sentence formatter). Data
fields may be of variable length and are preceded by delimiters
",".
"*" 2A checksum delimiter: follows last data field of the sentence. It
indicates that the following two alpha-numeric characters show
the HEX value of the checksum.
hh Checksum field: the absolute value calculated by exclusive-
OR'ing the eight data bits (no start bits or stop bits) of each
61162-1 Ó IEC:2010(E) – 19 –
character in the sentence between, but excluding, "$" and "*". The
hexadecimal value of the most significant and least significant
four bits of the result are converted to two ASCII characters (0-9,
A-F) for transmission. The most significant character is
transmitted first. The checksum field is required in all cases.
<CR><LF> 0D 0A End of sentence: sentence terminating delimiter.
7.3.4 Encapsulation sentences
7.3.4.1 Description
These sentences start with the "!" delimiter. The function of this special-purpose sentence
structure is to provide a means to convey information, when the specific data content is
unknown or greater information bandwidth is needed. This is similar to a modem that
transfers information without knowing how the information is to be decoded or interpreted.
The basic rules for encapsulation sentence structures are:
· the sentence begins with the "!" delimiter;
· only approved sentence formatters are allowed. Formatters used by conventional
parametric sentences cannot be reused. See 8.2;
· only valid characters are allowed. See 8.1 (Tables 1 and 2);
· only approved field types are allowed. See 8.2 (Table 5);
· only six-bit coding may be used to create encapsulated data fields. See 8.2 (Table 5);
· encapsulated data fields may consist of any number of parameters, and their content is
not identified or described by this standard;
· the sentence shall be defined with one encapsulated data field and any number of
parametric data fields separated by the "," data field delimiter. The encapsulated data
field shall always be the second to last data field in the sentence, not counting the
checksum field. See 7.2.3;
· the sentence contains a "total number of sentences" field. See 7.3.4.1;
· the sentence contains a "sentence number" field. See 7.3.4.1,
· the sentence contains a "sequential message identifier" field. See 7.3.4.1;
· the sentence contains a "fill bits" field immediately following the encapsulated data field.
The fill bits field shall always be the last data field in the sentence, not counting the
checksum field. See 7.3.4.1.
NOTE This method to convey information is to be used only when absolutely necessary, and will only be considered
when one or both of two conditions are true, and when there is no alternative.
Condition 1: The data parameters are unknown by devices having to convey the information. For example, the ABM
and BBM sentences meet this condition, because the content is not known to the Automatic Identification System
(AIS).
Condition 2: W hen information requires a significantly higher data rate than can be achieved by the IEC 61162-1
(4 800 Bd) and IEC 61162-2 (38 400 Bd) standards utilizing parametric sentences.
By encapsulating a large amount of information, the number of overhead characters, such as "," field delimiters can
be reduced, resulting in higher data transfer rates. It is very unusual for this second condition to be fulfilled. As an
example, an AIS has a data rate capability of 4 500 messages per minute, and satisfies this condition, resulting in
the VDM and VDO sentences.
7.3.4.2 Structure
The following provides a summary explanation of the approved encapsulation sentence
structure:
!aaccc,x1,x2,x3,c--c,x4*hh<CR><LF>
– 20 – 61162-1 Ó IEC:2010(E)
ASCII HEX description
"!" 21 start of sentence: starting delimiter.
aaccc "," x1 x2 x3 c--c x4 "*" hh 2C 2A address field: alphanumeric characters identifying type of talker,
and sentence formatter. The first two characters identify the
talker. The last three are the sentence formatter mnemonic code
identifying the data type and the string format of the successive
fields. Mnemonics will be used as far as possible to facilitate
readouts by users.
field delimiter: starts each field except address and checksum
fields. If it is followed by a null field, it is all that remains to
indicate no data in a field.
total number of sentences field: encapsulated information often
requires more than one sentence. This field represents the total
number of encapsulated sentences needed. This may be a fixed
or variable length, and is defined by the sentence definitions in
8.3.
sentence number field: encapsulated information often requires
more than one sentence. This field identifies which sentence of
the total number of sentences this is. This may be fixed or
variable length, and is defined by the sentence definitions in 8.3.
sequential message identifier field: this field distinguishes one
encapsulated message consisting of one or more sentences, from
another encapsulated message using the same sentence
formatter. This field is incremented each time an encapsulated
message is generated with the same formatter as a previously
encapsulated message. The value is reset to zero when it is
incremented beyond the defined maximum value. The maximum
value and size of this field are determined by the applicable
sentence definitions in Clause 8.
data sentence block: follows sequential message identifier field
and is a series of data fields consisting of one or more parametric
data fields and one encapsulated data field. Data field sequence
is fixed and identified by third and subsequent characters of the
address field (the "sentence formatter"). Individual data fields
may be of variable length and are preceded by delimiters ",". The
encapsulated data field shall always be the second to the last
data field in the sentence.
fill bits field: this field represents the number of fill bits added to
complete the last six-bit coded character. This field is required
and shall immediately follow the encapsulated data field. To
encapsulate, the number of binary bits shall be a multiple of six.
If it is not, one to five fill bits are added. This field shall be set to
zero when no fill bits have been added. The fill bits field shall
always be the last data field in the sentence. This shall not be a
null field.
checksum delimiter: follows the last data field of the sentence. It
indicates that the following two alphanumeric characters show the
HEX value of the checksum.
checksum Field: the absolute value calculated by exclusive-
OR'ing the 8 data bits (no start bits or stop bits) of each character
in the sentence, between, but excluding "!" and "*". The
hexadecimal value of the most significant and least significant 4
61162-1 Ó IEC:2010(E) – 21 –
bits of the result are converted to two ASCII characters (0-9, A-F
(upper case)) for transmission. The most significant character is
transmitted first. The checksum field is required in all transmitted
sentences.
end of sentence: sentence terminating delimiter.
<CR><LF> 0D 0A 7.3.5 Query sentences
7.3.5.1 Description
Query sentences are intended to request approved sentences to be transmitted in a form of
two-way communication. The use of query sentences implies that the listener shall have the
capability of being a talker with its own bus. Query sentences shall always be constructed
with the "$" – start of sentence delimiter.
The approved query sentence contains, in the order shown, the following elements:
ASCII HEX description
"$" 24 start of sentence
<aa> talker identifier of requester
<aa> talker identifier for device from which data is being requested
"Q" query character, identifies query address
"," data field delimiter
<ccc> approved sentence formatter of data being requested
"*" <checksum field> checksum field
<CR><LF>0D 0A end of sentence
7.3.5.2 Reply to query sentence
The reply to a query sentence is the approved sentence that was requested. The use
of query sentences requires cooperation between the devices that are interconnected.
A reply to a query sentence is not mandatory and there is no specified time delay between
the receipt of a query and the reply.
7.3.6 Proprietary sentences
These are sentences not included within this standard; these provide a means for
manufacturers to use the sentence structure definitions of this standard to transfer data
which does not fall within the scope of approved sentences. This will generally be for one of
the following reasons:
a) data is intended for another device from the same manufacturer, is device specific, and
not in a form or of a type of interest to the general user;
b) data is being used for test purposes prior to the adoption of approved sentences;
c) data is not of a type and general usefulness which merits the creation of an approved
sentence.
NOTE The manufacturer's reference list of mnemonic codes is a component of the equivalent specification
NMEA 0183. 2
___________
2 The NMEA Secretariat maintains the master reference list which comprises codes registered and formally
adopted by NMEA.
The address for the registration of manufacturer’s codes is:
NMEA 0183 Technical Standards Committee Phone: +1 410 975 9450
– 22 – 61162-1 Ó IEC:2010(E)
A proprietary sentence contains, in the order shown, the following elements:
ASCII HEX description
"$" 24 start of sentence
"P" 50 proprietary sentence ID
<aaa> manufacturer's mnemonic code (The NMEA secretariat
maintains the master reference list which comprises
codes registered and formally adopted by NMEA)
[<valid characters,”^” and ”,” >] manufacturer's data
"*”<checksum field> checksum field
<CR><LF> 0D 0A end of sentence
Proprietary sentences shall include checksums and conform to requirements limiting overall
sentence length. Manufacturer’s data fields shall contain only valid characters but may
include “^” and “,” for delimiting or as manufacturer’s data. Details of proprietary data fields
are not included in this standard and need not be submitted for approval. However, it is
required that such sentences be published in the manufacturer’s manuals for reference.
7.3.7 Command sentences
Command sentences are those that provide an ability to alter or change the configuration or
operation of a device. Examples of legacy command sentences are the “HTC -
Heading/Track control command” and the “ACA - AIS channel assignment” sentences.
When a command sentence is generated in response to a Query sentence, a means to
identify that the sentence has only a status report of current settings is required.
Some command sentences cannot be queried and provide a different sentence formatter for
status information, so they should not be misinterpreted. This is the case with the HTC
sentence. The HTD sentence is provided to determine the status of a heading control
system’s settings. There is a high possibility of misinterpretation if a device receives a
query sentence for a HTC sentence, and erroneously provides the HTC sentence.
The ACA sentence is an example of a command sentence that can also be queried to
determine the status of the current settings. The ACA sentence definition provides a field
that when set to any valid value, identifies the sentence as a status of current settings and
not a command to change settings. There is a high probability of misinterpreting this
sentence because the field is used for two distinct purposes at the same time.
To avoid any possibility of misinterpretation and to satisfy the requirements of the voyage
data recorder required to be carried on ships under the SOLAS Convention, a clear and
unambiguous means to identify that a command sentence is to be interpreted as a
command or that it contains status information only and is not a command shall be
provided.
Any sentence that contains one or more command fields shall be identified as a “command
sentence”. Command sentences shall contain the “Sentence status flag” field.
Field formatter Description
a Sentence status flag. This field is a required field for any sentence
designated as a command sentence. The field distinguishes the
contents of command sentence as being commands intended to
change settings or as being status information only.
7 Riggs Ave e-mail: info@nmea.org
Servana Pk, Maryland 21146, USA web site http://www.nmea.org
61162-1 Ó IEC:2010(E) – 23 –
This field shall not be null.
This field shall contain an “R” when the sentence is a status report of
current settings. This may occur when the sentence is provided in
response to a query or is autonomously generated.
This field shall contain a “C” when the sentence is a configuration
command to change settings. A sentence without a “C” in this field is
not a command. If a designated command sentence cannot be
queried, as stated in the sentence’s definition, this field shall always
be set to “C”.
Where data fields are NULL in a command sentence (sentence status
flag = C), there is no change in their setting. When a configuration
data field is NULL in a status report sentence (sentence status flag =
R), this data field is not configured.
7.3.8 Valid sentences
Approved sentences, query sentences and proprietary sentences are the only valid
sentences. Sentences of any other form are non-valid and shall not be transmitted on the
bus.
7.3.9 Multi-sentence messages
Multi-sentence messages may be transmitted where a data message exceeds the available
character space in a single sentence formatter. All the sentences in a multi-sentence
message use the same sentence formatter. The key fields supporting the multi-sentence
message capability shall always be included, without exception. These required fields are:
total number of sentences, sentence number, and sequential message identifier fields. Only
sentence definitions containing these fields may be used to form messages. The TUT and
VDM sentences are good examples of how a sentence is defined to provide these
capabilities.
The listener should be aware that a multi-sentence message may be interrupted by a higher
priority message such as an alarm sentence, and thus the original message should be
discarded as incomplete and has to await a re-transmission. The listener has to check that
multi-sentences are contiguous.
Should an error occur in any sentence of a multi-sentence message, the listener shall
discard the whole message and be prepared to receive the message again upon the next
transmission.
7.3.10 Sentence transmission timing
Frequency of sentence transmission when specified shall be in accordance with the
approved sentence definitions (see 8.3). When not specified, the rate shall be consistent
with the basic measurement or calculation cycle but generally not more frequently than
once per second.
It is desirable that sentences be transmitted with minimum inter-character spacing,
preferably as a near continuous burst, but under no circumstance shall the time to complete
the transmission of a sentence be greater than 1 s.
7.3.11 Additions to approved sentences
In order to allow for improvements or additions, future revisions of this standard may modify
existing sentences by adding new data fields after the last data field but before the
– 24 – 61162-1 Ó IEC:2010(E)
checksum delimiter character "*" and checksum field. Listeners shall determine the end of
the sentence by recognition of "<CR><LF>" and "*" rather than by counting field delimiters.
The checksum value shall be computed on all received characters between, but not
including, "$" or “!” and "*" whether or not the listener recognizes all fields.
7.4 Error detection and handling
Listening devices shall detect errors in data transmission including:
a) checksum error (see 7.2.4);
b) invalid characters (see 7.1.3);
c) incorrect length of address field (see 7.2.2), and data fields as specified within sentence
definitions;
d) time out of sentence transfer (see 7.3.10).
d) time out of sentence transfer (see 7.3.10).
Listening devices shall use only correct sentences, consistent with the version of
IEC 61162-1 supported by the talker devices.
7.5 Handling of deprecated sentences
Deprecated sentences are no longer recommended for sole use in new or revised designs.
These sentences are valid sentences, but due to changing circumstances it is desirable to
delete or replace these sentences.
Generally, in each of the deprecated sentences a reference is made to a replacement
sentence in the current edition of the standard. Manufacturers are urged to use the currently
recommended sentence in new or revised designs. It is desirable that manufacturers
provide both new and old sentences whenever possible for a period of time that will serve
as a phase-in period for new sentences.