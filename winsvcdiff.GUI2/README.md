# win-svc-diff

Windows 11 amd64 service-state snapshot and auditor implemented against the supplied technical specification.

## Layout

- `main.go` — elevation gate, application state, background operations, and workflow orchestration.
- `internal/wtw/main_api.go` — the complete Win32/SCM/registry/native-GUI boundary.
- `internal/wtw/wtw.go` — Win32-independent snapshot schema, normalization, diff engine, and remediation orchestration.
- `app_manifest.xml` — Common Controls v6, PerMonitorV2 DPI, and highest-available execution manifest.

## Build

The module targets Go 1.26.2+ and `windows/amd64`.

Run `go generate` once to create `app_manifest.syso`, then:

```text
go build -mod=mod -ldflags="-H=windowsgui" -o win-svc-diff.exe .
```

The application performs its explicit `runas` relaunch before creating any GUI or SCM state.
