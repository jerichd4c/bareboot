#include "efi.h"

/* Entry point called by the firmware after loading BOOTX64.EFI */
EFI_STATUS EFIAPI efi_main(EFI_HANDLE ImageHandle, EFI_SYSTEM_TABLE *SystemTable) {
     (void)ImageHandle; /* not used yet */

     EFI_SIMPLE_TEXT_OUTPUT_PROTOCOL *out = SystemTable->ConOut;

     out->ClearScreen(out);
     out->OutputString(out, u"Hello from bareboot!\r\n");
     out->OutputString(out, SystemTable->FirmwareVendor); /* who loaded us */
     out->OutputString(out, u"\r\n");

     /* Never return: returning would hand control back to the firmware
       and it would try the next boot option, hiding our message. */
    for (;;) {
        __asm__ volatile("hlt");
    }
    return EFI_SUCCESS;
}