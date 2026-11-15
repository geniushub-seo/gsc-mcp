# Contributing / 參與貢獻

## Before you start / 開始之前

Open an issue first for anything larger than a typo or a small fix, so we can agree on the approach.
比錯字或小修正大的改動，請先開 issue 討論做法。

## Build and check / 建置與檢查

```bash
go build ./...
make check   # go vet + go test ./... + golangci-lint run
```

`golangci-lint` v2 is used (see `.golangci.yml`). Run `gofmt -l .` and make sure it prints nothing.
本專案用 `golangci-lint` v2（見 `.golangci.yml`）。請跑 `gofmt -l .`，確認沒有輸出。

## Rules that CI cannot fully enforce / CI 擋不住的規則

- **stdout carries MCP JSON-RPC only.** Never use `fmt.Print*`, `println`, or write to `os.Stdout`; log with `slog` to stderr.
  **stdout 只能有 MCP JSON-RPC。** 不要用 `fmt.Print*`、`println` 或寫入 `os.Stdout`；日誌用 `slog` 寫到 stderr。
- **No credentials or customer data in commits, tests, or fixtures.** Use `example.com` for domains and fake keys in tests.
  **commit、測試與 fixture 不得含憑證或客戶資料。** 網域用 `example.com`，測試用假 key。
- **Verify API facts before writing them down.** Use `go doc google.golang.org/api/searchconsole/v1 <Type>` or Google's official documentation, not memory.
  **API 事實先查證再寫。** 用 `go doc google.golang.org/api/searchconsole/v1 <Type>` 或 Google 官方文件，不要憑記憶。
- New tool parameters need validation in the handler and a test that proves the parameter changes behaviour.
  新增的 tool 參數要在 handler 驗證，並附一條測試證明該參數真的改變行為。

## Pull requests / Pull request

One logical change per PR. Describe what changed and which of the checks above you ran.
一個 PR 一件事。說明改了什麼，以及上面哪些檢查你有實際跑過。
