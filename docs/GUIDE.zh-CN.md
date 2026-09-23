# 分步指南：下载你的全部 Costco 购物记录

本工具会把你 Costco 账户里的所有网上订单和所有仓库店收据（包括每一行商品明细）下载到本地 JSON 文件。

**你只需要做一件事：在弹出的浏览器窗口里登录 Costco。** 其余的——获取令牌、保存、下载、断点续传——全部自动完成。登录一次大约可以用 90 天。

## 开始之前

- **Go 1.24 或更高版本**：从 [go.dev/dl](https://go.dev/dl/) 安装，装好后运行 `go version` 确认。
- **git**
- **Chrome、Edge、Chromium 或 Brave 浏览器**中的任意一个（Windows 自带的 Edge 就可以）。

> 下文命令以 macOS / Linux 为准。Windows 用户请把 `./costco-cli` 换成 `.\costco-cli.exe`。

## 第 1 步：下载代码并编译（只需一次）

```bash
git clone https://github.com/ssjfrank/costco-go.git
cd costco-go
git checkout cursor/download-full-order-history-9b26
go build -o costco-cli ./cmd/costco-cli
```

Windows 最后一行改为：`go build -o costco-cli.exe ./cmd/costco-cli`

> 这个分支合并进 `main` 之后，就不需要 `git checkout` 那一行了。

## 第 2 步：运行

```bash
./costco-cli
```

## 第 3 步：在弹出的窗口里登录

程序会自动打开一个浏览器窗口，直接显示 Costco 的登录页面。终端里会提示：

```
You are not signed in to Costco yet, or your last sign-in has expired.
A browser window has opened on Costco's sign-in page. Sign in there with any method you normally use;
the window closes by itself once your order history starts loading.
```

在窗口里像平时一样登录即可：密码、通行密钥（**Sign in with a passkey**）、安全密钥、短信/邮件验证码（**Receive a Passcode**）都可以。

登录完成后，页面会跳回你的订单记录，**窗口随即自动关闭**，终端显示：

```
✓ Signed in. You won't need to sign in again until about 2026-12-22.
```

然后下载自动开始，你什么都不用再做。

> **关于通行密钥**：这个窗口使用的是一个全新的临时浏览器配置，看不到你平时 Chrome 里保存的密码和通行密钥。存在系统里的通行密钥（Mac 的 iCloud 钥匙串、Windows Hello）、实体安全密钥、用手机扫码登录都可以正常使用。如果你的通行密钥只存在 Chrome 的 Google 密码管理器里，请在登录页选择用手机登录，或者改用密码 / 验证码。

## 第 4 步：等待下载完成

程序默认往回查 10 年，把找到的所有记录保存到当前目录下的 `costco-history` 文件夹，并逐段显示进度：

```
Downloading Costco history from 2016-09-23 to 2026-09-23 into costco-history

[1/11] 2025-09-24 to 2026-09-23: online orders
[1/11] 2025-09-24 to 2026-09-23: 34 online order(s)
[1/11] 2025-09-24 to 2026-09-23: warehouse receipts
[1/11] 2025-09-24 to 2026-09-23: 52 receipt(s)
...
```

每张收据都要单独请求一次商品明细，收据多的话需要几分钟。中途可以随时按 **Ctrl + C** 中断，重新运行同一条命令就会从中断的地方继续。

结束时会打印汇总：

```
Downloaded 214 online orders and 388 warehouse receipts (7431 receipt line items)
  Date range:          2016-09-23 to 2026-09-23
  Online order total:  $41203.87
  Receipt total:       $76914.02
  Saved to:            costco-history
  Start here:          costco-history/manifest.json
```

## 第 5 步：查看结果

```
costco-history/
├── manifest.json     汇总和索引，建议从这里看起
├── orders.json       所有网上订单，合在一个文件里
├── receipts.json     所有收据（含商品明细），合在一个文件里
├── orders/           每个网上订单一个文件
└── receipts/         每张收据一个文件
```

`manifest.json` 里有订单数、收据数、商品行数、消费总额，以及每个文件对应的日期和金额。每张收据文件里有仓库店名称和地址、每件商品的数量和价格、税费明细、付款方式、即时折扣和会员号。

## 以后怎么更新

直接再运行一次 `./costco-cli`：

- 已经下载过的收据会自动跳过，只下载新的。
- 登录还有效就不会弹窗；过期了（大约 90 天）会自动再弹出登录窗口，登录后接着下载。
- 只想查最近的：`./costco-cli -since 2026-01-01`
- 全部重新下载：`./costco-cli -force`
- 只登录、不下载：`./costco-cli -cmd login`

## 常见问题

| 情况 | 解决办法 |
| --- | --- |
| 提示 `no Chrome, Edge, Chromium or Brave installation found` | 安装 Chrome 或 Edge；或者用 `-browser` 指定浏览器位置，例如 `./costco-cli -browser "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"`；也可以用文末的手动方式。 |
| 登录后窗口没有自动关闭 | 在那个窗口里点页面顶部的 **Orders & Returns（订单与退货）**。窗口会在订单页开始加载时关闭。 |
| 提示 `... closed before sign-in finished` | 登录完成前关掉了窗口或浏览器。重新运行 `./costco-cli` 即可。 |
| 提示 `timed out ... waiting for sign-in` | 10 分钟内没有完成登录。重新运行即可。 |
| 登录页面报错或拒绝登录 | 先确认在普通浏览器里能正常登录；如果普通浏览器可以、弹出的窗口不行，请用文末的手动方式。 |
| 日期范围被拒绝 | 把每次请求的时间段调小：`./costco-cli -window 90`。 |
| 结束时列出 `could not be downloaded` | 个别记录暂时取不到。重新运行同一条命令，只会补下载这些记录。 |
| 想用定时任务自动运行 | 加 `-no-browser`。这样登录过期时程序会直接报错提示，而不会弹出一个没人处理的窗口。 |

## 安全说明

- **本工具看不到你的密码、通行密钥或安全密钥。** 登录是你自己在浏览器里完成的；程序只读取登录成功后 costco.com 自己收到的那一条令牌响应，而且只接受来自 `signin.costco.com` 令牌接口的响应。
- 登录窗口使用临时浏览器配置，拿到令牌后立即删除，不会留下 Costco 的登录状态，也不会碰你平时用的浏览器配置。
- 除了登录窗口里 costco.com 本身加载的内容，程序只连接 Costco 自己的服务器（`signin.costco.com` 和 `ecom-api.costco.com`），不会把令牌或数据发给任何第三方。
- 令牌保存在 `~/.costco/tokens.json`，下载的数据保存在 `costco-history`，两者都设置为只有你自己的系统账户可以读取。项目的 `.gitignore` 已经排除了 `costco-history/` 和 `token.json`。
- 收据里有会员号、地址和付款信息，**不要**把 `costco-history` 或 `tokens.json` 上传到网上或分享给别人。
- 如果怀疑令牌泄露，先删除 `~/.costco/tokens.json`，再到 costco.com 更新账户安全设置。已经发出的令牌能否立即作废由 Costco 决定，本工具无法控制；最迟大约 90 天后它会自然过期。
- 登录窗口启动时带有 `--disable-blink-features=AutomationControlled` 参数。不加的话，Chrome 会把这个窗口标记为"自动化浏览器"，Costco 的登录页可能因此拒绝登录，尽管登录的其实是你本人。

## 附录：没有可用浏览器时的手动方式

1. 在任意浏览器里按 **F12** 打开开发者工具，切到 **Network（网络）**，勾选 **Preserve log（保留日志）**，过滤框输入 `token`。
2. 登录 costco.com，然后点 **Orders & Returns（订单与退货）**。
3. 找到地址以 `oauth2/v2.0/token` 结尾、方法为 `POST` 的请求，在 **Response（响应）** 标签里全选复制。
4. 把内容保存为 `token.json`，然后运行：

   ```bash
   ./costco-cli -cmd import-token < token.json
   ```

   Windows PowerShell：`Get-Content token.json | .\costco-cli.exe -cmd import-token`

5. 删除 `token.json`，再运行 `./costco-cli` 开始下载。
