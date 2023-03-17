package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/jclark/gps2phc/internal/pmc"
)

type hexUint8 int

func (i *hexUint8) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 8)
	if err != nil {
		return err
	}
	*i = hexUint8(v)
	return nil
}

func (i *hexUint8) String() string {
	return fmt.Sprintf("0x%02x", uint8(*i))
}

func hexUint8Flag(name string, value hexUint8, usage string) *hexUint8 {
	flag.CommandLine.Var(&value, name, usage)
	return &value
}

func main() {
	msg := createMsg()
	err := send(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func createMsg() pmc.MgmtMsg {
	utcOffset := flag.Int("utcOffset", 37, "TAI-UTC offset in seconds")
	clockClass := hexUint8Flag("clockClass", 0x6, "clock class")
	clockAccuracy := hexUint8Flag("clockAccuracy", 0x23, "clock accuracy")
	offsetScaledLogVariance := flag.Uint("offsetScaledLogVariance", 0xFFFF, "offset scaled log variance")
	timeSource := hexUint8Flag("timeSource", 0x20, "time source")
	leap61 := flag.Bool("leap61", false, "positive leap second at end of current UTC day")
	leap59 := flag.Bool("leap59", false, "negative leap second at end of current UTC day")
	currentUTCOffsetValid := flag.Bool("currentUtcOffsetValid", false, "current UTC offset is traceable")
	ptpTimescale := flag.Bool("ptpTimescale", false, "use PTP timescale")
	timeTraceable := flag.Bool("timeTraceable", false, "time is traceable")
	frequencyTraceable := flag.Bool("frequencyTraceable", false, "frequency is traceable")
	getMID := flag.Uint("get", 0, "management message ID to get")
	flag.Parse()
	if *getMID != 0 {
		return pmc.NewMgmtGetMsg(pmc.MgmtID(*getMID))
	}
	var gs pmc.GrandmasterSettings
	gs.UTCOffset = int16(*utcOffset)
	gs.ClockQuality.ClockClass = uint8(*clockClass)
	gs.ClockQuality.ClockAccuracy = uint8(*clockAccuracy)
	gs.ClockQuality.OffsetScaledLogVariance = uint16(*offsetScaledLogVariance)
	gs.TimeSource = uint8(*timeSource)
	if *leap61 {
		gs.TimeFlags |= pmc.Leap61
	}
	if *leap59 {
		gs.TimeFlags |= pmc.Leap59
	}
	if *currentUTCOffsetValid {
		gs.TimeFlags |= pmc.CurrentUTCOffsetValid
	}
	if *ptpTimescale {
		gs.TimeFlags |= pmc.PTPTimescale
	}
	if *timeTraceable {
		gs.TimeFlags |= pmc.TimeTraceable
	}
	if *frequencyTraceable {
		gs.TimeFlags |= pmc.FrequencyTraceable
	}
	return pmc.NewMgmtSetMsg(gs)
}

const serverSocketPath = "/var/run/ptp4l"
const clientSocketPath = "/tmp/client.sock"

func send(msg pmc.MgmtMsg) error {
	client := pmc.NewMgmtClient()

	serverAddr := net.UnixAddr{
		Name: serverSocketPath,
		Net:  "unixgram",
	}

	clientAddr := net.UnixAddr{
		Name: clientSocketPath,
		Net:  "unixgram",
	}

	// Clean up the client socket file if it exists
	os.Remove(clientSocketPath)

	conn, err := net.DialUnix("unixgram", &clientAddr, nil)
	if err != nil {
		return fmt.Errorf("could not create connection: %w", err)
	}
	defer func() {
		conn.Close()
		os.Remove(clientSocketPath)
	}()

	client.PrepareMsg(msg)

	data, err := msg.MarshalBinary()
	if err != nil {
		return fmt.Errorf("could not marshal message: %w", err)
	}
	_, err = conn.WriteTo(data, &serverAddr)
	if err != nil {
		return fmt.Errorf("could not write message: %w", err)
	}

	buf := make([]byte, 2048)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		return fmt.Errorf("could not read message: %w", err)
	}

	recvData := buf[:n]

	m, err := pmc.UnmarshalMgmtMsg(recvData)
	if err != nil {
		return fmt.Errorf("could not unmarshal message: %w", err)
	}

	fmt.Printf("Received: %+v\n", m)
	return nil
}
