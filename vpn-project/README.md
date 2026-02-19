# vpn-project

VLESS VPN проект на Go (`sing-box` runtime):

- Linux API/server manager: `cmd/server`
- Windows GUI client: `cmd/gui`
- Docker deployment: `docker-compose.yml`

## Architecture

- `internal/vpnserver/*` - server config/state, client provisioning, HTTP API.
- `internal/vpnclient/*` - API client, runtime orchestration (`sing-box`), QR helpers, SSH remote ops.
- `internal/ssh/*` - SSH execution and host key validation.

## Requirements

- Go `1.24.x` (toolchain pinned in `go.mod`)
- Docker + Docker Compose plugin (для server deployment)
- Для сборки GUI на Windows: C compiler (`gcc` или `zig cc`) для CGO/Fyne

## Local Run

### Server

```bash
go run ./cmd/server
```

Полезные env:

- `API_TOKEN`
- `VLESS_ENDPOINT`
- `VLESS_LISTEN_PORT`
- `VLESS_WS_PATH`
- `VLESS_TLS_CERT_PATH` / `VLESS_TLS_KEY_PATH`

### Windows GUI

```powershell
go run ./cmd/gui
```

Или собрать релизный пакет:

```powershell
.\build\build-windows.ps1
```

## Tests

Быстрый прогон (без GUI/cgo-ограничений):

```bash
go test ./internal/... ./cmd/server
```

Полный прогон:

```bash
go test ./...
```

Примечание: `cmd/gui` требует доступный C compiler (`gcc`/`zig cc`).

## Lint / Static Checks

Автоформатирование:

```bash
gofmt -w ./cmd ./internal
```

Проверки:

```bash
go vet ./internal/... ./cmd/server
./scripts/check-deadcode.sh
```

Vulnerability scan (если `govulncheck` установлен):

```bash
govulncheck ./cmd/server ./internal/...
```

## CI

Workflow: `.github/workflows/ci.yml`

Шаги:

1. `go test ./internal/... ./cmd/server`
2. `go vet ./internal/... ./cmd/server`
3. `gofmt` check
4. `govulncheck` (server/internal)
5. dead code check (`scripts/check-deadcode.sh`)

## Commit Format

Рекомендуется Conventional Commits:

- `cleanup: remove unused wireguard package`
- `refactor: extract runtime pid helpers`
- `perf: cache ...`
- `test: add coverage for ...`
- `docs: update smoke test for vless runtime`

## Release Checklist

1. `go test ./internal/... ./cmd/server`
2. Проверить `docker compose up -d --build` на staging VPS
3. Собрать Windows пакет `build-windows.ps1`
4. Пройти `SMOKE_TEST_WINDOWS_VM.md`
5. Обновить `CHANGELOG.md`

## Rollback

Смотри `ROLLBACK.md` для пошагового отката.

## Performance

Методология профилирования и baseline-замеров: `PERF.md`.
