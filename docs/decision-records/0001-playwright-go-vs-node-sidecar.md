# ADR 0001 — Playwright Go single-binary thay vì Node sidecar + gRPC

- **Status**: Accepted (May 2026)
- **Deciders**: Team meet-bot-go
- **Supersedes**: [docs/implementation-plan.md](../../../docs/implementation-plan.md) Phase 1 (Browser-driver Node sidecar MVP)

## Bối cảnh

Bản TypeScript gốc của `meet-teams-bot` chứa khoảng **3000 dòng selector + edge-case logic** cho Google Meet trong `src/meeting/meet/*` và phụ thuộc vào Playwright (Node). Khi chuyển backend sang Go, có 2 hướng tiếp cận chính:

### Phương án A — Node sidecar + gRPC (đề xuất ban đầu)

- 1 pod chạy 2 container: `bot-worker` (Go) ↔ `browser-driver` (Node + Playwright).
- `browser-driver` expose gRPC trên `localhost:9090`, **copy nguyên** `src/meeting/meet/*` từ TS, chỉ thay entrypoint thành `grpc.Server`.
- `bot-worker` (Go) là client gRPC, gọi `OpenMeetingPage`, `JoinMeeting`, `ObserveSpeakers` (server-stream), `StreamMixedAudio` (server-stream), v.v.

### Phương án B — Playwright Go single-binary

- 1 binary Go chạy [Playwright Go](https://github.com/playwright-community/playwright-go) trực tiếp (Playwright Go là wrapper trên cùng Playwright Node + browser binaries, expose API tương tự qua node child-process).
- Port toàn bộ Meet automation sang Go: selectors, MutationObserver inject, Web Audio binding.

## Quyết định

**Chọn phương án B (Playwright Go single-binary)**.

Codebase thực tế tại thời điểm ADR đã đi theo hướng này:
- [internal/infra/browser/playwright_driver.go](../../internal/infra/browser/playwright_driver.go) — `LaunchPersistentContext`, `NewPage`, `Close`.
- [internal/infra/meeting/meet/](../../internal/infra/meeting/meet) — port URL parser, state detector, open page, join, close, html cleaner.

Việc rewrite docs là **xác nhận hậu kỳ** thực tế thay vì revert lại plan ban đầu.

## Lý do chọn B (so với A)

| Tiêu chí | A — Sidecar Node + gRPC | B — Playwright Go |
|---|---|---|
| Reuse logic TS | Tốt nhất (copy nguyên) | Phải port — tốn công 1 lần |
| Dev complexity | 2 ngôn ngữ, 2 build chain, gRPC contract | 1 ngôn ngữ Go thuần |
| Container footprint | 2 image, 2 process, 2 lần Chromium dep | 1 image, 1 process |
| Latency Go ↔ Browser | gRPC localhost (~200μs/RPC) | trực tiếp playwright protocol |
| Streaming throughput (audio) | Gấp 1 hop gRPC | Native channel Go |
| CI matrix | Go + Node | chỉ Go |
| Deploy / debug | Khó hơn (logs 2 nơi) | Dễ (1 process) |
| Risk khi Meet đổi selector | Sửa TS, deploy lại sidecar | Sửa Go, deploy lại bot-worker |

Yếu tố quyết định:
1. **Đơn giản hoá ops**: 1 binary, 1 image, 1 log stream, 1 healthcheck. Không cần `shareProcessNamespace: true` hay liveness check kép.
2. **Latency thấp**: streaming audio realtime tránh thêm 1 hop gRPC. Resampler 48 kHz → 24 kHz và Float32 → Int16 chạy trong cùng goroutine với webhook delivery.
3. **Single-language stack**: tất cả developer trong team đã quen Go, không cần maintain TS toolchain phụ.
4. **Playwright Go API ổn định**: `playwright-community/playwright-go v0.5700.1` đã có `Page.AddInitScript`, `Page.ExposeFunction`, `Page.Locator`, `Browser.LaunchPersistentContext` — đủ cho 100% use case mà bản TS dùng.

## Hệ quả

### Lợi ích

- Ship nhanh hơn (1 image, không cần proto pipeline `buf` + 2 build).
- Pod tiêu tốn ~30 % ít RAM (loại 1 Node runtime).
- Test E2E đơn giản (chạy `go test` thôi).
- Không có `go/proto/buf.yaml`, `go/proto/buf.gen.yaml`, không có `internal/generated/`.

### Rủi ro & mitigation

| Rủi ro | Mức | Mitigation |
|---|---|---|
| Meet đổi DOM/selector → break Go selectors | **Cao** | Tách selectors ra `selectors.go` (đã làm: `internal/infra/meeting/meet/selectors.go`). Snapshot HTML mỗi join + alert khi `JoinMeeting` fail. Maintainer review weekly. |
| Phải port lại JS payload (audio capture, speakers observer, dialog observer) → ~600 dòng JS-trong-Go | Trung | Giữ JS payload nguyên dạng `const` raw string trong file Go, không cố Go-ify. Test bằng smoke test browserless (mock page).|
| Playwright Go chậm theo dõi upstream Playwright Node | Thấp | Phiên bản hiện dùng `0.5700.1` chỉ tụt ~6 tháng so với Node. API surface ổn định. |
| Race condition giữa goroutine Playwright và state machine | Trung | `MeetingContext` đã có `sync.RWMutex`; mọi mutation qua method `SetEndReason`/`SetError`. Test bằng `go test -race`. |

### Trade-off chấp nhận

- **Mất khả năng "copy nguyên TS Meet code"**: phải port ~3000 dòng. Chấp nhận vì đã port được phần lớn (open page, join, state detector, html cleaner, url parser, close). Còn ~3 module in-page (audio capture, speakers observer, dialog observer) là TODO Phase 2 cuối.
- **Không có gRPC contract làm boundary** giữa Go và browser logic. Boundary giờ là interface `meeting.Provider` Go thuần.

## Triển khai

- ✅ `internal/infra/browser/playwright_driver.go` thay cho `browser-driver/` Node.
- ✅ `internal/infra/meeting/meet/` chứa toàn bộ Meet logic (port từ `src/meeting/meet/*`).
- ❌ Không tạo `proto/buf.yaml`, không build `internal/generated/*`.
- ⏳ Cập nhật [docs/implementation-plan.md](../../../docs/implementation-plan.md): Phase 1 thành `Phase 1' — Playwright Go in-process`.
- ⏳ TODO Phase 2 cuối: port 3 JS payload (audio capture, speakers observer, dialog observer) bằng `Page.AddInitScript` + `Page.ExposeFunction`.

## References

- Playwright Go: https://github.com/playwright-community/playwright-go
- Bản TS gốc Meet automation: [src/meeting/meet/](../../../src/meeting/meet/)
- System design tổng: [docs/system-design.md](../../../docs/system-design.md)
- Implementation plan: [docs/implementation-plan.md](../../../docs/implementation-plan.md)
