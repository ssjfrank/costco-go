# 分步指南：下载你的全部 Costco 购物记录

本工具会把你 Costco 账户里的所有网上订单和所有仓库店收据（包括每一行商品明细）下载到本地 JSON 文件。

整个过程：**在你平时用的浏览器里登录 Costco → 按 F12，在 Console 里粘贴一条命令 → 回到终端按回车。** 其余的全部自动完成。登录一次大约可以用 90 天。

## 开始之前

- 一台 Mac（Apple 芯片 M1 及以后，或 Intel 芯片都可以）。
- 任意一个带开发者工具的浏览器：Chrome、Edge、Firefox、Safari 都可以。

> 下文以 Mac 为准，命令都在"终端"（Terminal）里运行：按 **Cmd + 空格**，输入 `终端` 或 `Terminal` 回车即可打开。

## 第 1 步：下载程序（只需一次）

把下面几行整段复制到终端里，回车：

```bash
mkdir -p ~/costco && cd ~/costco
ARCH=$(uname -m | sed 's/x86_64/amd64/')
curl -fL -o costco-cli "https://github.com/ssjfrank/costco-go/releases/latest/download/costco-cli-darwin-$ARCH"
chmod +x costco-cli
./costco-cli -version
```

看到类似 `costco-cli 1.0.0 (commit ...)` 就说明下载成功。程序和下载的数据都放在你个人目录下的 `costco` 文件夹里（`~/costco`）。

- `uname -m` 会自动判断你的 Mac 是 Apple 芯片（`arm64`）还是 Intel 芯片（`amd64`），下载对应的版本。
- 如果 `curl` 提示 `404`，说明还没有发布正式的 Release。把第三行换成临时下载地址再运行一次：

  ```bash
  curl -fL -o costco-cli "https://raw.githubusercontent.com/ssjfrank/costco-go/refs/heads/cursor/macos-download-9b26/costco-cli-darwin-$ARCH"
  ```

- 如果你是用浏览器下载的文件，而不是上面的 `curl`，Mac 可能会提示"无法打开，因为无法验证开发者"。在终端运行 `xattr -d com.apple.quarantine ~/costco/costco-cli` 即可。用 `curl` 下载不会遇到这个问题。
- 以后每次使用，先运行 `cd ~/costco` 进入这个文件夹。

