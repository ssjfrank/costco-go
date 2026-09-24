# costco-cli for macOS — temporary download

This branch only holds prebuilt macOS binaries of `costco-cli`, built from
commit `d6fc206` on
[`cursor/download-full-order-history-9b26`](https://github.com/ssjfrank/costco-go/tree/cursor/download-full-order-history-9b26)
with `scripts/build-release.sh`. Once GitHub Actions is enabled and a version
tag is pushed, the same files are published on the
[Releases](https://github.com/ssjfrank/costco-go/releases) page and this branch
can be deleted.

这个分支只存放 macOS 版 `costco-cli` 的预编译程序，供正式 Release 发布之前临时下载使用。

## 下载 / Download

在"终端"里运行（Apple 芯片和 Intel 芯片的 Mac 都适用）：

```bash
mkdir -p ~/costco && cd ~/costco
ARCH=$(uname -m | sed 's/x86_64/amd64/')
curl -fL -o costco-cli "https://raw.githubusercontent.com/ssjfrank/costco-go/refs/heads/cursor/macos-download-9b26/costco-cli-darwin-$ARCH"
chmod +x costco-cli
./costco-cli -version
```

应显示 `costco-cli 1.0.0 (commit d6fc206)`。然后按
[中文分步指南](https://github.com/ssjfrank/costco-go/blob/cursor/download-full-order-history-9b26/docs/GUIDE.zh-CN.md)
的第 2 步继续（在浏览器登录 Costco，按 F12 在 Console 里运行命令）。

| File | Mac |
| --- | --- |
| `costco-cli-darwin-arm64` | Apple Silicon (M1 and later) — ad-hoc signed, as Apple Silicon requires |
| `costco-cli-darwin-amd64` | Intel |

Verify a download with `shasum -a 256 costco-cli` against `SHA256SUMS`.
Downloading with `curl` avoids the macOS "cannot verify the developer" prompt;
if you downloaded through a browser instead, run
`xattr -d com.apple.quarantine costco-cli`.

To run downloads in a Docker container with only the token file mounted, see
[Running in a container](https://github.com/ssjfrank/costco-go/blob/cursor/download-full-order-history-9b26/README.md#running-in-a-container)
（中文说明见指南的"在 Docker 容器里运行"一节）.
