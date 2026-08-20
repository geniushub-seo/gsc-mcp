# 安裝 prompt（直接貼給 agent）

給完全沒有終端機經驗的使用者。複製下方程式碼區塊的**全部**內容，貼給 Claude Code、
Codex、Hermes 或任何能執行終端機指令的 AI agent，它會從頭把 gsc-mcp 裝到可以查
GSC 資料為止。你只需要在它打開的 Google 頁面選帳號、勾同意。

這份 prompt 涵蓋三個真實安裝踩過的坑：macOS 沒有 homebrew 且帳號不是管理員、
gcloud 需要 Python 3.10+ 而 macOS 內建的是 3.9.6、以及登入帳號在 Search Console
沒有任何網站權限時 doctor 仍會印出 OK。

```text
幫我安裝 gsc-mcp（Google Search Console 的本地 MCP server），裝到我能查 GSC 資料為止。

你負責在你的終端機裡執行每一條指令，不要叫我自己開終端機貼指令。我只做一件事：在你打開的
Google 頁面選帳號、勾同意。

步驟是固定的，照順序做，不要跳步也不要自己改順序。

第 1 步：裝 binary
  macOS / Linux:  curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
  Windows:        irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
記下它印出來的絕對安裝路徑，之後每條指令都用那個路徑，不要假設 gsc-mcp 在 PATH 上。

第 2 步：確認 gcloud 可用。先跑 gcloud --version，能跑就跳到第 3 步。不能跑就照下面的
順序處理，不要跳號、不要自己換順序。

  2a. Windows: winget install Google.CloudSDK
      裝完開新的 shell，或用絕對路徑
      %LOCALAPPDATA%\Google\Cloud SDK\google-cloud-sdk\bin\gcloud.cmd
      Windows 到這裡就結束，直接跳第 3 步。以下 2b–2e 只給 macOS / Linux。

  2b. 有 homebrew 嗎？跑 command -v brew
      有  → brew install --cask google-cloud-sdk，然後跳到 2e
      沒有 → 不要嘗試安裝 homebrew。裝它要求我的帳號是這台電腦的管理員，
             很多公司配發的電腦不是，會停在 "Need sudo access on macOS"。往下走 2c。

  2c. 找一個 3.10 以上的 Python。gcloud 是 Python 寫的，macOS 內建的
      /usr/bin/python3 是 3.9.6，太舊；而且 install.sh 自己就是 Python 程式，
      所以「解壓後補跑 install.sh」不能解決，一樣會 TypeError 中止。

      for p in python3.13 python3.12 python3.11 python3.10 "$HOME"/.local/share/uv/python/*/bin/python3; do
        c=$(command -v "$p" 2>/dev/null) || { [ -x "$p" ] && c="$p"; } || continue
        "$c" -c 'import sys;sys.exit(0 if sys.version_info>=(3,10) else 1)' 2>/dev/null && { echo "FOUND: $c"; break; }
      done

  2d. 2c 找不到就自己裝一個。uv 不需要管理員權限：
      curl -LsSf https://astral.sh/uv/install.sh | sh
      "$HOME/.local/bin/uv" python install 3.12
      裝完重跑 2c 拿到路徑。

  2e. 用它安裝 gcloud（Intel Mac 把 darwin-arm 換成 darwin-x86_64；
      走 2b 用 homebrew 裝的人跳過這格）：
      export CLOUDSDK_PYTHON=<2c 找到的路徑>
      tmp=$(mktemp -d)
      curl -fsSL https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-darwin-arm.tar.gz -o "$tmp/gcloud.tar.gz"
      mkdir -p "$HOME/.local/share" && tar -xzf "$tmp/gcloud.tar.gz" -C "$HOME/.local/share"
      "$HOME/.local/share/google-cloud-sdk/install.sh" --quiet --path-update=false

  記下兩件事，第 3 步每一條指令都要帶著：gcloud 的絕對路徑，以及
  export CLOUDSDK_PYTHON=<路徑>（只有走 2c–2e 這條才需要，homebrew 裝的不用）。

第 3 步：依序跑這六條
  gcloud auth login
  gcloud projects list
  gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
  gcloud auth application-default set-quota-project PROJECT_ID
  gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID
  <第 1 步的絕對路徑>/gsc-mcp doctor

PROJECT_ID 由你從第二條的輸出挑一個填進去。不要把 PROJECT_ID 這個字原樣送出，也不要在跑過
gcloud projects list 之前來問我要 project id。只有列表是空的才問我——那代表我還沒有 GCP 專案，
我得先去 https://console.cloud.google.com/projectcreate 開一個。

登入時我會選 Google 帳號。你在第一條之前提醒我一次：要選的是在 Search Console 後台
已經有那些網站權限的帳號，選錯的話後面整套授權要重做一遍。

四件必知的事，每件都害死過一次真實安裝：

1. gcloud auth login 和 gcloud auth application-default login 是兩次獨立登入，兩個都要做。
   只做後者，gcloud projects list 會回報憑證過期。
2. ADC 同意頁有兩個勾選框，兩個都要勾，少勾會失敗在 cloud-platform scope is required
   but not consented。
3. 新的 GCP 專案預設沒有啟用 Search Console API，第五條不能省，跳過會得到一個很像權限問題
   但其實不是的 403，細節裡寫 "reason": "SERVICE_DISABLED"。
4. 如果 set-quota-project 回報缺 serviceusage.services.use，那是我在那個 GCP 專案的權限
   不足。重試同一條沒有用，換一個我是 Owner 的專案，或叫我去要權限。

第 4 步：doctor 要同時滿足兩個條件才算成功——印出 list_sites OK，而且列出的 property
數量大於 0。只有 OK 但 0 個 property 代表認證通了、但我登入的帳號在 GSC 沒有任何網站，
那是選錯帳號，要重跑第 3 步的兩次登入換帳號，不要往下走。

兩個條件都滿足後，跑 <絕對路徑>/gsc-mcp setup 寫入 MCP 設定，重開我的 MCP client
或開新 session，確認工具列表有 gsc，再叫我試 list_sites。

任何一條失敗，先重跑 doctor、讀完輸出再動作。注意：目前 release 版的 doctor 即使失敗也可能
exit 0，不要只看退出碼，要讀 list_sites 那一行和 property 數量。

安全規則：不要 cat 我的憑證檔（確認存在用 ls），不要把任何 token、key 或憑證內容貼進對話或 log。
```
