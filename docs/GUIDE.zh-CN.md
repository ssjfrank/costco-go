# 分步指南：下载你的全部 Costco 购物记录

本工具会把你 Costco 账户里的所有网上订单和所有仓库店收据（包括每一行商品明细）下载到本地 JSON 文件。整个过程只需要做一次浏览器登录，之后大约 90 天内都不用再登录。

## 开始之前

你需要准备：

- **Go 1.24 或更高版本**：从 [go.dev/dl](https://go.dev/dl/) 安装，装好后运行 `go version` 确认。
- **git**
- **Chrome 或 Edge 浏览器**（下文以 Chrome 为例，Edge 操作相同）
- **你的 Costco 账户**。用密码、通行密钥（passkey）、安全密钥还是两步验证登录都可以——本工具从不接触你的登录凭据，只使用登录完成后浏览器拿到的令牌。

> 下文命令以 macOS / Linux 为准。Windows 用户请把 `./costco-cli` 换成 `.\costco-cli.exe`，其他差异会单独注明。

## 第 1 步：下载代码

```bash
git clone https://github.com/ssjfrank/costco-go.git
cd costco-go
git checkout cursor/download-full-order-history-9b26
```

> 这个分支合并进 `main` 之后，就不需要最后一行 `git checkout` 了。

## 第 2 步：编译

```bash
go build -o costco-cli ./cmd/costco-cli
```

Windows：

```powershell
go build -o costco-cli.exe ./cmd/costco-cli
```

运行 `./costco-cli -help`，能看到使用说明就说明编译成功。

## 第 3 步：填写基本信息（只需一次）

```bash
./costco-cli -cmd setup
```

按提示输入：

1. **Email**：你的 Costco 账户邮箱。
2. **Warehouse Number**：你常去的仓库店编号，印在收据上。不确定的话直接回车，使用默认值 `847`。

看到 `✓ Configuration saved` 即完成。

## 第 4 步：从浏览器复制登录令牌

这是唯一需要手动操作的步骤，请按顺序来——**先打开开发者工具，再登录**，否则会错过令牌请求。

1. 打开 Chrome，新建一个标签页。
2. 按 **F12**（macOS 按 **Cmd + Option + I**）打开开发者工具，切换到 **Network（网络）** 面板。
3. 勾选 **Preserve log（保留日志）**。登录过程中页面会跳转，不勾选的话记录会被清空。
4. 在过滤框里输入 `token`。
5. 在这个标签页里打开 [costco.com](https://www.costco.com) 并登录。如果你已经是登录状态，请先退出再重新登录，确保浏览器发出一次新的令牌请求。
6. 在请求列表里找到地址包含 `oauth2/v2.0/token`、方法为 `POST`、状态为 `200` 的请求。可能会有好几条，选择响应内容里**同时包含 `id_token` 和 `refresh_token`** 的那一条。
7. 点击该请求，切换到 **Response（响应）** 标签，在内容里按 **Ctrl + A**（macOS：**Cmd + A**）全选，再复制。

> 注意是 **Response** 标签，不是 Headers 或 Payload。复制出来的内容应该以 `{"id_token":` 之类的字段开头，是一整段 JSON。

## 第 5 步：导入令牌

令牌很长（通常好几千个字符）。直接粘贴进终端可能被截断——macOS 终端对单行输入有大约 1024 字符的限制——所以推荐先存成文件再导入。

1. 用文本编辑器新建一个文件，把复制的内容粘贴进去，保存为项目目录下的 `token.json`。
2. 导入：

   ```bash
   ./costco-cli -cmd import-token < token.json
   ```

   Windows PowerShell 不支持 `<`，请改用：

   ```powershell
   Get-Content token.json | .\costco-cli.exe -cmd import-token
   ```

3. 看到类似下面的输出就成功了：

   ```
   ✓ Tokens saved to ~/.costco/tokens.json
     ID token valid until:      ...
     Refresh token valid until: ...
   ```

4. **立即删除 `token.json`**。它能直接访问你的 Costco 账户，导入后已经没有用了：

   ```bash
   rm token.json
   ```

   Windows：`del token.json`

> 也可以不用文件：运行 `./costco-cli -cmd import-token`，粘贴后按 **Ctrl + D**（Windows 按 **Ctrl + Z** 再回车）。如果提示 `parsing JSON`，多半是粘贴被截断了，改用上面的文件方式。

## 第 6 步：下载全部记录

```bash
./costco-cli
```

默认会往回查 10 年，把找到的所有记录保存到当前目录下的 `costco-history` 文件夹。运行过程中会逐段显示进度：

```
Downloading Costco history from 2016-09-23 to 2026-09-23 into costco-history

[1/11] 2025-09-24 to 2026-09-23: online orders
[1/11] 2025-09-24 to 2026-09-23: 34 online order(s)
[1/11] 2025-09-24 to 2026-09-23: warehouse receipts
[1/11] 2025-09-24 to 2026-09-23: 52 receipt(s)
...
```

每张收据都要单独请求一次商品明细，请求之间还会间隔 250 毫秒，所以收据多的话需要几分钟。中途可以随时按 **Ctrl + C** 中断，已经下载的内容都保存着，重新运行同一条命令会接着下载剩下的部分。

结束时会打印汇总：

```
Downloaded 214 online orders and 388 warehouse receipts (7431 receipt line items)
  Date range:          2016-09-23 to 2026-09-23
  Online order total:  $41203.87
  Receipt total:       $76914.02
  Saved to:            costco-history
  Start here:          costco-history/manifest.json
```

## 第 7 步：查看结果

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

- **补充新记录**：再运行一次 `./costco-cli`。已经下载过的收据会自动跳过，只下载新的。
- **只查最近的**：加 `-since` 可以缩小范围、加快速度，例如 `./costco-cli -since 2026-01-01`。
- **全部重新下载**：加 `-force`。
- **令牌过期（大约 90 天）**：程序会提示 `no valid tokens found`，重做第 4、5 步即可，已下载的数据不受影响。

## 常见问题

| 提示信息 | 原因和解决办法 |
| --- | --- |
| `no valid tokens found` | 还没导入令牌，或令牌已过期。重做第 4、5 步。 |
| `parsing JSON` | 复制的不是 Response 内容，或者粘贴被截断。回到第 4 步重新复制，用文件方式导入。 |
| `refresh_token is missing` | 选错了请求。在第 4 步里找响应中同时有 `id_token` 和 `refresh_token` 的那一条。 |
| 开发者工具里找不到 token 请求 | 确认勾选了 Preserve log、过滤框填的是 `token`，然后退出 Costco 账户重新登录。 |
| 第一个时间段就报错 `fetching ... for ...` | 通常是令牌失效了。重新导入令牌后再运行。 |
| 日期范围被拒绝 | 把每次请求的时间段调小：`./costco-cli -window 90`。 |
| 结束时列出 `could not be downloaded` | 个别记录暂时取不到。重新运行同一条命令，只会补下载这些记录。 |

## 安全提醒

- 本工具只连接 Costco 自己的服务器（`signin.costco.com` 和 `ecom-api.costco.com`），不会把你的令牌或数据发送给任何第三方。
- 令牌保存在 `~/.costco/tokens.json`，下载的数据保存在 `costco-history`，两者都设置为只有你自己的系统账户可以读取。
- 收据里有会员号、地址和付款信息，**不要**把 `costco-history` 或 `tokens.json` 上传到网上、提交到 git 仓库，或分享给别人。
- 项目的 `.gitignore` 已经排除了 `costco-history/` 和 `token.json`，但如果你把输出目录或令牌文件放在别处，请自己注意。
- 如果怀疑令牌泄露，先删除 `~/.costco/tokens.json`，再到 costco.com 更新账户安全设置。已经发出的令牌能否立即作废由 Costco 决定，本工具无法控制；最迟大约 90 天后它会自然过期。
