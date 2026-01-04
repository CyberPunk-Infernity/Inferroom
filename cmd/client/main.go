package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"goLearning/pkg/utils"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
)

// ---- async msgs ----
type netMsg struct{ text string }
type netErr struct{ err error }
type localMsg struct{ text string }

type model struct {
	vp    viewport.Model
	input textinput.Model

	conn   net.Conn
	aesKey []byte

	lines []string

	incoming chan tea.Msg

	history   []string
	histIndex int
	histPath  string

	quitting bool
}

func newModel(conn net.Conn, aesKey []byte, w, h int, histPath string) model {
	ti := textinput.New()                                   //输入框（textinput）
	ti.Placeholder = "Type a message… (/help for commands)" //提示字符
	ti.Focus()
	ti.CharLimit = 0 //不限制长度
	ti.Width = bigger(10, w-2)

	vph := h - 3 //viewport高度-3，分别是输入框、空行（UI 间隔）、提示信息
	if vph < 1 { //至少要有 1 行能显示消息
		vph = 1
	}
	vp := viewport.New(w, vph) //消息滚动区（viewport）
	vp.SetContent("")          //初始化内容为空,后面每次收到消息都会 SetContent(...)

	hist := loadHistory(histPath)

	m := model{
		vp:       vp,
		input:    ti,
		conn:     conn,
		aesKey:   aesKey,
		lines:    make([]string, 0, 512),
		incoming: make(chan tea.Msg, 256), //Bubble Tea 通过 listen(incoming) 把它转成 Msg,这就是“异步消息不污染输入框”的关键通道
		history:  hist,
		histPath: histPath,
	}
	m.histIndex = len(m.history)
	return m
}

// 把一个 Go 的 channel，包装成 Bubble Tea 能调度的 tea.Cmd
func listen(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return netErr{err: fmt.Errorf("incoming channel closed")}
		}
		return msg
	}
}

func (m model) Init() tea.Cmd {
	// 网络读循环：收到的内容通过 m.incoming 发给 UI
	go func() {
		for {
			byteString, err := utils.SecureReadFrame(m.conn, m.aesKey)
			if err != nil {
				m.incoming <- netErr{err: err}
				close(m.incoming)
				return
			}
			message := string(byteString)

			// 服务器发来文件：FILE|name|size
			if strings.HasPrefix(message, "FILE|") {
				m.incoming <- localMsg{text: "[local] downloading file…\n"}
				if err := ReceiveFile(message, m.conn, m.aesKey); err != nil {
					m.incoming <- localMsg{text: fmt.Sprintf("[download error] %v\n", err)}
				} else {
					m.incoming <- localMsg{text: "[download success]\n"}
				}
				continue
			}

			m.incoming <- netMsg{text: message}
		}
	}()

	return listen(m.incoming)
}

func (m *model) appendLine(s string) {
	m.lines = append(m.lines, s)
	m.vp.SetContent(strings.Join(m.lines, ""))
	m.vp.GotoBottom()
}

