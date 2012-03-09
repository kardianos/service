package service

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

func NewService(name, displayName string) Service {
	return &windowsService{
		name:        name,
		displayName: displayName,
	}
}

type windowsService struct {
	name, displayName string
}

func (ws *windowsService) Install() error {
	pathToBinary, err := getModuleFileName()
	if err != nil {
		return err
	}

	scmManagerHandle, err := openSCManager()
	if err != nil {
		return err
	}
	defer closeServiceHandle(scmManagerHandle)

	serviceHandle, err := createService(scmManagerHandle, ws.name, ws.displayName, pathToBinary)
	if err != nil {
		return err
	}
	defer closeServiceHandle(serviceHandle)
	return nil
}

func (ws *windowsService) Remove() error {
	scmManagerHandle, err := openSCManager()
	if err != nil {
		return err
	}
	defer closeServiceHandle(scmManagerHandle)

	serviceHandle, err := openService(scmManagerHandle, ws.name)
	if err != nil {
		return err
	}
	defer closeServiceHandle(serviceHandle)

	err = deleteService(serviceHandle)
	if err != nil {
		return err
	}

	return nil
}

func (ws *windowsService) Run(onStart, onStop func() error) error {
	return runService(ws.name, onStart, onStop)
}

func (ws *windowsService) LogError(text string) error {
	return writeToEventLog(ws.name, text, levelError)
}
func (ws *windowsService) LogWarning(text string) error {
	return writeToEventLog(ws.name, text, levelWarning)
}
func (ws *windowsService) LogInfo(text string) error {
	return writeToEventLog(ws.name, text, levelInfo)
}

var (
	advapi = syscall.MustLoadDLL("advapi32.dll")

	createServiceProc      = advapi.MustFindProc("CreateServiceW")
	openServiceProc        = advapi.MustFindProc("OpenServiceW")
	deleteServiceProc      = advapi.MustFindProc("DeleteService")
	closeServiceHandleProc = advapi.MustFindProc("CloseServiceHandle")

	openEventLogProc          = advapi.MustFindProc("OpenEventLogW")
	registerEventSourceProc   = advapi.MustFindProc("RegisterEventSourceW")
	deregisterEventSourceProc = advapi.MustFindProc("DeregisterEventSource")
	reportEventProc           = advapi.MustFindProc("ReportEventW")

	openSCManagerProc = advapi.MustFindProc("OpenSCManagerW")

	kernel = syscall.MustLoadDLL("kernel32.dll")

	getModuleFileNameProc = kernel.MustFindProc("GetModuleFileNameW")
)

const (
	_STANDARD_RIGHTS_REQUIRED  = 0x000F0000
	_SERVICE_WIN32_OWN_PROCESS = 0x00000010
	_SERVICE_DEMAND_START      = 0x00000003
	_SERVICE_ERROR_NORMAL      = 0x00000001
)

const (
	_SC_MANAGER_CONNECT            = 0x0001
	_SC_MANAGER_CREATE_SERVICE     = 0x0002
	_SC_MANAGER_ENUMERATE_SERVICE  = 0x0004
	_SC_MANAGER_LOCK               = 0x0008
	_SC_MANAGER_QUERY_LOCK_STATUS  = 0x0010
	_SC_MANAGER_MODIFY_BOOT_CONFIG = 0x0020

	_SC_MANAGER_ALL_ACCESS = (_STANDARD_RIGHTS_REQUIRED |
		_SC_MANAGER_CONNECT |
		_SC_MANAGER_CREATE_SERVICE |
		_SC_MANAGER_ENUMERATE_SERVICE |
		_SC_MANAGER_LOCK |
		_SC_MANAGER_QUERY_LOCK_STATUS |
		_SC_MANAGER_MODIFY_BOOT_CONFIG)
)

const (
	_SERVICE_QUERY_CONFIG         = 0x0001
	_SERVICE_CHANGE_CONFIG        = 0x0002
	_SERVICE_QUERY_STATUS         = 0x0004
	_SERVICE_ENUMERATE_DEPENDENTS = 0x0008
	_SERVICE_START                = 0x0010
	_SERVICE_STOP                 = 0x0020
	_SERVICE_PAUSE_CONTINUE       = 0x0040
	_SERVICE_INTERROGATE          = 0x0080
	_SERVICE_USER_DEFINED_CONTROL = 0x0100

	_SERVICE_ALL_ACCESS = (_STANDARD_RIGHTS_REQUIRED |
		_SERVICE_QUERY_CONFIG |
		_SERVICE_CHANGE_CONFIG |
		_SERVICE_QUERY_STATUS |
		_SERVICE_ENUMERATE_DEPENDENTS |
		_SERVICE_START |
		_SERVICE_STOP |
		_SERVICE_PAUSE_CONTINUE |
		_SERVICE_INTERROGATE |
		_SERVICE_USER_DEFINED_CONTROL)
)