> **想自己从源代码编译？** 安装 [Go 1.24+](https://go.dev/dl/) 和 git，然后运行：
>
> ```bash
> git clone https://github.com/ssjfrank/costco-go.git && cd costco-go
> git checkout cursor/download-full-order-history-9b26
> go build -o costco-cli ./cmd/costco-cli
> ```
>
> 这个分支合并进 `main` 之后，就不需要 `git checkout` 那一行了。

## 第 2 步：在浏览器里登录 Costco

1. 用你平时的浏览器打开 [costco.com](https://www.costco.com) 并登录。密码、通行密钥、安全密钥、验证码都可以，用你平时的方式就行。
2. 点页面顶部的 **Orders & Returns（订单与退货）**，等订单列表出现。

> 一定要打开 Orders & Returns 这个页面。登录令牌是这个页面加载时才拿到的，在首页运行命令会提示找不到。

## 第 3 步：按 F12，在 Console 里运行命令

1. 在订单页按 **F12**（Mac 上按 **Cmd + Option + J**；Safari 需要先在"设置 → 高级"里勾选"显示网页开发者功能"，再按 **Cmd + Option + C**），切换到 **Console（控制台）** 标签。
2. 第一次往 Console 里粘贴东西时，浏览器会出于安全考虑拦一下，提示你先手动输入一段文字（英文界面是 `allow pasting`，中文界面以浏览器显示的为准）。照着输入并按回车，再粘贴即可。
3. 复制下面这一整行命令，粘贴到 Console 里，按回车：

```js
(async () => { const CLIENT = "a3a5186b-7c89-4b4c-93a8-dd604e930757", TOKEN_URL = "https://signin.costco.com/e0714dd4-784d-46d6-a278-3e29553483eb/b2c_1a_sso_wcs_signup_signin_209/oauth2/v2.0/token"; const toClipboard = typeof copy === "function" ? copy : null; const found = {}; let others = 0; for (const store of [sessionStorage, localStorage]) { for (let i = 0; i < store.length; i++) { let entry; try { entry = JSON.parse(store.getItem(store.key(i))); } catch (e) { continue; } if (!entry || !entry.secret || !/^(IdToken|RefreshToken)$/.test(entry.credentialType)) continue; if (entry.clientId !== CLIENT) { others++; continue; } found[entry.credentialType] = entry.secret; } } if (!found.RefreshToken) { console.error(others ? "This page has a sign-in for a different Costco app. Open Orders & Returns and run this again." : "No Costco sign-in on this page. Sign in, open Orders & Returns, then run this again."); return; } let tokens, response; try { response = await fetch(TOKEN_URL, {method: "POST", body: new URLSearchParams({client_id: CLIENT, grant_type: "refresh_token", refresh_token: found.RefreshToken})}); const text = await response.text(); try { tokens = JSON.parse(text); } catch (e) { tokens = {}; } } catch (e) { if (!found.IdToken) { console.error("Could not reach Costco: " + e); return; } console.warn("Could not reach Costco to refresh the sign-in; using the one on this page."); tokens = {id_token: found.IdToken, refresh_token: found.RefreshToken, refresh_token_expires_in: 7776000}; } if (response && (!response.ok || !tokens.refresh_token)) { console.error("Costco refused the sign-in (" + (tokens.error_description || tokens.error || response.status) + "). Sign out, sign in again and rerun this."); return; } const block = "-----BEGIN COSTCO SIGN-IN-----\n" + btoa(unescape(encodeURIComponent(JSON.stringify(tokens)))).match(/.{1,64}/g).join("\n") + "\n-----END COSTCO SIGN-IN-----"; if (toClipboard) toClipboard(block); console.log(block + "\n\n" + (toClipboard ? "Copied to your clipboard. " : "Copy everything from BEGIN to END. ") + "Now run costco-cli."); return block; })()
```

> 这条命令也可以随时用 `./costco-cli -cmd snippet` 打印出来。

Console 里会打印一段以 `-----BEGIN COSTCO SIGN-IN-----` 开头、`-----END COSTCO SIGN-IN-----` 结尾的内容，最后一行是：

```
Copied to your clipboard. Now run costco-cli.
```

说明你的登录信息**已经自动复制到剪贴板**了，不用自己去选中复制。

## 第 4 步：运行软件

回到终端运行：

```bash
cd ~/costco
./costco-cli
```

程序会自动从剪贴板读取刚才的登录信息：

```
You are not signed in to Costco yet, or your last sign-in has expired.
Found a Costco sign-in in your clipboard.
✓ Signed in. You won't need to sign in again until about 2026-12-22.

Downloading Costco history from 2016-09-24 to 2026-09-24 into costco-history
```

然后自动开始下载。

> **顺序反过来也可以**：先运行 `./costco-cli`，它会显示上面的操作说明和命令，并等你操作；你在浏览器里运行完命令后，回到终端按一下**回车**就行。如果剪贴板用不了，也可以把 Console 打印的那一整段（从 BEGIN 到 END）直接粘贴到终端里。

## 第 5 步：等待下载完成

程序默认往回查 10 年，把找到的所有记录保存到当前目录下的 `costco-history` 文件夹，并逐段显示进度：

```
[1/11] 2025-09-25 to 2026-09-24: online orders
[1/11] 2025-09-25 to 2026-09-24: 34 online order(s)
[1/11] 2025-09-25 to 2026-09-24: warehouse receipts
[1/11] 2025-09-25 to 2026-09-24: 52 receipt(s)
...
```

每张收据都要单独请求一次商品明细，收据多的话需要几分钟。中途可以随时按 **Ctrl + C** 中断，重新运行同一条命令就会从中断的地方继续。

结束时会打印汇总：

```
Downloaded 214 online orders and 388 warehouse receipts (7431 receipt line items)
  Date range:          2016-09-24 to 2026-09-24
  Online order total:  $41203.87
  Receipt total:       $76914.02
  Saved to:            costco-history
  Start here:          costco-history/manifest.json
```

## 第 6 步：查看结果

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

直接再运行一次 `cd ~/costco && ./costco-cli`：

- 已经下载过的收据会自动跳过，只下载新的。
- 登录还有效就直接下载；过期了（大约 90 天）会提示你重做第 2、3 步，然后按回车继续。
- 只想查最近的：`./costco-cli -since 2026-01-01`
- 全部重新下载：`./costco-cli -force`
- 只登录、不下载：`./costco-cli -cmd login`
- 更新程序本身：重新运行第 1 步的下载命令即可，已下载的数据和登录状态都不受影响。

## 在 Docker 容器里运行（可选）

登录信息保存在一个文件里：`~/.costco/tokens.json`。下载只需要这个文件，所以可以在 Mac 上登录一次，然后在容器里运行下载，只把这个文件挂载进去。需要先安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/)。

1. **先在 Mac 上登录一次**，生成登录文件（按上面的第 1～4 步，或者运行 `./costco-cli -cmd login`）。登录成功后会显示文件位置：

   ```
   ✓ Signed in. You won't need to sign in again until about 2026-12-23.
     Saved to /Users/你的用户名/.costco/tokens.json
   ```

   想把文件放在别处，登录时指定路径即可：

   ```bash
   COSTCO_TOKEN_FILE="$HOME/costco/tokens.json" ./costco-cli -cmd login
   ```

2. **构建镜像**（只需一次，直接从 GitHub 构建，不用下载源代码）：

   ```bash
   docker build -t costco-cli "https://github.com/ssjfrank/costco-go.git#cursor/download-full-order-history-9b26"
   ```

3. **运行下载**，把登录文件和输出目录挂载进去：

   ```bash
   mkdir -p "$HOME/costco"
   docker run --rm \
     --user "$(id -u):$(id -g)" \
     --mount type=bind,source="$HOME/.costco/tokens.json",target=/secrets/tokens.json \
     -v "$HOME/costco:/data" \
     costco-cli
   ```

   下载结果在 `~/costco/costco-history`，和直接在 Mac 上运行时一样。需要加参数的话写在最后，例如 `costco-cli -since 2026-01-01`。

注意事项：

- **容器里不能登录。** 容器里没人能回答登录提示，所以登录过期时会直接报错，并告诉你要替换哪个文件，例如 `Replace /secrets/tokens.json with a fresh sign-in`。这时回到 Mac 上运行 `./costco-cli -cmd login` 重新登录，再运行容器即可。
- **挂载登录文件用 `--mount`，不要用 `-v`。** 如果文件还不存在，`-v` 会在你的 Mac 上悄悄创建一个同名**文件夹**，之后登录也会失败，直到你把它删掉；`--mount` 则会直接报错，什么都不创建。如果已经遇到这种情况，程序会提示 `... is a directory, not a token file`，删掉那个文件夹、重新登录即可。
- **登录文件要可写。** 每次刷新令牌时，程序会把新令牌写回这个文件。只读挂载也能用，但刷新后的令牌不会保存，登录的有效期不会因为使用而延长。
- **`--user "$(id -u):$(id -g)"` 保留着就好。** 它让容器用你自己的身份读写文件，下载的文件也归你所有。
- 构建镜像时，`costco-history/`、`token.json`、`.costco/` 都会被排除，你的个人数据不会被打包进镜像。

## 常见问题

| Console 或终端里的提示 | 解决办法 |
| --- | --- |
| `No Costco sign-in on this page` | 没登录，或者不在订单页。登录后打开 **Orders & Returns**，在那个页面重新运行命令。 |
| `This page has a sign-in for a different Costco app` | 你在别的 Costco 页面上运行了命令。打开 **Orders & Returns** 再运行。 |
| `Costco refused the sign-in` | 浏览器里的登录已经失效。在 costco.com 退出再登录，打开订单页重新运行命令。 |
| Console 里粘贴不进去 | 按浏览器的提示先手动输入它要求的文字（英文界面是 `allow pasting`）并回车，再粘贴。 |
| 终端提示 `No Costco sign-in in the clipboard yet` | 剪贴板里没有登录信息（可能复制了别的东西）。重新运行 Console 命令后再按回车；或者把 Console 打印的整段内容粘贴到终端。 |
| 终端提示 `the sign-in is incomplete` 或 `damaged` | 粘贴时漏了内容。要从 `-----BEGIN` 一直复制到 `-----END`，或者直接用剪贴板方式（按回车）。 |
| Linux 上读不到剪贴板 | 安装 `xclip`、`xsel` 或 `wl-clipboard`，或者直接把整段内容粘贴到终端。 |
| 日期范围被拒绝 | 把每次请求的时间段调小：`./costco-cli -window 90`。 |
| 结束时列出 `could not be downloaded` | 个别记录暂时取不到。重新运行同一条命令，只会补下载这些记录。 |
| 想用定时任务自动运行 | 加 `-non-interactive`。这样登录过期时程序会直接报错提示，而不会一直等待输入。 |

## 安全说明

**往 Console 里粘贴代码之前，一定要清楚它在做什么。** 骗子常用"把这段代码粘贴到控制台"的方式盗取账号。只粘贴来自本项目的命令（或 `./costco-cli -cmd snippet` 打印的命令）。这条命令只做三件事：

1. 在当前页面的浏览器存储里找到 Costco 订单页保存的登录令牌；
2. 用它向 Costco 自己的登录服务器（`signin.costco.com`）换一个新令牌，和订单页自己做的完全一样；
3. 把结果打印出来并复制到剪贴板。

它不会把任何东西发给 Costco 以外的地方，你可以在 Console 的 Network（网络）标签里自己确认。

其他注意事项：

- 剪贴板里的登录信息等同于你的 Costco 登录状态。导入后，建议随便复制一段别的文字把它覆盖掉；不要把它发给任何人。
- 令牌保存在 `~/.costco/tokens.json`，下载的数据保存在 `~/costco/costco-history`，两者都设置为只有你自己的系统账户可以读取。如果你是在项目源代码目录里运行的，项目的 `.gitignore` 已经排除了 `costco-history/` 和 `token.json`。
- 下载的程序可以用 Release 里的 `SHA256SUMS` 核对：`shasum -a 256 costco-cli`，结果应与 `SHA256SUMS` 里对应的那一行一致。
- 收据里有会员号、地址和付款信息，**不要**把 `costco-history` 或 `tokens.json` 上传到网上或分享给别人。
- 如果怀疑令牌泄露，先删除 `~/.costco/tokens.json`，再到 costco.com 退出登录并更新账户安全设置。已经发出的令牌能否立即作废由 Costco 决定，本工具无法控制；最迟大约 90 天后它会自然过期。

## 附录：让程序自己打开登录窗口

不想用 Console 的话，可以运行 `./costco-cli -browser-login`。程序会用一个全新的临时配置打开 Chrome / Edge / Chromium / Brave，你在弹出的窗口里登录，窗口会在订单页开始加载时自动关闭。注意：这个临时配置看不到你平时浏览器里保存的密码和通行密钥，如果你的通行密钥只存在 Chrome 的密码管理器里，请用上面的 Console 方式。
