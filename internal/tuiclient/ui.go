package tuiclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/term"
)

// Page names for the root Pages primitive.
const (
	pageConnect = "connect"
	pageLogin   = "login"
	pageMain    = "main"
	pageModal   = "modal"
)

// pinnedSession hosts the portal and cannot be killed.
const pinnedSession = "dominion"

// UIOptions configures a UI run.
type UIOptions struct {
	Server   string // host[:port]; empty starts on the connect screen
	Scheme   string // http or https; empty uses the saved scheme
	Insecure bool   // skip TLS verification
	Version  string // reported in the header
}

// UI is the tview application.
type UI struct {
	app      *tview.Application
	pages    *tview.Pages
	cfg      Config
	client   *Client
	theme    string
	insecure bool
	version  string

	connectForm *tview.Form
	hostField   *tview.InputField
	schemeField *tview.DropDown
	connectErr  *tview.TextView
	savedList   *tview.List

	loginForm *tview.Form
	pinField  *tview.InputField

	header *tview.TextView
	list   *tview.List
	status *tview.TextView

	mu       sync.Mutex
	sessions []Session
	order    []Session
	page     string
	pollStop chan struct{}
}

// Run starts the terminal client and blocks until the user quits.
func Run(opts UIOptions) error {
	ui := &UI{
		app:      tview.NewApplication(),
		pages:    tview.NewPages(),
		cfg:      LoadConfig(),
		insecure: opts.Insecure,
		version:  opts.Version,
	}
	ui.theme = ui.cfg.Theme
	if ui.theme != "light" {
		ui.theme = "dark"
	}
	ui.setStyles(ui.theme)

	ui.build()
	ui.app.SetInputCapture(ui.capture)
	ui.app.SetRoot(ui.pages, true)
	ui.recolor()

	if opts.Server != "" {
		scheme := opts.Scheme
		if scheme == "" {
			scheme = ui.cfg.Scheme
		}
		base, err := NormalizeBase(scheme, opts.Server)
		if err != nil {
			ui.showConnect()
			ui.setConnectErr(err.Error())
			return ui.app.Run()
		}
		c, err := New(base, ui.insecure)
		if err != nil {
			ui.showConnect()
			ui.setConnectErr(err.Error())
			return ui.app.Run()
		}
		ui.client = c
		ui.showLogin("")
	} else {
		ui.showConnect()
	}
	return ui.app.Run()
}

func (ui *UI) build() {
	// --- connect page ---
	ui.hostField = tview.NewInputField().
		SetLabel("Server ").
		SetFieldWidth(30).
		SetText(ui.cfg.Host)
	ui.hostField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			ui.doConnect()
		}
	})

	scheme := ui.cfg.Scheme
	if scheme == "" {
		scheme = "http"
	}
	ui.schemeField = tview.NewDropDown().
		SetLabel("Scheme ").
		SetOptions([]string{"http", "https"}, nil)
	ui.schemeField.SetCurrentOption(indexOf([]string{"http", "https"}, scheme))

	ui.connectErr = tview.NewTextView().SetDynamicColors(true)
	ui.savedList = tview.NewList().ShowSecondaryText(true)
	ui.savedList.SetBorder(true).SetTitle(" Saved servers ")
	ui.savedList.SetSelectedFunc(func(i int, main, _ string, _ rune) {
		if i < 0 || i >= len(ui.cfg.Saved) {
			return
		}
		s := ui.cfg.Saved[i]
		ui.hostField.SetText(s.Host)
		ui.schemeField.SetCurrentOption(indexOf([]string{"http", "https"}, s.Scheme))
		ui.doConnect()
	})

	ui.connectForm = tview.NewForm().
		AddFormItem(ui.hostField).
		AddFormItem(ui.schemeField).
		AddButton("Connect", ui.doConnect).
		AddButton("Quit", ui.app.Stop)
	ui.connectForm.SetButtonsAlign(tview.AlignCenter)
	ui.connectForm.SetBorder(true).SetTitle(" dominion ")

	connect := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.connectForm, 9, 0, true).
		AddItem(ui.savedList, 0, 1, false).
		AddItem(ui.connectErr, 1, 0, false)
	ui.pages.AddPage(pageConnect, connect, true, true)

	// --- login page ---
	ui.pinField = tview.NewInputField().
		SetLabel("PIN ").
		SetMaskCharacter('*').
		SetFieldWidth(20)
	ui.pinField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			ui.doLogin(strings.TrimSpace(ui.pinField.GetText()))
		}
	})
	ui.loginForm = tview.NewForm().
		AddFormItem(ui.pinField).
		AddButton("Unlock", func() {
			ui.doLogin(strings.TrimSpace(ui.pinField.GetText()))
		}).
		AddButton("Change server", ui.showConnect)
	ui.loginForm.SetButtonsAlign(tview.AlignCenter)
	ui.loginForm.SetBorder(true).SetTitle(" dominion - unlock ")
	ui.pages.AddPage(pageLogin, centered(ui.loginForm, 46, 9), true, false)

	// --- main page ---
	ui.header = tview.NewTextView().SetDynamicColors(true)
	ui.status = tview.NewTextView().SetDynamicColors(true)
	ui.list = tview.NewList()
	ui.list.SetBorder(true).SetTitle(" Sessions ")
	ui.list.SetSelectedFunc(func(i int, _ string, _ string, _ rune) {
		ui.attachIndex(i)
	})
	main := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.header, 1, 0, false).
		AddItem(ui.list, 0, 1, true).
		AddItem(ui.status, 1, 0, false)
	ui.pages.AddPage(pageMain, main, true, false)
}

