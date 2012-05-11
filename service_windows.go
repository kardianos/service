package service

import (
	"syscall"
	"unicode/utf16"
	"fmt"
	"unsafe"
)

func newService(name, displayName, description string) Service {
	return &windowsService{
		name:        name,
		displayName: displayName,
		description: description,
	}
}

type windowsService struct {
	name, displayName, description string
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

	err = changeServiceDescription(serviceHandle, ws.description)
	if err != nil {
		return err
	}

	regKey, err := regCreateKey(`SYSTEM\CurrentControlSet\Services\Eventlog\Application\` + ws.name)
	if err != nil {
		return err
	}
	defer regCloseKey(regKey)

	regSetKeyValue(regKey, "EventMessageFile", `%SystemRoot%\System32\EventCreate.exe`)
	if err != nil {
		return err
	}
	regSetKeyValue(regKey, "CustomSource", uint32(1))
	if err != nil {
		return err
	}
	regSetKeyValue(regKey, "TypesSupported", uint32(7))
	if err != nil {
		return err
	}

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

	err = regDeleteKey(`SYSTEM\CurrentControlSet\Services\Eventlog\Application\` + ws.name)
	if err != nil {
		return err
	}

	return nil
}

func (ws *windowsService) Run(onStart, onStop func() error) error {
	return runService(ws.name, onStart, onStop)
}

func (ws *windowsService) LogError(format string, a ...interface{}) error {
	return writeToEventLog(ws.name, fmt.Sprintf(format, a ...), levelError)
}
func (ws *windowsService) LogWarning(format string, a ...interface{}) error {
	return writeToEventLog(ws.name, fmt.Sprintf(format, a ...), levelWarning)
}
func (ws *windowsService) LogInfo(format string, a ...interface{}) error {
	return writeToEventLog(ws.name, fmt.Sprintf(format, a ...), levelInfo)
}

var (
	advapi = syscall.MustLoadDLL("advapi32.dll")
	kernel = syscall.MustLoadDLL("kernel32.dll")

	//advapi32.dll
	createServiceProc      = advapi.MustFindProc("CreateServiceW")
	openServiceProc        = advapi.MustFindProc("OpenServiceW")
	deleteServiceProc      = advapi.MustFindProc("DeleteService")
	closeServiceHandleProc = advapi.MustFindProc("CloseServiceHandle")

	openEventLogProc          = advapi.MustFindProc("OpenEventLogW")
	registerEventSourceProc   = advapi.MustFindProc("RegisterEventSourceW")
	deregisterEventSourceProc = advapi.MustFindProc("DeregisterEventSource")
	reportEventProc           = advapi.MustFindProc("ReportEventW")

	openSCManagerProc        = advapi.MustFindProc("OpenSCManagerW")
	changeServiceConfig2Proc = advapi.MustFindProc("ChangeServiceConfig2W")

	// Registry
	regCloseKeyProc      = advapi.MustFindProc("RegCloseKey")
	regSetKeyValueExProc = advapi.MustFindProc("RegSetValueExW")
	regCreateKeyExProc   = advapi.MustFindProc("RegCreateKeyExW")
	regDeleteKeyProc     = advapi.MustFindProc("RegDeleteKeyW")

	// kernel32.dll
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
		uintptr(1),     //eventID
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

const (
	_SERVICE_CONFIG_DESCRIPTION = 1
)

func changeServiceDescription(h syscall.Handle, desc string) error {
	msg := &struct {
		desc uintptr
	}{
		desc: uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(desc))),
	}
	r0, _, e1 := changeServiceConfig2Proc.Call(
		uintptr(h),
		uintptr(_SERVICE_CONFIG_DESCRIPTION),
		uintptr(unsafe.Pointer(msg)),
	)
	if r0 == 0 {
		return e1
	}
	return nil
}

// registry functions

/*
LONG WINAPI RegCloseKey(
  __in  HKEY hKey
);
LONG WINAPI RegSetValueExW(
  __in        HKEY hKey,
  __in_opt    LPCTSTR lpValueName,
  __reserved  DWORD Reserved,
  __in        DWORD dwType,
  __in        const BYTE *lpData,
  __in        DWORD cbData
);
LONG WINAPI RegCreateKeyExW(
  __in        HKEY hKey,
  __in        LPCTSTR lpSubKey,
  __reserved  DWORD Reserved,
  __in_opt    LPTSTR lpClass,
  __in        DWORD dwOptions,
  __in        REGSAM samDesired,
  __in_opt    LPSECURITY_ATTRIBUTES lpSecurityAttributes,
  __out       PHKEY phkResult,
  __out_opt   LPDWORD lpdwDisposition
);
LONG WINAPI RegDeleteKeyExW(
  __in        HKEY hKey,
  __in        LPCTSTR lpSubKey,
  __in        REGSAM samDesired,
  __reserved  DWORD Reserved
);
*/

func StringToUTF16PtrLen(s string) (ptr uintptr, l uintptr) {
	u := syscall.StringToUTF16(s)
	l = uintptr(len(u) * 2) //size in uint8, length of uint16
	ptr = uintptr(unsafe.Pointer(&u[0]))
	return
}

const (
	_HKEY_LOCAL_MACHINE = 0x80000002

	_REG_SZ    = 1
	_REG_DWORD = 4

	_KEY_ALL_ACCESS = 0xF003F
)

func regCloseKey(h syscall.Handle) error {
	r0, _, e1 := regCloseKeyProc.Call(
		uintptr(h),
	)
	if r0 != 0 {
		return e1
	}
	return nil
}

func regSetKeyValue(h syscall.Handle, keyName string, data interface{}) error {
	var dataPtr, dataLen, dataType uintptr
	switch v := data.(type) {
	case uint32:
		dataPtr, dataLen = uintptr(unsafe.Pointer(&v)), 4
		dataType = _REG_DWORD
	case string:
		// The comment on MSDN regarding escaping back-slashes, are c-lang specific.
		// The API just takes a normal NUL terminated string, no special escaping required.
		dataPtr, dataLen = StringToUTF16PtrLen(v)
		dataType = _REG_SZ
	}
	r0, _, e1 := regSetKeyValueExProc.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(keyName))),
		0,
		dataType,
		dataPtr,
		dataLen,
	)
	if r0 != 0 {
		return e1
	}
	return nil
}

func regCreateKey(keyName string) (h syscall.Handle, err error) {
	r0, _, e1 := regCreateKeyExProc.Call(
		uintptr(_HKEY_LOCAL_MACHINE),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(keyName))),
		0,
		0, //class
		0, //no options
		uintptr(_KEY_ALL_ACCESS),
		0, //no security
		uintptr(unsafe.Pointer(&h)),
		0, //can return if created or opened
	)
	if r0 != 0 {
		err = e1
		return
	}
	return
}

func regDeleteKey(keyName string) error {
	r0, _, e1 := regDeleteKeyProc.Call(
		uintptr(_HKEY_LOCAL_MACHINE),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(keyName))),
	)
	if r0 != 0 {
		return e1
	}
	return nil
}
