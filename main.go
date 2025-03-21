package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	commitCache = make(map[string]commit)
)

type commit struct {
	hash    string
	date    time.Time
	message string
}

type xinStatus struct {
	tabs            *container.AppTabs
	cards           []fyne.CanvasObject
	boundStrings    []binding.ExternalString
	boundBools      []binding.ExternalBool
	log             *widget.TextGrid
	repoCommit      commit
	config          Config
	upgradeProgress *widget.ProgressBar
	hasReboot       bool
	window          fyne.Window
	ci              *Status
}

type Status struct {
	card              *widget.Card
	buttonBox         *fyne.Container
	commit            commit
	client            *ssh.Client
	sshConn           ssh.Conn
	conn              net.Conn
	clientEstablished bool
	upToDate          bool

	ConfigurationRevision string `json:"configurationRevision"`
	NeedsRestart          bool   `json:"needs_restart"`
	NixosVersion          string `json:"nixosVersion"`
	NixpkgsRevision       string `json:"nixpkgsRevision"`
	SystemDiff            string `json:"system_diff"`
	Host                  string `json:"host"`
	Name                  string `json:"name"`
	MAC                   string `json:"mac"`
	Port                  int32  `json:"port"`
	Uname                 string `json:"uname_a"`
	Uptime                string `json:"uptime"`
}

func (s *Status) PrettyName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Host
}

func (s *Status) SshClose() error {
	s.card.Subtitle = "can't connect"
	fyne.Do(s.card.Refresh)

	s.clientEstablished = false
	return s.client.Close()
	// s.sshConn.Close()

	// return s.conn.Close()
}

func makeSshClient(x *xinStatus) (*ssh.ClientConfig, error) {
	khFile := path.Clean(path.Join(os.Getenv("HOME"), ".ssh/known_hosts"))
	hostKeyCB, err := knownhosts.New(khFile)
	if err != nil {
		return nil, err
	}

	key, err := os.ReadFile(x.config.PrivKeyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User:              "root",
		HostKeyAlgorithms: []string{"ssh-ed25519"},
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		Timeout:         2 * time.Second,
		HostKeyCallback: hostKeyCB,
	}, nil

}

func (s *Status) RunCmd(cmd string, x *xinStatus) error {
	ds := fmt.Sprintf("%s:%d", s.Host, s.Port)
	sshConf, err := makeSshClient(x)
	if err != nil {
		return err
	}

	s.client, err = ssh.Dial("tcp", ds, sshConf)
	if err != nil {
		return err
	}

	session, err := s.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	_, err = session.Output(cmd)
	if err != nil {
		return err
	}

	return nil
}

type Config struct {
	Statuses    []*Status `json:"statuses"`
	Repo        string    `json:"repo"`
	PrivKeyPath string    `json:"priv_key_path"`
	FlakeRSS    string    `json:"flake_rss"`
	CIHost      string    `json:"ci_host"`
}

func (c *commit) getInfo(repo string) error {
	msgCmd := exec.Command("git", "log", "--format=%B", "-n", "1", c.hash)
	msgCmd.Dir = repo
	msg, err := msgCmd.Output()
	if err != nil {
		return err
	}
	c.message = trim(msg)

	dateCmd := exec.Command("git", "log", "--format=%ci", c.hash)
	dateCmd.Dir = repo
	d, err := dateCmd.Output()
	if err != nil {
		return err
	}
	dateStr := trim(d)
	date, err := time.Parse("2006-01-02 15:04:05 -0700", dateStr)
	if err != nil {
		return err
	}

	c.date = date

	return nil
}

func NewCommit(c string) *commit {
	return &commit{
		hash: c,
	}
}

func trim(b []byte) string {
	head := bytes.Split(b, []byte("\n"))
	if head != nil {
		return string(head[0])
	}
	return ""
}

func (x *xinStatus) aliveCount() float64 {
	alive := 0
	x.hasReboot = false
	for _, s := range x.config.Statuses {
		if s.clientEstablished {
			alive = alive + 1
			if s.NeedsRestart {
				x.hasReboot = true
			}
		}
	}
	return float64(alive)
}

func (x *xinStatus) uptodateCount() float64 {
	utd := 0
	for _, s := range x.config.Statuses {
		if s.upToDate {
			utd = utd + 1
		}
	}
	return float64(utd)
}

func (x *xinStatus) getCommit(c string) (*commit, error) {
	commit := &commit{
		hash: c,
	}
	if c == "DIRTY" {
		return commit, nil
	}

	if commit, ok := commitCache[c]; ok {
		return &commit, nil
	} else {
		commit := NewCommit(c)
		err := commit.getInfo(x.config.Repo)
		if err != nil {
			return nil, err
		}
		commitCache[c] = *commit
	}

	return commit, nil
}

