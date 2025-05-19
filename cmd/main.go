package main

import (
	"context"
	"log"

	"github.com/kostromin59/funpay"
)

func main() {
	ss := funpay.New("zrd0d72w6okaxj8dbzinrjey8qdr9h38", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")
	err := ss.Update(context.Background())
	if err != nil {
		log.Println(err)
		return
	}
	err = ss.GetAllMessages(context.Background())
	// msgs := ss.NewMessages()
	if err != nil {
		log.Println(err)
		return
	}
}
