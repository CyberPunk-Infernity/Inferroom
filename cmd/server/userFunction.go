package main

import (
	"fmt"
	"goLearning/pkg/utils"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ReceiveFile(massage string, conn net.Conn, name string) error {
	// massage 是 ReadFrame(conn) 读到的那一帧
	// 格式：FILE|filename|size

	parts := strings.Split(massage, "|")
	if len(parts) != 3 || parts[0] != "FILE" {
		return fmt.Errorf("bad file header: %q", massage)
	}
	filename := filepath.Base(parts[1])

	size, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || size < 0 {
		return fmt.Errorf("bad size in header: %v", parts[2])
	}

	os.MkdirAll("uploads", 0755) //创建目录，不存在就创建，存在就忽略
	dstPath := filepath.Join("uploads", filename)

	//写文件，开一个文件句柄
	writerHandler, err := os.Create(dstPath)
	if err != nil {
		// 如果创建失败：仍然要把后续 size 字节的“数据帧”消费掉，否则协议会乱
		// 这里的消费方式：不断 ReadFrame，然后累计丢弃，直到丢弃够 size
		var discarded int64
		for discarded < size {
			chunk, rerr := utils.SecureReadFrame(conn, aesKey)
			if rerr != nil {
				return fmt.Errorf("discard chunks err: %w", rerr)
			}
			discarded += int64(len(chunk))
		}
		return fmt.Errorf("create file err: %w", err)
	}
	defer writerHandler.Close()

	// 循环收 chunk，直到写够 size 字节
	var got int64
	for got < size {
		chunk, err := utils.SecureReadFrame(conn, aesKey)
		if err != nil {
			return fmt.Errorf("read chunk: %w (got %d/%d)", err, got, size)
		}

		n, err := writerHandler.Write(chunk) //写文件内容
		if err != nil {
			return fmt.Errorf("write file: %w", err)
		}
		got += int64(n)
	}

	broadcast(fmt.Sprintf("%s uploaded a file: %s\n", name, filename))
	return nil
}

func fileList() (string, error) {
	items, err := os.ReadDir("uploads")
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	for _, item := range items {
		name := item.Name()

		if item.IsDir() {
			sb.WriteString(fmt.Sprintf("[DIR]  %s\n", name))
		} else {
			info, err := item.Info()
			if err != nil {
				continue // 读不到信息就跳过
			}
			sb.WriteString(fmt.Sprintf(
				"[FILE] %-20s %d bytes\n",
				name,
				info.Size(),
			))
		}
	}

	return sb.String(), nil
}

func fileUpload(filename string, conn net.Conn) error {
	//先发一帧：FILE|<filename>|<size>（这是文件头，文本）
	//再发若干帧：每帧是一段文件二进制（例如 32KB）
	//接收端按照 size 累计写入，收满结束（不需要 FILE_END）
	localpath := filepath.Join("uploads", filename)

	f, err := os.Open(localpath) //只读打开
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	//获取这个文件的“元信息”（metadata），返回一个 os.FileInfo。里面包含：文件大小、是否目录、权限、最后修改时间 等信息.
	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	if stat.IsDir() {
		return fmt.Errorf("path is a directory, not a file")
	}
	size := stat.Size()
	if size < 0 {
		return fmt.Errorf("invalid file size")
	}

	// 1) 发送“文件头”一帧（文本）
	header := fmt.Sprintf("FILE|%s|%d", filename, size)
	if err := utils.SecureWriteFrame(conn, aesKey, []byte(header)); err != nil {
		return fmt.Errorf("send header: %w", err)
	}

	// 2) 分块发送文件内容：每块一个 frame（二进制）
	buf := make([]byte, 32*1024)
	var sent int64

	for { //依然循环发送，一大堆异常处理
		n, rerr := f.Read(buf)
		if n > 0 {
			if err := utils.SecureWriteFrame(conn, aesKey, buf[:n]); err != nil {
				return fmt.Errorf("send chunk: %w", err)
			}
			sent += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("read file: %w", rerr)
		}
	}

	if sent != size {
		return fmt.Errorf("sent %d bytes, want %d", sent, size)
	}
	return nil
}