func (x *xinStatus) updateRepoInfo() error {
	switch {
	case (x.config.Repo != "" && x.config.FlakeRSS == ""):
		revCmd := exec.Command("git", "rev-parse", "HEAD")
		revCmd.Dir = x.config.Repo
		currentRev, err := revCmd.Output()
		if err != nil {
			return err
		}

		commit, err := x.getCommit(trim(currentRev))
		if err != nil {
			return err
		}
		x.repoCommit = *commit
	default:
		resp := &Feed{}
		res, err := http.Get(x.config.FlakeRSS)
		if err != nil {
			return err
		}
		if res == nil {
			return fmt.Errorf("invalid response")
		}

		defer res.Body.Close()

		if err = xml.NewDecoder(res.Body).Decode(&resp); err != nil {
			return err
		}
		cmit, err := resp.LatestHash()
		if err != nil {
			return err
		}
		x.repoCommit = *cmit
	}

	return nil
}

func (x *xinStatus) updateHostInfo() error {
	sshConf, err := makeSshClient(x)
	if err != nil {
		return err
	}
	upToDateCount := len(x.config.Statuses)
	for _, s := range x.config.Statuses {
		s := s
		var err error
		sshReset := func(reason string, err error) {
			upToDateCount--
			s.clientEstablished = false

			if len(s.buttonBox.Objects) > 1 {
				s.buttonBox.RemoveAll()
			}

			if errors.Is(err, os.ErrDeadlineExceeded) {
				log.Println("Exceeded")
			}

			log.Println(reason, err)
		}
		ds := fmt.Sprintf("%s:%d", s.Host, s.Port)
		if !s.clientEstablished {
			log.Printf("establishing connection to %q", s.Host)
			wakeButton := widget.NewButton("Wake", func() {
				go func() {
					log.Printf("sending wake to %s", s.Host)
					mac, err := net.ParseMAC(s.MAC)
					if err != nil {
						log.Println(err)
					}
					sendMagicPacket(mac)
				}()
			})
			restartButton := widget.NewButton("Reboot", func() {
				go func() {
					cnf := dialog.NewConfirm("Confirmation", fmt.Sprintf("Are you sure you want to reboot %q?", s.Host), func(doit bool) {
						if doit {
							err := s.RunCmd("xin reboot", x)
							if err != nil {
								log.Println(err)
							}
							s.SshClose()
						}
					}, x.window)
					cnf.SetDismissText("Cancel")
					cnf.SetConfirmText("Ok")
					cnf.Show()

				}()
			})

			updateButton := widget.NewButton("Update", func() {
				go func() {
					err := s.RunCmd("xin update", x)
					if err != nil {
						log.Println(err)
					}
					s.SshClose()
				}()
			})

			if len(s.buttonBox.Objects) == 0 {
				s.buttonBox.Add(wakeButton)
			}

			dialer := &net.Dialer{
				Timeout:   time.Second * 5,
				KeepAlive: time.Second * 5,
			}

			conn, err := dialer.Dial("tcp", ds)
			if err != nil {
				sshReset(fmt.Sprintf("can't dial %s", ds), err)
				continue
			}

			err = conn.SetDeadline(time.Now().Add(time.Second * 15))
			if err != nil {
				sshReset("deadline hit Dial", err)
				continue
			}

			clientConn, chans, reqs, err := ssh.NewClientConn(conn, ds, sshConf)
			if err != nil {
				sshReset("can't create SSH client connection", err)
				continue
			}

			s.conn = conn
			s.client = ssh.NewClient(clientConn, chans, reqs)
			s.clientEstablished = true

			if len(s.buttonBox.Objects) == 1 {
				s.buttonBox.RemoveAll()
				s.buttonBox.Add(restartButton)
				s.buttonBox.Add(updateButton)
			}

			if s.Host == x.config.CIHost {
				x.ci = s
			}
		}

		err = s.conn.SetDeadline(time.Now().Add(time.Second * 25))
		if err != nil {
			sshReset("hit deadline session", err)
			s.conn.Close()
			continue
		}

		session, err := s.client.NewSession()
		if err != nil {
			sshReset("can't create session", err)
			continue
		}
		defer session.Close()

		output, err := session.Output("xin status")
		if err != nil {
			sshReset("can't run command", err)
			continue
		}

		err = json.Unmarshal(output, s)
		if err != nil {
			sshReset("can't unmarshal output", err)
			continue
		}

		if s.ConfigurationRevision != x.repoCommit.hash {
			s.card.Subtitle = fmt.Sprintf("%.8s", s.ConfigurationRevision)
			upToDateCount = upToDateCount - 1
		} else {
			s.card.Subtitle = ""
		}

		fyne.Do(s.card.Refresh)

		commit, err := x.getCommit(s.ConfigurationRevision)
		if err != nil {
			x.Log(err.Error())
			continue
		}
		s.commit = *commit

		s.upToDate = false
		if s.commit.hash == x.repoCommit.hash {
			s.upToDate = true
		}
	}

	fyne.Do(func() {
		x.upgradeProgress.SetValue(float64(upToDateCount))
	})

	return nil
}

