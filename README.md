# 社畜小精靈

先從系統匣或選單列結束舊版小精靈和舊版提醒清單。解壓縮後，Windows 開啟 ClockSprite.exe，macOS 開啟 ClockSprite.app。既有設定、待辦、完成紀錄、打卡確認及日曆快取使用原有資料位置。

右鍵點桌面上的小精靈，選「設定」。Windows 也可右鍵點右下角系統匣圖示，Mac 可點右上角選單列圖示。所有設定都在同一個桌面視窗。

- 打卡與語錄：預設上班 08:30、下班 18:00，語錄每 60 分鐘隨機顯示一句，收錄原有語錄、牛馬語錄及幹話語錄，各 100 句。勾選「強制提醒上下班打卡」會主動跳出打卡提醒，取消後只列在待辦。辦公日曆每日台灣時間 09:00 自動更新。當天 09:00 後啟動，若日曆尚未成功更新，會立即補同步。
- 指定日期：輸入提醒名稱、用「選日期」開啟月曆，選取日期後輸入時間，按「儲存提醒」。
- 每週固定：勾選「每週固定」，選擇星期一到日中的任意幾天，輸入時間後儲存。
- 清空只重設表單，已儲存的提醒會留在清單。
- 勾選「登入電腦時自動啟動」即可啟用。將程式移動到其他資料夾後，請重新勾選。
- 到點時，小精靈的對話泡泡會顯示提醒。按泡泡上的「確認」即可完成該筆，多筆提醒會逐筆顯示。「待辦與排程」顯示排程、待確認打卡和待完成提醒。「完成紀錄」只顯示已完成提醒，可以在該頁取消完成或清空完成紀錄。
- 蜜袋鼯會在桌面循環播放打字動畫。拖住小精靈右下角把手可以縮放，尺寸會記住。拖曳角色或雲朵可以移動。設定中的「角色圖示」可以切換動畫蜜袋鼯、動畫狐狸，或匯入 PNG／GIF。隱藏角色後，提醒繼續執行。

資料根目錄：Windows 為 %APPDATA%\ClockSprite，macOS 為 ~/Library/Application Support/ClockSprite。
- Settings/settings.json 儲存打卡時間、語錄間隔和隱藏設定。
- Settings/reminders.json 儲存自訂提醒排程。
- Logs/clock.json 儲存待確認打卡和已確認紀錄。
- Logs/reminders.json 儲存待完成提醒與完成紀錄。
- Cache/calendar.json 儲存辦公日曆資料和最近更新日期。
- 首次啟動新版會自動讀取舊設定和紀錄，舊檔會保留。

Windows 版本支援 64 位元 x86。macOS 版本支援 macOS 12 以上的 Intel 與 Apple Silicon，未經 Apple 公證。

## 從原始碼打包

需要 Go 1.24 以上與 Python 3。在 PowerShell 執行下列指令，會產生 Windows x64 和 macOS Universal 兩個 ZIP。

```powershell
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'
go build -trimpath -ldflags '-H=windowsgui -s -w' -o ClockSprite.exe .
$env:GOOS = 'darwin'; $env:GOARCH = 'amd64'
go build -trimpath -o Unified-amd64 .
$env:GOARCH = 'arm64'
go build -trimpath -o Unified-arm64 .
python package.py
```

辦公日曆資料來源為[政府行政機關辦公日曆表](https://data.gov.tw/dataset/14718)。

## 授權

程式碼與文件採 [MIT 授權](LICENSE)。角色圖片與動畫、語錄及辦公日曆資料的授權各自適用，程式碼的 MIT 授權不涵蓋這些內容。


## 時段內重複提醒

在小精靈的「設定 → 自訂提醒」選擇指定日期或每週固定星期。「時間」是第一次提醒的時間。勾選「時段內重複」後，填寫結束時間和間隔分鐘，再按「儲存提醒」。

例如設定週一到週五、09:00 到 17:00、每隔 90 分鐘，會在 09:00、10:30、12:00、13:30、15:00、16:30 各提醒一次。若某次恰好落在結束時間，也會提醒。時段須在同一天，間隔以整數分鐘設定。

每個排定時間可獨立確認。前一次尚未確認時，後續到期的提醒會加入待辦。若在時段中途新建排程，會從下一個排定時間開始。

## 自訂幹話語錄

右鍵點小精靈，開啟「設定」後按「幹話語錄」。編輯區每行一句，新增、修改或刪除後按「儲存」。儲存後，清單會與其他內建語錄一起隨機抽選。

- 「匯入文字檔」讀取 UTF-8 的 TXT 檔，把新句子追加到編輯區。空行與重複句子會略過。按「儲存」才會套用。
- 「匯出文字檔」把編輯區目前的清單存成 UTF-8 TXT，包含尚未儲存的修改。
- 已儲存的清單位於資料根目錄的 `Settings/nonsense-quotes.json`。

## 下載與發行

Windows x64 和 macOS Universal 的 ZIP 可從 [GitHub Releases](https://github.com/sunlin-lin/ClockSprite/releases) 下載。發行流程會在版本號更新合併到 `main`，或推送相符的 `v` 版本標籤後，測試、建置並上傳兩個 ZIP。
