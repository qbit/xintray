package main

import (
	"encoding/xml"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type Feed struct {
	XMLName xml.Name `xml:"feed"`
	Text    string   `xml:",chardata"`
	Xmlns   string   `xml:"xmlns,attr"`
	Media   string   `xml:"media,attr"`
	Lang    string   `xml:"lang,attr"`
	ID      string   `xml:"id"`
	Link    []struct {
		Text string `xml:",chardata"`
		Type string `xml:"type,attr"`
		Rel  string `xml:"rel,attr"`
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Title   string    `xml:"title"`
	Updated time.Time `xml:"updated"`
	Entry   []struct {
		Text string `xml:",chardata"`
		ID   string `xml:"id"`
		Link struct {
			Text string `xml:",chardata"`
			Type string `xml:"type,attr"`
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
		} `xml:"link"`
		Title     string    `xml:"title"`
		Updated   time.Time `xml:"updated"`
		Thumbnail struct {
			Text   string `xml:",chardata"`
			Height string `xml:"height,attr"`
			Width  string `xml:"width,attr"`
			URL    string `xml:"url,attr"`
		} `xml:"thumbnail"`
		Author struct {
			Text string `xml:",chardata"`
			Name string `xml:"name"`
			URI  string `xml:"uri"`
		} `xml:"author"`
		Content struct {
			Text string `xml:",chardata"`
			Type string `xml:"type,attr"`
		} `xml:"content"`
	} `xml:"entry"`
}

func (f *Feed) LatestHash() (*commit, error) {
	doc, err := html.Parse(strings.NewReader(f.Entry[0].Content.Text))
	if err != nil {
		return nil, err
	}

	cmitMsg := ""
	var h func(*html.Node)
	h = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "pre" {
			cmitMsg = n.FirstChild.Data
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			h(child)
		}
	}
	h(doc)

	return &commit{
		hash: strings.Split(f.Entry[0].ID, "/")[1],
		// TODO: use x/html to pull out the info?
		message: cmitMsg,
		//message: html.UnescapeString(f.Entry[0].Content.Text),
		date: f.Entry[0].Updated,
	}, nil
}