// ---- screen switching -------------------------------------------------------

func (ui *UI) showConnect() {
	ui.client = nil
	ui.refreshSaved()
	ui.setConnectErr("")
	ui.showPage(pageConnect)
}

func (ui *UI) showLogin(msg string) {
	ui.pinField.SetText("")
	ui.loginForm.SetTitle(" dominion - unlock ")
	ui.setLoginErr(msg)
	ui.showPage(pageLogin)
}

func (ui *UI) showMain() {
	ui.updateHeader()
	ui.renderSessions()
	ui.showPage(pageMain)
}

func (ui *UI) showPage(p string) {
	ui.page = p
	ui.pages.SwitchToPage(p)
	switch p {
	case pageConnect:
		ui.app.SetFocus(ui.hostField)
	case pageLogin:
		ui.app.SetFocus(ui.pinField)
	case pageMain:
		ui.app.SetFocus(ui.list)
	}
}

// ---- connect / login --------------------------------------------------------

func (ui *UI) doConnect() {
	host := strings.TrimSpace(ui.hostField.GetText())
	_, scheme := ui.schemeField.GetCurrentOption()
	base, err := NormalizeBase(scheme, host)
	if err != nil {
		ui.setConnectErr(err.Error())
		return
	}
	c, err := New(base, ui.insecure)
	if err != nil {
		ui.setConnectErr(err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := c.Healthz(ctx); err != nil {
		ui.setConnectErr(fmt.Sprintf("could not reach %s: %v", base, err))
		return
	}
	ui.client = c
	ui.cfg.Host = host
	ui.cfg.Scheme = scheme
	_ = SaveConfig(ui.cfg)
	ui.setConnectErr("")
	ui.showLogin("")
}

func (ui *UI) doLogin(pin string) {
	if ui.client == nil {
		ui.showConnect()
		return
	}
	if pin == "" {
		ui.setLoginErr("enter the PIN")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := ui.client.Login(ctx, pin); err != nil {
		ui.setLoginErr(loginErrorText(err))
		return
	}
	ui.showMain()
	ui.refreshNow()
	ui.startPoll()
}

func loginErrorText(err error) string {
	switch {
	case errors.Is(err, ErrBadPIN):
		return "incorrect PIN"
	case errors.Is(err, ErrTooMany):
		return "too many attempts, try again later"
	default:
		return err.Error()
	}
}

func (ui *UI) setConnectErr(msg string) {
	ui.connectErr.SetText(msg)
}

func (ui *UI) setLoginErr(msg string) {
	if msg == "" {
		ui.loginForm.SetTitle(" dominion - unlock ")
		return
	}
	ui.loginForm.SetTitle(" dominion - " + msg + " ")
}

func (ui *UI) refreshSaved() {
	ui.savedList.Clear()
	if len(ui.cfg.Saved) == 0 {
		ui.savedList.AddItem("(none saved)", "connect once to remember a server", 0, nil)
		return
	}
	for _, s := range ui.cfg.Saved {
		ui.savedList.AddItem(s.Name, s.Scheme+"://"+s.Host, 0, nil)
	}
}

// ---- sessions ---------------------------------------------------------------

func (ui *UI) refreshNow() {
	if ui.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sessions, err := ui.client.Sessions(ctx)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			ui.stopPoll()
			ui.showLogin("session expired")
			return
		}
		ui.setStatus("refresh failed: " + err.Error())
		return
	}
	ui.mu.Lock()
	ui.sessions = sessions
	ui.mu.Unlock()
	ui.renderSessions()
}

func (ui *UI) renderSessions() {
	ui.mu.Lock()
	sessions := append([]Session(nil), ui.sessions...)
	ui.mu.Unlock()

	sort.SliceStable(sessions, func(i, j int) bool {
		pi, pj := sessions[i].Name == pinnedSession, sessions[j].Name == pinnedSession
		if pi != pj {
			return pi
		}
		return sessions[i].Name < sessions[j].Name
	})

	ui.list.Clear()
	ui.order = nil
	for _, s := range sessions {
		secondary := fmt.Sprintf("%d window(s)", s.Windows)
		if s.Attached {
			secondary += ", attached"
		}
		if s.Name == pinnedSession {
			secondary += " - hosts the portal"
		}
		ui.list.AddItem(s.Name, secondary, 0, nil)
		ui.order = append(ui.order, s)
	}
	if len(sessions) == 0 {
		ui.list.AddItem("(no sessions)", "press n to create one", 0, nil)
	}
	ui.updateHeader()
	ui.setStatus("enter attach  n new  x kill  r refresh  l lock  s settings  L logout  q quit")
}

func (ui *UI) updateHeader() {
	host := ui.cfg.Host
	if ui.client != nil {
		host = ui.client.BaseURL()
	}
	if host == "" {
		host = "(not connected)"
	}
	fmt.Fprintf(ui.header, "[::b]dominion %s[-:-:-]  |  %s  |  theme: %s", ui.version, host, ui.theme)
}

func (ui *UI) setStatus(msg string) {
	ui.status.SetText(msg)
}

func (ui *UI) attachIndex(i int) {
	if i < 0 || i >= len(ui.order) {
		return
	}
	ui.attach(ui.order[i].Name)
}

func (ui *UI) promptNew() {
	input := tview.NewInputField().SetLabel("Name ").SetFieldWidth(30)
	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			name := strings.TrimSpace(input.GetText())
			ui.closeModal()
			if name != "" {
				ui.createSession(name)
			}
		case tcell.KeyEscape:
			ui.closeModal()
		}
	})
	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().SetText("Create a tmux session"), 1, 0, false).
		AddItem(input, 1, 0, true)
	box.SetBorder(true).SetTitle(" new session ")
	ui.showModal(centered(box, 46, 5), input)
}

