//go:build windows

package folderpick

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	clsctxInprocServer = 0x1
	fosPickFolders     = 0x20
	fosForceFileSystem = 0x40
	fosNoChangeDir     = 0x8
	fosPathMustExist   = 0x800
	sigdnFileSysPath   = 0x80058000
	hresultCancel      = 0x800704C7
	sFalse             = syscall.Errno(1)
)

var (
	clsidFileOpenDialog = windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidIShellItem       = windows.GUID{Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE, Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}

	ole32                           = windows.NewLazySystemDLL("ole32.dll")
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	user32                          = windows.NewLazySystemDLL("user32.dll")
	procCoCreateInstance            = ole32.NewProc("CoCreateInstance")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	procGetForegroundWindow         = user32.NewProc("GetForegroundWindow")
)

type iUnknownVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

type iFileOpenDialogVtbl struct {
	iUnknownVtbl
	Show                uintptr
	SetFileTypes        uintptr
	SetFileTypeIndex    uintptr
	GetFileTypeIndex    uintptr
	Advise              uintptr
	Unadvise            uintptr
	SetOptions          uintptr
	GetOptions          uintptr
	SetDefaultFolder    uintptr
	SetFolder           uintptr
	GetFolder           uintptr
	GetCurrentSelection uintptr
	SetFileName         uintptr
	GetFileName         uintptr
	SetTitle            uintptr
	SetOkButtonLabel    uintptr
	SetFileNameLabel    uintptr
	GetResult           uintptr
	AddPlace            uintptr
	SetDefaultExtension uintptr
	Close               uintptr
	SetClientGuid       uintptr
	ClearClientData     uintptr
	SetFilter           uintptr
	GetResults          uintptr
	GetSelectedItems    uintptr
}

type iFileOpenDialog struct {
	vtbl *iFileOpenDialogVtbl
}

type iShellItemVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	BindToHandler  uintptr
	GetParent      uintptr
	GetDisplayName uintptr
	GetAttributes  uintptr
	Compare        uintptr
}

type iShellItem struct {
	vtbl *iShellItemVtbl
}

func Pick(start string) (string, bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && !errors.Is(err, sFalse) {
		return "", false, fmt.Errorf("initialize COM: %w", err)
	}
	defer windows.CoUninitialize()

	var dialog *iFileOpenDialog
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)),
		uintptr(unsafe.Pointer(&dialog)),
	)
	if uint32(hr) != 0 || dialog == nil {
		return "", false, fmt.Errorf("create folder dialog: HRESULT 0x%08X", uint32(hr))
	}
	defer syscall.SyscallN(dialog.vtbl.Release, uintptr(unsafe.Pointer(dialog)))

	options := uintptr(fosPickFolders | fosForceFileSystem | fosNoChangeDir | fosPathMustExist)
	if hr, _, _ = syscall.SyscallN(dialog.vtbl.SetOptions, uintptr(unsafe.Pointer(dialog)), options); uint32(hr) != 0 {
		return "", false, fmt.Errorf("configure folder dialog: HRESULT 0x%08X", uint32(hr))
	}

	title, err := windows.UTF16PtrFromString("Choose download folder")
	if err == nil {
		_, _, _ = syscall.SyscallN(dialog.vtbl.SetTitle, uintptr(unsafe.Pointer(dialog)), uintptr(unsafe.Pointer(title)))
	}

	if folder := usableFolder(start); folder != "" {
		if item, itemErr := shellItemFromPath(folder); itemErr == nil {
			_, _, _ = syscall.SyscallN(dialog.vtbl.SetFolder, uintptr(unsafe.Pointer(dialog)), uintptr(unsafe.Pointer(item)))
			_, _, _ = syscall.SyscallN(item.vtbl.Release, uintptr(unsafe.Pointer(item)))
		}
	}

	owner, _, _ := procGetForegroundWindow.Call()
	if hr, _, _ = syscall.SyscallN(dialog.vtbl.Show, uintptr(unsafe.Pointer(dialog)), owner); uint32(hr) == hresultCancel {
		return "", true, nil
	} else if uint32(hr) != 0 {
		return "", false, fmt.Errorf("show folder dialog: HRESULT 0x%08X", uint32(hr))
	}

	var item *iShellItem
	if hr, _, _ = syscall.SyscallN(dialog.vtbl.GetResult, uintptr(unsafe.Pointer(dialog)), uintptr(unsafe.Pointer(&item))); uint32(hr) != 0 || item == nil {
		return "", false, fmt.Errorf("read selected folder: HRESULT 0x%08X", uint32(hr))
	}
	defer syscall.SyscallN(item.vtbl.Release, uintptr(unsafe.Pointer(item)))

	var name *uint16
	if hr, _, _ = syscall.SyscallN(item.vtbl.GetDisplayName, uintptr(unsafe.Pointer(item)), sigdnFileSysPath, uintptr(unsafe.Pointer(&name))); uint32(hr) != 0 || name == nil {
		return "", false, fmt.Errorf("read selected folder path: HRESULT 0x%08X", uint32(hr))
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(name))

	path, err := validResult(windows.UTF16PtrToString(name))
	if err != nil {
		return "", false, err
	}
	return path, false, nil
}

func shellItemFromPath(path string) (*iShellItem, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var item *iShellItem
	hr, _, _ := procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&iidIShellItem)),
		uintptr(unsafe.Pointer(&item)),
	)
	if uint32(hr) != 0 || item == nil {
		return nil, fmt.Errorf("create shell item: HRESULT 0x%08X", uint32(hr))
	}
	return item, nil
}
