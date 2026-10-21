# client_chat_profile

Bounded client overlay for viewing another player's formation card when that
player has no pet equipped.

Original fault (build 0759331, QA round): player 1 opens player 2's profile from
chat, `CSFindPlayerInfo` succeeds, then the original
`UI.UIHeroFormation.InitData(FormationItemSimpleInfo)` unconditionally runs
`newobj UI.UIPet::.ctor(petInfo)`. The server legitimately omits `petInfo` for a
player without a pet, `UIPet::.ctor` immediately calls `PetSimpleInfo::get_petId`,
and the resulting `NullReferenceException` leaves the formation page stuck.

## Change

One instruction only, at `IL_005c` of
`UI.UIHeroFormation::InitData(FormationItemSimpleInfo)`:

```text
before: ldarg.0; ldarg.1; callvirt get_petInfo; newobj UI.UIPet::.ctor(PetSimpleInfo); stfld pet
after:  ldarg.0; ldarg.1; callvirt get_petInfo; call HYJS.ChatProfile.ProfilePet::Create(PetSimpleInfo); stfld pet
```

`ProfilePet.Create` returns `null` when `petInfo` is absent and still calls the
original `new UIPet(data)` when it is present. No pet is fabricated, no pet is
equipped, and the server protocol is unchanged. The other two
`UIHeroFormation.InitData` overloads, own-formation data, attribute/combat
calculation and every `Battle.Engine` body are untouched. The original
`UpdatePet` and attribute aggregation already branch on a null `formation.pet`,
so a null pet is an existing supported state.

## Files

- `ProfilePet.cs` — new `SoulFlowerChatProfile.dll` (`HYJS.ChatProfile.ProfilePet.Create`).
- `Injector.cs` — dnlib binder. It resolves the single `InitData` overload, validates the
  `ldarg.0; ldarg.1; callvirt get_petInfo; newobj UIPet; stfld pet` shape, rewrites that
  one instruction and adds no original type, field or method.
- `build.py` — copies the main DLL from the pinned APK, compiles helper + injector,
  injects, then verifies the overlay invariants. Writes `Assembly-CSharp.dll`,
  `SoulFlowerChatProfile.dll`, `manifest.json` and the build logs.

## Usage

`--base-apk`, `--base-sha256` and `--output-directory` are all required; the
output directory must be new and inside the project (the manifest stores
`package_native` payload paths relative to the project root).

```bash
# QA transport-only package
.analysis-venv/bin/python tools/client_chat_profile/build.py \
  --base-apk build/hyjs-b33-local-qa.apk \
  --base-sha256 1d5b7352d415795cf4e0c99c275e9c29d2fdaa67cfd0249c3a8d5b49b091cde7 \
  --output-directory analysis/feedback33-20261021/client/overlay-chat-profile-qa

# original baseline
.analysis-venv/bin/python tools/client_chat_profile/build.py \
  --base-apk build/hyjs-b31-r5.apk \
  --base-sha256 5b0d177ad7823d13c852d93c00cb5dd4ffee5b78b9b97a6a5a6b47d80ba96278 \
  --output-directory build/overlay-chat-profile-b31-r5
```

Packaging stays with Root and reuses the same input APK:

```bash
.analysis-venv/bin/python tools/client_release/package_native.py \
  --base-apk build/hyjs-b31-r5.apk \
  --overlay <output-directory> \
  --output <new apk> --record <new record>
```

`package_native.py` re-checks `baseAPKSHA256`, `inputManagedSHA256`,
`inputDexSHA256`, the source pins, `payloadFiles`, `replacementEntries` and
`allowedChangedMethods`, and copies every inherited Java/Dex/native entry
unchanged.

## Pinned input content

The whole-APK SHA differs between the QA transport package and the baseline
because four HTTP addresses differ, so the APK digest is attested by the caller
while the patch target is pinned by content:

- `UI.UIHeroFormation::InitData` ABI: `2001011281c4`
  (instance, one `ProtoBuf.Message.FormationItemSimpleInfo` parameter).
- Raw target body SHA-256:
  `a5a35cadd296a3e41e3ffa274f8b2aa47c7f05af37034599ab2cd0498b379fe4`
  (identical in both inputs above, and in `build/hyjs-b31-r2.apk` /
  `build/hyjs-opening-fonts-test.apk`).
- Toolchain: `.tools/native-batch/dotnet/dotnet`, its bundled Roslyn `csc.dll`,
  and `.tools/native-batch/injector/bin/Debug/net8.0/dnlib.dll`. Nothing is
  downloaded or upgraded.

## Verification done (static, no device)

Running the tool on both `build/hyjs-b33-local-qa.apk` and
`build/hyjs-b31-r5.apk` produced a `STATIC_VERIFIED_NOT_PACKAGED` manifest that
asserts, on the actual output:

- exactly one original method body changed
  (`UI.UIHeroFormation::InitData`); 26,945 other raw bodies and 547
  `Battle.Engine` bodies are byte-identical;
- within the target body every byte change is confined to the five bytes of the
  rewritten instruction (four of them differ in value); all 116 instruction
  offsets, operand tokens, branch offsets, `MaxStack` (3), code size (352) and the
  single exception handler are preserved;
- the new operand is a `MemberRef` to `HYJS.ChatProfile.ProfilePet::Create` in
  `SoulFlowerChatProfile`, signature `UIPet(PetSimpleInfo)`;
- `TypeDef`/`Field`/`MethodDef`/`Param`/`Property`/`Event`/`TypeSpec`/`MethodSpec`/
  `ModuleRef` counts, type/field/method ABI and assembly identity are unchanged;
  only one `TypeRef`, one `MemberRef` and one `AssemblyRef` (flags 0, empty
  PublicKeyToken) are added;
- the helper targets `v2.0.50727`, references only `Assembly-CSharp` and
  `mscorlib`, defines only `HYJS.ChatProfile.ProfilePet::Create`, and its IL
  contains one null test, one `ldnull` result and exactly one `newobj`
  (`UI.UIPet::.ctor`).

The same patched target body SHA-256
(`3d2f9530235537b96c60ed0d66da91037467f6d14f82491e54c1dd46ffc9cce5`) and the
same helper SHA-256
(`3ff2569d91a34aab5f66c9092eb311bc8ad1b6d2801b6dc73b8dcd994879df91`) came out
of both input APKs, so the patch is independent of the four URL differences.

Negative checks: a wrong `--base-sha256` stops before creating the output
directory, and an existing output directory is refused instead of overwritten.

## Not executed here

- No APK packaging, signing, install or launch; no phone or emulator action.
- No gameplay/UI check of the fixed formation card, and no check that an equipped
  pet still renders on device. Those stay with Root, together with the six chat
  buttons on the real device.
- No server, resource or configuration change; no full Go/frontend build.
