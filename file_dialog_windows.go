package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

var (
	clsidFileOpenDialog = win.CLSID{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	clsidFileSaveDialog = win.CLSID{0xC0B4E2F3, 0xBA21, 0x4773, [8]byte{0x8D, 0xBA, 0x33, 0x5E, 0xC9, 0x46, 0xEB, 0x8B}}
	iidIFileDialog      = win.IID{0x42F85136, 0xDB7E, 0x439C, [8]byte{0x85, 0xF1, 0xE4, 0x07, 0x5D, 0x13, 0x5F, 0xC8}}
	iidIShellItem       = win.IID{0x43826D1E, 0xE718, 0x42EE, [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}

	shell32CreateItemFromParsingName = syscall.NewLazyDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")
)

const (
	hresultCancelled = 0x800704C7
	sigDNFileSysPath = 0x80058000

	fosOverwritePrompt = 0x00000002
	fosNoChangeDir     = 0x00000008
	fosForceFilesystem = 0x00000040
	fosPathMustExist   = 0x00000800
	fosFileMustExist   = 0x00001000
)

func showHistoryFileDialog(owner walk.Form, title, defaultPath string, save bool) (string, bool, error) {
	path, ok, err := showHistoryFileDialogWithOwner(owner, title, defaultPath, save)
	if err == nil || owner == nil {
		return path, ok, err
	}
	return showHistoryFileDialogWithOwner(nil, title, defaultPath, save)
}

func showHistoryFileDialogWithOwner(owner walk.Form, title, defaultPath string, save bool) (string, bool, error) {
	if defaultPath == "" {
		defaultPath = defaultHistoryPath()
	}
	dir := filepath.Dir(defaultPath)
	if save {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", false, err
		}
	} else if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		dir = ""
		defaultPath = filepath.Base(defaultPath)
	}

	hr := win.OleInitialize()
	if hr != win.S_OK && hr != win.S_FALSE {
		return "", false, hresultError("OleInitialize", hr)
	}
	defer win.OleUninitialize()

	var clsid win.CLSID
	if save {
		clsid = clsidFileSaveDialog
	} else {
		clsid = clsidFileOpenDialog
	}
	var dialog *iFileDialog
	hr = win.CoCreateInstance(&clsid, nil, win.CLSCTX_INPROC_SERVER, &iidIFileDialog, (*unsafe.Pointer)(unsafe.Pointer(&dialog)))
	if win.FAILED(hr) {
		return "", false, hresultError("CoCreateInstance(IFileDialog)", hr)
	}
	defer dialog.Release()

	resources := newHistoryFileDialogResources(title, defaultPath)
	if err := configureHistoryFileDialog(dialog, resources, dir, save); err != nil {
		return "", false, err
	}
	defer runtime.KeepAlive(resources)

	var hwnd win.HWND
	if owner != nil {
		hwnd = owner.Handle()
	}
	hr = dialog.Show(hwnd)
	if isCancelledHResult(hr) {
		return "", false, nil
	}
	if win.FAILED(hr) {
		return "", false, hresultError("IFileDialog.Show", hr)
	}

	var item *iShellItem
	hr = dialog.GetResult(&item)
	if win.FAILED(hr) {
		return "", false, hresultError("IFileDialog.GetResult", hr)
	}
	if item == nil {
		return "", false, fmt.Errorf("Windows file picker did not return a selected path")
	}
	defer item.Release()

	var pathPtr *uint16
	hr = item.GetDisplayName(sigDNFileSysPath, &pathPtr)
	if win.FAILED(hr) {
		return "", false, hresultError("IShellItem.GetDisplayName", hr)
	}
	if pathPtr == nil {
		return "", false, fmt.Errorf("Windows file picker returned an empty path")
	}
	defer win.CoTaskMemFree(uintptr(unsafe.Pointer(pathPtr)))

	return utf16PtrToString(pathPtr), true, nil
}