func (m model) saveAndQuit() (tea.Model, tea.Cmd) {
	_ = saveHistory(m.histPath, m.history)
	_ = m.conn.Close()
	m.quitting = true
	return m, tea.Quit
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.input.Width = bigger(10, msg.Width-2)
		vph := msg.Height - 3
		if vph < 1 {
			vph = 1
		}
		m.vp.Height = vph
		m.vp.SetContent(strings.Join(m.lines, ""))
		m.vp.GotoBottom()
		return m, nil

	case netMsg:
		m.appendLine(msg.text)
		return m, listen(m.incoming)

	case localMsg:
		m.appendLine(msg.text)
		return m, listen(m.incoming)

	case netErr:
		m.appendLine(fmt.Sprintf("\n[net error] %v\n", msg.err))
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m.saveAndQuit()

		case "enter":
			line := strings.TrimSpace(m.input.Value())
			if line == "" {
				return m, nil
			}

			// history（避免连续重复）
			if len(m.history) == 0 || m.history[len(m.history)-1] != line {
				m.history = append(m.history, line)
			}
			m.histIndex = len(m.history)

			// 本地命令
			switch {
			case line == "/help":
				m.appendLine(renderHelp())
				m.input.SetValue("")
				return m, nil

			case strings.HasPrefix(line, "/upload "):
				arg := strings.TrimSpace(strings.TrimPrefix(line, "/upload "))
				if arg == "" {
					m.appendLine("[local] usage: /upload <filepath>\n")
					m.input.SetValue("")
					return m, nil
				}
				// 不做进度条，只提示开始/结果；上传放到异步 cmd，避免 UI 卡死
				m.appendLine(fmt.Sprintf("[local] uploading %s …\n", arg))
				m.input.SetValue("")
				return m, uploadCmd(m.conn, m.aesKey, arg)

			case line == "/exit":
				// 仍然通知服务器
				_ = utils.SecureWriteFrame(m.conn, m.aesKey, []byte(line+"\n"))
				return m.saveAndQuit()
			}

			// 聊天内容原样发给服务器
			if err := utils.SecureWriteFrame(m.conn, m.aesKey, []byte(line+"\n")); err != nil {
				m.appendLine(fmt.Sprintf("[send error] %v\n", err))
			}
			m.input.SetValue("")
			return m, nil

		case "up":
			// 输入框空：滚动消息；输入框非空：翻历史
			if strings.TrimSpace(m.input.Value()) == "" {
				m.vp.LineUp(1)
				return m, nil
			}
			if len(m.history) == 0 {
				return m, nil
			}
			if m.histIndex > 0 {
				m.histIndex--
			}
			m.input.SetValue(m.history[m.histIndex])
			m.input.CursorEnd()
			return m, nil

		case "down":
			if strings.TrimSpace(m.input.Value()) == "" {
				m.vp.LineDown(1)
				return m, nil
			}
			if len(m.history) == 0 {
				return m, nil
			}
			if m.histIndex < len(m.history)-1 {
				m.histIndex++
				m.input.SetValue(m.history[m.histIndex])
				m.input.CursorEnd()
			} else {
				m.histIndex = len(m.history)
				m.input.SetValue("")
			}
			return m, nil
		}
	}

	// 默认：交给 input 处理输入
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return "Bye!\n"
	}
	help := "Enter: 发送消息 • ↑↓: 滚动消息面板 • (typing + ↑↓): 历史记录 • Ctrl+C: 断开链接"
	return fmt.Sprintf("%s\n\n> %s\n%s\n", m.vp.View(), m.input.View(), help)
}

func uploadCmd(conn net.Conn, key []byte, path string) tea.Cmd {
	return func() tea.Msg {
		// 复用 file_transfer.go 的 fileUpload(path, conn, aesKey)
		if err := fileUpload(path, conn, key); err != nil {
			return localMsg{text: fmt.Sprintf("[upload error] %v\n", err)}
		}
		return localMsg{text: "[upload success]\n"}
	}
}

func renderHelp() string {
	return strings.Join([]string{
		"\n================= Command List =================\n",
		"/help                     查看命令列表\n",
		"/onlineUsers              查看当前在线用户列表\n",
		"/setName <yourName>       设置你的网名\n",
		"/upload <filepath>        上传文件\n",
		"/fileList                 查看服务器文件列表\n",
		"/download <filename>      下载文件\n",
		"/exit                     断开链接\n",
		"================================================\n\n",
	}, "")
}

func loadHistory(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return []string{}
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func saveHistory(path string, history []string) error {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// keep last N
	const N = 500
	start := 0
	if len(history) > N {
		start = len(history) - N
	}
	for _, h := range history[start:] {
		if _, err := fmt.Fprintln(f, h); err != nil {
			return err
		}
	}
	return nil
}

func bigger(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: ./client <host> <port> <key(base64)>")
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

	// handshake加密握手
	if err := utils.SecureWriteFrame(conn, aesKey, []byte("Infernity")); err != nil {
		fmt.Println("handshake failed:", err)
		_ = conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})

	histPath := filepath.Join(os.TempDir(), "chatclient.history")

	p := tea.NewProgram(
		newModel(conn, aesKey, 80, 24, histPath),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Println("TUI error:", err)
	}
	_ = conn.Close()
}
