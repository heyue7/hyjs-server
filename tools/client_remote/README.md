# Remote client overlay

The API origin comes from `--origin`, the `HYJS_API_ORIGIN` process environment,
or the private `tools/client_remote/.env` file, in that priority order. Copy
`.env.example` to `.env` and fill in your deployment value before building.
Only HTTP(S) origins are accepted; credentials, paths, query strings and fragments
are rejected. Actual deployment values stay outside Git. Both build and packaging
must use the same origin; the manifest and wallet payload are checked against it.
Fixed qualified input: current `build/hyjs-local.apk`, SHA
`d1ec347effaa84356133b7a81532fc79e79031983cbff6ce59e095ddb31fe3cb`.

The overlay changes one `ldstr` operand in each of:

- `ServerConfig.GetServerUrl`: remote origin plus `/sao/`.
- `Battle.FieldPVEWorldBossBattleOver.End`: settlement HTTP bridge.
- `Room.RoomNetField.SCCurrentPosition`: original scene HTTP bridge.
- `VoicePlugin.InitGVoice`: current local voice HTTP bridge.

It also replaces only `origin` in `assets/local-wallet.json`. Registration and
customer feedback already receive the origin from `ServerConfig`; wallet HTTP
requests read this JSON. All three DEX files stay byte exact, including encrypted
remember-password / auto-login and modern IME. The original 547 engine methods,
all other method bodies, metadata inventories, branding and resource entries
remain unchanged. No managed types, methods or fields are added.

The TCP gateway is supplied by original server responses, with no TCP port in
this patch. `SCGetServerList.serverIp` must advertise the public host and HTTP
port. `SCUserLogin.ip/port` and `SCRefreshGatewayIPAndPort.ip/port` must advertise
the public TCP host/port. Original login appends `/sao/` to `serverIp`.
`SCNoticeUrl.noticeUrl` must also be public. Deployment owns these values.

Build only after the coordinator freezes the Git baseline and grants the client
compile window. Use a new output directory and record path:

```sh
rtk proxy .analysis-venv/bin/python tools/client_remote/build.py --output-directory analysis/client-remote-20261018/overlay-r1
rtk proxy .analysis-venv/bin/python tools/client_remote/package.py --overlay analysis/client-remote-20261018/overlay-r1 --record analysis/client-remote-20261018/apk-build-r1.json
```

Packaging uses the existing signing identity, preserves unrelated ZIP compressed
payloads and only writes `build/hyjs-remote-candidate.apk`. It never installs or
promotes current/rollback. Once the remote deployment is ready, install without
clearing data and validate the actual remote connection on the dedicated account;
do not use protected accounts. The coordinator owns final promotion.

Inputs excluded from Git include signing material, credentials, phone state,
runtime caches, original/assembled APKs and generated analysis/build artifacts.
The project already supplies shared packager helpers and the local .NET/dnlib
runtime; this tool does not acquire or install another toolchain.