type historyFileDialogResources struct {
	filterName []uint16
	filterSpec []uint16
	allName    []uint16
	allSpec    []uint16
	title      []uint16
	fileName   []uint16
	defaultExt []uint16
	filters    []comdlgFilterSpec
}

func newHistoryFileDialogResources(title, defaultPath string) *historyFileDialogResources {
	fileName := filepath.Base(defaultPath)
	if fileName == "." || fileName == string(filepath.Separator) {
		fileName = "history.json"
	}
	resources := &historyFileDialogResources{
		filterName: syscall.StringToUTF16("Pingaro History (*.json)"),
		filterSpec: syscall.StringToUTF16("*.json"),
		allName:    syscall.StringToUTF16("All Files (*.*)"),
		allSpec:    syscall.StringToUTF16("*.*"),
		title:      syscall.StringToUTF16(title),
		fileName:   syscall.StringToUTF16(fileName),
		defaultExt: syscall.StringToUTF16("json"),
	}
	resources.filters = []comdlgFilterSpec{
		{Name: &resources.filterName[0], Spec: &resources.filterSpec[0]},
		{Name: &resources.allName[0], Spec: &resources.allSpec[0]},
	}
	return resources
}

func configureHistoryFileDialog(dialog *iFileDialog, resources *historyFileDialogResources, dir string, save bool) error {
	if hr := dialog.SetFileTypes(uint32(len(resources.filters)), &resources.filters[0]); win.FAILED(hr) {
		return hresultError("IFileDialog.SetFileTypes", hr)
	}
	if hr := dialog.SetFileTypeIndex(1); win.FAILED(hr) {
		return hresultError("IFileDialog.SetFileTypeIndex", hr)
	}
	if hr := dialog.SetTitle(&resources.title[0]); win.FAILED(hr) {
		return hresultError("IFileDialog.SetTitle", hr)
	}
	if hr := dialog.SetFileName(&resources.fileName[0]); win.FAILED(hr) {
		return hresultError("IFileDialog.SetFileName", hr)
	}
	if hr := dialog.SetDefaultExtension(&resources.defaultExt[0]); win.FAILED(hr) {
		return hresultError("IFileDialog.SetDefaultExtension", hr)
	}

	options := uint32(fosForceFilesystem | fosNoChangeDir | fosPathMustExist)
	if save {
		options |= fosOverwritePrompt
	} else {
		options |= fosFileMustExist
	}
	if hr := dialog.SetOptions(options); win.FAILED(hr) {
		return hresultError("IFileDialog.SetOptions", hr)
	}

	if dir != "" {
		if folder, err := shellItemFromPath(dir); err == nil && folder != nil {
			if hr := dialog.SetFolder(folder); win.FAILED(hr) {
				folder.Release()
				return hresultError("IFileDialog.SetFolder", hr)
			}
			folder.Release()
		}
	}
	return nil
}

func shellItemFromPath(path string) (*iShellItem, error) {
	pathText := syscall.StringToUTF16(path)
	var item *iShellItem
	hr := shCreateItemFromParsingName(&pathText[0], nil, &iidIShellItem, (*unsafe.Pointer)(unsafe.Pointer(&item)))
	runtime.KeepAlive(pathText)
	if win.FAILED(hr) {
		return nil, hresultError("SHCreateItemFromParsingName", hr)
	}
	return item, nil
}

func utf16PtrToString(ptr *uint16) string {
	if ptr == nil {
		return ""
	}
	var chars []uint16
	for p := uintptr(unsafe.Pointer(ptr)); ; p += unsafe.Sizeof(uint16(0)) {
		c := *(*uint16)(unsafe.Pointer(p))
		if c == 0 {
			break
		}
		chars = append(chars, c)
	}
	return syscall.UTF16ToString(chars)
}

func hresultError(operation string, hr win.HRESULT) error {
	return fmt.Errorf("%s failed (HRESULT 0x%08X)", operation, uint32(hr))
}

func isCancelledHResult(hr win.HRESULT) bool {
	return uint32(hr) == hresultCancelled
}

type comdlgFilterSpec struct {
	Name *uint16
	Spec *uint16
}

