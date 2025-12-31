package main

import (
	"fmt"
	"goLearning/pkg/utils"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: ./client <host> <port> <key>")
		return
	}

	host := os.Args[1]
	port := os.Args[2]

	keyStr := os.Args[3]
	aesKey, err := utils.ParseKey(keyStr)
	if err != nil {
		panic(err)
	}

	conn, err := net.Dial("tcp", host+":"+port)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// 握手：先发加密 Infernity
	if err := utils.SecureWriteFrame(conn, aesKey, []byte("Infernity")); err != nil {
		fmt.Println("handshake failed:", err)
		return
	}

	//开一个 goroutine 读服务器返回
	go func() {
		for {
			resByte, err := utils.SecureReadFrame(conn, aesKey)
			res := string(resByte)
			if err != nil {
				fmt.Println("server closed:", err)
				return
			}
			if strings.HasPrefix(res, "FILE|") { // 上传文件，这里是给服务器看的
				if err := ReceiveFile(res, conn, aesKey); err != nil {
					fmt.Println("download error:", err)
				} else {
					fmt.Println("download success")
				}
			} else {
				fmt.Print(res)
			}
		}
	}()

	//成熟的命令行读取用户输入
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "> ",
		HistoryFile:     filepath.Join(os.TempDir(), "chatclient.history"),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt { // Ctrl+C
			continue
		}
		if err == io.EOF { // Ctrl+D
			return
		}
		cmd := line + "\n"
		// 命令判定
		if strings.HasPrefix(cmd, "/help") {
			var helpList = strings.Join([]string{
				"",
				"================= Command List =================",
				"",
				"/help                     查看命令列表",
				"/onlineUsers              查看当前在线用户列表",
				"/setName <yourName>       设置你的网名",
				"/upload <filepath>        上传文件",
				"/fileList                 查看文件上传列表",
				"/download <filename>      下载文件",
				"/exit                     断开链接",
				"",
				"================================================",
			}, "\n")
			fmt.Println(helpList)
		} else if strings.HasPrefix(cmd, "/upload ") { //上传文件
			path := strings.TrimSpace(strings.TrimPrefix(cmd, "/upload ")) //去掉前缀，去掉特殊换行符
			if err := fileUpload(path, conn, aesKey); err != nil {
				fmt.Println("upload error:", err)
			} else {
				fmt.Println("upload success")
			}
		} else {
			//普通发送消息
			if err := utils.SecureWriteFrame(conn, aesKey, []byte(cmd)); err != nil {
				fmt.Println("write error:", err)
			}
		}
	}
}
