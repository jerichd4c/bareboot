#ifndef BAREBOOT_EFI_H
#define BAREBOOT_EFI_H

/* Minimal UEFI definitions (UEFI Specification 2.x), only what bareboot uses. */

typedef unsigned char      UINT8;
typedef unsigned short     UINT16;
typedef unsigned int       UINT32;
typedef unsigned long long UINT64;
typedef UINT64             UINTN;      /* native width on x86_64 */
typedef UINT8              BOOLEAN;
typedef UINT16             CHAR16;     /* UEFI strings are UTF-16 */
typedef void              *EFI_HANDLE;
typedef void              *EFI_EVENT;  /* opaque event handle */
typedef UINTN              EFI_STATUS;

/* UEFI uses the Microsoft x64 calling convention */
#define EFIAPI __attribute__((ms_abi))
#define EFI_SUCCESS 0

/* Text colors: foreground 0x0-0xF, background 0x0-0x7 */
#define EFI_BLACK        0x0
#define EFI_BLUE         0x1
#define EFI_GREEN        0x2
#define EFI_CYAN         0x3
#define EFI_RED          0x4
#define EFI_MAGENTA      0x5
#define EFI_BROWN        0x6
#define EFI_LIGHTGRAY    0x7
#define EFI_DARKGRAY     0x8
#define EFI_LIGHTBLUE    0x9
#define EFI_LIGHTGREEN   0xA
#define EFI_LIGHTCYAN    0xB
#define EFI_LIGHTRED     0xC
#define EFI_LIGHTMAGENTA 0xD
#define EFI_YELLOW       0xE
#define EFI_WHITE        0xF
#define EFI_TEXT_ATTR(fg, bg) ((fg) | ((bg) << 4))

/* Common header at the start of every UEFI table */
typedef struct {
    UINT64 Signature;
    UINT32 Revision;
    UINT32 HeaderSize;
    UINT32 CRC32;
    UINT32 Reserved;
} EFI_TABLE_HEADER;

/* ---------- Text output (screen) ---------- */

typedef struct EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL;

typedef EFI_STATUS (EFIAPI *EFI_TEXT_RESET)(EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *This, BOOLEAN ExtendedVerification);
typedef EFI_STATUS (EFIAPI *EFI_TEXT_STRING)(EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *This, CHAR16 *String);
typedef EFI_STATUS (EFIAPI *EFI_TEXT_SET_ATTRIBUTE)(EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *This, UINTN Attribute);
typedef EFI_STATUS (EFIAPI *EFI_TEXT_CLEAR_SCREEN)(EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *This);
typedef EFI_STATUS (EFIAPI *EFI_TEXT_ENABLE_CURSOR)(EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *This, BOOLEAN Visible);

/* Field order must match the spec exactly; unused members stay void* */
struct EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL {
    EFI_TEXT_RESET         Reset;
    EFI_TEXT_STRING        OutputString;
    void                  *TestString;
    void                  *QueryMode;
    void                  *SetMode;
    EFI_TEXT_SET_ATTRIBUTE SetAttribute;
    EFI_TEXT_CLEAR_SCREEN  ClearScreen;
    void                  *SetCursorPosition;
    EFI_TEXT_ENABLE_CURSOR EnableCursor;
    void                  *Mode;
};

/* ---------- Text input (keyboard) ---------- */

typedef struct {
    UINT16 ScanCode;     /* special keys (arrows, F1...), 0 for normal keys */
    CHAR16 UnicodeChar;  /* printable character, 0 for special keys */
} EFI_INPUT_KEY;

typedef struct EFI_SIMPLE_TEXT_INPUT_PROTOCOL EFI_SIMPLE_TEXT_INPUT_PROTOCOL;

typedef EFI_STATUS (EFIAPI *EFI_INPUT_RESET)(EFI_SIMPLE_TEXT_INPUT_PROTOCOL *This, BOOLEAN ExtendedVerification);
typedef EFI_STATUS (EFIAPI *EFI_INPUT_READ_KEY)(EFI_SIMPLE_TEXT_INPUT_PROTOCOL *This, EFI_INPUT_KEY *Key);

struct EFI_SIMPLE_TEXT_INPUT_PROTOCOL {
    EFI_INPUT_RESET    Reset;
    EFI_INPUT_READ_KEY ReadKeyStroke;
    EFI_EVENT          WaitForKey;   /* signaled when a key is available */
};

/* ---------- Boot Services ---------- */

typedef EFI_STATUS (EFIAPI *EFI_WAIT_FOR_EVENT)(UINTN NumberOfEvents, EFI_EVENT *Event, UINTN *Index);
typedef EFI_STATUS (EFIAPI *EFI_SET_WATCHDOG_TIMER)(UINTN Timeout, UINT64 WatchdogCode, UINTN DataSize, CHAR16 *WatchdogData);

/* Only declared up to SetWatchdogTimer; later members are not used */
typedef struct {
    EFI_TABLE_HEADER       Hdr;
    /* Task priority */
    void                  *RaiseTPL;
    void                  *RestoreTPL;
    /* Memory */
    void                  *AllocatePages;
    void                  *FreePages;
    void                  *GetMemoryMap;
    void                  *AllocatePool;
    void                  *FreePool;
    /* Events and timers */
    void                  *CreateEvent;
    void                  *SetTimer;
    EFI_WAIT_FOR_EVENT     WaitForEvent;
    void                  *SignalEvent;
    void                  *CloseEvent;
    void                  *CheckEvent;
    /* Protocol handlers */
    void                  *InstallProtocolInterface;
    void                  *ReinstallProtocolInterface;
    void                  *UninstallProtocolInterface;
    void                  *HandleProtocol;
    void                  *Reserved;
    void                  *RegisterProtocolNotify;
    void                  *LocateHandle;
    void                  *LocateDevicePath;
    void                  *InstallConfigurationTable;
    /* Images */
    void                  *LoadImage;
    void                  *StartImage;
    void                  *Exit;
    void                  *UnloadImage;
    void                  *ExitBootServices;
    /* Miscellaneous */
    void                  *GetNextMonotonicCount;
    void                  *Stall;
    EFI_SET_WATCHDOG_TIMER SetWatchdogTimer;
} EFI_BOOT_SERVICES;

/* ---------- System Table: entry point to every firmware service ---------- */

typedef struct {
    EFI_TABLE_HEADER                 Hdr;
    CHAR16                          *FirmwareVendor;
    UINT32                           FirmwareRevision;
    EFI_HANDLE                       ConsoleInHandle;
    EFI_SIMPLE_TEXT_INPUT_PROTOCOL  *ConIn;
    EFI_HANDLE                       ConsoleOutHandle;
    EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *ConOut;
    EFI_HANDLE                       StandardErrorHandle;
    EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *StdErr;
    void                            *RuntimeServices;
    EFI_BOOT_SERVICES               *BootServices;
    UINTN                            NumberOfTableEntries;
    void                            *ConfigurationTable;
} EFI_SYSTEM_TABLE;

#endif