func (x *xinStatus) Log(s string) {
	log.Println(s)
	/*
		text := x.log.Text()
		now := time.Now()
		log.Println(s)
		x.log.SetText(strings.Join([]string{
			fmt.Sprintf("%s: %s", now.Format(time.RFC822), s),
			text,
		}, "\n"))
	*/
}

func (c *Config) Load(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &c)
}

func (s *Status) ToTable() *widget.Table {
	t := widget.NewTable(
		// Length
		func() (int, int) {
			return 7, 2
		},
		// CreateCell
		func() fyne.CanvasObject {
			//ct := container.NewScroll(container.NewMax(widget.NewLabel("")))
			ct := container.NewStack(container.NewVScroll(widget.NewLabel("")))
			//ct := container.NewMax(widget.NewLabel(""))
			return ct
		},
		// UpdateCell
		func(i widget.TableCellID, o fyne.CanvasObject) {
			ctnr := o.(*fyne.Container)
			content := ctnr.Objects[0].(*container.Scroll).Content.(*widget.Label)
			if i.Col == 0 {
				switch i.Row {
				case 0:
					content.SetText("NixOS Version")
				case 1:
					content.SetText("NixPkgs Revision")
				case 2:
					content.SetText("Uname")
				case 3:
					content.SetText("Uptime")
				case 4:
					content.SetText("Configuration Revision")
				case 5:
					content.SetText("Restart?")
				case 6:
					content.SetText("System Diff")
				}
			}
			if i.Col == 1 {
				switch i.Row {
				case 0:
					content.SetText(s.NixosVersion)
				case 1:
					content.SetText(s.NixpkgsRevision)
				case 2:
					content.SetText(s.Uname)
				case 3:
					content.SetText(s.Uptime)
				case 4:
					content.SetText(s.ConfigurationRevision)
				case 5:
					str := "No"
					if s.NeedsRestart {
						str = "Yes"
					}
					content.SetText(str)
				case 6:
					text, err := base64.StdEncoding.DecodeString(s.SystemDiff)
					if err != nil {
						fmt.Println("decode error:", err)
						return
					}
					content.SetText(string(text))
				}

			}
		},
		// OnSelected
		// func (i widget.TableCellID) {}
		// OnUnselected
		// func (i widget.TableCellID) {}
	)

	fyne.Do(t.Refresh)

	t.SetColumnWidth(0, 300.0)
	t.SetColumnWidth(1, 600.0)
	t.SetRowHeight(6, 600.0)

	return t
}

func buildCards(stat *xinStatus) fyne.CanvasObject {
	var cards []fyne.CanvasObject
	sort.Slice(stat.config.Statuses, func(i, j int) bool {
		return stat.config.Statuses[i].PrettyName() < stat.config.Statuses[j].PrettyName()
	})
	for _, s := range stat.config.Statuses {
		// TODO: maybe not needed once loopvar stuff is solid?
		s := s
		commitBStr := binding.BindString(&s.commit.message)
		bsl := widget.NewLabelWithData(commitBStr)

		verBStr := binding.BindString(&s.NixosVersion)
		bvl := widget.NewLabelWithData(verBStr)

		uptimeBStr := binding.BindString(&s.Uptime)
		uvl := widget.NewLabelWithData(uptimeBStr)

		restartBBool := binding.BindBool(&s.NeedsRestart)
		bbl := widget.NewCheckWithData("Reboot", restartBBool)
		bbl.Disable()

		stat.boundStrings = append(stat.boundStrings, commitBStr)
		stat.boundStrings = append(stat.boundStrings, verBStr)
		stat.boundStrings = append(stat.boundStrings, uptimeBStr)
		stat.boundBools = append(stat.boundBools, restartBBool)

		buttonHBox := container.NewHBox()

		card := widget.NewCard(s.PrettyName(), "",
			container.NewVBox(
				container.NewHBox(bvl),
				container.NewHBox(uvl),
				container.NewHBox(bbl),
				container.NewHBox(bsl),
				buttonHBox,
			),
		)

		s.card = card
		s.buttonBox = buttonHBox
		cards = append(cards, card)
		stat.cards = append(stat.cards, card)
	}

	stat.upgradeProgress = widget.NewProgressBar()
	stat.upgradeProgress.Min = 0
	stat.upgradeProgress.Max = stat.aliveCount()
	stat.upgradeProgress.TextFormatter = func() string {
		return fmt.Sprintf("%.0f of %.0f hosts up-to-date",
			stat.upgradeProgress.Value, stat.upgradeProgress.Max)
	}

	bsCommitMsg := binding.BindString(&stat.repoCommit.message)
	bsCommitHash := binding.BindString(&stat.repoCommit.hash)

	stat.boundStrings = append(stat.boundStrings, bsCommitMsg)
	stat.boundStrings = append(stat.boundStrings, bsCommitHash)

	ciStart := widget.NewButton("CI Start", func() {
		go func() {
			err := stat.ci.RunCmd("xin ci start", stat)
			if err != nil {
				log.Println(err)
			}
		}()
	})
	ciUpdate := widget.NewButton("CI Update", func() {
		go func() {
			err := stat.ci.RunCmd("xin ci update", stat)
			if err != nil {
				log.Println(err)
			}
		}()
	})
	updateAll := widget.NewButton("Update All", func() {
		for _, s := range stat.config.Statuses {
			host := s
			log.Printf("updating %s", host.Host)
			go func() {
				err := host.RunCmd("xin update", stat)
				if err != nil {
					log.Println(err)
				}
			}()
		}
	})

	statusCard := widget.NewCard("Xin Status", "", container.NewVBox(
		widget.NewLabelWithData(bsCommitMsg),
		container.NewHBox(ciStart, ciUpdate, updateAll),
		stat.upgradeProgress,
	))
	stat.cards = append(cards, statusCard)

	return container.NewVBox(
		statusCard,
		container.NewGridWithColumns(3, cards...),
	)
}

