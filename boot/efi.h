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
typedef UINTN              EFI_STATUS;

/* UEFI uses the Microsoft x64 calling convention */
#define EFIAPI __attribute__((ms_abi))
#define EFI_SUCCESS 0

/* Common header at the start of every UEFI table */
typedef struct {
    UINT64 Signature;
    UINT32 Revision;
    UINT32 HeaderSize;
    UINT32 CRC32;
    UINT32 Reserved;
} EFI_TABLE_HEADER;

typedef struct EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL;

typedef EFI_STATUS (EFIAPI *EFI_TEXT_RESET)(EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *This, BOOLEAN ExtendedVerification);
typedef EFI_STATUS (EFIAPI *EFI_TEXT_STRING)(EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *This, CHAR16 *String);
typedef EFI_STATUS (EFIAPI *EFI_TEXT_CLEAR_SCREEN)(EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *This);

/* Text console protocol. Field order must match the spec exactly;
   members we don't use yet are kept as void* placeholders. */
struct EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL {
    EFI_TEXT_RESET        Reset;
    EFI_TEXT_STRING       OutputString;
    void                 *TestString;
    void                 *QueryMode;
    void                 *SetMode;
    void                 *SetAttribute;
    EFI_TEXT_CLEAR_SCREEN ClearScreen;
    void                 *SetCursorPosition;
    void                 *EnableCursor;
    void                 *Mode;
};

/* Entry point to every firmware service */
typedef struct {
    EFI_TABLE_HEADER                 Hdr;
    CHAR16                          *FirmwareVendor;
    UINT32                           FirmwareRevision;
    EFI_HANDLE                       ConsoleInHandle;
    void                            *ConIn;
    EFI_HANDLE                       ConsoleOutHandle;
    EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *ConOut;
    EFI_HANDLE                       StandardErrorHandle;
    EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *StdErr;
    void                            *RuntimeServices;
    void                            *BootServices;
    UINTN                            NumberOfTableEntries;
    void                            *ConfigurationTable;
} EFI_SYSTEM_TABLE;

#endif