func (ui *UI) createSession(name string) {
	if ui.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := ui.client.Create(ctx, name); err != nil {
		ui.setStatus("create failed: " + err.Error())
		return
	}
	ui.refreshNow()
}

func (ui *UI) promptKillSelected() {
	i := ui.list.GetCurrentItem()
	if i < 0 || i >= len(ui.order) {
		return
	}
	s := ui.order[i]
	if s.Name == pinnedSession {
		ui.setStatus("the " + pinnedSession + " session cannot be killed")
		return
	}
	text := fmt.Sprintf("Kill session %q?\nEverything running in it will be terminated.", s.Name)
	if s.Attached {
		text = fmt.Sprintf("Kill session %q?\nIt is attached right now; everything running in it will be terminated.", s.Name)
	}
	m := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Kill", "Cancel"}).
		SetDoneFunc(func(idx int, _ string) {
			ui.closeModal()
			if idx == 0 {
				ui.killSession(s.Name)
			}
		})
	ui.showModal(m, m)
}

func (ui *UI) killSession(name string) {
	if ui.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := ui.client.Kill(ctx, name); err != nil {
		ui.setStatus("kill failed: " + err.Error())
		return
	}
	ui.refreshNow()
}

// ---- lock / logout / settings ----------------------------------------------

func (ui *UI) doLock() {
	if ui.client == nil {
		return
	}
	ui.stopPoll()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ui.client.Lock(ctx)
	ui.showLogin("Locked - enter PIN to continue")
}

func (ui *UI) doLogout() {
	if ui.client == nil {
		return
	}
	ui.stopPoll()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ui.client.Logout(ctx)
	ui.showLogin("")
}

func (ui *UI) showSettings() {
	options := []string{"dark", "light"}
	dd := tview.NewDropDown().
		SetLabel("Theme ").
		SetOptions(options, func(text string, _ int) { ui.applyTheme(text) })
	dd.SetCurrentOption(indexOf(options, ui.theme))
	form := tview.NewForm().
		AddFormItem(dd).
		AddButton("Done", ui.closeModal)
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetBorder(true).SetTitle(" settings ")
	ui.showModal(centered(form, 42, 7), dd)
}

// ---- modals -----------------------------------------------------------------

func (ui *UI) showModal(p tview.Primitive, focus tview.Primitive) {
	ui.pages.AddPage(pageModal, p, true, true)
	if focus == nil {
		focus = p
	}
	ui.app.SetFocus(focus)
}

func (ui *UI) closeModal() {
	ui.pages.RemovePage(pageModal)
	ui.showPage(ui.page)
}

// ---- theming ----------------------------------------------------------------

func (ui *UI) setStyles(mode string) {
	bg, fg := tcell.ColorBlack, tcell.ColorWhite
	if mode == "light" {
		bg, fg = tcell.ColorWhite, tcell.ColorBlack
	}
	tview.Styles.PrimitiveBackgroundColor = bg
	tview.Styles.PrimaryTextColor = fg
	tview.Styles.BorderColor = fg
	tview.Styles.TitleColor = fg
	tview.Styles.SecondaryTextColor = tcell.ColorGray
}

