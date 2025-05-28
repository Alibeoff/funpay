package funpay

import (
	"context"
	"fmt"
	"net/url"

	"github.com/PuerkitoBio/goquery"
)

type Chat struct {
	ID          int64
	UserName    string
	Link        string
	LastMessage int64
	isRead      bool
	MessageList []Message
	NewMessage  []Message
}

type Message struct {
	ID       string
	Author   string
	DateTime string
	Text     string
}

func (fp *Funpay) UpdateChatList(ctx context.Context) error {
	const op = "Funpay.UpdateChatList"
	resp, err := fp.RequestHTML(ctx, fp.baseURL+"/chat/")
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	res, err := fp.ParseChatID(resp)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	fmt.Println(res.ID)
	return nil
}

type result struct {
	ID     string
	Status bool
}

func (fp *Funpay) ParseChatID(doc *goquery.Document) ([]result, error) {
	var res result
	const op = "Funpay.ParseChatID"
	ds := doc.Find(".contact-list")
	html, err := ds.Html()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	items, err := ParseContacts(html)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	for _, item := range items {
		q, _ := url.Parse(item.Link)
		link := q.Query().Get("node")
		res.ID = link
		res.Status = item.isRead
		fmt.Printf("link: %s\n", link)
	}
	return res, nil
}
