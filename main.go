package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var commitCache = make(map[string]commit)

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
}

type Status struct {
	card              *widget.Card
	commit            commit
	client            *ssh.Client
	clientEstablished bool
	upToDate          bool

	ConfigurationRevision string `json:"configurationRevision"`
	NeedsRestart          bool   `json:"needs_restart"`
	NixosVersion          string `json:"nixosVersion"`
	NixpkgsRevision       string `json:"nixpkgsRevision"`
	Host                  string `json:"host"`
	Port                  int32  `json:"port"`
}

type Config struct {
	Statuses    []*Status `json:"statuses"`
	Repo        string    `json:"repo"`
	PrivKeyPath string    `json:"priv_key_path"`
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
	return string(head[0])
}

func (x *xinStatus) aliveCount() float64 {
	alive := 0
	for _, s := range x.config.Statuses {
		if s.clientEstablished {
			alive = alive + 1
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

	return nil
}

func (x *xinStatus) updateHostInfo() error {
	khFile := path.Clean(path.Join(os.Getenv("HOME"), ".ssh/known_hosts"))
	hostKeyCB, err := knownhosts.New(khFile)
	if err != nil {
		return fmt.Errorf("can't parse %q: %q", khFile, err)
	}

	key, err := os.ReadFile(x.config.PrivKeyPath)
	if err != nil {
		return fmt.Errorf("can't load key %q: %q", x.config.PrivKeyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return fmt.Errorf("can't parse key: %q", err)
	}

	sshConf := &ssh.ClientConfig{
		User:              "root",
		HostKeyAlgorithms: []string{"ssh-ed25519"},
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		Timeout:         2 * time.Second,
		HostKeyCallback: hostKeyCB,
	}

	upToDateCount := len(x.config.Statuses)
	for _, s := range x.config.Statuses {
		var err error
		ds := fmt.Sprintf("%s:%d", s.Host, s.Port)
		if !s.clientEstablished {
			s.client, err = ssh.Dial("tcp", ds, sshConf)
			if err != nil {
				s.card.Subtitle = "can't connect"
				upToDateCount = upToDateCount - 1
				x.Log(fmt.Sprintf("can't Dial host %q (%q): %q", s.Host, ds, err))
				s.card.Refresh()
				continue
			}
			s.clientEstablished = true
		}

		session, err := s.client.NewSession()
		if err != nil {
			x.Log(fmt.Sprintf("can't create session: %q", err))
			upToDateCount = upToDateCount - 1
			s.clientEstablished = false
			continue
		}
		defer session.Close()

		output, err := session.Output("xin-status")
		if err != nil {
			x.Log(fmt.Sprintf("can't run command: %q", err))
			upToDateCount = upToDateCount - 1
			continue
		}

		err = json.Unmarshal(output, s)
		if err != nil {
			x.Log(err.Error())
			upToDateCount = upToDateCount - 1
			continue
		}

		if s.ConfigurationRevision != x.repoCommit.hash {
			s.card.Subtitle = fmt.Sprintf("%.8s", s.ConfigurationRevision)
			upToDateCount = upToDateCount - 1
		} else {
			s.card.Subtitle = ""
		}
		s.card.Refresh()

		commit, err := x.getCommit(s.ConfigurationRevision)
		if err != nil {
			x.Log(err.Error())
			continue
		}
		s.commit = *commit

		s.upToDate = false
		if s.commit == x.repoCommit {
			s.upToDate = true
		}
	}

	x.upgradeProgress.SetValue(float64(upToDateCount))

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
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &c)
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

func buildCards(stat *xinStatus) fyne.CanvasObject {
	var cards []fyne.CanvasObject
	sort.Slice(stat.config.Statuses, func(i, j int) bool {
		return stat.config.Statuses[i].Host < stat.config.Statuses[j].Host
	})
	for _, s := range stat.config.Statuses {
		commitBStr := binding.BindString(&s.commit.message)
		bsl := widget.NewLabelWithData(commitBStr)

		restartBBool := binding.BindBool(&s.NeedsRestart)
		bbl := widget.NewCheckWithData("Reboot", restartBBool)
		bbl.Disable()

		stat.boundStrings = append(stat.boundStrings, commitBStr)
		stat.boundBools = append(stat.boundBools, restartBBool)

		card := widget.NewCard(s.Host, "",
			container.NewVBox(
				container.NewHBox(bbl),
				container.NewHBox(bsl),
			),
		)

		s.card = card
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

	statusCard := widget.NewCard("Xin Status", "", container.NewVBox(
		widget.NewLabelWithData(bsCommitMsg),
		stat.upgradeProgress,
	))
	stat.cards = append(cards, statusCard)
	return container.NewVBox(
		statusCard,
		container.NewGridWithColumns(3, cards...),
	)
}

func main() {
	status := &xinStatus{}
	dataPath := path.Clean(path.Join(os.Getenv("HOME"), ".xin.json"))
	err := status.config.Load(dataPath)
	if err != nil {
		log.Fatal(err)
	}

	a := app.New()
	w := a.NewWindow("xintray")

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
		tabs.Append(container.NewTabItem(s.Host, s.ToTable()))
	}

	tabs.SetTabLocation(container.TabLocationLeading)

	iconImg := buildImage(status)
	a.SetIcon(iconImg)

	if desk, ok := a.(desktop.App); ok {
		iconImg := buildImage(status)
		m := fyne.NewMenu("xintray",
			fyne.NewMenuItem("Show", func() {
				w.Show()
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
		container.NewTabItem("Config", container.NewMax(widget.NewCard("Config", "", nil))),
		container.NewTabItem("Logs", container.NewMax(status.log)),
	))
	w.SetCloseIntercept(func() {
		w.Hide()
	})
	w.ShowAndRun()
}
