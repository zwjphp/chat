package main

import (
	"fmt"
	"net"
	"strings"
	"time"
)

type Client struct {
	C    chan string // 用户发送数据的管道
	Name string      // 用户名
	Addr string      // 网络地址
}

var onlineMap map[string]Client

var message = make(chan string)

// 新开一个协程，转发消息，只要消息来了，遍历map，给map每个成员都发送消息
func Manager() {
	// 给map分配空间
	onlineMap = make(map[string]Client)

	for {
		msg := <-message // 没有消息前，这里会阻塞

		// 遍历map，给map每个成员都发送此消息
		for _, cli := range onlineMap {
			cli.C <- msg
		}
	}
}

func WriteMsgToClient(cli Client, conn net.Conn) {
	for msg := range cli.C { // 给当前客户端发送信息
		conn.Write([]byte(msg + "\n"))
	}
}

func MakeMsg(cli Client, msg string) (buf string) {
	buf = "[" + cli.Addr + "]" + cli.Name + " " + msg
	return
}

func HandleConn(conn net.Conn) { // 处理用户连接
	defer conn.Close()

	// 获取客户端的网络地址
	cliAddr := conn.RemoteAddr().String()

	// 创建一个结构体，默认，用户名和网络地址一样
	cli := Client{make(chan string), cliAddr, cliAddr}
	// 把结构体添加到map
	onlineMap[cliAddr] = cli
	// 新开一个协程，专门给当前客户端发送信息
	go WriteMsgToClient(cli, conn)
	// 广播某个在线
	message <- MakeMsg(cli, "login")
	// 提示，我是谁
	cli.C <- MakeMsg(cli, "I am here")

	isQuit := make(chan bool) // 对方是否主动退出

	hasData := make(chan bool) // 对方是否有数据发送

	// 新建一个协程，接收用户发送过来的数据
	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := conn.Read(buf)
			if n == 0 { // 对方断开，或者，出问题
				isQuit <- true
				fmt.Println("conn.Read err = ", err)
				return
			}
			msg := string(buf[:n-1]) // 通过windows nc测试，多一个换行
			if len(msg) == 3 && msg == "who" {
				// 遍历map，给当前用户发送所有成员
				conn.Write([]byte("user list:\n"))
				for _, tmp := range onlineMap {
					msg = tmp.Addr + ":" + tmp.Name + "\n"
					conn.Write([]byte(msg))
				}
			} else if len(msg) >= 8 && msg[:6] == "rename" {
				// rename|mike
				name := strings.Split(msg, "|")[1]
				cli.Name = name
				onlineMap[cliAddr] = cli
				conn.Write([]byte("rename ok\n"))
			} else {
				// 转发此内容
				message <- MakeMsg(cli, msg)
			}

			hasData <- true // 代表有数据
		}
	}()

	for {
		// 通过select检测channel的流动
		select {
		case <-isQuit:
			delete(onlineMap, cliAddr) // 当前用户从map移除
			message <- MakeMsg(cli, "login out")
			return
		case <-hasData:
		case <-time.After(60 * time.Second):
			delete(onlineMap, cliAddr) // 当前用户从map移除
			message <- MakeMsg(cli, "time out leave out")
			return
		}
	}
}

func main() {
	// 监听
	listener, err := net.Listen("tcp", ":8000")
	if err != nil {
		fmt.Println("net.Listen err = ", err)
		return
	}

	defer listener.Close()

	go Manager()

	// 主协程，循环等待用户连接
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("listener.Accept err = ", err)
			continue
		}
		go HandleConn(conn)
	}
}
