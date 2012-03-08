package service

/*
#include <windows.h>

SERVICE_STATUS          gSvcStatus; 
SERVICE_STATUS_HANDLE   gSvcStatusHandle; 
HANDLE                  ghSvcStopEvent = NULL;

void SvcInstall(void);
void WINAPI SvcCtrlHandler( DWORD ); 
void WINAPI SvcMain( DWORD, LPTSTR * ); 

void ReportSvcStatus( DWORD, DWORD, DWORD );

static char *goServiceName;

int goSigStart = 0;
int goAckStart = 0;
HANDLE goWaitStart = NULL;

int goSigStop = 0;
int goAckStop = 0;
HANDLE goWaitStop = NULL;

int goSigError = 0;
char *errorText;

void
continueStart(int response) {
	goSigStart = 0;
	goAckStart = response;
	SetEvent(goWaitStart);
}

void
continueStop(int response) {
	goSigStop = 0;
	goAckStop = response;
	SetEvent(goWaitStop);
}

void
signalError(char *text) {
	errorText = text;
	goSigError = 1;
}

void
initService(char *serviceName) {
	if(!goServiceName) {
		free(goServiceName);
	}
	goServiceName = serviceName;
    SERVICE_TABLE_ENTRY DispatchTable[] = {
        { serviceName, (LPSERVICE_MAIN_FUNCTION) SvcMain },
        { NULL, NULL }
    };

    // This call returns when the service has stopped. 
    // The process should simply terminate when the call returns.
    if (!StartServiceCtrlDispatcher( DispatchTable )) {
        signalError("StartServiceCtrlDispatcher"); 
    }
} 

// Entry point for the service
//   dwArgc - Number of arguments in the lpszArgv array
//   lpszArgv - Array of strings. The first string is the name of
//     the service and subsequent strings are passed by the process
//     that called the StartService function to start the service.
VOID WINAPI SvcMain( DWORD dwArgc, LPTSTR *lpszArgv )
{
    // Register the handler function for the service

    gSvcStatusHandle = RegisterServiceCtrlHandler( 
        goServiceName, 
        SvcCtrlHandler);

    if( !gSvcStatusHandle ) {
		signalError("RegisterServiceCtrlHandler");
        return; 
    } 

    // These SERVICE_STATUS members remain as set here
    gSvcStatus.dwServiceType = SERVICE_WIN32_OWN_PROCESS; 
    gSvcStatus.dwServiceSpecificExitCode = 0;    

    // Report initial status to the SCM
    ReportSvcStatus( SERVICE_START_PENDING, NO_ERROR, 3000 );

	goWaitStart = CreateEvent(NULL, TRUE, FALSE, NULL);
	goWaitStop = CreateEvent(NULL, TRUE, FALSE, NULL);

	// signal go to start
	// wait for go to confirm
	goSigStart = 1;
	WaitForSingleObject(goWaitStart, INFINITE);
	if(goAckStart != 1) {
		return;
	}

    // TODO: Declare and set any required variables.
    //   Be sure to periodically call ReportSvcStatus() with 
    //   SERVICE_START_PENDING. If initialization fails, call
    //   ReportSvcStatus with SERVICE_STOPPED.

    // Create an event. The control handler function, SvcCtrlHandler,
    // signals this event when it receives the stop control code.
    ghSvcStopEvent = CreateEvent(
                         NULL,    // default security attributes
                         TRUE,    // manual reset event
                         FALSE,   // not signaled
                         NULL);   // no name

    if ( ghSvcStopEvent == NULL)
    {
        ReportSvcStatus( SERVICE_STOPPED, NO_ERROR, 0 );
        return;
    }

    // Report running status when initialization is complete.
	ReportSvcStatus( SERVICE_RUNNING, NO_ERROR, 0 );

    while(1) {

		WaitForSingleObject(ghSvcStopEvent, INFINITE);

		// Signal service Stopped
		ReportSvcStatus( SERVICE_STOPPED, NO_ERROR, 0 );
		return;
	}
}

//   Sets the current service status and reports it to the SCM.
//   dwCurrentState - The current state (see SERVICE_STATUS)
//   dwWin32ExitCode - The system error code
//   dwWaitHint - Estimated time for pending operation, in milliseconds
VOID ReportSvcStatus( DWORD dwCurrentState, DWORD dwWin32ExitCode, DWORD dwWaitHint) {
    static DWORD dwCheckPoint = 1;

    // Fill in the SERVICE_STATUS structure.
    gSvcStatus.dwCurrentState = dwCurrentState;
    gSvcStatus.dwWin32ExitCode = dwWin32ExitCode;
    gSvcStatus.dwWaitHint = dwWaitHint;

    if (dwCurrentState == SERVICE_START_PENDING) {
		gSvcStatus.dwControlsAccepted = 0;
    } else {
		gSvcStatus.dwControlsAccepted = SERVICE_ACCEPT_STOP;
	}

    if ( (dwCurrentState == SERVICE_RUNNING) || (dwCurrentState == SERVICE_STOPPED) ) {
        gSvcStatus.dwCheckPoint = 0;
    } else {
		gSvcStatus.dwCheckPoint = dwCheckPoint++;
	}

    // Report the status of the service to the SCM.
    SetServiceStatus( gSvcStatusHandle, &gSvcStatus );
}

// Called by SCM whenever a control code is sent to the service
// using the ControlService function.
VOID WINAPI SvcCtrlHandler( DWORD dwCtrl ) {
	// Handle the requested control code. 
	switch(dwCtrl) {  
		case SERVICE_CONTROL_STOP: 
			ReportSvcStatus(SERVICE_STOP_PENDING, NO_ERROR, 0);

			goSigStop = 1;
			WaitForSingleObject(goWaitStop, INFINITE);
			if(goAckStop != 1) {
				return;
			}

			// Now signal on the initial thread to stop blocking.
			SetEvent(ghSvcStopEvent);
			return;
		case SERVICE_CONTROL_INTERROGATE: 
			ReportSvcStatus(gSvcStatus.dwCurrentState, NO_ERROR, 0);
			break; 

		default: 
			break;
	}
}

*/
import "C"
import (
	"errors"
	"time"
)

// Starts a windows service routine.  Service must be registered first.
// Call blocks until an error occurs or the service stops.  If onStart returns
// an error, service will not start.  If onStop returns an error, the service will
// not stop.
func runService(serviceName string, onStart, onStop func() error) error {
	// We alloc a c string here, but do not free it here
	cname := C.CString(serviceName)

	retErr := make(chan error, 1)

	go func() {
		// Check C vars on timer.
		ticker := time.NewTicker(time.Second * 1)
		defer ticker.Stop()
		for _ = range ticker.C {
			if C.goSigStart == 1 {
				err := onStart()
				if err != nil {
					// An error was returned.
					// Signal to NOT start the service.
					C.continueStart(-1)
					retErr <- err
					return
				}
				C.continueStart(1)
			} else if C.goSigStop == 1 {
				err := onStop()
				if err != nil {
					// An error was returned.
					// Signal to NOT stop the service.
					C.continueStop(-1)
					retErr <- err
					return
				}
				C.continueStop(1)
				retErr <- nil
			} else if C.goSigError == 1 {
				// Check for service errors.
				errText := "SERVICE ERROR: "
				if C.errorText != nil {
					errText += C.GoString(C.errorText)
				}
				retErr <- errors.New(errText)
				return
			}
		}
	}()
	C.initService(cname)

	return <-retErr
}
