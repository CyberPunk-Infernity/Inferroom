package utils

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// 防止被恶意发超大长度撑爆内存
	MaxFrameSize = 64 * 1024 * 1024 // 64MB
)

// writes one frame: [4-byte length][payload]
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return fmt.Errorf("payload too large: %d", len(payload))
	}

	//准备 4 字节长度头
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))

	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}

	// 这里用 Write 循环确保全部写完（防止短写）
	total := 0
	for total < len(payload) {
		n, err := w.Write(payload[total:])
		if err != nil {
			return err
		}
		total += n
	}
	return nil
}

func ReadFrame(r io.Reader) ([]byte, error) { //r io.Reader：通常是 net.Conn 或 bufio.Reader
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil { //io.ReadFull会一直读，直到把 lenBuf 填满 4 字节，如果连接断了/超时/读不到够 4 字节，就返回错误
		return nil, err
	}

	//解析长度，把 4 字节还原成 uint32 长度 n
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n == 0 { //允许空消息
		return nil, nil
	}
	if n > MaxFrameSize {
		return nil, fmt.Errorf("frame too large: %d", n)
	}

	//构造n长度的字符串
	massage := make([]byte, n)
	if _, err := io.ReadFull(r, massage); err != nil { //无论 TCP 怎么拆包，这里都会读到恰好 n 个字节才返回
		return nil, err
	}
	return massage, nil
}