type eventLevel uint32

const (
	levelError   eventLevel = 0x0001
	levelWarning eventLevel = 0x0002
	levelInfo    eventLevel = 0x0004
)

func writeToEventLog(title, text string, level eventLevel) error {
	eventSource, err := registerEventSource(title)
	if err != nil {
		return err
	}
	err = reportEvent(eventSource, title, text, level)
	if err != nil {
		return err
	}
	err = deregisterEventSource(eventSource)
	if err != nil {
		return err
	}
	return nil
}

func registerEventSource(title string) (syscall.Handle, error) {
	r0, _, e1 := registerEventSourceProc.Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))))
	if r0 == 0 {
		return syscall.Handle(0), e1
	}
	return syscall.Handle(r0), nil
}
func deregisterEventSource(eventSrouce syscall.Handle) error {
	r0, _, e1 := deregisterEventSourceProc.Call(uintptr(eventSrouce))
	if r0 == 0 {
		return e1
	}
	return nil
}

/*
const (
	_EVENTLOG_ERROR_TYPE       = 0x0001
	_EVENTLOG_WARNING_TYPE     = 0x0002
	_EVENTLOG_INFORMATION_TYPE = 0x0004
)
*/

func reportEvent(eventSource syscall.Handle, title, text string, level eventLevel) error {
	msg := [...]uintptr{
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(text)))}
	r0, _, e1 := reportEventProc.Call(
		uintptr(eventSource),
		uintptr(level), //type
		uintptr(0),     //category
		uintptr(3), //eventID
		uintptr(0),
		uintptr(2),
		uintptr(0),
		uintptr(unsafe.Pointer(&msg[0])),
		0)
	if r0 == 0 {
		return e1
	}
	return nil
}

func closeServiceHandle(service syscall.Handle) error {
	r0, _, e1 := closeServiceHandleProc.Call(uintptr(service))
	if r0 == 0 {
		return e1
	}
	return nil
}
func createService(scManager syscall.Handle, serviceName, serviceDisplayName, pathToBinary string) (syscall.Handle, error) {
	r0, _, e1 := createServiceProc.Call(
		uintptr(scManager),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(serviceName))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(serviceDisplayName))),
		uintptr(uint32(_SERVICE_ALL_ACCESS)),
		uintptr(uint32(_SERVICE_WIN32_OWN_PROCESS)),
		uintptr(uint32(_SERVICE_DEMAND_START)),
		uintptr(uint32(_SERVICE_ERROR_NORMAL)),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(pathToBinary))),
		0, 0, 0, 0, 0) //+4 optional params not here
	if r0 == 0 {
		return syscall.Handle(0), e1
	}
	return syscall.Handle(r0), nil
}
func deleteService(serviceHandle syscall.Handle) error {
	r0, _, e1 := deleteServiceProc.Call(uintptr(serviceHandle))
	if r0 == 0 {
		return e1
	}
	return nil
}
func openService(scManager syscall.Handle, serviceName string) (syscall.Handle, error) {
	r0, _, e1 := openServiceProc.Call(uintptr(scManager), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(serviceName))), uintptr(uint32(_SC_MANAGER_ALL_ACCESS)))
	if r0 == 0 {
		return syscall.Handle(0), e1
	}
	return syscall.Handle(r0), nil
}
func getModuleFileName() (string, error) {
	var n uint32
	b := make([]uint16, syscall.MAX_PATH)
	size := uint32(len(b))

	r0, _, e1 := getModuleFileNameProc.Call(0, uintptr(unsafe.Pointer(&b[0])), uintptr(size))
	n = uint32(r0)
	if n == 0 {
		return "", e1
	}
	return string(utf16.Decode(b[0:n])), nil
}

func openSCManager() (syscall.Handle, error) {
	r0, _, e1 := openSCManagerProc.Call(uintptr(0), uintptr(0), uintptr(uint32(_SC_MANAGER_ALL_ACCESS)))
	if r0 == 0 {
		return syscall.Handle(0), e1
	}
	return syscall.Handle(r0), nil
}
