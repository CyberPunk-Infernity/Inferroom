# Inferroom

一个用 Go 写的轻量聊天室：基于 TCP 长连接、**自定义帧协议**（4-byte length + payload）、**AES-GCM 端到端加密**，支持命令行交互、在线用户列表、改名、以及**文件上传/下载**。

---

## ✨ 项目特点

- **可靠拆包/粘包处理**：自定义 Frame 协议 `[4-byte length][payload]`，读写都用 `io.ReadFull` + 循环写，确保完整收发。
- **安全加密传输**：通信内容在 Frame 之上使用 **AES-GCM** 加密（nonce + ciphertext+tag）。
- **抗滥用保护**：限制最大帧长度 `MaxFrameSize = 64MB`，避免被恶意长度撑爆内存。
- **Key 传参友好**：支持 base64 / hex 直接解析，不合法则 fallback 到 `sha256(keyStr)` 生成 32 字节 key。
- **命令行体验**：客户端用 readline 提供历史记录与更友好的输入体验。
- **文件上传/下载**：
  - 上传：先发 `FILE|<filename>|<size>` 头帧，再分块发送二进制帧。
  - 服务端存储到 `uploads/` 并广播上传消息。
  - 下载：支持 `/download <filename>` 从服务端回传到客户端。
- **基础聊天室能力**：在线列表、设置昵称、广播消息、退出等。

---

## 🧠 协议简介

### 1) Frame 封包（解决 TCP 粘包/拆包）
- 写入：`[4-byte 大端长度][payload]`
- 读取：先读 4 字节长度，再读对应 payload（允许空消息） 并对长度做上限校验（64MB）。

### 2) Secure Frame（加密层）
- `plaintext -> AES-GCM -> WriteFrame`
- `ReadFrame -> AES-GCM decrypt -> plaintext`

---

## 🚀 快速开始（从编译到启动）

### 0) 环境要求
- Go 1.20+
- macOS / Linux / Windows 均可

检查 Go 是否安装成功：
```bash
go version
```

### 1) 拉取代码

```
git clone https://github.com/CyberPunk-Infernity/Inferroom.git
cd Inferroom
```

------

### 2) 初始化依赖（首次运行必做）

如果仓库里已经有 `go.mod`：

```
go mod tidy
```

如果你是第一次建仓、还没有 `go.mod`（仅在确实不存在时执行一次）：

```
go mod init goLearning
go mod tidy
```

------

### 3) 编译

```
mkdir -p bin

go build -o bin/server ./cmd/server
go build -o bin/client ./cmd/client
```

------

### 4) 启动服务端

服务端通过命令行参数接收监听端口：

```
./bin/server 9000
```

启动后服务端会在控制台输出一段 **base64 的 AES key**（用于客户端连接加密通信），例如：

```
Server listening on :9000
AES Key (base64): <COPY_THIS_KEY>
```

✅ **把这段 key 复制下来**，下一步启动客户端要用。

------

### 5) 启动客户端并连接

客户端参数：`host port key`

```
./bin/client 127.0.0.1 9000 <COPY_THIS_KEY>
```


连接成功后会进入交互式输入
