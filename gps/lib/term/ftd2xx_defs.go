//go:build ignore

package term

/*
#include <pthread.h>
#include <ftd2xx.h>
*/
import "C"

const (
	ftOK                      = C.FT_OK
	ftInvalidHandle           = C.FT_INVALID_HANDLE
	ftDeviceNotFound          = C.FT_DEVICE_NOT_FOUND
	ftDeviceNotOpened         = C.FT_DEVICE_NOT_OPENED
	ftIOError                 = C.FT_IO_ERROR
	ftInsufficientResources   = C.FT_INSUFFICIENT_RESOURCES
	ftInvalidParameter        = C.FT_INVALID_PARAMETER
	ftInvalidBaudRate         = C.FT_INVALID_BAUD_RATE
	ftDeviceNotOpenedForErase = C.FT_DEVICE_NOT_OPENED_FOR_ERASE
	ftDeviceNotOpenedForWrite = C.FT_DEVICE_NOT_OPENED_FOR_WRITE
	ftFailedToWriteDevice     = C.FT_FAILED_TO_WRITE_DEVICE
	ftEEPROMReadFailed        = C.FT_EEPROM_READ_FAILED
	ftEEPROMWriteFailed       = C.FT_EEPROM_WRITE_FAILED
	ftEEPROMEraseFailed       = C.FT_EEPROM_ERASE_FAILED
	ftEEPROMNotPresent        = C.FT_EEPROM_NOT_PRESENT
	ftEEPROMNotProgrammed     = C.FT_EEPROM_NOT_PROGRAMMED
	ftInvalidArgs             = C.FT_INVALID_ARGS
	ftNotSupported            = C.FT_NOT_SUPPORTED
	ftOtherError              = C.FT_OTHER_ERROR
	ftDeviceListNotReady      = C.FT_DEVICE_LIST_NOT_READY
	ftOpenBySerialNumber      = C.FT_OPEN_BY_SERIAL_NUMBER
	ftBits8                   = C.FT_BITS_8
	ftBits7                   = C.FT_BITS_7
	ftStopBits1               = C.FT_STOP_BITS_1
	ftStopBits2               = C.FT_STOP_BITS_2
	ftParityNone              = C.FT_PARITY_NONE
	ftParityOdd               = C.FT_PARITY_ODD
	ftParityEven              = C.FT_PARITY_EVEN
	ftFlowNone                = C.FT_FLOW_NONE
	ftFlowRTSCTS              = C.FT_FLOW_RTS_CTS
	ftFlowXONXOFF             = C.FT_FLOW_XON_XOFF
	ftPurgeRX                 = C.FT_PURGE_RX
	ftPurgeTX                 = C.FT_PURGE_TX
	ftEventRXChar             = C.FT_EVENT_RXCHAR
	ftEventModemStatus        = C.FT_EVENT_MODEM_STATUS
	ftModemCTS                = C.MS_CTS_ON
	ftModemDSR                = C.MS_DSR_ON
	ftModemRI                 = C.MS_RING_ON
	ftModemDCD                = C.MS_RLSD_ON
	ftDefaultRXTimeout        = C.FT_DEFAULT_RX_TIMEOUT
)

// The pthread types are declared so that godefs renders the EVENT_HANDLE
// fields; sizes and offsets come from the Go types with unsafe.
type pthreadCond C.pthread_cond_t

type pthreadMutex C.pthread_mutex_t

type ftEventHandle C.EVENT_HANDLE

type ftDeviceListInfoNode C.FT_DEVICE_LIST_INFO_NODE
