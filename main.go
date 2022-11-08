package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	statusPKPath      string
	currentCommitHash string
)

type xinStatus struct {
	widget.Icon

	debug        bool
	tabs         *container.AppTabs
	cards        []fyne.CanvasObject
	boundStrings []binding.ExternalString
	log          *widget.TextGrid
}

func (x *xinStatus) prependLog(s string) {
	text := x.log.Text()
	now := time.Now()
	x.log.SetText(strings.Join([]string{
		fmt.Sprintf("%s: %s", now.Format(time.RFC822), s),
		text,
	}, "\n"))
}

type Config struct {
	Statuses    []*Status `json:"statuses"`
	Repo        string    `json:"repo"`
	PrivKeyPath string    `json:"priv_key_path"`
}

func (c *Config) Load(file string) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &c)
}

type Status struct {
	ConfigurationRevision string `json:"configurationRevision"`
	NeedsRestart          bool   `json:"needs_restart"`
	NixosVersion          string `json:"nixosVersion"`
	NixpkgsRevision       string `json:"nixpkgsRevision"`
	Host                  string `json:"host"`
	Port                  int32  `json:"port"`
	User                  string `json:"user"`
}

func (s *Status) DialString() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func (s *Status) Update() error {
	khFile := path.Clean(path.Join(os.Getenv("HOME"), ".ssh/known_hosts"))
	hostKeyCB, err := knownhosts.New(khFile)
	if err != nil {
		return fmt.Errorf("can't parse %q: %q", khFile, err)
	}

	key, err := os.ReadFile(statusPKPath)
	if err != nil {
		return fmt.Errorf("can't load key %q: %q", statusPKPath, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return fmt.Errorf("can't parse key: %q", err)
	}

	socket := os.Getenv("SSH_AUTH_SOCK")
	agentConn, err := net.Dial("unix", socket)
	if err != nil {
		return fmt.Errorf("can't Dial agent: %q, %q", socket, err)
	}
	agentClient := agent.NewClient(agentConn)

	sshConf := &ssh.ClientConfig{
		User:              s.User,
		HostKeyAlgorithms: []string{"ssh-ed25519"},
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		Timeout:         2 * time.Second,
		HostKeyCallback: hostKeyCB,
	}
	conn, err := ssh.Dial("tcp", s.DialString(), sshConf)
	if err != nil {
		return fmt.Errorf("can't Dial host %q (%q): %q", s.Host, s.DialString(), err)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("can't create session: %q", err)
	}
	defer session.Close()

	output, err := session.Output("xin-status")
	if err != nil {
		return fmt.Errorf("can't run command: %q", err)
	}

	return json.Unmarshal(output, s)
}

func (s *Status) ToTable() *widget.Table {
	t := widget.NewTable(
		func() (int, int) {
			return 4, 2
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(i widget.TableCellID, o fyne.CanvasObject) {
			if i.Col == 0 {
				switch i.Row {
				case 0:
					o.(*widget.Label).SetText("NixOS Version")
				case 1:
					o.(*widget.Label).SetText("Nixpkgs Revision")
				case 2:
					o.(*widget.Label).SetText("Configuration Revision")
				case 3:
					o.(*widget.Label).SetText("Restart?")
				}
			}
			if i.Col == 1 {
				switch i.Row {
				case 0:
					o.(*widget.Label).SetText(s.NixosVersion)
				case 1:
					o.(*widget.Label).SetText(s.NixpkgsRevision)
				case 2:
					o.(*widget.Label).SetText(s.ConfigurationRevision)
				case 3:
					str := "No"
					if s.NeedsRestart {
						str = "Yes"
					}
					o.(*widget.Label).SetText(str)
				}
			}
		},
	)

	t.Refresh()

	t.SetColumnWidth(0, 200.0)
	t.SetColumnWidth(1, 33.0)

	return t
}

func buildCards(c *Config, stat *xinStatus) fyne.CanvasObject {
	var cards []fyne.CanvasObject
	for _, s := range c.Statuses {
		boundStr := binding.BindString(&s.ConfigurationRevision)
		bsl := widget.NewLabelWithData(boundStr)

		stat.boundStrings = append(stat.boundStrings, boundStr)

		//circle := canvas.NewCircle(theme.SelectionColor())
		//circle.FillColor = color.RGBA{48, 190, 37, 0}
		//circle.StrokeWidth = 30
		//circle.StrokeColor = theme.TextColor()
		//if s.ConfigurationRevision == "DIRTY" {
		//	circle.FillColor = theme.ErrorColor()
		//}
		//circle.Resize(fyne.NewSize(250, 250))

		//card := widget.NewCard(s.Host, "", container.NewVBox(bsl, circle))
		card := widget.NewCard(s.Host, "", container.NewVBox(bsl))
		cards = append(cards, card)
	}
	stat.cards = cards
	return container.NewVBox(
		widget.NewCard("Some commit message", "somehash", nil),
		container.NewGridWithColumns(2, cards...),
	)
}

func doUpdate(c *Config, status *xinStatus) error {
	for _, h := range c.Statuses {
		err := h.Update()
		if err != nil {
			status.prependLog(err.Error())
		}
	}
	return nil
}

func main() {
	status := &xinStatus{}
	data := &Config{}
	dataPath := path.Clean(path.Join(os.Getenv("HOME"), ".xin.json"))
	err := data.Load(dataPath)
	if err != nil {
		log.Fatal(err)
	}

	statusPKPath = data.PrivKeyPath

	a := app.New()
	w := a.NewWindow("xintray")

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Status", theme.ComputerIcon(), buildCards(data, status)),
	)

	status.tabs = tabs
	status.log = widget.NewTextGrid()

	err = doUpdate(data, status)

	go func() {
		for {
			log.Println("updating host info")
			err = doUpdate(data, status)
			if err != nil {
				status.log.SetText(err.Error())
			}
			for _, s := range status.boundStrings {
				s.Reload()
			}
			time.Sleep(1 * time.Minute)
		}
	}()

	for _, s := range data.Statuses {
		tabs.Append(container.NewTabItem(s.Host, s.ToTable()))
	}

	tabs.SetTabLocation(container.TabLocationLeading)

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu("xintray",
			fyne.NewMenuItem("Show", func() {
				w.Show()
			}))
		desk.SetSystemTrayMenu(m)
	}

	status.log.SetText("starting...")

	w.SetContent(container.NewAppTabs(
		container.NewTabItem("Hosts", tabs),
		container.NewTabItem("Logs", container.NewMax(status.log)),
	))
	w.SetCloseIntercept(func() {
		w.Hide()
	})
	w.ShowAndRun()
}
