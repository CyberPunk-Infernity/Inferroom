package main

import (
	"fmt"
	"goLearning/pkg/utils"
	"net"
	"os"
	"strings"
)

type User struct {
	Name string
	IP   string
	Port string
	Conn net.Conn
}

var UserList []User
var aesKey []byte

func main() {
	selfPort := os.Args[1]
	ln, err := net.Listen("tcp", ":"+selfPort) // 监听所有网卡的 xxxx 端口
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	// 生成 32 字节 AES key，并打印 base64 给 client 用
	key, keyB64, err := utils.NewRandomKeyBase64(32)
	if err != nil {
		panic(err)
	}
	aesKey = key

	fmt.Println("listening on :" + selfPort)
	fmt.Println("AES key (base64):", keyB64)

	for {
		conn, err := ln.Accept() // 阻塞等待新连接
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}

		go handle(conn)
	}
}

// 每个连接一个 goroutine
func handle(conn net.Conn) {
	fmt.Println("new connection from", conn.RemoteAddr())

	// 握手：client 必须先发一条加密的 "Infernity"
	hello, err := utils.SecureReadFrame(conn, aesKey)
	if err != nil || string(hello) != "Infernity" {
		_ = conn.Close()
		return
	}

	// 获取名字，写入列表
	name, _ := utils.RandomString(5)
	addUser(name, conn)

	defer func() {
		// 这里做统一清理：无论怎么退出都删
		UserList = removeUser(UserList, conn)
		broadcast(fmt.Sprintf("%s 离开了房间。\n", name))
		_ = conn.Close()
	}()

	for {
		massageByte, err := utils.SecureReadFrame(conn, aesKey)
		massage := string(massageByte) //无语，和你说不下去，典型的强类型语言思维
		if err != nil {
			fmt.Println("read error:", err)
			return
		}

		//命令判定
		if strings.HasPrefix(massage, "/onlineUsers") { //获取在线用户列表
			var sb strings.Builder

			total := len(UserList)
			sb.WriteString(fmt.Sprintf("当前在线人数：%d\n", total))

			for i, user := range UserList {
				sb.WriteString(fmt.Sprintf("%d) %s  %s\n", i+1, user.Name, user.IP))
			}

			if err := utils.SecureWriteFrame(conn, aesKey, []byte(sb.String())); err != nil {
				fmt.Println("write error:", err)
			}
		} else if strings.HasPrefix(massage, "/setName") { //设置用户名
			nickname := strings.TrimSpace(strings.TrimPrefix(massage, "/setName "))
			for i := range UserList { //要用下标改，用range 里拿到的 user 是切片元素的拷贝（副本），你改的是副本的 Name，不会写回 UserList
				if UserList[i].Conn == conn {
					UserList[i].Name = nickname
					name = nickname //全局变量也要改
					break
				}
			}
			_ = utils.SecureWriteFrame(conn, aesKey, []byte("[SYSTEM] 修改成功！\n"))
		} else if strings.HasPrefix(massage, "FILE|") { // 上传文件，这里是给服务器看的
			if err := ReceiveFile(massage, conn, name); err != nil {
				fmt.Println("upload error:", err)
			}
		} else if strings.HasPrefix(massage, "/fileList") { // 获取上传文件列表
			list, err := fileList()
			if err != nil {
				fmt.Println("fileList error:", err)
			}
			if len(list) == 0 {
				_ = utils.SecureWriteFrame(conn, aesKey, []byte("文件列表为空！\n"))
			}
			if err := utils.SecureWriteFrame(conn, aesKey, []byte(list)); err != nil {
				fmt.Println("write error:", err)
			}
		} else if strings.HasPrefix(massage, "/download") { //下载文件
			filename := strings.TrimSpace(strings.TrimPrefix(massage, "/download ")) //去掉前缀，去掉特殊换行符
			if err := fileUpload(filename, conn); err != nil {
				fmt.Println("upload error:", err)
			} else {
				fmt.Println("upload success")
			}
		} else if strings.HasPrefix(massage, "/exit") { // 断开链接
			UserList = removeUser(UserList, conn)
			_ = utils.SecureWriteFrame(conn, aesKey, []byte("Bye!"))
			conn.Close()
		} else {
			broadcast(fmt.Sprintf("%s say: %s", name, massage)) //massage自带换行，不用加
		}
	}
}

// 添加用户
func addUser(name string, conn net.Conn) {
	parts := strings.Split(conn.RemoteAddr().String(), ":") //冒号分隔字符串
	user := User{Name: name, IP: parts[0], Port: parts[1], Conn: conn}
	UserList = append(UserList, user)

	broadcast(fmt.Sprintf("%s 加入了房间。\n", name))
}

// 删除用户
func removeUser(users []User, c net.Conn) []User {
	for i := range users {
		if users[i].Conn == c {
			return append(users[:i], users[i+1:]...)
		}
	}
	return users
}

// 广播发送消息
func broadcast(massage string) {
	for _, user := range UserList {
		if err := utils.SecureWriteFrame(user.Conn, aesKey, []byte("[SYSTEM] "+massage)); err != nil {
			fmt.Println("write error:", err)
		}
	}
}

// 单独发送消息(私聊)
func unicast(name string, massage string) {
	for _, user := range UserList {
		if user.Name == name {
			if err := utils.SecureWriteFrame(user.Conn, aesKey, []byte(massage)); err != nil {
				fmt.Println("write error:", err)
			}
			break
		}
	}
}