func (ui *UI) recolor() {
	bg, fg := tcell.ColorBlack, tcell.ColorWhite
	if ui.theme == "light" {
		bg, fg = tcell.ColorWhite, tcell.ColorBlack
	}
	for _, box := range []*tview.Box{ui.header.Box, ui.list.Box, ui.status.Box, ui.connectErr.Box} {
		box.SetBackgroundColor(bg)
	}
	ui.header.SetTextColor(fg)
	ui.status.SetTextColor(fg)
	ui.list.SetMainTextColor(fg)
	ui.list.SetSecondaryTextColor(tcell.ColorGray)
	ui.list.SetSelectedBackgroundColor(tcell.ColorBlue)
	ui.list.SetSelectedTextColor(tcell.ColorWhite)
	ui.savedList.SetMainTextColor(fg)
	ui.savedList.SetSecondaryTextColor(tcell.ColorGray)
}

// applyTheme switches the palette, persists the choice, and redraws.
func (ui *UI) applyTheme(mode string) {
	if mode != "light" {
		mode = "dark"
	}
	ui.theme = mode
	ui.setStyles(mode)
	ui.recolor()
	ui.updateHeader()
	ui.cfg.Theme = mode
	_ = SaveConfig(ui.cfg)
	ui.app.Draw()
}

// ---- attach -----------------------------------------------------------------

func (ui *UI) attach(name string) {
	if ui.client == nil {
		return
	}
	ui.stopPoll()
	ui.app.Suspend(func() {
		if err := ui.runAttach(name); err != nil && !errors.Is(err, ErrDetach) {
			fmt.Fprintf(os.Stderr, "\r\ndominion: %v\r\n", err)
			time.Sleep(1500 * time.Millisecond)
		}
	})
	ui.startPoll()
	ui.refreshNow()
}

func (ui *UI) runAttach(name string) error {
	fd := int(os.Stdin.Fd())
	cols, rows, err := term.GetSize(fd)
	if err != nil || cols <= 0 || rows <= 0 {
		cols, rows = 80, 24
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ws, err := ui.client.DialAttach(ctx, name, uint16(cols), uint16(rows))
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return errors.New("session expired; please log in again")
		}
		return err
	}
	defer ws.Close()

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	resizes := make(chan Winsize, 4)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-winch:
				c, r, err := term.GetSize(fd)
				if err != nil {
					continue
				}
				select {
				case resizes <- Winsize{Cols: uint16(c), Rows: uint16(r)}:
				default:
				}
			}
		}
	}()

	det := NewDetachReader(os.Stdin)
	go func() {
		select {
		case <-det.Done():
			fmt.Fprint(os.Stderr, "\r\n[detached]\r\n")
			cancel()
		case <-ctx.Done():
		}
	}()

	err = Pump(ctx, ws, det, os.Stdout, resizes)
	if errors.Is(err, context.Canceled) {
		return ErrDetach
	}
	var ce *CloseError
	if errors.As(err, &ce) {
		switch ce.Code {
		case 4001:
			return errors.New("session expired; please log in again")
		case 1000, 1001:
			return nil
		}
	}
	return err
}

// ---- polling ----------------------------------------------------------------

func (ui *UI) startPoll() {
	ui.stopPoll()
	stop := make(chan struct{})
	ui.pollStop = stop
	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				ui.app.QueueUpdateDraw(func() { ui.refreshNow() })
			}
		}
	}()
}

func (ui *UI) stopPoll() {
	if ui.pollStop != nil {
		close(ui.pollStop)
		ui.pollStop = nil
	}
}

// ---- input ------------------------------------------------------------------

func (ui *UI) capture(ev *tcell.EventKey) *tcell.EventKey {
	if ui.page != pageMain || ui.pages.HasPage(pageModal) {
		return ev
	}
	if ev.Key() == tcell.KeyCtrlC {
		ui.app.Stop()
		return nil
	}
	switch ev.Rune() {
	case 'q':
		ui.app.Stop()
		return nil
	case 'n':
		ui.promptNew()
		return nil
	case 'x':
		ui.promptKillSelected()
		return nil
	case 'r':
		ui.refreshNow()
		return nil
	case 'l':
		ui.doLock()
		return nil
	case 'L':
		ui.doLogout()
		return nil
	case 's':
		ui.showSettings()
		return nil
	}
	return ev
}

// ---- helpers ----------------------------------------------------------------

// centered places p in the middle of the screen with the given size.
func centered(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
}

func indexOf(opts []string, value string) int {
	for i, o := range opts {
		if o == value {
			return i
		}
	}
	return 0
}
