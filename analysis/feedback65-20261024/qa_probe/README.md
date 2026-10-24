# Temporary QA caller evidence

`NativeBatchRunner-used-77dd92ee.cs` is the exact source used in the phone checks (SHA25677dd92ee74a3af26f1efb023a26b5c0b37fabec16c679d2dfadb289fcc562d0e). Its diagnostic opening-page command lacked native day options and its Wanted page command passed a text tab. Final r4 Wanted verification entered through the actual original frontline Wanted portrait, so those diagnostic commands were not used.

The current `NativeBatchRunner.cs` fixes the Wanted tab to integer1 and removes the incomplete opening-page shortcut. This later diagnostic-source correction is compiled only, without altering the qualified APK or claiming it ran in the earlier phone checks. The probe is never shipped as the formal caller.