func main() {
	log.SetPrefix("xintray: ")
	status := &xinStatus{}
	dataPath := path.Clean(path.Join(os.Getenv("HOME"), ".xin.json"))
	err := status.config.Load(dataPath)
	if err != nil {
		log.Fatal(err)
	}

	a := app.New()
	a.Settings().SetTheme(&xinTheme{})
	w := a.NewWindow("xintray")
	if w == nil {
		log.Fatalln("unable to create window")
	}

	status.window = w

	ctrlQ := &desktop.CustomShortcut{KeyName: fyne.KeyQ, Modifier: fyne.KeyModifierControl}
	ctrlW := &desktop.CustomShortcut{KeyName: fyne.KeyW, Modifier: fyne.KeyModifierControl}
	w.Canvas().AddShortcut(ctrlQ, func(shortcut fyne.Shortcut) {
		a.Quit()
	})
	w.Canvas().AddShortcut(ctrlW, func(shortcut fyne.Shortcut) {
		w.Hide()
	})

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Status", theme.ComputerIcon(), buildCards(status)),
	)

	status.tabs = tabs
	status.log = widget.NewTextGrid()

	err = status.updateRepoInfo()
	if err != nil {
		status.log.SetText(err.Error())
	}

	go func() {
		for {
			err = status.updateRepoInfo()
			if err != nil {
				status.log.SetText(err.Error())
			}

			err = status.updateHostInfo()
			if err != nil {
				status.log.SetText(err.Error())
			}
			for _, s := range status.boundStrings {
				s.Reload()
			}
			for _, s := range status.boundBools {
				s.Reload()
			}
			time.Sleep(3 * time.Second)
			status.upgradeProgress.Max = status.aliveCount()
		}
	}()

	for _, s := range status.config.Statuses {
		tabs.Append(container.NewTabItem(s.PrettyName(), s.ToTable()))
	}

	tabs.SetTabLocation(container.TabLocationLeading)

	iconImg := buildImage(status)
	a.SetIcon(iconImg)

	if desk, ok := a.(desktop.App); ok {
		iconImg := buildImage(status)
		m := fyne.NewMenu("xintray",
			fyne.NewMenuItem("Show", func() {
				w.Show()
			}),
			fyne.NewMenuItem("Run CI", func() {
				err := status.ci.RunCmd("xin ci start", status)
				if err != nil {
					log.Println(err)
				}
			}),
			fyne.NewMenuItem("Update", func() {
				err := status.ci.RunCmd("xin ci update", status)
				if err != nil {
					log.Println(err)
				}
			}))
		desk.SetSystemTrayMenu(m)
		desk.SetSystemTrayIcon(iconImg)
		a.SetIcon(iconImg)
		go func() {
			for {
				img := buildImage(status)
				desk.SetSystemTrayIcon(img)
				a.SetIcon(img)
				time.Sleep(3 * time.Second)
			}
		}()
	}

	status.log.SetText("starting...")

	w.SetContent(container.NewAppTabs(
		container.NewTabItem("Hosts", tabs),
		container.NewTabItem("Config", container.NewStack(widget.NewCard("Config", "", nil))),
		container.NewTabItem("Logs", container.NewStack(status.log)),
	))
	w.SetCloseIntercept(func() {
		w.Hide()
	})
	w.ShowAndRun()
}
