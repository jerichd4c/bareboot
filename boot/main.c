#include "efi.h"

/* Screen colors used by the banner */
#define COLOR_TEXT   EFI_TEXT_ATTR(EFI_WHITE,      EFI_BLUE)
#define COLOR_FRAME  EFI_TEXT_ATTR(EFI_LIGHTCYAN,  EFI_BLUE)
#define COLOR_TITLE  EFI_TEXT_ATTR(EFI_YELLOW,     EFI_BLUE)
#define COLOR_LABEL  EFI_TEXT_ATTR(EFI_LIGHTGREEN, EFI_BLUE)
#define COLOR_HINT   EFI_TEXT_ATTR(EFI_LIGHTGRAY,  EFI_BLUE)

static EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *out;

/* Prints a string with the given color attribute */
static void print(CHAR16 *text, UINTN attr) {
    out->SetAttribute(out, attr);
    out->OutputString(out, text);
}

/* Blocks until a key is pressed and discards it */
static void wait_for_key(EFI_SYSTEM_TABLE *st) {
    EFI_INPUT_KEY key;
    UINTN index;

    st->ConIn->Reset(st->ConIn, 0);  /* drop keys pressed before this point */
    st->BootServices->WaitForEvent(1, &st->ConIn->WaitForKey, &index);
    st->ConIn->ReadKeyStroke(st->ConIn, &key);
}

/* Entry point called by the firmware after loading BOOTX64.EFI */
EFI_STATUS EFIAPI efi_main(EFI_HANDLE ImageHandle, EFI_SYSTEM_TABLE *SystemTable) {
    (void)ImageHandle;
    out = SystemTable->ConOut;

    /* The boot manager arms a 5-minute watchdog before starting us;
       if it expires the firmware resets the machine. Timeout 0 disables it. */
    SystemTable->BootServices->SetWatchdogTimer(0, 0, 0, 0);

    out->SetAttribute(out, COLOR_TEXT);
    out->ClearScreen(out);       /* fills the whole screen with the background color */
    out->EnableCursor(out, 0);   /* hide the cursor (some firmwares don't support it) */

    print(u"\r\n", COLOR_TEXT);
    print(u"  ====================================================\r\n", COLOR_FRAME);
    print(u"                   B A R E B O O T\r\n",                     COLOR_TITLE);
    print(u"  ====================================================\r\n\r\n", COLOR_FRAME);

    print(u"  Programa de arranque propio, sin sistema operativo.\r\n", COLOR_TEXT);
    print(u"  Universidad Rafael Urdaneta - Sistemas Operativos\r\n\r\n", COLOR_TEXT);

    print(u"  Integrantes:\r\n",          COLOR_LABEL);
    print(u"    - Nombre Apellido 1\r\n", COLOR_TEXT);
    print(u"    - Nombre Apellido 2\r\n\r\n", COLOR_TEXT);

    print(u"  Firmware: ", COLOR_LABEL);
    print(SystemTable->FirmwareVendor, COLOR_TEXT);
    print(u"\r\n\r\n", COLOR_TEXT);

    print(u"  Presione cualquier tecla para continuar el arranque...", COLOR_HINT);
    wait_for_key(SystemTable);

    /* Restore the default look before handing control back */
    out->SetAttribute(out, EFI_TEXT_ATTR(EFI_LIGHTGRAY, EFI_BLACK));
    out->ClearScreen(out);
    out->EnableCursor(out, 1);

    /* Returning gives control back to the boot manager, which tries the next
       boot option (e.g. the installed OS). No disk is read or written. */
    return EFI_SUCCESS;
}