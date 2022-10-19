package main

import (
	"fmt"
	"image/color"
	"net"
	"time"

	"fyne.io/fyne/v2/theme"

	"fyne.io/fyne/v2/canvas"

	"fyne.io/fyne/v2/container"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/widget"

	"github.com/atotto/clipboard"
)

var ListenPort string = "23022"
var RemoteIp string = "127.0.0.1"
var RemotePort string = "23025"

func main() {
	myApp := app.New()
	myWin := myApp.NewWindow("Tcp Forward")

	ListenPortBox := makeInputUI("ListenPort", "23022", &ListenPort)
	RemoteIpBox := makeInputUI("RemoteIp", "127.0.0.1", &RemoteIp)
	RemotePortBox := makeInputUI("RemotePort", "23025", &RemotePort)

	preview := makeOutpuUI()

	text := canvas.NewText("wait", color.White)

	startBtn := widget.NewButton("Start", func() {
		fmt.Println("ListenPort:" + ListenPort)
		fmt.Println("RemoteIp:" + RemoteIp)
		fmt.Println("RemotePort:" + RemotePort)
		text.Text = "runing"
		text.Refresh()
		go TcpTask(ListenPort, RemoteIp, RemotePort, preview)
	})
	startBtn.Importance = widget.HighImportance

	copyTextBtn := widget.NewButton("copy text", func() {
		clipboard.WriteAll(preview.String())
	})

	box := container.NewVBox(ListenPortBox, RemoteIpBox, RemotePortBox, startBtn, text, copyTextBtn)
	boxWin := container.NewVSplit(box, preview)

	myWin.SetContent(boxWin)

	myWin.CenterOnScreen()
	myWin.Resize(fyne.Size{Width: 960, Height: 720})
	myWin.ShowAndRun()
}

func makeInputUI(name string, hint string, put *string) *fyne.Container {
	Entry := widget.NewEntry()
	Entry.SetPlaceHolder(hint)
	Entry.OnChanged = func(content string) {
		*put = content
	}

	Box := container.NewVBox(widget.NewLabel(name), Entry)

	return Box
}

func makeOutpuUI() *widget.RichText {

	preview := widget.NewRichTextWithText("")
	preview.Wrapping = fyne.TextWrapWord
	preview.Scroll = 2
	preview.Resize(fyne.Size{Width: 700, Height: 460})

	return preview
}

func preViewPrint(preview *widget.RichText, content string) {

	var style widget.RichTextStyle = widget.RichTextStyle{
		ColorName: theme.ColorNameForeground,
		Inline:    false,
		SizeName:  theme.SizeNameSubHeadingText,
		TextStyle: fyne.TextStyle{Bold: true},
	}

	var prText = &widget.TextSegment{
		Style: style,
		Text:  content,
	}

	var Head []widget.RichTextSegment
	Head = append(Head, prText)

	preview.Segments = append(Head, preview.Segments...)

	preview.Refresh()
}

var ServerFd net.Conn

var clientbuf = make(chan []byte, 1024)
var serverbuf = make(chan []byte, 1024)

var clients = make(map[net.Conn]bool)

func TcpTask(listenPort string, remoteIp string, remotePort string, preview *widget.RichText) {
	fmt.Println("tcp forward start")

	var ShutDownCh chan struct{} = make(chan struct{})

	//使用 net.Listen 监听连接的地址与端口
	fmt.Printf("Local listen port %s\n", listenPort)
	fmt.Printf("remote IP %s\n", remoteIp)
	fmt.Printf("remote Port %s\n", remotePort)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", listenPort))

	if err != nil {
		fmt.Printf("listen fail, err: %v\n", err)
		return
	}
	defer listener.Close()

	ServerFd, err := net.Dial("tcp", fmt.Sprintf("%s:%s", remoteIp, remotePort))
	if err != nil {
		fmt.Printf("server conn %s fail\n", fmt.Sprintf("%s:%s", remoteIp, remotePort))
		return
	}
	fmt.Println("server connection success")
	defer ServerFd.Close()

	go ForwardSelect(ServerFd, ShutDownCh)
	go ServerToCh(ServerFd, preview, ShutDownCh)

	preViewPrint(preview, "server conn suc, start listen")

	go func() {
		select {
		case <-ShutDownCh:
			fmt.Println("ShutDownCh listener close")
			for client := range clients {
				client.Close()
			}
			listener.Close()
		}
	}()

	for {
		select {
		case <-ShutDownCh:
			fmt.Println("ShutDownCh TcpTask")
			return
		default:
			// 等待连接
			client, err := listener.Accept()
			if err != nil {
				fmt.Printf("accept fail, err: %v\n", err)
				continue
			}
			// 对每个新连接创建一个协程进行收发数据
			go process(client, ServerFd, preview, ShutDownCh)
		}
	}
}

func process(client net.Conn, server net.Conn, preview *widget.RichText, ShutDownCh chan struct{}) {
	defer func() {
		delete(clients, client)
		client.Close()
	}()

	fmt.Printf("Accept clint %d\n", client)

	if !clients[client] {
		clients[client] = true
	}

	for {
		select {
		case <-ShutDownCh:
			fmt.Println("ShutDownCh process")
			return
		default:
			var buf = make([]byte, 1024, 1024)
			//接受数据
			n, err := client.Read(buf[:])
			if err != nil {
				fmt.Printf("read from connect failed, err: %v\n", err)
				return
			}
			var loghead string = fmt.Sprintf(">>>>>>>>>>>>>>> len %d [%s]", n, time.Now())
			preViewPrint(preview, loghead)
			var logbody string
			for i := 0; i < n; i++ {
				logbody = logbody + fmt.Sprintf("%02x ", buf[i])
			}
			preViewPrint(preview, logbody+"\n")

			clientbuf <- buf[:n]
		}
	}
}

func ServerToCh(server net.Conn, preview *widget.RichText, ShutDownCh chan struct{}) {
	var buf = make([]byte, 1024, 1024)

	for {
		n, err := server.Read(buf[:])
		if err != nil {
			fmt.Printf("read from server connect failed, err: %v\n", err)
			close(ShutDownCh)
			return
			//os.Exit(1)
		}

		var loghead string = fmt.Sprintf("<<<<<<<<<<<<<<< len %d [%s]", n, time.Now())
		preViewPrint(preview, loghead)

		var logbody string
		for i := 0; i < n; i++ {
			logbody = logbody + fmt.Sprintf("%02x ", buf[i])
		}
		preViewPrint(preview, logbody+"\n")

		serverbuf <- buf[:n]
	}

}

func ForwardSelect(server net.Conn, ShutDownCh chan struct{}) {

	for {
		select {
		case <-ShutDownCh:
			return
		case serverMsg := <-serverbuf:
			for clinet, ok := range clients {
				if ok {
					clinet.Write(serverMsg)
				}
			}
		case clientMsg := <-clientbuf:
			server.Write(clientMsg)
		default:

		}
	}
}