type iFileDialog struct {
	lpVtbl *iFileDialogVtbl
}

type iFileDialogVtbl struct {
	QueryInterface      uintptr
	AddRef              uintptr
	Release             uintptr
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
}

func (d *iFileDialog) Release() uint32 {
	ret, _, _ := syscall.Syscall(d.lpVtbl.Release, 1,
		uintptr(unsafe.Pointer(d)),
		0,
		0)
	return uint32(ret)
}

func (d *iFileDialog) Show(owner win.HWND) win.HRESULT {
	ret, _, _ := syscall.Syscall(d.lpVtbl.Show, 2,
		uintptr(unsafe.Pointer(d)),
		uintptr(owner),
		0)
	return win.HRESULT(ret)
}

func (d *iFileDialog) SetFileTypes(count uint32, filters *comdlgFilterSpec) win.HRESULT {
	ret, _, _ := syscall.Syscall(d.lpVtbl.SetFileTypes, 3,
		uintptr(unsafe.Pointer(d)),
		uintptr(count),
		uintptr(unsafe.Pointer(filters)))
	return win.HRESULT(ret)
}

func (d *iFileDialog) SetFileTypeIndex(index uint32) win.HRESULT {
	ret, _, _ := syscall.Syscall(d.lpVtbl.SetFileTypeIndex, 2,
		uintptr(unsafe.Pointer(d)),
		uintptr(index),
		0)
	return win.HRESULT(ret)
}

func (d *iFileDialog) SetOptions(options uint32) win.HRESULT {
	ret, _, _ := syscall.Syscall(d.lpVtbl.SetOptions, 2,
		uintptr(unsafe.Pointer(d)),
		uintptr(options),
		0)
	return win.HRESULT(ret)
}

func (d *iFileDialog) SetFolder(folder *iShellItem) win.HRESULT {
	ret, _, _ := syscall.Syscall(d.lpVtbl.SetFolder, 2,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(folder)),
		0)
	return win.HRESULT(ret)
}

func (d *iFileDialog) SetFileName(name *uint16) win.HRESULT {
	ret, _, _ := syscall.Syscall(d.lpVtbl.SetFileName, 2,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(name)),
		0)
	return win.HRESULT(ret)
}

func (d *iFileDialog) SetTitle(title *uint16) win.HRESULT {
	ret, _, _ := syscall.Syscall(d.lpVtbl.SetTitle, 2,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(title)),
		0)
	return win.HRESULT(ret)
}

func (d *iFileDialog) GetResult(item **iShellItem) win.HRESULT {
	ret, _, _ := syscall.Syscall(d.lpVtbl.GetResult, 2,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(item)),
		0)
	return win.HRESULT(ret)
}

func (d *iFileDialog) SetDefaultExtension(extension *uint16) win.HRESULT {
	ret, _, _ := syscall.Syscall(d.lpVtbl.SetDefaultExtension, 2,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(extension)),
		0)
	return win.HRESULT(ret)
}

type iShellItem struct {
	lpVtbl *iShellItemVtbl
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

func (s *iShellItem) Release() uint32 {
	ret, _, _ := syscall.Syscall(s.lpVtbl.Release, 1,
		uintptr(unsafe.Pointer(s)),
		0,
		0)
	return uint32(ret)
}

func (s *iShellItem) GetDisplayName(sigdn uint32, path **uint16) win.HRESULT {
	ret, _, _ := syscall.Syscall(s.lpVtbl.GetDisplayName, 3,
		uintptr(unsafe.Pointer(s)),
		uintptr(sigdn),
		uintptr(unsafe.Pointer(path)))
	return win.HRESULT(ret)
}

func shCreateItemFromParsingName(path *uint16, bindCtx unsafe.Pointer, riid win.REFIID, item *unsafe.Pointer) win.HRESULT {
	ret, _, _ := shell32CreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(bindCtx),
		uintptr(unsafe.Pointer(riid)),
		uintptr(unsafe.Pointer(item)))
	return win.HRESULT(ret)
}
