package funpay

import (
	"context"
	"fmt"
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

func (fp *Funpay) GetSiteAllMessages(ctx context.Context) error {

	const op = "Funpay.GetAllMessages"
	resp, err := fp.RequestHTML(ctx, fp.baseURL+"/chat/")
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	err = fp.GetMessageInfo(resp)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
